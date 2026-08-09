package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/pgx"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations
var migrationsFS embed.FS

type PostgresStorage struct {
	db *sql.DB
}

func NewPostgresStorage(dsn string) (*PostgresStorage, error) {
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %w", err)
	}
	if err = conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping db: %w", err)
	}

	driver, err := pgx.WithInstance(conn, &pgx.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to create migrate driver: %w", err)
	}
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to create migrate source: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "pgx", driver)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrate instance: %w", err)
	}
	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}
	return &PostgresStorage{db: conn}, nil
}

func (p *PostgresStorage) Close() error {
	return p.db.Close()
}

func (p *PostgresStorage) Save(ctx context.Context, id, url string) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO urls (id, original_url) VALUES ($1, $2)
         ON CONFLICT (original_url) DO NOTHING`,
		id, url)
	return err
}

func (p *PostgresStorage) Get(ctx context.Context, id string) (string, error) {
	var original string
	err := p.db.QueryRowContext(ctx,
		`SELECT original_url FROM urls WHERE id = $1`, id).Scan(&original)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return original, err
}

func (p *PostgresStorage) FindIDByURL(ctx context.Context, url string) (string, bool) {
	var id string
	err := p.db.QueryRowContext(ctx,
		`SELECT id FROM urls WHERE original_url = $1`, url).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false
	}
	return id, err == nil
}

func (p *PostgresStorage) Exists(ctx context.Context, id string) bool {
	var exists bool
	err := p.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM urls WHERE id = $1)`, id).Scan(&exists)
	return err == nil && exists
}

func (p *PostgresStorage) Load(ctx context.Context) error {
	return nil
}

func (p *PostgresStorage) SaveToFile(ctx context.Context) error {
	return nil
}

func (p *PostgresStorage) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

func (p *PostgresStorage) BatchSave(ctx context.Context, items []BatchItem) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, item := range items {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO urls (id, original_url) VALUES ($1, $2) ON CONFLICT (original_url) DO NOTHING`,
			item.ID, item.URL)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
