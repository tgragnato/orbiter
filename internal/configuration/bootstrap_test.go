//nolint:testpackage // accesses unexported defaultSettings; cannot be moved to configuration_test
package configuration

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

const colCount = "count"

func TestRunMigrationsSkipsAppliedVersion(t *testing.T) {
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
		WillReturnRows(sqlmock.NewRows([]string{colCount}).AddRow(1))

	err = RunMigrations(context.Background(), sqlDB)
	if err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}

	err = mock.ExpectationsWereMet()
	if err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRunMigrationsCreateTableError(t *testing.T) {
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
	`)).WillReturnError(assertError("boom"))

	err = RunMigrations(context.Background(), sqlDB)
	if err == nil {
		t.Fatalf("RunMigrations() error = nil, want non-nil")
	}
}

func TestBootstrap(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}

	defer sqlDB.Close()

	// Map seeding order is not deterministic, so we do not enforce call order.
	mock.MatchExpectationsInOrder(false)

	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM schema_migrations WHERE version = $1`)).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{colCount}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS app_settings").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(
		`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
	)).
		WithArgs(1, "initial_schema").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM app_settings`)).
		WillReturnRows(sqlmock.NewRows([]string{colCount}).AddRow(0))

	for range len(defaultSettings) {
		mock.ExpectExec("INSERT INTO app_settings").WillReturnResult(sqlmock.NewResult(1, 1))
	}

	addGetSettingExpectation(mock, KeyYahooCredentials, []byte(`{"api_key":""}`), now)
	addGetSettingExpectation(mock, KeyPortfolioBaseCurrency, []byte(`{"currency":"EUR"}`), now)

	svc, err := Bootstrap(context.Background(), sqlDB)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	if svc == nil {
		t.Fatalf("Bootstrap() returned nil service")
	}

	err = mock.ExpectationsWereMet()
	if err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func addGetSettingExpectation(mock sqlmock.Sqlmock, key string, value []byte, now time.Time) {
	rows := sqlmock.NewRows([]string{"key", "scope", "description", "value_json", "created_at", "updated_at"}).
		AddRow(key, "global", "desc", value, now, now)

	mock.ExpectQuery("FROM app_settings").WithArgs(key).WillReturnRows(rows)
}

type assertError string

func (e assertError) Error() string { return string(e) }
