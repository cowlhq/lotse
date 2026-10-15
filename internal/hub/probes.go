package hub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"lotse.sh/internal/auth"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/probe"
	"lotse.sh/internal/protocol"
	"lotse.sh/internal/store"
)

type wsWriter = *websocket.Conn

// probeConns are the external probes connected now.
type probeConns struct {
	mu   sync.Mutex
	conn map[*websocket.Conn]int64
	// types are the checks each connected probe runs.
	types map[int64][]string
}

type probeConn struct {
	conn    *websocket.Conn
	probeID int64
}

func (p *probeConns) add(c *websocket.Conn, id int64, types []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		p.conn, p.types = map[*websocket.Conn]int64{}, map[int64][]string{}
	}
	p.conn[c] = id
	p.types[id] = types
}

// runs says whether a probe runs checks of a type. The hub's own runs all
// there are; another, those it said it runs, or, until it said, those
// every probe runs.
func (p *probeConns) runs(id int64, typ string) bool {
	if id == store.BuiltinProbe {
		return true
	}
	if slices.Contains(probe.HubTypes, typ) {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	types, ok := p.types[id]
	if !ok {
		types = protocol.LegacyChecks
	}
	return slices.Contains(types, typ)
}

func (p *probeConns) remove(c *websocket.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.conn, c)
}

func (p *probeConns) all() []probeConn {
	p.mu.Lock()
	defer p.mu.Unlock()
	list := make([]probeConn, 0, len(p.conn))
	for c, id := range p.conn {
		list = append(list, probeConn{c, id})
	}
	return list
}

func (p *probeConns) online(id int64) bool {
	if id == store.BuiltinProbe {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, x := range p.conn {
		if x == id {
			return true
		}
	}
	return false
}

func (p *probeConns) closeAll() {
	for _, pc := range p.all() {
		go pc.conn.Close(websocket.StatusGoingAway, "hub stopping")
	}
}

// enrollProbe gives a probe with the probe token a credential of its own.
func (h *hub) enrollProbe(w http.ResponseWriter, r *http.Request) {
	var req protocol.ProbeEnrollRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.Token == "" {
		writeError(w, http.StatusBadRequest, "token, slug and name are needed")
		return
	}
	req.Slug = strings.TrimSpace(req.Slug)
	if !slugPattern.MatchString(req.Slug) || req.Slug == "hub" {
		writeError(w, http.StatusBadRequest, "a probe's name is lowercase letters, digits and dashes, and not hub")
		return
	}
	credential, credHash := auth.NewToken("lotse_prb_")
	p, err := h.db.EnrollProbe(r.Context(), auth.HashToken(req.Token), req.Slug,
		cmpOr(strings.TrimSpace(req.Name), req.Slug), strings.TrimSpace(req.Location), credHash, time.Now().Unix())
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "this probe token is not accepted")
		return
	}
	if err != nil {
		h.log.Error("cannot enroll a probe", "err", err)
		writeError(w, http.StatusInternalServerError, "the hub cannot enroll this probe")
		return
	}
	h.log.Info("probe enrolled", "probe", p.Slug, "location", p.Location)
	writeJSON(w, http.StatusOK, protocol.ProbeEnrollResponse{Credential: credential, ProbeID: p.ID})
}

// connectProbe is the WebSocket a probe keeps open: it gets its checks and
// sends their results.
func (h *hub) connectProbe(w http.ResponseWriter, r *http.Request) {
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	version := r.Header.Get("X-Lotse-Version")
	types := protocol.LegacyChecks
	if h := r.Header.Get(protocol.ChecksHeader); h != "" {
		types = strings.Split(h, ",")
	}
	p, err := h.db.ProbeByCredential(r.Context(), auth.HashToken(token), version, time.Now().Unix())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "this probe's credential is not accepted")
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: protocol.Subprotocols()})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)
	v, err := protocol.ParseSubprotocol(conn.Subprotocol())
	if err != nil || !slices.Contains(protocol.Versions, v) {
		conn.Close(websocket.StatusPolicyViolation, "no protocol version in common; update the probe or the hub")
		return
	}
	ctx := context.WithoutCancel(r.Context())
	log := h.log.With("probe", p.Slug)
	h.probes.add(conn, p.ID, types)
	log.Info("probe connected", "version", version)
	defer func() {
		h.probes.remove(conn)
		log.Info("probe disconnected")
	}()
	h.reassign(ctx)

	for {
		var m protocol.Message
		if err := wsjson.Read(ctx, conn, &m); err != nil {
			return
		}
		if m.Type != protocol.TypeResult || m.Result == nil {
			continue
		}
		res := m.Result
		h.checkResult(ctx, p.ID, res.CheckID, time.Unix(res.TS, 0), probe.Result{
			OK: res.OK, LatencyMS: res.LatencyMS, StatusCode: res.StatusCode, CertNotAfter: res.CertNotAfter, Error: res.Error,
		})
	}
}

