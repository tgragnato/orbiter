package configuration

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRunMigrationsSkipsAppliedVersion(t *testing.T) {
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
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	if err := RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRunMigrationsCreateTableError(t *testing.T) {
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
	`)).WillReturnError(assertErr("boom"))

	if err := RunMigrations(context.Background(), db); err == nil {
		t.Fatalf("RunMigrations() error = nil, want non-nil")
	}
}

func TestBootstrap(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

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
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM schema_migrations WHERE version = $1`)).WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS app_settings").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`)).WithArgs(1, "initial_schema").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM app_settings`)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	for i := 0; i < len(defaultSettings); i++ {
		mock.ExpectExec("INSERT INTO app_settings").WillReturnResult(sqlmock.NewResult(1, 1))
	}

	addGetSettingExpectation(mock, KeyCostBasisMethod, []byte(`{"method":"PMC"}`), now)
	addGetSettingExpectation(mock, KeyDataProvider, []byte(`{"provider":"YAHOO","currency":"EUR"}`), now)
	addGetSettingExpectation(mock, KeyTAAParameters, []byte(`{"rebalance_threshold":0.05}`), now)
	addGetSettingExpectation(mock, KeyCoreSatelliteTargets, []byte(`{"core_ratio":0.8,"satellite_ratio":0.2}`), now)
	addGetSettingExpectation(mock, KeyTUIPreferences, []byte(`{"show_percentages":true,"number_format":"2dp"}`), now)
	addGetSettingExpectation(mock, KeyYahooCredentials, []byte(`{"api_key":""}`), now)

	svc, err := Bootstrap(context.Background(), db)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if svc == nil {
		t.Fatalf("Bootstrap() returned nil service")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func addGetSettingExpectation(mock sqlmock.Sqlmock, key string, value []byte, now time.Time) {
	rows := sqlmock.NewRows([]string{"key", "scope", "description", "value_json", "created_at", "updated_at"}).
		AddRow(key, "global", "desc", value, now, now)

	mock.ExpectQuery("FROM app_settings").WithArgs(key).WillReturnRows(rows)
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
