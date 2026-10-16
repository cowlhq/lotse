package hub

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
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
	// Server is an ntfy channel's server, https://ntfy.sh unless set.
	Server string `json:"server,omitempty"`
	// ChatID is the Telegram chat a bot sends to.
	ChatID string `json:"chat_id,omitempty"`
}

// channelTypes are the types of channel, as the API names them.
var channelTypes = []string{"email", "googlechat", "webhook", "slack", "discord", "telegram", "ntfy", "pushover"}

// defaultNtfy is where an ntfy channel publishes unless it says otherwise.
const defaultNtfy = "https://ntfy.sh"

// secretFields are the secrets each type of channel has.
var secretFields = map[string][]string{
	"email":      {"password"},
	"googlechat": {"url"},
	"webhook":    {"url", "signing_key"},
	"slack":      {"url"},
	"discord":    {"url"},
	"telegram":   {"bot_token"},
	"ntfy":       {"topic", "token"},
	"pushover":   {"token", "user"},
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
	case "googlechat", "webhook", "slack", "discord":
		if spec.To != nil || spec.SMTP != nil || spec.Server != "" || spec.ChatID != "" {
			return nil, fmt.Errorf("a %s channel has nothing but its secrets", typ)
		}
		spec = channelSpec{}
		if err := needSecrets(sec, "url"); err != nil {
			return nil, err
		}
		if v, ok := sec.sealed["url"]; ok && !isHTTP(v) {
			return nil, errors.New("url is not an http or https address")
		}
	case "telegram":
		spec = channelSpec{ChatID: strings.TrimSpace(spec.ChatID)}
		if spec.ChatID == "" {
			return nil, errors.New("a telegram channel needs chat_id, the chat its bot sends to")
		}
		if err := needSecrets(sec, "bot_token"); err != nil {
			return nil, err
		}
	case "ntfy":
		spec = channelSpec{Server: strings.TrimSuffix(strings.TrimSpace(spec.Server), "/")}
		if spec.Server == defaultNtfy {
			spec.Server = ""
		}
		if spec.Server != "" && !isHTTP(spec.Server) {
			return nil, errors.New("server is not an http or https address")
		}
		if err := needSecrets(sec, "topic"); err != nil {
			return nil, err
		}
	case "pushover":
		spec = channelSpec{}
		if err := needSecrets(sec, "token", "user"); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("type is one of " + strings.Join(channelTypes, ", "))
	}
	return json.Marshal(spec)
}

