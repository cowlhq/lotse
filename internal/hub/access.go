package hub

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"lotse.sh/internal/store"
)

// Access (decision 0023): a user or an API token sees a scope and what is
// below it, the global scope meaning everything. What is outside answers
// 404, as if it were not there. Whether something is inside is decided by
// the tree, by the scopes an object belongs to, never by comparing paths:
// a host in no environment has only its name for a path.

// accessOf is the scope a request may see: its API token's, or its user's.
func accessOf(ctx context.Context) (id int64, path string) {
	if t, ok := tokenOf(ctx); ok {
		return t.ScopeID, t.ScopePath
	}
	if u, ok := userOf(ctx); ok && u.ScopeID != 0 {
		return u.ScopeID, u.ScopePath
	}
	return store.GlobalScope, ""
}

// scopeOf is the ID of the scope a request may see.
func scopeOf(ctx context.Context) int64 {
	id, _ := accessOf(ctx)
	return id
}

// restricted says whether a request sees less than everything.
func restricted(ctx context.Context) bool {
	id, _ := accessOf(ctx)
	return id != store.GlobalScope
}

// isAdmin says whether the logged-in user may change anything: an admin of
// the whole hub. What belongs to the whole hub, such as users, tokens, the
// mail server and probes, is theirs alone.
func isAdmin(ctx context.Context) bool {
	u, ok := userOf(ctx)
	return ok && u.Role == "admin" && !restricted(ctx)
}

// isAnyAdmin says whether the logged-in user is an admin, of the whole hub
// or of a scope; what they may change is then what they see.
func isAnyAdmin(ctx context.Context) bool {
	u, ok := userOf(ctx)
	return ok && u.Role == "admin"
}

// errNotVisible is what an object outside a request's access is.
var errNotVisible = errors.New("not in the scope this request may see")

func notFound(err error) bool {
	return errors.Is(err, store.ErrNotFound) || errors.Is(err, errNotVisible) || errors.Is(err, errBadSubject)
}

// sees says whether an object is in the scope with this ID: whether the
// scope is the object's, or above it.
func (t *tree) sees(kind string, id, scopeID int64) bool {
	x, ok := t.scopes[scopeID]
	return ok && covers(t.chain(kind, id), x.Kind, x.ID)
}

// treeFor is the tree, for a request that sees less than everything; nil
// for one that sees everything, which needs no tree to tell.
func (s apiServer) treeFor(ctx context.Context) (*tree, error) {
	if !restricted(ctx) {
		return nil, nil
	}
	return s.loadTree(ctx)
}

// visible says whether a request may see an object, given treeFor's tree.
func visible(ctx context.Context, t *tree, kind string, id int64) bool {
	if t == nil {
		return true
	}
	scope, _ := accessOf(ctx)
	return t.sees(kind, id, scope)
}

// see fails with errNotVisible for an object outside the request's access.
// It does not check that the object is there; a request that sees
// everything is never refused.
func (s apiServer) see(ctx context.Context, kind string, id int64) error {
	t, err := s.treeFor(ctx)
	if err != nil {
		return err
	}
	if !visible(ctx, t, kind, id) {
		return errNotVisible
	}
	return nil
}

// seeSubject is see for a subject written kind:id.
func (s apiServer) seeSubject(ctx context.Context, subject string) error {
	kind, idText, _ := strings.Cut(subject, ":")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		return errBadSubject
	}
	return s.see(ctx, kind, id)
}

// mayChange says whether the logged-in user may change an object: an admin
// who sees it. It fails with errNotVisible for one outside their access,
// which the caller answers with 404.
func (s apiServer) mayChange(ctx context.Context, kind string, id int64) (bool, error) {
	if !isAnyAdmin(ctx) {
		return false, nil
	}
	return true, s.see(ctx, kind, id)
}

// scopedPaths are what a user kept to a scope may ask the API for; anything
// else, such as users, tokens, probes, the mail server and the hub's own
// update, belongs to the whole hub. Each handler here checks what it is
// asked about against the user's access.
var scopedPaths = []struct {
	methods string
	path    *regexp.Regexp
}{
	{"GET", regexp.MustCompile(`^/api/(health|updates|live)$`)},
	{"GET PATCH", regexp.MustCompile(`^/api/auth/me$`)},
	{"POST", regexp.MustCompile(`^/api/auth/logout$`)},
	{"GET", regexp.MustCompile(`^/api/scopes$`)},
	{"GET PATCH DELETE", regexp.MustCompile(`^/api/scopes/\d+$`)},
	{"GET POST", regexp.MustCompile(`^/api/scopes/\d+/enrollment$`)},
	{"POST", regexp.MustCompile(`^/api/scopes/\d+/update$`)},
	{"POST", regexp.MustCompile(`^/api/environments$`)},
	{"GET", regexp.MustCompile(`^/api/hosts$`)},
	{"GET", regexp.MustCompile(`^/api/hosts/\d+(/series)?$`)},
	{"DELETE", regexp.MustCompile(`^/api/hosts/\d+/agent$`)},
	{"PUT", regexp.MustCompile(`^/api/hosts/\d+/environment$`)},
	{"POST", regexp.MustCompile(`^/api/hosts/\d+/update$`)},
	{"GET", regexp.MustCompile(`^/api/filesystems/\d+/series$`)},
	{"GET", regexp.MustCompile(`^/api/containers/\d+(/series)?$`)},
	{"GET", regexp.MustCompile(`^/api/(events|changes|config)$`)},
	{"PUT", regexp.MustCompile(`^/api/settings$`)},
	{"GET POST", regexp.MustCompile(`^/api/checks$`)},
	{"GET PATCH DELETE", regexp.MustCompile(`^/api/checks/\d+$`)},
	{"GET", regexp.MustCompile(`^/api/checks/\d+/(series|results)$`)},
	{"GET", regexp.MustCompile(`^/api/alerts(/history|/\d+)?$`)},
	{"POST", regexp.MustCompile(`^/api/alerts/\d+/acknowledge$`)},
	{"GET POST", regexp.MustCompile(`^/api/channels$`)},
	{"PATCH DELETE", regexp.MustCompile(`^/api/channels/\d+$`)},
	{"POST", regexp.MustCompile(`^/api/channels/\d+/test$`)},
	{"GET POST", regexp.MustCompile(`^/api/maintenance$`)},
	{"DELETE", regexp.MustCompile(`^/api/maintenance/\d+$`)},
	// The read API, export and apply look at the access themselves.
	{"GET POST", regexp.MustCompile(`^/api/v1/`)},
}

// scopedAllowed says whether a user kept to a scope may make a request.
func scopedAllowed(r *http.Request) bool {
	method := r.Method
	if method == http.MethodHead {
		method = http.MethodGet
	}
	for _, p := range scopedPaths {
		if p.path.MatchString(r.URL.Path) && strings.Contains(" "+p.methods+" ", " "+method+" ") {
			return true
		}
	}
	return false
}

// withoutReply leaves out of a channel's error what the server answered,
// for a request kept to a scope: a channel may point anywhere, even into the
// hub's own network, and what answers there is not theirs to read.
func withoutReply(ctx context.Context, msg string) string {
	rest, ok := strings.CutPrefix(msg, "the server answered ")
	if !ok || !restricted(ctx) {
		return msg
	}
	status, _, _ := strings.Cut(rest, ": ")
	return "the server answered " + status
}
