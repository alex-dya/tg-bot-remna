package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Employee struct {
	Username        string
	RemnawaveUUID   string
	SubscriptionURL string
	TelegramID      *int64
	ActivatedAt     *time.Time
	CreatedAt       time.Time
	CreatedBy       int64
	ExpiresAt       *time.Time
}

type DB struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*DB, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &DB{pool: pool}, nil
}

func (d *DB) Close() { d.pool.Close() }

// Migrate выполняет встроенную схему. Идемпотентно.
func (d *DB) Migrate(ctx context.Context, schema string) error {
	_, err := d.pool.Exec(ctx, schema)
	return err
}

func normalize(u string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(u), "@"))
}

func (d *DB) AddEmployee(ctx context.Context, e Employee) error {
	e.Username = normalize(e.Username)
	_, err := d.pool.Exec(ctx, `
		INSERT INTO bot_employees
			(username, remnawave_uuid, subscription_url, created_by, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, e.Username, e.RemnawaveUUID, e.SubscriptionURL, e.CreatedBy, e.ExpiresAt)
	return err
}

func (d *DB) GetByUsername(ctx context.Context, username string) (*Employee, error) {
	username = normalize(username)
	row := d.pool.QueryRow(ctx, `
		SELECT username, remnawave_uuid, subscription_url, telegram_id,
		       activated_at, created_at, created_by, expires_at
		  FROM bot_employees WHERE username = $1
	`, username)
	var e Employee
	if err := row.Scan(
		&e.Username, &e.RemnawaveUUID, &e.SubscriptionURL, &e.TelegramID,
		&e.ActivatedAt, &e.CreatedAt, &e.CreatedBy, &e.ExpiresAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

func (d *DB) MarkActivated(ctx context.Context, username string, tgID int64) error {
	username = normalize(username)
	_, err := d.pool.Exec(ctx, `
		UPDATE bot_employees
		   SET telegram_id = $2,
		       activated_at = COALESCE(activated_at, NOW())
		 WHERE username = $1
	`, username, tgID)
	return err
}

func (d *DB) Delete(ctx context.Context, username string) (string, bool, error) {
	username = normalize(username)
	var uuid string
	err := d.pool.QueryRow(ctx,
		`DELETE FROM bot_employees WHERE username = $1 RETURNING remnawave_uuid`,
		username).Scan(&uuid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return uuid, true, nil
}

func (d *DB) List(ctx context.Context, limit int) ([]Employee, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT username, remnawave_uuid, subscription_url, telegram_id,
		       activated_at, created_at, created_by, expires_at
		  FROM bot_employees
		 ORDER BY created_at DESC
		 LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Employee
	for rows.Next() {
		var e Employee
		if err := rows.Scan(
			&e.Username, &e.RemnawaveUUID, &e.SubscriptionURL, &e.TelegramID,
			&e.ActivatedAt, &e.CreatedAt, &e.CreatedBy, &e.ExpiresAt,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (d *DB) Count(ctx context.Context) (int, error) {
	var n int
	err := d.pool.QueryRow(ctx, `SELECT COUNT(*) FROM bot_employees`).Scan(&n)
	return n, err
}
