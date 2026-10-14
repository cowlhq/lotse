package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"lotse.sh/internal/apply"
	"lotse.sh/internal/auth"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/store"
)

// API tokens (decision 0019): lotse apply and export, and the read API,
// authenticate with one. A token works on its scope and what is below it.

// tokenPrefix marks an API token, as lotse_agt_ marks an agent's.
const tokenPrefix = "lotse_tok_"

func toToken(t store.Token) api.Token {
	return api.Token{
		Id: t.ID, Name: t.Name, Kind: api.TokenKind(t.Kind), ScopeId: t.ScopeID, ScopePath: t.ScopePath,
		CreatedBy: t.CreatedBy, CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt, Revoked: t.RevokedAt != nil,
	}
}

func (s apiServer) ListTokens(ctx context.Context, _ api.ListTokensRequestObject) (api.ListTokensResponseObject, error) {
	if !isAdmin(ctx) {
		return api.ListTokens403JSONResponse{Error: forbidden}, nil
	}
	list, err := s.db.Tokens(ctx)
	if err != nil {
		return nil, err
	}
	out := make(api.ListTokens200JSONResponse, len(list))
	for i, t := range list {
		out[i] = toToken(t)
	}
	return out, nil
}

func (s apiServer) CreateToken(ctx context.Context, req api.CreateTokenRequestObject) (api.CreateTokenResponseObject, error) {
	if !isAdmin(ctx) {
		return api.CreateToken403JSONResponse{Error: forbidden}, nil
	}
	name := strings.TrimSpace(req.Body.Name)
	if msg := checkName(name); msg != "" {
		return api.CreateToken400JSONResponse{Error: msg}, nil
	}
	kind := string(req.Body.Kind)
	if kind != "read" && kind != "apply" {
		return api.CreateToken400JSONResponse{Error: "kind is read or apply"}, nil
	}
	secret, hash := auth.NewToken(tokenPrefix)
	t, err := s.db.CreateToken(ctx, actor(ctx), store.Token{Name: name, Kind: kind, ScopeID: req.Body.ScopeId, CreatedBy: actor(ctx).Name, CreatedAt: time.Now().Unix()}, hash)
	if errors.Is(err, store.ErrNotFound) {
		return api.CreateToken400JSONResponse{Error: "no such scope"}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.CreateToken201JSONResponse{Token: toToken(t), Secret: secret}, nil
}

func (s apiServer) RevokeToken(ctx context.Context, req api.RevokeTokenRequestObject) (api.RevokeTokenResponseObject, error) {
	if !isAdmin(ctx) {
		return api.RevokeToken403JSONResponse{Error: forbidden}, nil
	}
	err := s.db.RevokeToken(ctx, actor(ctx), req.Id, time.Now().Unix())
	if errors.Is(err, store.ErrNotFound) {
		return api.RevokeToken404JSONResponse{Error: "no such token, or revoked already"}, nil
	}
	return api.RevokeToken204Response{}, err
}

// applier is who applies: a token, or an admin through the UI's session.
func applyActor(ctx context.Context) (store.Actor, string, bool) {
	if t, ok := tokenOf(ctx); ok {
		if t.Kind != "apply" {
			return store.Actor{}, "", false
		}
		id := t.ID
		return store.Actor{Kind: "token", ID: &id, Name: t.Name, Via: "apply"}, t.ScopePath, true
	}
	if !isAdmin(ctx) {
		return store.Actor{}, "", false
	}
	a := actor(ctx)
	a.Via = "apply"
	return a, "", true
}

func (s apiServer) ApplyDocument(ctx context.Context, req api.ApplyDocumentRequestObject) (api.ApplyDocumentResponseObject, error) {
	who, within, ok := applyActor(ctx)
	if !ok {
		return api.ApplyDocument403JSONResponse{Error: "applying needs an apply token, or an admin"}, nil
	}
	b, _ := json.Marshal(req.Body)
	var doc apply.Document
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return api.ApplyDocument400JSONResponse{Error: "the document is not valid: " + err.Error()}, nil
	}
	opts := applyOptions{
		dryRun: ptrValue(req.Params.DryRun), prune: ptrValue(req.Params.Prune),
		pruneScope: strings.Trim(ptrValue(req.Params.Scope), "/"), within: within,
	}
	if opts.prune && req.Params.Scope == nil {
		opts.pruneScope = within
	}
	res, err := s.applyDocument(ctx, who, doc, opts)
	var bad applyError
	if errors.As(err, &bad) {
		return api.ApplyDocument400JSONResponse{Error: bad.msg}, nil
	}
	if err != nil {
		return nil, err
	}
	out := api.ApplyDocument200JSONResponse{Applied: res.Applied, Warnings: res.Warnings, Changes: []api.ApplyChange{}}
	b, _ = json.Marshal(res.Changes)
	json.Unmarshal(b, &out.Changes)
	return out, nil
}

func (s apiServer) ExportScope(ctx context.Context, req api.ExportScopeRequestObject) (api.ExportScopeResponseObject, error) {
	path := strings.Trim(ptrValue(req.Params.Scope), "/")
	if t, ok := tokenOf(ctx); ok {
		if t.ScopePath != "" && path != t.ScopePath && !strings.HasPrefix(path, t.ScopePath+"/") {
			if req.Params.Scope != nil {
				return api.ExportScope403JSONResponse{Error: "the token is for " + t.ScopePath}, nil
			}
			path = t.ScopePath
		}
	} else if !isAdmin(ctx) {
		return api.ExportScope403JSONResponse{Error: forbidden}, nil
	}
	doc, err := s.exportDocument(ctx, path)
	if errors.Is(err, store.ErrNotFound) {
		return api.ExportScope404JSONResponse{Error: "there is no scope " + path}, nil
	}
	if err != nil {
		return nil, err
	}
	out, err := apply.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return api.ExportScope200ApplicationyamlResponse{Body: bytes.NewReader(out), ContentLength: int64(len(out))}, nil
}