// needSecrets says which of a channel's required secrets are missing.
func needSecrets(sec secrets, fields ...string) error {
	var missing []string
	for _, f := range fields {
		if !sec.has(f) {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("the channel needs %s", strings.Join(missing, " and "))
	}
	return nil
}

func isHTTP(v string) bool {
	u, err := url.Parse(v)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
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
	}
	// Every other type is its secrets, and a little spec.
	v := map[string]string{}
	for _, f := range secretFields[c.Type] {
		if v[f], err = sec.get(f); err != nil {
			return nil, err
		}
	}
	switch c.Type {
	case "googlechat":
		return notify.GoogleChat{URL: v["url"]}, nil
	case "webhook":
		return notify.Webhook{URL: v["url"], SigningKey: v["signing_key"]}, nil
	case "slack":
		return notify.Slack{URL: v["url"]}, nil
	case "discord":
		return notify.Discord{URL: v["url"]}, nil
	case "telegram":
		return notify.Telegram{BotToken: v["bot_token"], ChatID: spec.ChatID}, nil
	case "ntfy":
		return notify.Ntfy{Server: cmp.Or(spec.Server, defaultNtfy), Topic: v["topic"], Token: v["token"]}, nil
	case "pushover":
		return notify.Pushover{Token: v["token"], User: v["user"]}, nil
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
	// A request kept to a scope sees the channels in it, not those of the
	// scopes above, which belong to others.
	t, err := s.treeFor(ctx)
	if err != nil {
		return nil, err
	}
	out := api.ListChannels200JSONResponse{}
	for _, c := range list {
		if (chain == nil || slices.Contains(chain, c.ScopeID)) && (t == nil || visible(ctx, t, t.scopes[c.ScopeID].Kind, c.ScopeID)) {
			out = append(out, s.toChannel(c))
		}
	}
	return out, nil
}

// mayChangeChannel says whether the user may change a channel: an admin
// who sees its scope.
func (s apiServer) mayChangeChannel(ctx context.Context, scopeID int64) (bool, error) {
	x, err := s.db.Scope(ctx, scopeID)
	if err != nil {
		return false, err
	}
	return s.mayChange(ctx, x.Kind, x.ID)
}

func (s apiServer) CreateChannel(ctx context.Context, req api.CreateChannelRequestObject) (api.CreateChannelResponseObject, error) {
	b := req.Body
	if ok, err := s.mayChangeChannel(ctx, b.ScopeId); notFound(err) {
		return api.CreateChannel400JSONResponse{Error: "no such scope"}, nil
	} else if err != nil {
		return nil, err
	} else if !ok {
		return api.CreateChannel403JSONResponse{Error: forbidden}, nil
	}
	name, key := strings.TrimSpace(b.Name), strings.TrimSpace(b.Key)
	if msg := checkName(name); msg != "" {
		return api.CreateChannel400JSONResponse{Error: msg}, nil
	}
	if !slugPattern.MatchString(key) {
		return api.CreateChannel400JSONResponse{Error: "a key has lowercase letters, digits and dashes, up to 40"}, nil
	}
	sec := secrets{sealed: map[string]string{}, refs: map[string]secretRef{}}
	if err := sec.apply(secretFields[string(b.Type)], ptrValue(b.Secrets), !restricted(ctx), nil); err != nil {
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
	c, err := s.db.Channel(ctx, req.Id)
	ok := false
	if err == nil {
		ok, err = s.mayChangeChannel(ctx, c.ScopeID)
	}
	if notFound(err) {
		return api.UpdateChannel404JSONResponse{Error: "no such channel"}, nil
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return api.UpdateChannel403JSONResponse{Error: forbidden}, nil
	}
	name := strings.TrimSpace(req.Body.Name)
	if msg := checkName(name); msg != "" {
		return api.UpdateChannel400JSONResponse{Error: msg}, nil
	}
	sec, err := s.openSecrets(c.Secrets, c.SecretRefs)
	if err != nil {
		return nil, err
	}
	if err := sec.apply(secretFields[c.Type], ptrValue(req.Body.Secrets), !restricted(ctx), maps.Clone(sec.refs)); err != nil {
		return api.UpdateChannel400JSONResponse{Error: err.Error()}, nil
	}
	spec, err := checkChannel(c.Type, req.Body.Spec, sec)
	if err != nil {
		return api.UpdateChannel400JSONResponse{Error: err.Error()}, nil
	}
	if msg := refsStay(restricted(ctx), c.SecretRefs, c.Spec, spec, "channel"); msg != "" {
		return api.UpdateChannel400JSONResponse{Error: msg}, nil
	}
	c.Name, c.Spec = name, spec
	c.Secrets, c.SecretRefs = s.closeSecrets(sec)
	if c, err = s.db.UpdateChannel(ctx, actor(ctx), c); err != nil {
		return nil, err
	}
	return api.UpdateChannel200JSONResponse(s.toChannel(c)), nil
}

func (s apiServer) DeleteChannel(ctx context.Context, req api.DeleteChannelRequestObject) (api.DeleteChannelResponseObject, error) {
	c, err := s.db.Channel(ctx, req.Id)
	ok := false
	if err == nil {
		ok, err = s.mayChangeChannel(ctx, c.ScopeID)
	}
	if notFound(err) {
		return api.DeleteChannel404JSONResponse{Error: "no such channel"}, nil
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return api.DeleteChannel403JSONResponse{Error: forbidden}, nil
	}
	err = s.db.DeleteChannel(ctx, actor(ctx), req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return api.DeleteChannel404JSONResponse{Error: "no such channel"}, nil
	}
	return api.DeleteChannel204Response{}, err
}

func (s apiServer) TestChannel(ctx context.Context, req api.TestChannelRequestObject) (api.TestChannelResponseObject, error) {
	c, err := s.db.Channel(ctx, req.Id)
	ok := false
	if err == nil {
		ok, err = s.mayChangeChannel(ctx, c.ScopeID)
	}
	if notFound(err) {
		return api.TestChannel404JSONResponse{Error: "no such channel"}, nil
	}
	if err == nil && !ok {
		return api.TestChannel403JSONResponse{Error: forbidden}, nil
	}
	if err != nil {
		return nil, err
	}
	sender, err := s.sender(ctx, c)
	if err == nil {
		err = sender.Send(ctx, notify.Message{Hub: s.URL(), Test: true})
	}
	res := testResult(err)
	if res.Error != nil {
		res.Error = ptr(withoutReply(ctx, *res.Error))
	}
	return api.TestChannel200JSONResponse(res), nil
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
		if err := m.sec.apply([]string{"password"}, map[string]api.SecretInput{"password": *b.Password}, true, nil); err != nil {
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
