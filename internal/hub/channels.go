package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/notify"
	"lotse.sh/internal/store"
)

// Channels (design 8.4): where notifications go. A channel is defined on a
// scope and used, by its key, in notify.channels on that scope or below.

// channelSpec is what a channel keeps that is not secret.
type channelSpec struct {
	// To and SMTP are an email channel's: its recipients, and its own
	// mail server instead of the hub's.
	To   []string     `json:"to,omitempty"`
	SMTP *notify.SMTP `json:"smtp,omitempty"`
}

// secretFields are the secrets each type of channel has.
var secretFields = map[string][]string{
	"email":      {"password"},
	"googlechat": {"url"},
	"webhook":    {"url", "signing_key"},
}

// checkChannel normalizes what a channel is and says what is wrong with
// it, if anything.
func checkChannel(typ string, raw map[string]any, sec secrets) (json.RawMessage, error) {
	b, _ := json.Marshal(raw)
	var spec channelSpec
	if err := json.Unmarshal(b, &spec); err != nil {
		return nil, fmt.Errorf("the spec is not valid: %v", err)
	}
	switch typ {
	case "email":
		if err := notify.CheckAddresses(spec.To); err != nil {
			return nil, err
		}
		if spec.SMTP != nil {
			normalizeSMTP(spec.SMTP)
			if err := spec.SMTP.Check(); err != nil {
				return nil, err
			}
		} else if sec.has("password") {
			return nil, errors.New("a password needs a mail server of the channel's own")
		}
	case "googlechat", "webhook":
		if spec.To != nil || spec.SMTP != nil {
			return nil, fmt.Errorf("a %s channel has nothing but its secrets", typ)
		}
		spec = channelSpec{}
		if !sec.has("url") {
			return nil, errors.New("the channel needs its address, url")
		}
		if v, ok := sec.sealed["url"]; ok {
			u, err := url.Parse(v)
			if err != nil || u.Scheme != "https" && u.Scheme != "http" || u.Host == "" {
				return nil, errors.New("url is not an http or https address")
			}
		}
	default:
		return nil, errors.New("type is email, googlechat or webhook")
	}
	return json.Marshal(spec)
}

func normalizeSMTP(s *notify.SMTP) {
	s.Host = strings.TrimSpace(s.Host)
	if s.TLS == "" {
		s.TLS = "starttls"
	}
	if s.Port == 0 {
		s.Port = 587
		if s.TLS == "tls" {
			s.Port = 465
		}
	}
}

// sender makes what sends to a channel.
func (h *hub) sender(ctx context.Context, c store.Channel) (notify.Sender, error) {
	sec, err := h.openSecrets(c.Secrets, c.SecretRefs)
	if err != nil {
		return nil, err
	}
	var spec channelSpec
	json.Unmarshal(c.Spec, &spec)
	switch c.Type {
	case "email":
		server := spec.SMTP
		password, err := sec.get("password")
		if err != nil {
			return nil, err
		}
		if server == nil {
			m, err := h.mailServer(ctx)
			if err != nil {
				return nil, err
			}
			server, password = &m.SMTP, m.SMTP.Password
		}
		s := *server
		s.Password = password
		return notify.Email{Server: s, To: spec.To}, nil
	case "googlechat", "webhook":
		u, err := sec.get("url")
		if err != nil {
			return nil, err
		}
		if c.Type == "googlechat" {
			return notify.GoogleChat{URL: u}, nil
		}
		key, err := sec.get("signing_key")
		if err != nil {
			return nil, err
		}
		return notify.Webhook{URL: u, SigningKey: key}, nil
	}
	return nil, fmt.Errorf("a channel of type %s", c.Type)
}

func (s apiServer) toChannel(c store.Channel) api.Channel {
	var spec map[string]any
	json.Unmarshal(c.Spec, &spec)
	if spec == nil {
		spec = map[string]any{}
	}
	sec, err := s.openSecrets(c.Secrets, c.SecretRefs)
	if err != nil {
		s.log.Error("cannot open a channel's secrets", "channel", c.Slug, "err", err)
	}
	return api.Channel{
		Id: c.ID, ScopeId: c.ScopeID, ScopePath: c.ScopePath, Key: c.Slug, Name: c.Name,
		Type: api.ChannelType(c.Type), Spec: spec, Secrets: sec.info(secretFields[c.Type]),
	}
}

// scopeChain is a scope and the scopes above it, nearest first.
func (h *hub) scopeChain(ctx context.Context, id int64) ([]int64, error) {
	var chain []int64
	for {
		x, err := h.db.Scope(ctx, id)
		if err != nil {
			return nil, err
		}
		chain = append(chain, x.ID)
		if x.ParentID == nil {
			return chain, nil
		}
		id = *x.ParentID
	}
}

