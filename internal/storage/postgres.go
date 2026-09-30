package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // нужен только для migrate-драйвера
	"go.uber.org/zap"
)

//go:embed migrations
var migrationsFS embed.FS

const (
	maxConns        = int32(20)
	minConns        = int32(2)
	connMaxLifetime = time.Hour
	connMaxIdleTime = 5 * time.Minute
	healthCheckP    = time.Minute
)

type PostgresStorage struct {
	pool *pgxpool.Pool
}

func NewPostgresStorage(ctx context.Context, dsn string) (*PostgresStorage, error) {
	if err := runMigrations(dsn); err != nil {
		return nil, err
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = maxConns
	cfg.MinConns = minConns
	cfg.MaxConnLifetime = connMaxLifetime
	cfg.MaxConnIdleTime = connMaxIdleTime
	cfg.HealthCheckPeriod = healthCheckP

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pgxpool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &PostgresStorage{pool: pool}, nil
}

// runMigrations открывает отдельное *sql.DB только на время миграций
// и сразу закрывает. golang-migrate требует именно *sql.DB.
func runMigrations(dsn string) error {
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open db for migrations: %w", err)
	}
	defer conn.Close()

	driver, err := migratepgx.WithInstance(conn, &migratepgx.Config{})
	if err != nil {
		return fmt.Errorf("migrate driver: %w", err)
	}
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migrate source: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "pgx", driver)
	if err != nil {
		return fmt.Errorf("migrate instance: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

func (p *PostgresStorage) Close() error {
	p.pool.Close()
	return nil
}

func (p *PostgresStorage) ensureUser(ctx context.Context, tx pgx.Tx, userID string) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
		userID)
	if err != nil {
		return fmt.Errorf("ensure user: %w", err)
	}
	return nil
}

func (p *PostgresStorage) Save(ctx context.Context, id, url, userID string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := p.ensureUser(ctx, tx, userID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO urls (id, original_url, user_id) VALUES ($1, $2, $3)
		 ON CONFLICT (original_url) DO NOTHING`,
		id, url, userID)
	if err != nil {
		return fmt.Errorf("insert url: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (p *PostgresStorage) Get(ctx context.Context, id string) (string, error) {
	var original string
	var deleted bool
	err := p.pool.QueryRow(ctx,
		`SELECT original_url, is_deleted FROM urls WHERE id = $1`, id).
		Scan(&original, &deleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get url: %w", err)
	}
	if deleted {
		return "", ErrGone
	}
	return original, nil
}

func (p *PostgresStorage) Exists(ctx context.Context, id string) bool {
	var exists bool
	err := p.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM urls WHERE id = $1)`, id).
		Scan(&exists)
	if err != nil {
		zap.L().Error("Exists query failed",
			zap.Error(err),
			zap.String("id", id),
		)
		return false
	}
	return exists
}

func (p *PostgresStorage) FindIDByURL(ctx context.Context, url string) (string, bool) {
	var id string
	err := p.pool.QueryRow(ctx,
		`SELECT id FROM urls WHERE original_url = $1 AND is_deleted = false`, url).
		Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false
	}
	if err != nil {
		zap.L().Error("FindIDByURL query failed",
			zap.Error(err),
			zap.String("url", url),
		)
		return "", false
	}
	return id, true
}

func (p *PostgresStorage) Load(ctx context.Context) error {
	return nil
}

func (p *PostgresStorage) SaveToFile(ctx context.Context) error {
	return nil
}

func (p *PostgresStorage) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

func (p *PostgresStorage) BatchSave(ctx context.Context, items []BatchItem, userID string) error {
	if len(items) == 0 {
		return nil
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := p.ensureUser(ctx, tx, userID); err != nil {
		return err
	}

	batch := &pgx.Batch{}
	for _, item := range items {
		batch.Queue(
			`INSERT INTO urls (id, original_url, user_id) VALUES ($1, $2, $3)
			 ON CONFLICT (original_url) DO NOTHING`,
			item.ID, item.URL, userID)
	}

	br := tx.SendBatch(ctx, batch)
	for range items {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return fmt.Errorf("batch insert: %w", err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("close batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (p *PostgresStorage) GetAllByUser(ctx context.Context, userID string) ([]URLPair, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, original_url FROM urls WHERE user_id = $1 AND is_deleted = false`, userID)
	if err != nil {
		return nil, fmt.Errorf("query user urls: %w", err)
	}
	defer rows.Close()

	pairs := make([]URLPair, 0, 16)
	for rows.Next() {
		var id, original string
		if err := rows.Scan(&id, &original); err != nil {
			return nil, fmt.Errorf("scan url: %w", err)
		}
		pairs = append(pairs, URLPair{
			ShortURL:    id,
			OriginalURL: original,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate urls: %w", err)
	}
	return pairs, nil
}

func (p *PostgresStorage) DeleteURLs(ctx context.Context, ids []string, userID string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := p.pool.Exec(ctx,
		`UPDATE urls SET is_deleted = true WHERE id = ANY($1::text[]) AND user_id = $2`,
		ids, userID)
	if err != nil {
		return fmt.Errorf("delete urls: %w", err)
	}
	return nil
}
