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

// ensureUser создаёт пользователя, если его нет
func (p *PostgresStorage) ensureUser(ctx context.Context, tx *sql.Tx, userID string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
		userID)
	return err
}

func (p *PostgresStorage) Save(ctx context.Context, id, url, userID string) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := p.ensureUser(ctx, tx, userID); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO urls (id, original_url, user_id) VALUES ($1, $2, $3)
		 ON CONFLICT (original_url) DO NOTHING`,
		id, url, userID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (p *PostgresStorage) Get(ctx context.Context, id string) (string, error) {
	var original string
	var deleted bool
	err := p.db.QueryRowContext(ctx,
		`SELECT original_url, is_deleted FROM urls WHERE id = $1`, id).
		Scan(&original, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if deleted {
		return "", ErrGone
	}
	return original, nil
}

func (p *PostgresStorage) FindIDByURL(ctx context.Context, url string) (string, bool) {
	var id string
	err := p.db.QueryRowContext(ctx,
		`SELECT id FROM urls WHERE original_url = $1 AND is_deleted = false`, url).Scan(&id)
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

func (p *PostgresStorage) BatchSave(ctx context.Context, items []BatchItem, userID string) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := p.ensureUser(ctx, tx, userID); err != nil {
		return err
	}

	for _, item := range items {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO urls (id, original_url, user_id) VALUES ($1, $2, $3)
			 ON CONFLICT (original_url) DO NOTHING`,
			item.ID, item.URL, userID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *PostgresStorage) GetAllByUser(ctx context.Context, userID string) ([]URLPair, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, original_url FROM urls WHERE user_id = $1 AND is_deleted = false`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pairs []URLPair
	for rows.Next() {
		var id, original string
		if err := rows.Scan(&id, &original); err != nil {
			return nil, err
		}
		pairs = append(pairs, URLPair{
			ShortURL:    id,
			OriginalURL: original,
		})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return pairs, nil
}

func (p *PostgresStorage) DeleteURLs(ctx context.Context, ids []string, userID string) error {
	if len(ids) == 0 {
		return nil
	}
	// Преобразуем срез в массив для использования с ANY
	query := `UPDATE urls SET is_deleted = true WHERE id = ANY($1::text[]) AND user_id = $2`
	_, err := p.db.ExecContext(ctx, query, ids, userID)
	return err
}
