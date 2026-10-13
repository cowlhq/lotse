// Package hub runs the hub: API, web interface and storage.
package hub

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"lotse.sh/internal/auth"
	"lotse.sh/internal/protocol"
	"lotse.sh/internal/secret"
	"lotse.sh/internal/store"
	"lotse.sh/internal/ui"
	"lotse.sh/internal/update"
	"lotse.sh/internal/version"
)

// Config is what the hub needs to start. ConfigFromEnv reads it from
// LOTSE_* variables.
type Config struct {
	// DataDir holds hub.json, the SQLite database and everything else the
	// hub keeps.
	DataDir string
	// Listen is the address to serve HTTP on.
	Listen string
	// DatabaseURL is a Postgres URL or a SQLite path. Set, it skips the
	// database step of setup (decision 0004); otherwise setup chooses and
	// saves it in hub.json.
	DatabaseURL string
	// URL is where people and agents reach the hub. Set, it wins over what
	// setup saved.
	URL string
	// ReleaseURL is where hub updates come from; empty means GitHub. Like
	// the agents', only the machine itself names another place.
	ReleaseURL string
	// ReleasesAPI is where the hub learns about new releases.
	ReleasesAPI string
	// SecretKey seals what the hub keeps but must read again, such as
	// enrollment tokens. Empty means the key in DataDir/secret.key, made on
	// first start.
	SecretKey string
}

func ConfigFromEnv() Config {
	return Config{
		DataDir:     cmp.Or(os.Getenv("LOTSE_DATA_DIR"), "/var/lib/lotse"),
		Listen:      cmp.Or(os.Getenv("LOTSE_LISTEN"), ":8080"),
		DatabaseURL: os.Getenv("LOTSE_DATABASE_URL"),
		URL:         os.Getenv("LOTSE_URL"),
		SecretKey:   os.Getenv("LOTSE_SECRET_KEY"),
		ReleaseURL:  os.Getenv("LOTSE_RELEASE_URL"),
		ReleasesAPI: cmp.Or(os.Getenv("LOTSE_RELEASES_API"), DefaultReleasesAPI),
	}
}

// Run serves the hub until ctx ends. Without a database, it first serves
// only setup, until setup has chosen one; then it opens the database and
// serves everything, on the same address.
func Run(ctx context.Context, cfg Config, log *slog.Logger) error {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	saved, err := readHubFile(cfg.DataDir)
	if err != nil {
		return err
	}
	key, err := secret.LoadKey(cfg.SecretKey, cfg.DataDir)
	if err != nil {
		return err
	}
	box, err := secret.NewBox(key)
	if err != nil {
		return err
	}
	setup := &setupState{}
	shared := shared{
		box:         box,
		log:         log,
		cfg:         cfg,
		setup:       setup,
		publicURL:   new(atomic.Pointer[string]),
		logins:      auth.NewLimiter(5, 15*time.Minute),
		chosen:      make(chan string, 1),
		releases:    &releases{api: cfg.ReleasesAPI},
		restart:     make(chan struct{}),
		restartOnce: new(sync.Once),
	}
	shared.setPublicURL(cmp.Or(cfg.URL, saved.URL, defaultURL(cfg.Listen)))
	dbURL := cmp.Or(cfg.DatabaseURL, saved.DatabaseURL)

	var root swap
	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           &root,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	errs := make(chan error, 1)
	go func() { errs <- server.Serve(ln) }()
	stop := func() error {
		log.Info("lotse hub stopping")
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return err
		}
		if err := <-errs; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
	log.Info("lotse hub starting", "version", version.Version, "listen", cfg.Listen)

	if dbURL == "" {
		// Setup, the database step: nothing to store anything in yet.
		setup.start()
		root.set((&hub{shared: shared}).routes())
		shared.announceSetup("choose a database")
		select {
		case dbURL = <-shared.chosen:
		case err := <-errs:
			return err
		case <-ctx.Done():
			return stop()
		}
	}

	db, err := store.Open(ctx, dbURL)
	if err != nil {
		stop()
		return err
	}
	defer db.Close()
	h, err := newHub(ctx, shared, db)
	if err != nil {
		stop()
		return err
	}
	log.Info("lotse hub ready", "database", db.Dialect())
	if setup.pending() {
		shared.announceSetup("create the admin")
	}

	// Shutdown does not wait for WebSockets; agents are told to go.
	server.RegisterOnShutdown(h.agents.closeAll)
	writerCtx, stopWriter := context.WithCancel(context.WithoutCancel(ctx))
	writerDone := make(chan struct{})
	go func() {
		h.writer.run(writerCtx)
		close(writerDone)
	}()
	go h.keepLive(ctx)
	go h.retain(ctx)
	go h.releases.watch(ctx)
	defer func() {
		// What arrived until the very end is still written.
		stopWriter()
		<-writerDone
	}()
	root.set(h.routes())

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	case <-shared.restart:
		// An update is installed: stop, and let the starter run it.
		if err := stop(); err != nil {
			return err
		}
		return update.ErrRestart
	}
	return stop()
}

