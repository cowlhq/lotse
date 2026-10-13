package hub

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"lotse.sh/internal/auth"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/store"
)

// setupState is the one-time setup token (decision 0005): set while the
// hub has no database or no admin, and gone once it has both.
type setupState struct {
	mu    sync.Mutex
	value string
	// databaseChosen keeps a second database step from racing the first.
	databaseChosen bool
}

func (s *setupState) start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.value == "" {
		b := make([]byte, 24)
		rand.Read(b)
		s.value = base64.RawURLEncoding.EncodeToString(b)
	}
}

func (s *setupState) finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value = ""
}

func (s *setupState) token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value
}

func (s *setupState) pending() bool { return s.token() != "" }

func (s *setupState) valid(token string) bool {
	t := s.token()
	return t != "" && subtle.ConstantTimeCompare([]byte(token), []byte(t)) == 1
}

// hubFile is hub.json in the data directory: what setup decided.
type hubFile struct {
	DatabaseURL string `json:"database_url,omitempty"`
	URL         string `json:"url,omitempty"`
}

func readHubFile(dir string) (hubFile, error) {
	var f hubFile
	b, err := os.ReadFile(filepath.Join(dir, "hub.json"))
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	return f, json.Unmarshal(b, &f)
}

// updateHubFile changes hub.json, written whole and then renamed, so it is
// never half there. It may hold a database password, so only the hub can
// read it.
func updateHubFile(dir string, change func(*hubFile)) error {
	f, err := readHubFile(dir)
	if err != nil {
		return err
	}
	change(&f)
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".hub.json.tmp")
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "hub.json"))
}

// postgresGuess is the database of the compose file's postgres profile;
// setup offers it when it answers.
const postgresGuess = "postgres://lotse:lotse@postgres:5432/lotse?sslmode=disable"

func (s apiServer) setupStep() *api.SetupStep {
	if !s.setup.pending() {
		return nil
	}
	step := api.SetupStepAdmin
	if s.db == nil {
		step = api.SetupStepDatabase
	}
	return &step
}

func (s apiServer) CheckSetup(ctx context.Context, req api.CheckSetupRequestObject) (api.CheckSetupResponseObject, error) {
	step := s.setupStep()
	if step == nil {
		return api.CheckSetup409JSONResponse{Error: "this hub is already set up"}, nil
	}
	if !s.setup.valid(req.Body.Token) {
		return api.CheckSetup200JSONResponse{Valid: false}, nil
	}
	out := api.CheckSetup200JSONResponse{Valid: true, Step: step}
	if *step == api.SetupStepDatabase {
		pingCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if store.Ping(pingCtx, postgresGuess) == nil {
			out.Postgres = ptr(postgresGuess)
		}
	}
	return out, nil
}

func (s apiServer) SetupDatabase(ctx context.Context, req api.SetupDatabaseRequestObject) (api.SetupDatabaseResponseObject, error) {
	if step := s.setupStep(); step == nil || *step != api.SetupStepDatabase {
		return api.SetupDatabase409JSONResponse{Error: "the database is already chosen"}, nil
	}
	if !s.setup.valid(req.Body.Token) {
		return api.SetupDatabase403JSONResponse{Error: "this setup link is not valid"}, nil
	}
	var dbURL string
	switch req.Body.Kind {
	case "sqlite":
		dbURL = filepath.Join(s.cfg.DataDir, "lotse.db")
	case "postgres":
		dbURL = strings.TrimSpace(ptrValue(req.Body.Url))
		if !strings.HasPrefix(dbURL, "postgres://") && !strings.HasPrefix(dbURL, "postgresql://") {
			return api.SetupDatabase400JSONResponse{Error: "a Postgres URL starts with postgres://"}, nil
		}
	default:
		return api.SetupDatabase400JSONResponse{Error: "the database is sqlite or postgres"}, nil
	}
	s.setup.mu.Lock()
	if s.setup.databaseChosen {
		s.setup.mu.Unlock()
		return api.SetupDatabase409JSONResponse{Error: "the database is already chosen"}, nil
	}
	s.setup.databaseChosen = true
	s.setup.mu.Unlock()
	undo := func() {
		s.setup.mu.Lock()
		s.setup.databaseChosen = false
		s.setup.mu.Unlock()
	}

	// Open it once, migrations included, so that what fails fails here,
	// where the person setting up sees why.
	openCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	db, err := store.Open(openCtx, dbURL)
	if err != nil {
		undo()
		return api.SetupDatabase400JSONResponse{Error: "cannot use this database: " + err.Error()}, nil
	}
	db.Close()
	if err := updateHubFile(s.cfg.DataDir, func(f *hubFile) { f.DatabaseURL = dbURL }); err != nil {
		undo()
		return nil, err
	}
	s.log.Info("setup chose the database", "kind", req.Body.Kind)
	s.chosen <- dbURL
	return api.SetupDatabase204Response{}, nil
}

func (s apiServer) SetupAdmin(ctx context.Context, req api.SetupAdminRequestObject) (api.SetupAdminResponseObject, error) {
	if step := s.setupStep(); step == nil || *step != api.SetupStepAdmin {
		return api.SetupAdmin409JSONResponse{Error: "the hub is already set up, or has no database yet"}, nil
	}
	if !s.setup.valid(req.Body.Token) {
		return api.SetupAdmin403JSONResponse{Error: "this setup link is not valid"}, nil
	}
	b := req.Body
	if _, err := mail.ParseAddress(b.Email); err != nil {
		return api.SetupAdmin400JSONResponse{Error: "this is not an email address"}, nil
	}
	if err := auth.CheckNewPassword(b.Password); err != nil {
		return api.SetupAdmin400JSONResponse{Error: err.Error()}, nil
	}
	u, err := url.Parse(strings.TrimSuffix(strings.TrimSpace(b.Url), "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return api.SetupAdmin400JSONResponse{Error: "the hub's address is an http or https URL"}, nil
	}
	hash, err := auth.HashPassword(b.Password)
	if err != nil {
		return nil, err
	}
	user, err := s.db.CreateUser(ctx, store.User{
		Email: b.Email, Name: strings.TrimSpace(b.Name), PasswordHash: hash, Role: "admin",
		CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		return nil, err
	}
	if err := updateHubFile(s.cfg.DataDir, func(f *hubFile) { f.URL = u.String() }); err != nil {
		return nil, err
	}
	if s.cfg.URL == "" {
		s.setPublicURL(u.String())
	}
	s.setup.finish()
	s.log.Info("setup is done", "admin", user.Email)
	cookie, err := s.startSession(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return api.SetupAdmin200JSONResponse{
		Body:    toUser(user),
		Headers: api.SetupAdmin200ResponseHeaders{SetCookie: &cookie},
	}, nil
}

func ptr[T any](v T) *T { return &v }
