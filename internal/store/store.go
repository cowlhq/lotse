// Package store keeps everything the hub knows, in SQLite or in Postgres.
//
// SQL is written by hand with ? placeholders and kept to what both databases
// understand; Rebind turns the placeholders into $1, $2, … for Postgres.
// Times are Unix seconds, and JSON is stored as text and never queried inside.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Dialect is the database a store runs on.
type Dialect int

const (
	SQLite Dialect = iota
	Postgres
)

func (d Dialect) String() string {
	if d == Postgres {
		return "postgres"
	}
	return "sqlite"
}

//go:embed migrations
var migrations embed.FS

// Store is the hub's database.
type Store struct {
	dialect Dialect
	// db takes all writes. For SQLite it has a single connection, since
	// SQLite allows one writer at a time anyway.
	db *sql.DB
	// read serves queries. For SQLite it is a pool of read-only connections
	// that WAL lets run beside the writer; for Postgres it is db.
	read *sql.DB
}

// Open connects to the database at url and brings its schema up to date.
// A postgres:// or postgresql:// URL means Postgres; anything else is the
// path of a SQLite file, optionally prefixed with sqlite:.
func Open(ctx context.Context, url string) (*Store, error) {
	s, err := open(url)
	if err != nil {
		return nil, err
	}
	if err := s.migrate(ctx); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func open(url string) (*Store, error) {
	if strings.HasPrefix(url, "postgres://") || strings.HasPrefix(url, "postgresql://") {
		db, err := sql.Open("pgx", url)
		if err != nil {
			return nil, err
		}
		return &Store{dialect: Postgres, db: db, read: db}, nil
	}
	path := strings.TrimPrefix(url, "sqlite:")
	if path == "" {
		return nil, errors.New("no database given")
	}
	// _txlock=immediate takes the write lock when a transaction begins, so
	// a transaction that reads and then writes never fails halfway with
	// SQLITE_BUSY.
	dsn := "file:" + path + "?_txlock=immediate" +
		"&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	read, err := sql.Open("sqlite", dsn+"&_pragma=query_only(1)")
	if err != nil {
		db.Close()
		return nil, err
	}
	read.SetMaxOpenConns(4)
	return &Store{dialect: SQLite, db: db, read: read}, nil
}

// Ping says whether the database at url answers, without changing it.
func Ping(ctx context.Context, url string) error {
	s, err := open(url)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.db.PingContext(ctx)
}

// Backup writes a consistent copy of a SQLite database to path, while it
// is in use.
func (s *Store) Backup(ctx context.Context, path string) error {
	if s.dialect != SQLite {
		return errors.New("only a SQLite database is backed up by lotse")
	}
	os.Remove(path)
	_, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path)
	return err
}

// Close closes the database.
func (s *Store) Close() error {
	err := s.db.Close()
	if s.read != s.db {
		err = errors.Join(err, s.read.Close())
	}
	return err
}

// Dialect says which database the store runs on.
func (s *Store) Dialect() Dialect { return s.dialect }

func (s *Store) migrate(ctx context.Context) error {
	files, err := fs.Sub(migrations, "migrations/"+s.dialect.String())
	if err != nil {
		return err
	}
	dialect := goose.DialectSQLite3
	if s.dialect == Postgres {
		dialect = goose.DialectPostgres
	}
	provider, err := goose.NewProvider(dialect, s.db, files)
	if err != nil {
		return err
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrating the %s database: %w", s.dialect, err)
	}
	return nil
}

// rebind turns the ? placeholders of q into $1, $2, … for Postgres. A ?
// inside a quoted string or identifier is left alone.
func (s *Store) rebind(q string) string {
	if s.dialect != Postgres || !strings.Contains(q, "?") {
		return q
	}
	var b strings.Builder
	b.Grow(len(q) + 8)
	n := 0
	var quote byte
	for i := 0; i < len(q); i++ {
		c := q[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '?':
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// tx runs f in a transaction, which it commits if f returns nil.
func (s *Store) tx(ctx context.Context, f func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := f(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, s.rebind(q), args...)
}

func (s *Store) queryRow(ctx context.Context, q string, args ...any) *sql.Row {
	return s.read.QueryRowContext(ctx, s.rebind(q), args...)
}
