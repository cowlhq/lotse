package hub

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket/wsjson"

	"lotse.sh/internal/auth"
	"lotse.sh/internal/config"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/probe"
	"lotse.sh/internal/protocol"
	"lotse.sh/internal/store"
)

// Checks (design 8.3): probes run them, each at the check's interval. A
// check is down when the majority of the probes with a fresh result see it
// fail, and unknown when none has one.

// assignment is who runs a check, and how often.
type assignment struct {
	probe.Assignment
	probes []int64
}

// checkState is what the hub knows about checks between results.
type checkState struct {
	mu sync.Mutex
	// assigned is every check's assignment, rebuilt by reassign.
	assigned map[int64]assignment
	// latest is each check's newest result per probe, and fails how many
	// runs in a row failed there.
	latest map[int64]map[int64]resultAt
	fails  map[int64]map[int64]int
}

type resultAt struct {
	at time.Time
	r  probe.Result
}

// domainEvery is how often a domain's expiry is looked up.
const domainEvery = 6 * time.Hour

// reassign works out which probe runs which check, and tells them all.
func (h *hub) reassign(ctx context.Context) {
	checks, err := h.db.Checks(ctx, 0)
	if err != nil {
		h.log.Error("cannot list checks", "err", err)
		return
	}
	probes, err := h.db.Probes(ctx)
	if err != nil {
		h.log.Error("cannot list probes", "err", err)
		return
	}
	assigned := map[int64]assignment{}
	perProbe := map[int64][]probe.Assignment{}
	for _, c := range checks {
		if c.Type == "heartbeat" {
			continue
		}
		values, err := h.effective(ctx, "check:"+strconv.FormatInt(c.ID, 10))
		if err != nil {
			continue
		}
		a := assignment{Assignment: probe.Assignment{CheckID: c.ID, Type: c.Type, Spec: c.Spec, Interval: time.Minute, Timeout: 10 * time.Second}}
		names := []string{"all"}
		for _, v := range values {
			switch v.Key {
			case "check.interval":
				a.Interval, _ = config.Duration(v.Value)
			case "check.timeout":
				a.Timeout, _ = config.Duration(v.Value)
			case "check.probes":
				var all string
				if json.Unmarshal(v.Value, &all) != nil {
					json.Unmarshal(v.Value, &names)
				}
			}
		}
		if h.checkInterval > 0 {
			a.Interval = h.checkInterval
		}
		if c.Type == "domain" && h.checkInterval == 0 {
			// Registries answer slowly, and dates change seldom: a few
			// times a day is often enough, and polite.
			a.Interval, a.Timeout = max(a.Interval, domainEvery), max(a.Timeout, 30*time.Second)
		}
		for _, p := range probes {
			if p.RevokedAt != nil || !(slices.Contains(names, "all") || slices.Contains(names, p.Slug)) || !h.probes.runs(p.ID, probe.Needs(c.Type, c.Spec)) {
				continue
			}
			a.probes = append(a.probes, p.ID)
			perProbe[p.ID] = append(perProbe[p.ID], a.Assignment)
		}
		assigned[c.ID] = a
	}
	h.checks.mu.Lock()
	h.checks.assigned = assigned
	h.checks.mu.Unlock()

	h.builtinProbe.Set(context.WithoutCancel(ctx), perProbe[store.BuiltinProbe])
	for _, pc := range h.probes.all() {
		h.sendChecks(ctx, pc.conn, perProbe[pc.probeID])
	}
}

// sendChecks tells an external probe what to run.
func (h *hub) sendChecks(ctx context.Context, conn wsWriter, list []probe.Assignment) {
	out := make([]protocol.ProbeCheck, len(list))
	for i, a := range list {
		out[i] = protocol.ProbeCheck{CheckID: a.CheckID, Type: a.Type, Spec: a.Spec,
			Interval: int(a.Interval / time.Second), Timeout: int(a.Timeout / time.Second)}
	}
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	wsjson.Write(wctx, conn, protocol.Message{Type: protocol.TypeChecks, Checks: &out})
}

