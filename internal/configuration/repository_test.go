package configuration_test

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/tgragnato/orbiter/internal/configuration"
)

func TestPostgresRepositoryGet(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}

	defer sqlDB.Close()

	repo := configuration.NewPostgresRepository(sqlDB)

	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"key", "scope", "description", "value_json", "created_at", "updated_at"}).
		AddRow(configuration.KeyYahooCredentials, "credentials", "desc", []byte(`{"api_key":"test"}`), now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT key, scope, description, value_json, created_at, updated_at
		FROM app_settings
		WHERE key = $1
	`)).WithArgs(configuration.KeyYahooCredentials).WillReturnRows(rows)

	setting, err := repo.Get(context.Background(), configuration.KeyYahooCredentials)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if setting.Key != configuration.KeyYahooCredentials {
		t.Fatalf("setting.Key = %q, want %q", setting.Key, configuration.KeyYahooCredentials)
	}

	err = mock.ExpectationsWereMet()
	if err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPostgresRepositoryGetNotFound(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}

	defer sqlDB.Close()

	repo := configuration.NewPostgresRepository(sqlDB)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT key, scope, description, value_json, created_at, updated_at
		FROM app_settings
		WHERE key = $1
	`)).WithArgs("unknown").WillReturnError(sql.ErrNoRows)

	_, err = repo.Get(context.Background(), "unknown")
	if !errors.Is(err, configuration.ErrSettingNotFound) {
		t.Fatalf("Get() error = %v, want ErrSettingNotFound", err)
	}
}

//nolint:funlen // test combines related Set/Count/List operations on a single DB connection
func TestPostgresRepositorySetCountAndList(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}

	defer sqlDB.Close()

	repo := configuration.NewPostgresRepository(sqlDB)

	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO app_settings (key, scope, description, value_json)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (key)
		DO UPDATE SET
			scope = EXCLUDED.scope,
			description = EXCLUDED.description,
			value_json = EXCLUDED.value_json,
			updated_at = NOW()
	`)).
		WithArgs(
			configuration.KeyPortfolioBaseCurrency,
			"portfolio",
			"desc",
			[]byte(`{"currency":"EUR"}`),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.Set(context.Background(), configuration.Setting{
		Key:         configuration.KeyPortfolioBaseCurrency,
		Scope:       "portfolio",
		Description: "desc",
		ValueJSON:   []byte(`{"currency":"EUR"}`),
		CreatedAt:   time.Time{},
		UpdatedAt:   time.Time{},
	})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM app_settings`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	count, err := repo.Count(context.Background())
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}

	if count != 2 {
		t.Fatalf("Count() = %d, want 2", count)
	}

	rows := sqlmock.NewRows([]string{"key", "scope", "description", "value_json", "created_at", "updated_at"}).
		AddRow(configuration.KeyPortfolioBaseCurrency, "portfolio", "desc1", []byte(`{"currency":"EUR"}`), now, now).
		AddRow(configuration.KeyYahooCredentials, "credentials", "desc2", []byte(`{"api_key":"token"}`), now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT key, scope, description, value_json, created_at, updated_at
		FROM app_settings
		ORDER BY key
	`)).WillReturnRows(rows)

	settings, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(settings) != 2 {
		t.Fatalf("List() len = %d, want 2", len(settings))
	}

	err = mock.ExpectationsWereMet()
	if err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRunMigrations(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}

	defer sqlDB.Close()

	mock.ExpectExec(regexp.QuoteMeta(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)).WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM schema_migrations WHERE version = $1`)).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS app_settings").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(
		`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
	)).
		WithArgs(1, "initial_schema").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = configuration.RunMigrations(context.Background(), sqlDB)
	if err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}

	err = mock.ExpectationsWereMet()
	if err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
