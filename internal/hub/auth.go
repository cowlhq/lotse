package hub

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"lotse.sh/internal/auth"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/protocol"
	"lotse.sh/internal/store"
)

const (
	sessionCookie = "lotse_session"
	// A session lasts this long after it was last used.
	sessionLifetime = 30 * 24 * time.Hour
	// It is extended at most this often, not on every request.
	sessionExtendEvery = time.Hour
)

type ctxKey int

const (
	ctxRequest ctxKey = iota
	ctxUser
)

// request is what handlers need to know about the HTTP request, which the
// generated strict handlers do not see.
type request struct {
	ip      string
	secure  bool
	session string // hash of the session token, if there is one
}

func requestOf(ctx context.Context) request {
	r, _ := ctx.Value(ctxRequest).(request)
	return r
}

func userOf(ctx context.Context) (store.User, bool) {
	u, ok := ctx.Value(ctxUser).(store.User)
	return u, ok
}

// Paths that work without a session. Agents prove themselves with their
// own token.
func publicPath(path string) bool {
	return path == "/api/health" || path == "/api/auth/login" ||
		strings.HasPrefix(path, "/api/setup/") || path == protocol.ConnectPath || path == protocol.EnrollPath ||
		path == protocol.CheckPath
}

// guard stands before the API:
//   - during the database step of setup, only health and setup answer;
//   - a request that changes something and comes from another site's page
//     is refused (the Origin header says where a browser request comes
//     from);
//   - everything but the public paths needs a session.
func (h *hub) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if !strings.HasPrefix(path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if h.db == nil && path != "/api/health" && !strings.HasPrefix(path, "/api/setup/") {
			writeError(w, http.StatusServiceUnavailable, "the hub is not set up yet")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if origin := r.Header.Get("Origin"); origin != "" && !h.allowedOrigin(r, origin) {
				writeError(w, http.StatusForbidden, "this request comes from another site")
				return
			}
		}
		info := request{ip: clientIP(r), secure: h.secureRequest(r)}
		if c, err := r.Cookie(sessionCookie); err == nil && h.db != nil {
			info.session = auth.HashToken(c.Value)
		}
		ctx := context.WithValue(r.Context(), ctxRequest, info)
		if !publicPath(path) {
			user, err := h.sessionUser(ctx, info.session)
			if err != nil {
				if !errors.Is(err, store.ErrNotFound) {
					h.log.Error("cannot check a session", "err", err)
				}
				writeError(w, http.StatusUnauthorized, "log in first")
				return
			}
			ctx = context.WithValue(ctx, ctxUser, user)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// allowedOrigin says whether a browser request may come from origin: the
// hub's own address as the request reached it, or the one people use.
func (h *hub) allowedOrigin(r *http.Request, origin string) bool {
	o, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if o.Host == r.Host {
		return true
	}
	pub, err := url.Parse(h.URL())
	return err == nil && o.Host == pub.Host
}

// secureRequest says whether the browser reached the hub over HTTPS, in
// which case the session cookie is only ever sent over HTTPS.
func (h *hub) secureRequest(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" || strings.HasPrefix(h.URL(), "https://")
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (h *hub) sessionUser(ctx context.Context, idHash string) (store.User, error) {
	if idHash == "" {
		return store.User{}, store.ErrNotFound
	}
	now := time.Now()
	s, u, err := h.db.SessionUser(ctx, idHash, now.Unix())
	if err != nil {
		return u, err
	}
	if now.Unix()-s.LastSeenAt > int64(sessionExtendEvery/time.Second) {
		if err := h.db.ExtendSession(ctx, idHash, now.Unix(), now.Add(sessionLifetime).Unix()); err != nil {
			h.log.Error("cannot extend a session", "err", err)
		}
	}
	return u, nil
}

// startSession logs a user in and returns the cookie for it.
func (h *hub) startSession(ctx context.Context, userID int64) (string, error) {
	token, hash := auth.NewToken("lotse_ses_")
	now := time.Now()
	if err := h.db.CreateSession(ctx, store.Session{
		IDHash: hash, UserID: userID, CreatedAt: now.Unix(),
		ExpiresAt: now.Add(sessionLifetime).Unix(), LastSeenAt: now.Unix(),
	}); err != nil {
		return "", err
	}
	c := http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: requestOf(ctx).secure,
		MaxAge: int(sessionLifetime / time.Second),
	}
	return c.String(), nil
}

func endedSession(ctx context.Context) string {
	c := http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: requestOf(ctx).secure, MaxAge: -1,
	}
	return c.String()
}

// A hash to check passwords against when there is no such user, so that
// the answer takes as long either way.
var dummyHash, _ = auth.HashPassword("not anybody's password")

func (s apiServer) Login(ctx context.Context, req api.LoginRequestObject) (api.LoginResponseObject, error) {
	info := requestOf(ctx)
	keys := []string{"ip:" + info.ip, "email:" + strings.ToLower(strings.TrimSpace(req.Body.Email))}
	if !s.logins.Allowed(keys...) {
		return api.Login429JSONResponse{Error: auth.ErrTooManyAttempts.Error()}, nil
	}
	user, err := s.db.UserByEmail(ctx, req.Body.Email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	hash := user.PasswordHash
	if err != nil || user.DisabledAt != nil {
		hash = dummyHash
	}
	if !auth.CheckPassword(hash, req.Body.Password) || hash == dummyHash {
		s.logins.Failed(keys...)
		return api.Login401JSONResponse{Error: "wrong email or password"}, nil
	}
	s.logins.Succeeded(keys...)
	cookie, err := s.startSession(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return api.Login200JSONResponse{Body: toUser(user), Headers: api.Login200ResponseHeaders{SetCookie: &cookie}}, nil
}

func (s apiServer) Logout(ctx context.Context, _ api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	if id := requestOf(ctx).session; id != "" {
		if err := s.db.DeleteSession(ctx, id); err != nil {
			return nil, err
		}
	}
	return api.Logout204Response{Headers: api.Logout204ResponseHeaders{SetCookie: ptr(endedSession(ctx))}}, nil
}

func (s apiServer) GetMe(ctx context.Context, _ api.GetMeRequestObject) (api.GetMeResponseObject, error) {
	u, _ := userOf(ctx)
	return api.GetMe200JSONResponse(toUser(u)), nil
}

func toUser(u store.User) api.User {
	return api.User{Id: u.ID, Email: u.Email, Name: u.Name, Role: api.UserRole(u.Role)}
}
