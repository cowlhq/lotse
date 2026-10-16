package hub

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/store"
)

// A slug is a scope's key in its parent: lowercase letters, digits and
// dashes, starting and ending with a letter or digit.
var slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)

const maxName = 80

// actor is who makes a change through the API: the logged-in user, in the
// UI.
func actor(ctx context.Context) store.Actor {
	u, _ := userOf(ctx)
	id := u.ID
	return store.Actor{Kind: "user", ID: &id, Name: u.Email, Via: "ui"}
}

const forbidden = "only an admin may do this"

// toScope leaves out the global scope as a parent: to the API, customers
// have none.
func toScope(x store.Scope) api.Scope {
	parent := x.ParentID
	if parent != nil && *parent == store.GlobalScope {
		parent = nil
	}
	return api.Scope{
		Id: x.ID, ParentId: parent, Kind: api.ScopeKind(x.Kind), Slug: x.Slug, Name: x.Name, Path: x.Path,
	}
}

func (s apiServer) ListScopes(ctx context.Context, _ api.ListScopesRequestObject) (api.ListScopesResponseObject, error) {
	list, err := s.db.Scopes(ctx)
	if err != nil {
		return nil, err
	}
	t, err := s.treeFor(ctx)
	if err != nil {
		return nil, err
	}
	out := api.ListScopes200JSONResponse{}
	for _, x := range list {
		if visible(ctx, t, x.Kind, x.ID) {
			out = append(out, toScope(x))
		}
	}
	return out, nil
}