// shared is what the hub keeps across the switch from setup to the
// database.
type shared struct {
	log       *slog.Logger
	cfg       Config
	setup     *setupState
	publicURL *atomic.Pointer[string]
	logins    *auth.Limiter
	box       *secret.Box
	// chosen gets the database URL that setup chose.
	chosen   chan string
	releases *releases
	// restart is closed when the hub has installed an update.
	restart     chan struct{}
	restartOnce *sync.Once
}

func (s shared) setPublicURL(u string) { s.publicURL.Store(&u) }

// URL is where people and agents reach the hub.
func (s shared) URL() string { return *s.publicURL.Load() }

func (s shared) announceSetup(step string) {
	s.log.Warn(fmt.Sprintf("lotse is not set up yet. Open this link to %s: %s/setup?token=%s",
		step, s.URL(), s.setup.token()))
}

type hub struct {
	shared
	// db is nil during the database step of setup.
	db     *store.Store
	agents agents
	live   live
	writer *writer
	// How often agents read their machine and its containers.
	hostInterval, containerInterval time.Duration
}

// newHub makes the hub for a database. Without users, setup goes on with
// the admin step.
func newHub(ctx context.Context, s shared, db *store.Store) (*hub, error) {
	h := &hub{
		shared:            s,
		db:                db,
		writer:            &writer{db: db, log: s.log},
		hostInterval:      15 * time.Second,
		containerInterval: 30 * time.Second,
	}
	users, err := db.CountUsers(ctx)
	if err != nil {
		return nil, err
	}
	if users == 0 {
		h.setup.start()
	} else {
		h.setup.finish()
	}
	return h, nil
}

func (h *hub) routes() http.Handler {
	mux := http.NewServeMux()
	h.apiHandler(mux)
	mux.HandleFunc("GET "+protocol.ConnectPath, h.connectAgent)
	mux.HandleFunc("POST "+protocol.EnrollPath, h.enroll)
	mux.HandleFunc("GET /api/live", h.connectLive)
	mux.Handle("/api/", http.NotFoundHandler())
	mux.Handle("/", ui.Handler(ui.Files()))
	return h.guard(mux)
}

// swap is a handler that can be replaced while the server runs.
type swap struct{ h atomic.Pointer[http.Handler] }

func (s *swap) set(h http.Handler) { s.h.Store(&h) }

func (s *swap) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h := s.h.Load(); h != nil {
		(*h).ServeHTTP(w, r)
		return
	}
	http.Error(w, "starting", http.StatusServiceUnavailable)
}

func defaultURL(listen string) string {
	_, port, err := net.SplitHostPort(listen)
	if err != nil || port == "" {
		return "http://localhost"
	}
	return fmt.Sprintf("http://localhost:%s", port)
}