func (s apiServer) ListChannels(ctx context.Context, req api.ListChannelsRequestObject) (api.ListChannelsResponseObject, error) {
	list, err := s.db.Channels(ctx)
	if err != nil {
		return nil, err
	}
	var chain []int64
	if req.Params.Scope != nil {
		if chain, err = s.scopeChain(ctx, *req.Params.Scope); errors.Is(err, store.ErrNotFound) {
			return api.ListChannels200JSONResponse{}, nil
		} else if err != nil {
			return nil, err
		}
	}
	out := api.ListChannels200JSONResponse{}
	for _, c := range list {
		if chain == nil || slices.Contains(chain, c.ScopeID) {
			out = append(out, s.toChannel(c))
		}
	}
	return out, nil
}

func (s apiServer) CreateChannel(ctx context.Context, req api.CreateChannelRequestObject) (api.CreateChannelResponseObject, error) {
	if !isAdmin(ctx) {
		return api.CreateChannel403JSONResponse{Error: forbidden}, nil
	}
	b := req.Body
	name, key := strings.TrimSpace(b.Name), strings.TrimSpace(b.Key)
	if msg := checkName(name); msg != "" {
		return api.CreateChannel400JSONResponse{Error: msg}, nil
	}
	if !slugPattern.MatchString(key) {
		return api.CreateChannel400JSONResponse{Error: "a key has lowercase letters, digits and dashes, up to 40"}, nil
	}
	sec := secrets{sealed: map[string]string{}, refs: map[string]secretRef{}}
	if err := sec.apply(secretFields[string(b.Type)], ptrValue(b.Secrets)); err != nil {
		return api.CreateChannel400JSONResponse{Error: err.Error()}, nil
	}
	spec, err := checkChannel(string(b.Type), b.Spec, sec)
	if err != nil {
		return api.CreateChannel400JSONResponse{Error: err.Error()}, nil
	}
	sealed, refs := s.closeSecrets(sec)
	c, err := s.db.CreateChannel(ctx, actor(ctx), store.Channel{
		ScopeID: b.ScopeId, Slug: key, Name: name, Type: string(b.Type), Spec: spec,
		Secrets: sealed, SecretRefs: refs, CreatedAt: time.Now().Unix(),
	})
	switch {
	case errors.Is(err, store.ErrExists):
		return api.CreateChannel409JSONResponse{Error: "the scope has a channel with this key"}, nil
	case errors.Is(err, store.ErrNotFound):
		return api.CreateChannel400JSONResponse{Error: "no such scope"}, nil
	case err != nil:
		return nil, err
	}
	return api.CreateChannel201JSONResponse(s.toChannel(c)), nil
}

func (s apiServer) UpdateChannel(ctx context.Context, req api.UpdateChannelRequestObject) (api.UpdateChannelResponseObject, error) {
	if !isAdmin(ctx) {
		return api.UpdateChannel403JSONResponse{Error: forbidden}, nil
	}
	c, err := s.db.Channel(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return api.UpdateChannel404JSONResponse{Error: "no such channel"}, nil
	}
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Body.Name)
	if msg := checkName(name); msg != "" {
		return api.UpdateChannel400JSONResponse{Error: msg}, nil
	}
	sec, err := s.openSecrets(c.Secrets, c.SecretRefs)
	if err != nil {
		return nil, err
	}
	if err := sec.apply(secretFields[c.Type], ptrValue(req.Body.Secrets)); err != nil {
		return api.UpdateChannel400JSONResponse{Error: err.Error()}, nil
	}
	spec, err := checkChannel(c.Type, req.Body.Spec, sec)
	if err != nil {
		return api.UpdateChannel400JSONResponse{Error: err.Error()}, nil
	}
	c.Name, c.Spec = name, spec
	c.Secrets, c.SecretRefs = s.closeSecrets(sec)
	if c, err = s.db.UpdateChannel(ctx, actor(ctx), c); err != nil {
		return nil, err
	}
	return api.UpdateChannel200JSONResponse(s.toChannel(c)), nil
}

func (s apiServer) DeleteChannel(ctx context.Context, req api.DeleteChannelRequestObject) (api.DeleteChannelResponseObject, error) {
	if !isAdmin(ctx) {
		return api.DeleteChannel403JSONResponse{Error: forbidden}, nil
	}
	err := s.db.DeleteChannel(ctx, actor(ctx), req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return api.DeleteChannel404JSONResponse{Error: "no such channel"}, nil
	}
	return api.DeleteChannel204Response{}, err
}

func (s apiServer) TestChannel(ctx context.Context, req api.TestChannelRequestObject) (api.TestChannelResponseObject, error) {
	if !isAdmin(ctx) {
		return api.TestChannel403JSONResponse{Error: forbidden}, nil
	}
	c, err := s.db.Channel(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return api.TestChannel404JSONResponse{Error: "no such channel"}, nil
	}
	if err != nil {
		return nil, err
	}
	sender, err := s.sender(ctx, c)
	if err == nil {
		err = sender.Send(ctx, notify.Message{Hub: s.URL(), Test: true})
	}
	return api.TestChannel200JSONResponse(testResult(err)), nil
}

