// Package hub runs the hub: API, web interface and storage.
package hub

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"lotse.sh/internal/store"
	"lotse.sh/internal/ui"
	"lotse.sh/internal/version"
)

// Config is what the hub needs to start. ConfigFromEnv reads it from
// LOTSE_* variables.
type Config struct {
	// DataDir holds the SQLite database and everything else the hub keeps.
	DataDir string
	// Listen is the address to serve HTTP on.
	Listen string
	// DatabaseURL is a Postgres URL or a SQLite path. Empty means lotse.db
	// in DataDir.
	DatabaseURL string
	// URL is where people and agents reach the hub. Empty means
	// http://localhost and the port of Listen.
	URL string
}

func ConfigFromEnv() Config {
	return Config{
		DataDir:     cmp.Or(os.Getenv("LOTSE_DATA_DIR"), "/var/lib/lotse"),
		Listen:      cmp.Or(os.Getenv("LOTSE_LISTEN"), ":8080"),
		DatabaseURL: os.Getenv("LOTSE_DATABASE_URL"),
		URL:         os.Getenv("LOTSE_URL"),
	}
}

// Run serves the hub until ctx ends.
func Run(ctx context.Context, cfg Config, log *slog.Logger) error {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	dbURL := cmp.Or(cfg.DatabaseURL, filepath.Join(cfg.DataDir, "lotse.db"))
	db, err := store.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	h, err := newHub(ctx, db, log)
	if err != nil {
		return err
	}
	log.Info("lotse hub starting", "version", version.Version, "database", db.Dialect(), "listen", cfg.Listen)
	if h.setupToken != "" {
		base := cmp.Or(cfg.URL, defaultURL(cfg.Listen))
		log.Warn("lotse is not set up yet. Open this link to set it up: " + base + "/setup?token=" + h.setupToken)
	}

	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           h.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	errs := make(chan error, 1)
	go func() { errs <- server.ListenAndServe() }()
	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
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

type hub struct {
	db  *store.Store
	log *slog.Logger
	// setupToken is set while the hub has no users. Whoever shows it may set
	// the hub up (decision 0005). A new one is made at every start.
	setupToken string
}

func newHub(ctx context.Context, db *store.Store, log *slog.Logger) (*hub, error) {
	h := &hub{db: db, log: log}
	users, err := db.CountUsers(ctx)
	if err != nil {
		return nil, err
	}
	if users == 0 {
		h.setupToken = randomToken()
	}
	return h, nil
}

func (h *hub) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("POST /api/setup/check", h.checkSetup)
	mux.Handle("/api/", http.NotFoundHandler())
	mux.Handle("/", ui.Handler(ui.Files()))
	return mux
}

func defaultURL(listen string) string {
	_, port, err := net.SplitHostPort(listen)
	if err != nil || port == "" {
		return "http://localhost"
	}
	return fmt.Sprintf("http://localhost:%s", port)
}

func randomToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