// checkResult records a run and works out the check's state.
func (h *hub) checkResult(ctx context.Context, probeID, checkID int64, at time.Time, r probe.Result) {
	x := store.CheckResult{CheckID: checkID, ProbeID: probeID, TS: at.Unix(), OK: r.OK, Error: r.Error}
	if r.LatencyMS > 0 {
		x.LatencyMS = &r.LatencyMS
	}
	if r.StatusCode != 0 {
		x.StatusCode = &r.StatusCode
	}
	if r.CertNotAfter != 0 {
		x.CertNotAfter = &r.CertNotAfter
	}
	h.writer.add(func(b *store.Batch) { b.CheckResults = append(b.CheckResults, x) })

	h.checks.mu.Lock()
	if h.checks.latest == nil {
		h.checks.latest = map[int64]map[int64]resultAt{}
	}
	if h.checks.latest[checkID] == nil {
		h.checks.latest[checkID] = map[int64]resultAt{}
	}
	h.checks.latest[checkID][probeID] = resultAt{at, r}
	if h.checks.fails == nil {
		h.checks.fails = map[int64]map[int64]int{}
	}
	if h.checks.fails[checkID] == nil {
		h.checks.fails[checkID] = map[int64]int{}
	}
	if _, known := h.checks.fails[checkID][probeID]; !known {
		// The first result since the hub started: the count goes on from
		// the results it kept, so a restart does not clear a failing check.
		n, err := h.db.FailedInARow(ctx, checkID, probeID)
		if err != nil {
			h.log.Warn("cannot count a check's failures", "check_id", checkID, "err", err)
		}
		h.checks.fails[checkID][probeID] = n
	}
	if r.OK {
		h.checks.fails[checkID][probeID] = 0
	} else {
		h.checks.fails[checkID][probeID]++
	}
	a := h.checks.assigned[checkID]
	results := h.checks.latest[checkID]
	var fresh []probe.Result
	for _, id := range a.probes {
		if ra, ok := results[id]; ok && time.Since(ra.at) < 3*a.Interval+a.Timeout {
			fresh = append(fresh, ra.r)
		}
	}
	h.checks.mu.Unlock()

	state, reason := quorum(fresh)
	h.setCheckState(ctx, checkID, state, reason)
}

// standing is how a check stands for the alert rules, from its probes'
// fresh results: how many runs in a row failed on most of them, the
// earliest expiry of a certificate they saw, and an error one of them got.
// fresh is false when no probe has a fresh result.
type standing struct {
	failed       int
	certNotAfter int64
	reason       string
	fresh        bool
}

func (h *hub) checkStanding(id int64, now time.Time) standing {
	h.checks.mu.Lock()
	defer h.checks.mu.Unlock()
	a := h.checks.assigned[id]
	var out standing
	var counts []int
	for _, p := range a.probes {
		ra, ok := h.checks.latest[id][p]
		if !ok || now.Sub(ra.at) >= 3*a.Interval+a.Timeout {
			continue
		}
		counts = append(counts, h.checks.fails[id][p])
		if c := ra.r.CertNotAfter; c > 0 && (out.certNotAfter == 0 || c < out.certNotAfter) {
			out.certNotAfter = c
		}
		if !ra.r.OK && ra.r.Error != "" {
			out.reason = ra.r.Error
		}
	}
	if len(counts) == 0 {
		return out
	}
	// The count that more than half of the probes reach: with three
	// probes, the second highest.
	slices.Sort(counts)
	slices.Reverse(counts)
	out.failed, out.fresh = counts[len(counts)/2], true
	return out
}

// quorum is a check's state from the probes' fresh results: down when
// most fail, unknown when there are none.
func quorum(fresh []probe.Result) (state, reason string) {
	if len(fresh) == 0 {
		return "unknown", ""
	}
	failed := 0
	for _, r := range fresh {
		if !r.OK {
			failed++
			reason = r.Error
		}
	}
	if failed*2 > len(fresh) {
		return "down", reason
	}
	return "up", ""
}

// setCheckState records a check's state, and an event if it changed.
func (h *hub) setCheckState(ctx context.Context, checkID int64, state, reason string) {
	changed, err := h.db.SetCheckState(ctx, checkID, state, time.Now().Unix())
	if err != nil {
		h.log.Error("cannot save a check's state", "check_id", checkID, "err", err)
		return
	}
	if !changed {
		return
	}
	data := map[string]string{}
	if reason != "" {
		data["error"] = reason
	}
	h.writer.add(func(b *store.Batch) {
		b.Events = append(b.Events, store.Event{
			TS: time.Now().Unix(), SubjectKind: "check", SubjectID: checkID, Kind: "check." + state, Data: mustJSON(data),
		})
	})
}