func testResult(err error) api.TestResult {
	if err != nil {
		return api.TestResult{Sent: false, Error: ptr(err.Error())}
	}
	return api.TestResult{Sent: true}
}

// The mail server.

// mailServerSettings is the mail server with its password.
type mailServerSettings struct {
	notify.SMTP
	sec secrets
}

var errNoMailServer = errors.New("there is no mail server; set one in the settings")

// mailServer returns the hub's mail server, its password resolved.
func (h *hub) mailServer(ctx context.Context) (mailServerSettings, error) {
	var m mailServerSettings
	stored, err := h.db.MailServer(ctx)
	if err != nil {
		return m, err
	}
	if stored == nil {
		return m, errNoMailServer
	}
	json.Unmarshal(stored.Spec, &m.SMTP)
	if m.sec, err = h.openSecrets(stored.Secrets, stored.SecretRefs); err != nil {
		return m, err
	}
	m.SMTP.Password, err = m.sec.get("password")
	return m, err
}

func toMailServer(m mailServerSettings) api.MailServer {
	return api.MailServer{
		Host: m.Host, Port: m.Port, Username: m.Username, From: m.From, Tls: api.MailServerTls(m.TLS),
		Password: m.sec.info([]string{"password"})["password"],
	}
}

func (s apiServer) GetMailServer(ctx context.Context, _ api.GetMailServerRequestObject) (api.GetMailServerResponseObject, error) {
	stored, err := s.db.MailServer(ctx)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return api.GetMailServer404JSONResponse{Error: errNoMailServer.Error()}, nil
	}
	var m mailServerSettings
	json.Unmarshal(stored.Spec, &m.SMTP)
	if m.sec, err = s.openSecrets(stored.Secrets, stored.SecretRefs); err != nil {
		return nil, err
	}
	return api.GetMailServer200JSONResponse(toMailServer(m)), nil
}

func (s apiServer) SetMailServer(ctx context.Context, req api.SetMailServerRequestObject) (api.SetMailServerResponseObject, error) {
	if !isAdmin(ctx) {
		return api.SetMailServer403JSONResponse{Error: forbidden}, nil
	}
	b := req.Body
	m := mailServerSettings{SMTP: notify.SMTP{Host: b.Host, Port: b.Port, Username: strings.TrimSpace(ptrValue(b.Username)), From: strings.TrimSpace(b.From), TLS: string(b.Tls)}}
	normalizeSMTP(&m.SMTP)
	if err := m.Check(); err != nil {
		return api.SetMailServer400JSONResponse{Error: err.Error()}, nil
	}
	stored, err := s.db.MailServer(ctx)
	if err != nil {
		return nil, err
	}
	m.sec = secrets{sealed: map[string]string{}, refs: map[string]secretRef{}}
	if stored != nil {
		if m.sec, err = s.openSecrets(stored.Secrets, stored.SecretRefs); err != nil {
			return nil, err
		}
	}
	if b.Password != nil {
		if err := m.sec.apply([]string{"password"}, map[string]api.SecretInput{"password": *b.Password}); err != nil {
			return api.SetMailServer400JSONResponse{Error: err.Error()}, nil
		}
	}
	spec, _ := json.Marshal(m.SMTP)
	sealed, refs := s.closeSecrets(m.sec)
	if err := s.db.SetMailServer(ctx, actor(ctx), &store.MailServer{Spec: spec, Secrets: sealed, SecretRefs: refs, UpdatedAt: time.Now().Unix()}); err != nil {
		return nil, err
	}
	return api.SetMailServer200JSONResponse(toMailServer(m)), nil
}

func (s apiServer) DeleteMailServer(ctx context.Context, _ api.DeleteMailServerRequestObject) (api.DeleteMailServerResponseObject, error) {
	if !isAdmin(ctx) {
		return api.DeleteMailServer403JSONResponse{Error: forbidden}, nil
	}
	return api.DeleteMailServer204Response{}, s.db.SetMailServer(ctx, actor(ctx), nil)
}

func (s apiServer) TestMailServer(ctx context.Context, req api.TestMailServerRequestObject) (api.TestMailServerResponseObject, error) {
	if !isAdmin(ctx) {
		return api.TestMailServer403JSONResponse{Error: forbidden}, nil
	}
	to := string(req.Body.To)
	if err := notify.CheckAddresses([]string{to}); err != nil {
		return api.TestMailServer400JSONResponse{Error: err.Error()}, nil
	}
	m, err := s.mailServer(ctx)
	if errors.Is(err, errNoMailServer) {
		return api.TestMailServer404JSONResponse{Error: err.Error()}, nil
	}
	if err == nil {
		err = notify.Email{Server: m.SMTP, To: []string{to}}.Send(ctx, notify.Message{Hub: s.URL(), Test: true})
	}
	return api.TestMailServer200JSONResponse(testResult(err)), nil
}