func (s apiServer) ListProbes(ctx context.Context, _ api.ListProbesRequestObject) (api.ListProbesResponseObject, error) {
	probes, err := s.db.Probes(ctx)
	if err != nil {
		return nil, err
	}
	out := make(api.ListProbes200JSONResponse, len(probes))
	for i, p := range probes {
		out[i] = api.Probe{
			Id: p.ID, Slug: p.Slug, Name: p.Name, Location: p.Location, Builtin: p.Builtin,
			Online: p.RevokedAt == nil && s.probes.online(p.ID), Version: p.Version, LastSeen: p.LastSeen,
			Revoked: ptr(p.RevokedAt != nil),
		}
		if p.Builtin {
			out[i].Version = versionString()
		}
	}
	return out, nil
}

func (s apiServer) RevokeProbe(ctx context.Context, req api.RevokeProbeRequestObject) (api.RevokeProbeResponseObject, error) {
	if !isAdmin(ctx) {
		return api.RevokeProbe403JSONResponse{Error: forbidden}, nil
	}
	err := s.db.RevokeProbe(ctx, actor(ctx), req.Id, time.Now().Unix())
	switch {
	case errors.Is(err, store.ErrNotFound):
		return api.RevokeProbe404JSONResponse{Error: "no such probe"}, nil
	case err != nil:
		return api.RevokeProbe400JSONResponse{Error: err.Error()}, nil
	}
	for _, pc := range s.probes.all() {
		if pc.probeID == req.Id {
			pc.conn.Close(websocket.StatusPolicyViolation, "this probe's credential was revoked")
		}
	}
	s.reassign(context.WithoutCancel(ctx))
	return api.RevokeProbe204Response{}, nil
}

func (s apiServer) GetProbeEnrollment(ctx context.Context, _ api.GetProbeEnrollmentRequestObject) (api.GetProbeEnrollmentResponseObject, error) {
	if !isAdmin(ctx) {
		return api.GetProbeEnrollment403JSONResponse{Error: forbidden}, nil
	}
	t, err := s.db.EnrollmentToken(ctx, store.GlobalScope)
	if errors.Is(err, store.ErrNotFound) {
		return api.GetProbeEnrollment404JSONResponse{Error: "there is no probe token yet"}, nil
	}
	if err != nil {
		return nil, err
	}
	e, err := s.enrollment(t)
	if err != nil {
		return nil, err
	}
	return api.GetProbeEnrollment200JSONResponse(e), nil
}

func (s apiServer) NewProbeEnrollment(ctx context.Context, _ api.NewProbeEnrollmentRequestObject) (api.NewProbeEnrollmentResponseObject, error) {
	if !isAdmin(ctx) {
		return api.NewProbeEnrollment403JSONResponse{Error: forbidden}, nil
	}
	token, hash := auth.NewToken("lotse_enr_")
	if err := s.db.NewEnrollmentToken(ctx, actor(ctx), store.GlobalScope, hash, s.box.Seal(token), time.Now().Unix()); err != nil {
		return nil, err
	}
	t, err := s.db.EnrollmentToken(ctx, store.GlobalScope)
	if err != nil {
		return nil, err
	}
	e, err := s.enrollment(t)
	if err != nil {
		return nil, err
	}
	return api.NewProbeEnrollment200JSONResponse(e), nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