// toCheck is a check for the API. Only admins see a heartbeat's ping URL,
// which is as good as its secret.
func (s apiServer) toCheck(ctx context.Context, c store.Check) api.Check {
	var spec map[string]any
	json.Unmarshal(c.Spec, &spec)
	out := api.Check{
		Id: c.ID, EnvironmentId: c.EnvironmentID, Slug: c.Slug, Path: c.Path, Name: c.Name,
		Type: api.CheckType(c.Type), Spec: spec, State: api.CheckState(c.State),
		StateSince: c.StateSince, LastPing: c.LastPing,
	}
	if c.PingSealed != nil && isAnyAdmin(ctx) {
		if token, err := s.box.Open(*c.PingSealed); err == nil {
			out.PingUrl = ptr(s.URL() + "/ping/" + token)
		}
	}
	s.checks.mu.Lock()
	results := s.checks.latest[c.ID]
	var newest *resultAt
	for id, ra := range results {
		if id == store.BuiltinProbe || newest == nil {
			r := ra
			newest = &r
		}
	}
	for _, ra := range results {
		if !ra.r.OK && ra.r.Error != "" {
			out.Error = ptr(ra.r.Error)
		}
	}
	s.checks.mu.Unlock()
	if newest != nil {
		if newest.r.LatencyMS > 0 {
			out.LatencyMs = ptr(newest.r.LatencyMS)
		}
		if newest.r.CertNotAfter > 0 {
			out.CertNotAfter = ptr(newest.r.CertNotAfter)
		}
	}
	return out
}

func (s apiServer) ListChecks(ctx context.Context, req api.ListChecksRequestObject) (api.ListChecksResponseObject, error) {
	list, err := s.db.Checks(ctx, ptrValue(req.Params.Environment))
	if err != nil {
		return nil, err
	}
	t, err := s.treeFor(ctx)
	if err != nil {
		return nil, err
	}
	out := api.ListChecks200JSONResponse{}
	for _, c := range list {
		if visible(ctx, t, "check", c.ID) {
			out = append(out, s.toCheck(ctx, c))
		}
	}
	return out, nil
}

func (s apiServer) CreateCheck(ctx context.Context, req api.CreateCheckRequestObject) (api.CreateCheckResponseObject, error) {
	b := req.Body
	if ok, err := s.mayChange(ctx, "environment", b.EnvironmentId); notFound(err) {
		return api.CreateCheck400JSONResponse{Error: "no such environment"}, nil
	} else if err != nil {
		return nil, err
	} else if !ok {
		return api.CreateCheck403JSONResponse{Error: forbidden}, nil
	}
	name := strings.TrimSpace(b.Name)
	if msg := checkName(name); msg != "" {
		return api.CreateCheck400JSONResponse{Error: msg}, nil
	}
	slug := strings.TrimSpace(ptrValue(b.Slug))
	if slug == "" {
		slug = slugify(name)
	}
	if !slugPattern.MatchString(slug) {
		return api.CreateCheck400JSONResponse{Error: "a key has lowercase letters, digits and dashes, up to 40"}, nil
	}
	raw, _ := json.Marshal(b.Spec)
	spec, err := probe.Normalize(string(b.Type), raw)
	if err != nil {
		return api.CreateCheck400JSONResponse{Error: err.Error()}, nil
	}
	c := store.Check{EnvironmentID: b.EnvironmentId, Slug: slug, Name: name, Type: string(b.Type), Spec: spec, CreatedAt: time.Now().Unix()}
	var pingHash string
	if c.Type == "heartbeat" {
		token, hash := auth.NewToken("")
		sealed := s.box.Seal(token)
		c.PingSealed, pingHash = &sealed, hash
	}
	c, err = s.db.CreateCheck(ctx, actor(ctx), c, pingHash)
	switch {
	case errors.Is(err, store.ErrExists):
		return api.CreateCheck409JSONResponse{Error: "the environment has a check with this key"}, nil
	case errors.Is(err, store.ErrNotFound):
		return api.CreateCheck400JSONResponse{Error: "no such environment"}, nil
	case err != nil:
		return nil, err
	}
	s.reassign(context.WithoutCancel(ctx))
	return api.CreateCheck201JSONResponse(s.toCheck(ctx, c)), nil
}

