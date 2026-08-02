package configuration

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPostgresRepositoryGet(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresRepository(db)
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{"key", "scope", "description", "value_json", "created_at", "updated_at"}).
		AddRow(KeyCostBasisMethod, "global", "desc", []byte(`{"method":"PMC"}`), now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT key, scope, description, value_json, created_at, updated_at
		FROM app_settings
		WHERE key = $1
	`)).WithArgs(KeyCostBasisMethod).WillReturnRows(rows)

	setting, err := repo.Get(context.Background(), KeyCostBasisMethod)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if setting.Key != KeyCostBasisMethod {
		t.Fatalf("setting.Key = %q, want %q", setting.Key, KeyCostBasisMethod)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPostgresRepositoryGetNotFound(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresRepository(db)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT key, scope, description, value_json, created_at, updated_at
		FROM app_settings
		WHERE key = $1
	`)).WithArgs("unknown").WillReturnError(sql.ErrNoRows)

	_, err = repo.Get(context.Background(), "unknown")
	if !errors.Is(err, ErrSettingNotFound) {
		t.Fatalf("Get() error = %v, want ErrSettingNotFound", err)
	}
}

func TestPostgresRepositorySetCountAndList(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresRepository(db)
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
	`)).WithArgs(KeyDataProvider, "global", "desc", []byte(`{"provider":"YAHOO","currency":"EUR"}`)).WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Set(context.Background(), Setting{
		Key:         KeyDataProvider,
		Scope:       "global",
		Description: "desc",
		ValueJSON:   []byte(`{"provider":"YAHOO","currency":"EUR"}`),
	}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM app_settings`)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	count, err := repo.Count(context.Background())
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("Count() = %d, want 2", count)
	}

	rows := sqlmock.NewRows([]string{"key", "scope", "description", "value_json", "created_at", "updated_at"}).
		AddRow(KeyCostBasisMethod, "global", "desc1", []byte(`{"method":"PMC"}`), now, now).
		AddRow(KeyDataProvider, "global", "desc2", []byte(`{"provider":"YAHOO","currency":"EUR"}`), now, now)

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

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRunMigrations(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)).WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM schema_migrations WHERE version = $1`)).WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS app_settings").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`)).WithArgs(1, "create_app_settings").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM schema_migrations WHERE version = $1`)).WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS holdings").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`)).WithArgs(2, "create_holdings_and_allocation_type").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM schema_migrations WHERE version = $1`)).WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS corporate_actions").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`)).WithArgs(3, "create_corporate_actions_and_accounting_tables").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM schema_migrations WHERE version = $1`)).WithArgs(4).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS cash_flows").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`)).WithArgs(4, "create_twr_analytics_tables").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM schema_migrations WHERE version = $1`)).WithArgs(5).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS ml_model_checkpoints").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`)).WithArgs(5, "create_ml_model_checkpoints").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM schema_migrations WHERE version = $1`)).WithArgs(6).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE holdings ADD COLUMN IF NOT EXISTS pmc").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`)).WithArgs(6, "add_transactions_and_pmc").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM schema_migrations WHERE version = $1`)).WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE holdings ADD COLUMN IF NOT EXISTS taa_enabled").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`)).WithArgs(7, "add_taa_enabled_to_holdings").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
