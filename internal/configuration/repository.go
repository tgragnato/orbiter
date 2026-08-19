package configuration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Repository stores and retrieves configuration settings.
type Repository interface {
	Get(ctx context.Context, key string) (Setting, error)
	Set(ctx context.Context, setting Setting) error
	List(ctx context.Context) ([]Setting, error)
	Count(ctx context.Context) (int, error)
}

// PostgresRepository persists settings in PostgreSQL.
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository builds a new PostgreSQL-backed configuration repository.
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// Get returns a setting by key.
func (r *PostgresRepository) Get(ctx context.Context, key string) (Setting, error) {
	const query = `
		SELECT key, scope, description, value_json, created_at, updated_at
		FROM app_settings
		WHERE key = $1
	`

	var row Setting

	err := r.db.QueryRowContext(ctx, query, key).Scan(
		&row.Key,
		&row.Scope,
		&row.Description,
		&row.ValueJSON,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Setting{}, ErrSettingNotFound
	}

	if err != nil {
		return Setting{}, fmt.Errorf("scan setting %s: %w", key, err)
	}

	return row, nil
}

// Set creates or updates a setting.
func (r *PostgresRepository) Set(ctx context.Context, setting Setting) error {
	const query = `
		INSERT INTO app_settings (key, scope, description, value_json)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (key)
		DO UPDATE SET
			scope = EXCLUDED.scope,
			description = EXCLUDED.description,
			value_json = EXCLUDED.value_json,
			updated_at = NOW()
	`

	_, err := r.db.ExecContext(ctx, query, setting.Key, setting.Scope, setting.Description, setting.ValueJSON)
	if err != nil {
		return fmt.Errorf("exec set setting %s: %w", setting.Key, err)
	}

	return nil
}

// List returns all settings sorted by key.
func (r *PostgresRepository) List(ctx context.Context) ([]Setting, error) {
	const query = `
		SELECT key, scope, description, value_json, created_at, updated_at
		FROM app_settings
		ORDER BY key
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query settings: %w", err)
	}

	defer func() { _ = rows.Close() }()

	settings := make([]Setting, 0)

	for rows.Next() {
		var row Setting

		err = rows.Scan(&row.Key, &row.Scope, &row.Description, &row.ValueJSON, &row.CreatedAt, &row.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan setting row: %w", err)
		}

		settings = append(settings, row)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate settings rows: %w", err)
	}

	return settings, nil
}

// Count returns the number of stored settings.
func (r *PostgresRepository) Count(ctx context.Context) (int, error) {
	const query = `SELECT COUNT(*) FROM app_settings`

	var count int

	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("scan settings count: %w", err)
	}

	return count, nil
}