func (s apiServer) GetCheck(ctx context.Context, req api.GetCheckRequestObject) (api.GetCheckResponseObject, error) {
	c, err := s.db.Check(ctx, req.Id)
	if err == nil {
		err = s.see(ctx, "check", req.Id)
	}
	if notFound(err) {
		return api.GetCheck404JSONResponse{Error: "no such check"}, nil
	}
	if err != nil {
		return nil, err
	}
	results, err := s.db.LatestResults(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	names, err := s.probeNames(ctx)
	if err != nil {
		return nil, err
	}
	out := api.CheckDetail{Probes: []api.CheckResult{}}
	base := s.toCheck(ctx, c)
	b, _ := json.Marshal(base)
	json.Unmarshal(b, &out)
	out.Probes = []api.CheckResult{}
	for _, r := range results {
		out.Probes = append(out.Probes, toCheckResult(r, names))
	}
	return api.GetCheck200JSONResponse(out), nil
}

func (s apiServer) probeNames(ctx context.Context) (map[int64]string, error) {
	probes, err := s.db.Probes(ctx)
	if err != nil {
		return nil, err
	}
	names := map[int64]string{}
	for _, p := range probes {
		names[p.ID] = p.Name
	}
	return names, nil
}

func toCheckResult(r store.CheckResult, names map[int64]string) api.CheckResult {
	out := api.CheckResult{Probe: names[r.ProbeID], Ts: r.TS, Ok: r.OK, LatencyMs: r.LatencyMS, StatusCode: r.StatusCode, CertNotAfter: r.CertNotAfter}
	if r.Error != "" {
		out.Error = ptr(r.Error)
	}
	return out
}

func (s apiServer) UpdateCheck(ctx context.Context, req api.UpdateCheckRequestObject) (api.UpdateCheckResponseObject, error) {
	c, err := s.db.Check(ctx, req.Id)
	ok := false
	if err == nil {
		ok, err = s.mayChange(ctx, "check", req.Id)
	}
	if notFound(err) {
		return api.UpdateCheck404JSONResponse{Error: "no such check"}, nil
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return api.UpdateCheck403JSONResponse{Error: forbidden}, nil
	}
	name := strings.TrimSpace(req.Body.Name)
	if msg := checkName(name); msg != "" {
		return api.UpdateCheck400JSONResponse{Error: msg}, nil
	}
	raw, _ := json.Marshal(req.Body.Spec)
	spec, err := probe.Normalize(c.Type, raw)
	if err != nil {
		return api.UpdateCheck400JSONResponse{Error: err.Error()}, nil
	}
	c, err = s.db.UpdateCheck(ctx, actor(ctx), c.ID, name, spec)
	if err != nil {
		return nil, err
	}
	s.reassign(context.WithoutCancel(ctx))
	return api.UpdateCheck200JSONResponse(s.toCheck(ctx, c)), nil
}

func (s apiServer) DeleteCheck(ctx context.Context, req api.DeleteCheckRequestObject) (api.DeleteCheckResponseObject, error) {
	_, err := s.db.Check(ctx, req.Id)
	ok := false
	if err == nil {
		ok, err = s.mayChange(ctx, "check", req.Id)
	}
	if notFound(err) {
		return api.DeleteCheck404JSONResponse{Error: "no such check"}, nil
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return api.DeleteCheck403JSONResponse{Error: forbidden}, nil
	}
	err = s.db.DeleteCheck(ctx, actor(ctx), req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return api.DeleteCheck404JSONResponse{Error: "no such check"}, nil
	}
	if err != nil {
		return nil, err
	}
	s.checks.mu.Lock()
	delete(s.checks.latest, req.Id)
	delete(s.checks.fails, req.Id)
	s.checks.mu.Unlock()
	s.reassign(context.WithoutCancel(ctx))
	return api.DeleteCheck204Response{}, nil
}

func (s apiServer) GetCheckSeries(ctx context.Context, req api.GetCheckSeriesRequestObject) (api.GetCheckSeriesResponseObject, error) {
	if msg := checkPoints(req.Params.Points); msg != "" {
		return api.GetCheckSeries400JSONResponse{Error: msg}, nil
	}
	_, err := s.db.Check(ctx, req.Id)
	if err == nil {
		err = s.see(ctx, "check", req.Id)
	}
	if notFound(err) {
		return api.GetCheckSeries404JSONResponse{Error: "no such check"}, nil
	}
	if err != nil {
		return nil, err
	}
	sr, err := s.series(ctx, "check", req.Id, req.Params.From, req.Params.To, req.Params.Points, 10*time.Second)
	return api.GetCheckSeries200JSONResponse(sr), err
}

func (s apiServer) ListCheckResults(ctx context.Context, req api.ListCheckResultsRequestObject) (api.ListCheckResultsResponseObject, error) {
	if err := s.see(ctx, "check", req.Id); notFound(err) {
		return api.ListCheckResults200JSONResponse{}, nil
	} else if err != nil {
		return nil, err
	}
	from, to := timeRange(req.Params.From, req.Params.To)
	results, err := s.db.Results(ctx, req.Id, from, to, 200)
	if err != nil {
		return nil, err
	}
	names, err := s.probeNames(ctx)
	if err != nil {
		return nil, err
	}
	out := make(api.ListCheckResults200JSONResponse, len(results))
	for i, r := range results {
		out[i] = toCheckResult(r, names)
	}
	return out, nil
}

// slugify makes a key from a name, as the UI does.
func slugify(name string) string {
	r := strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss")
	s := r.Replace(strings.ToLower(name))
	var b strings.Builder
	dash := false
	for _, c := range s {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			b.WriteRune(c)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if len(out) > 40 {
		out = strings.TrimSuffix(out[:40], "-")
	}
	return out
}
