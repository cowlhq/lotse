// Package db opens the hub's database, SQLite or Postgres (decision 0014),
// and brings its schema up to date.
//
// SQL is written by hand with ? placeholders and kept to what both databases
// understand; the DB turns the placeholders into $1, $2, … for Postgres.
// Times are Unix seconds, and JSON is stored as text and never queried inside.
// Each feature keeps its own queries; this package only runs them.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Dialect is the database a DB runs on.
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

// Migrations holds the schema, in migrations/sqlite and migrations/postgres,
// with the same files in each.
//
//go:embed migrations
var Migrations embed.FS

// DB is the hub's database.
type DB struct {
	dialect Dialect
	// writer takes all writes. For SQLite it has a single connection,
	// since SQLite allows one writer at a time anyway. reader serves
	// queries: for SQLite a pool of read-only connections that WAL lets run
	// beside the writer; for Postgres the same pool as writer.
	writer, reader *sql.DB
	// write and read are what queries go to: the pools, or, within Tx, the
	// one transaction.
	write, read querier
	inTx        bool
}

// querier is what a pool and a transaction both do.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Open connects to the database at url and brings its schema up to date.
// A postgres:// or postgresql:// URL means Postgres; anything else is the
// path of a SQLite file, optionally prefixed with sqlite:.
func Open(ctx context.Context, url string) (*DB, error) {
	d, err := open(url)
	if err != nil {
		return nil, err
	}
	if err := d.migrate(ctx); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

func open(url string) (*DB, error) {
	if strings.HasPrefix(url, "postgres://") || strings.HasPrefix(url, "postgresql://") {
		pool, err := sql.Open("pgx", url)
		if err != nil {
			return nil, err
		}
		return newDB(Postgres, pool, pool), nil
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
	writer, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	writer.SetMaxOpenConns(1)
	reader, err := sql.Open("sqlite", dsn+"&_pragma=query_only(1)")
	if err != nil {
		writer.Close()
		return nil, err
	}
	reader.SetMaxOpenConns(4)
	return newDB(SQLite, writer, reader), nil
}

func newDB(dialect Dialect, writer, reader *sql.DB) *DB {
	return &DB{dialect: dialect, writer: writer, reader: reader, write: writer, read: reader}
}

// Close closes the database.
func (d *DB) Close() error {
	err := d.writer.Close()
	if d.reader != d.writer {
		err = errors.Join(err, d.reader.Close())
	}
	return err
}

// Dialect says which database d runs on.
func (d *DB) Dialect() Dialect { return d.dialect }

// Ping says whether the database answers.
func (d *DB) Ping(ctx context.Context) error { return d.writer.PingContext(ctx) }

// Tx runs f with a DB whose every query goes to one transaction, committed
// if f returns nil: all of it happens, or none. Within Tx, Tx runs f in the
// same transaction.
func (d *DB) Tx(ctx context.Context, f func(*DB) error) error {
	if d.inTx {
		return f(d)
	}
	tx, err := d.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	in := *d
	in.write, in.read, in.inTx = tx, tx, true
	if err := f(&in); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Exec runs a statement that changes something.
func (d *DB) Exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return d.write.ExecContext(ctx, d.rebind(q), args...)
}

// Query runs a query that returns rows.
func (d *DB) Query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return d.read.QueryContext(ctx, d.rebind(q), args...)
}

// QueryRow runs a query that returns at most one row.
func (d *DB) QueryRow(ctx context.Context, q string, args ...any) *sql.Row {
	return d.read.QueryRowContext(ctx, d.rebind(q), args...)
}

func (d *DB) migrate(ctx context.Context) error {
	files, err := fs.Sub(Migrations, "migrations/"+d.dialect.String())
	if err != nil {
		return err
	}
	dialect := goose.DialectSQLite3
	if d.dialect == Postgres {
		dialect = goose.DialectPostgres
	}
	provider, err := goose.NewProvider(dialect, d.writer, files)
	if err != nil {
		return err
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrating the %s database: %w", d.dialect, err)
	}
	return nil
}

// rebind turns the ? placeholders of q into $1, $2, … for Postgres. A ?
// inside a quoted string or identifier is left alone.
func (d *DB) rebind(q string) string {
	if d.dialect != Postgres || !strings.Contains(q, "?") {
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