func (s apiServer) GetScope(ctx context.Context, req api.GetScopeRequestObject) (api.GetScopeResponseObject, error) {
	x, err := s.db.Scope(ctx, req.Id)
	if err == nil {
		err = s.see(ctx, x.Kind, x.ID)
	}
	if notFound(err) || x.Kind == "global" {
		return api.GetScope404JSONResponse{Error: "no such scope"}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.GetScope200JSONResponse(toScope(x)), nil
}

func (s apiServer) RenameScope(ctx context.Context, req api.RenameScopeRequestObject) (api.RenameScopeResponseObject, error) {
	x, err := s.db.Scope(ctx, req.Id)
	ok := false
	if err == nil {
		ok, err = s.mayChange(ctx, x.Kind, x.ID)
	}
	if notFound(err) || x.Kind == "global" {
		return api.RenameScope404JSONResponse{Error: "no such scope"}, nil
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return api.RenameScope403JSONResponse{Error: forbidden}, nil
	}
	name := strings.TrimSpace(req.Body.Name)
	if msg := checkName(name); msg != "" {
		return api.RenameScope400JSONResponse{Error: msg}, nil
	}
	if err := s.db.RenameScope(ctx, actor(ctx), req.Id, name); err != nil {
		return nil, err
	}
	if x, err = s.db.Scope(ctx, req.Id); err != nil {
		return nil, err
	}
	return api.RenameScope200JSONResponse(toScope(x)), nil
}

func (s apiServer) DeleteScope(ctx context.Context, req api.DeleteScopeRequestObject) (api.DeleteScopeResponseObject, error) {
	x, err := s.db.Scope(ctx, req.Id)
	ok := false
	if err == nil {
		ok, err = s.mayChange(ctx, x.Kind, x.ID)
	}
	if notFound(err) {
		return api.DeleteScope404JSONResponse{Error: "no such scope"}, nil
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return api.DeleteScope403JSONResponse{Error: forbidden}, nil
	}
	switch err := s.db.DeleteScope(ctx, actor(ctx), req.Id); {
	case errors.Is(err, store.ErrNotFound):
		return api.DeleteScope404JSONResponse{Error: "no such scope"}, nil
	case errors.Is(err, store.ErrNotEmpty):
		return api.DeleteScope409JSONResponse{Error: "it still has something in it, or users are kept to it"}, nil
	case err != nil:
		return nil, err
	}
	return api.DeleteScope204Response{}, nil
}

func (s apiServer) CreateEnvironment(ctx context.Context, req api.CreateEnvironmentRequestObject) (api.CreateEnvironmentResponseObject, error) {
	if !isAnyAdmin(ctx) {
		return api.CreateEnvironment403JSONResponse{Error: forbidden}, nil
	}
	var levels [3]store.ScopeName
	for i, l := range []api.ScopeName{req.Body.Customer, req.Body.Project, req.Body.Environment} {
		slug, name := strings.TrimSpace(l.Slug), strings.TrimSpace(l.Name)
		if !slugPattern.MatchString(slug) {
			return api.CreateEnvironment400JSONResponse{Error: "a key has lowercase letters, digits and dashes, up to 40: " + strconv.Quote(slug)}, nil
		}
		if msg := checkName(name); msg != "" {
			return api.CreateEnvironment400JSONResponse{Error: msg}, nil
		}
		levels[i] = store.ScopeName{Slug: slug, Name: name}
	}
	// An admin of a scope adds only below it. Paths of scopes name them
	// once, so here they can be compared.
	if _, within := accessOf(ctx); within != "" {
		path := levels[0].Slug + "/" + levels[1].Slug + "/" + levels[2].Slug
		if path != within && !strings.HasPrefix(path, within+"/") {
			return api.CreateEnvironment403JSONResponse{Error: "you can add environments only within " + within}, nil
		}
	}
	env, err := s.db.EnsureEnvironment(ctx, actor(ctx), levels)
	if err != nil {
		return nil, err
	}
	return api.CreateEnvironment201JSONResponse(toScope(env)), nil
}

func (s apiServer) SetHostEnvironment(ctx context.Context, req api.SetHostEnvironmentRequestObject) (api.SetHostEnvironmentResponseObject, error) {
	_, err := s.db.Host(ctx, req.Id)
	ok := false
	if err == nil {
		ok, err = s.mayChange(ctx, "host", req.Id)
	}
	if notFound(err) {
		return api.SetHostEnvironment404JSONResponse{Error: "no such host"}, nil
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return api.SetHostEnvironment403JSONResponse{Error: forbidden}, nil
	}
	envID := req.Body.EnvironmentId
	if envID == nil && restricted(ctx) {
		return api.SetHostEnvironment400JSONResponse{Error: "a host stays in an environment you can see"}, nil
	}
	if envID != nil {
		x, err := s.db.Scope(ctx, *envID)
		if err == nil {
			err = s.see(ctx, x.Kind, x.ID)
		}
		if notFound(err) || x.Kind != "environment" {
			return api.SetHostEnvironment400JSONResponse{Error: "no such environment"}, nil
		}
		if err != nil {
			return nil, err
		}
	}
	if err := s.db.SetHostEnvironment(ctx, actor(ctx), req.Id, envID); err != nil {
		return nil, err
	}
	return api.SetHostEnvironment204Response{}, nil
}

func (s apiServer) ListChanges(ctx context.Context, req api.ListChangesRequestObject) (api.ListChangesResponseObject, error) {
	kind, idText, _ := strings.Cut(req.Params.Subject, ":")
	id, err := strconv.ParseInt(idText, 10, 64)
	if kind == "" || err != nil {
		return api.ListChanges400JSONResponse{Error: "subject must be kind:id, such as host:3"}, nil
	}
	if err := s.see(ctx, kind, id); notFound(err) {
		return api.ListChanges200JSONResponse{}, nil
	} else if err != nil {
		return nil, err
	}
	list, err := s.db.Changes(ctx, kind, id, 200)
	if err != nil {
		return nil, err
	}
	out := make(api.ListChanges200JSONResponse, len(list))
	for i, c := range list {
		out[i] = api.Change{
			Id: c.ID, At: c.At, BatchId: c.BatchID, ActorKind: api.ChangeActorKind(c.ActorKind),
			ActorName: c.ActorName, Via: api.ChangeVia(c.Via), SubjectKind: c.SubjectKind,
			SubjectId: c.SubjectID, SubjectPath: c.SubjectPath, Field: c.Field,
		}
		// Left out when missing: a nil RawMessage in an interface would be
		// written as null.
		if c.Old != nil {
			out[i].Old = c.Old
		}
		if c.New != nil {
			out[i].New = c.New
		}
	}
	return out, nil
}

func checkName(name string) string {
	if name == "" {
		return "a name is needed"
	}
	if len([]rune(name)) > maxName {
		return "a name has at most 80 characters"
	}
	return ""
}
