package configuration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type migration struct {
	version int
	name    string
	sql     string
}

// migrations is the ordered list of schema migrations. Each migration runs
// exactly once and is recorded in schema_migrations. All migrations are
// idempotent (IF NOT EXISTS / IF EXISTS) so partial failures are safe to retry.
//
//nolint:gochecknoglobals // package-level migration list is intentional; effectively read-only after init
var migrations = []migration{
	{
		version: 1,
		name:    "initial_schema",
		sql: `
			CREATE TABLE IF NOT EXISTS app_settings (
				key         TEXT        PRIMARY KEY,
				scope       TEXT        NOT NULL,
				description TEXT        NOT NULL DEFAULT '',
				value_json  JSONB       NOT NULL,
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			CREATE TABLE IF NOT EXISTS holdings (
				id              BIGSERIAL      PRIMARY KEY,
				symbol          TEXT           NOT NULL,
				quantity        NUMERIC(20,8)  NOT NULL,
				market_price    NUMERIC(20,8)  NOT NULL,
				pmc             NUMERIC(20,10) NOT NULL DEFAULT 0,
				allocation_type TEXT           NOT NULL DEFAULT 'SATELLITE'
				                CHECK (allocation_type IN ('CORE','SATELLITE')),
				taa_enabled     BOOLEAN        NOT NULL DEFAULT true,
				currency        TEXT           NOT NULL DEFAULT 'EUR',
				updated_at      TIMESTAMPTZ    NOT NULL DEFAULT NOW()
			);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_holdings_symbol
				ON holdings (symbol);

			CREATE TABLE IF NOT EXISTS dividend_income_records (
				id                      BIGSERIAL      PRIMARY KEY,
				symbol                  TEXT           NOT NULL,
				ex_date                 DATE           NOT NULL,
				payment_date            DATE,
				quantity                NUMERIC(20,8)  NOT NULL,
				cash_dividend_per_share NUMERIC(20,10) NOT NULL,
				income_amount           NUMERIC(20,10) NOT NULL,
				currency                TEXT           NOT NULL DEFAULT 'EUR',
				created_at              TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
				UNIQUE (symbol, ex_date)
			);

			CREATE TABLE IF NOT EXISTS ml_model_checkpoints (
				id           BIGSERIAL   PRIMARY KEY,
				model_name   TEXT        NOT NULL,
				metrics_json JSONB       NOT NULL DEFAULT '{}',
				model_data   BYTEA       NOT NULL,
				is_active    BOOLEAN     NOT NULL DEFAULT false,
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_ml_model_checkpoints_name_active
				ON ml_model_checkpoints (model_name, is_active, created_at DESC);

			CREATE TABLE IF NOT EXISTS transactions (
				id               BIGSERIAL      PRIMARY KEY,
				symbol           TEXT           NOT NULL,
				transaction_type TEXT           NOT NULL CHECK (transaction_type IN ('BUY','SELL')),
				quantity         NUMERIC(20,8)  NOT NULL CHECK (quantity > 0),
				price            NUMERIC(20,10) NOT NULL CHECK (price > 0),
				fee              NUMERIC(20,10) NOT NULL DEFAULT 0 CHECK (fee >= 0),
				allocation_type  TEXT           NOT NULL DEFAULT 'SATELLITE'
				                 CHECK (allocation_type IN ('CORE','SATELLITE')),
				currency         TEXT           NOT NULL DEFAULT 'EUR',
				executed_at      TIMESTAMPTZ    NOT NULL,
				created_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_transactions_symbol_executed_at
				ON transactions (symbol, executed_at);

			CREATE TABLE IF NOT EXISTS cash_flows (
				id           BIGSERIAL      PRIMARY KEY,
				portfolio_id TEXT           NOT NULL,
				flow_type    TEXT           NOT NULL CHECK (flow_type IN ('DEPOSIT','WITHDRAWAL')),
				amount       NUMERIC(20,10) NOT NULL CHECK (amount > 0),
				asset        TEXT           NOT NULL DEFAULT 'CASH',
				currency     TEXT           NOT NULL DEFAULT 'EUR',
				occurred_at  TIMESTAMPTZ    NOT NULL,
				created_at   TIMESTAMPTZ    NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_cash_flows_portfolio_occurred_at
				ON cash_flows (portfolio_id, occurred_at);

			CREATE TABLE IF NOT EXISTS nav_snapshots (
				id                   BIGSERIAL      PRIMARY KEY,
				portfolio_id         TEXT           NOT NULL,
				snapshot_at          TIMESTAMPTZ    NOT NULL,
				nav                  NUMERIC(20,10) NOT NULL CHECK (nav > 0),
				related_cash_flow_id BIGINT         REFERENCES cash_flows (id) ON DELETE SET NULL,
				created_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_nav_snapshots_portfolio_snapshot_at
				ON nav_snapshots (portfolio_id, snapshot_at);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_nav_snapshots_portfolio_date_regular
				ON nav_snapshots (portfolio_id, snapshot_at)
				WHERE related_cash_flow_id IS NULL;

			CREATE TABLE IF NOT EXISTS stock_splits (
				id         BIGSERIAL      PRIMARY KEY,
				symbol     TEXT           NOT NULL,
				split_date DATE           NOT NULL,
				factor     NUMERIC(10,6)  NOT NULL CHECK (factor > 0 AND factor <> 1),
				created_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
				UNIQUE (symbol, split_date)
			);
			CREATE INDEX IF NOT EXISTS idx_stock_splits_symbol_date
				ON stock_splits (symbol, split_date);

			CREATE TABLE IF NOT EXISTS fx_rates (
				id             BIGSERIAL      PRIMARY KEY,
				base_currency  TEXT           NOT NULL,
				quote_currency TEXT           NOT NULL,
				rate_date      DATE           NOT NULL,
				rate           NUMERIC(20,10) NOT NULL CHECK (rate > 0),
				created_at     TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
				UNIQUE (base_currency, quote_currency, rate_date)
			);
			CREATE INDEX IF NOT EXISTS idx_fx_rates_lookup
				ON fx_rates (base_currency, quote_currency, rate_date);

			CREATE TABLE IF NOT EXISTS watchlist (
				id           BIGSERIAL      PRIMARY KEY,
				symbol       TEXT           NOT NULL,
				market_price NUMERIC(20,8)  NOT NULL DEFAULT 0,
				created_at   TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
				UNIQUE (symbol)
			);

			CREATE TABLE IF NOT EXISTS eod_candles (
				id            BIGSERIAL      PRIMARY KEY,
				symbol        TEXT           NOT NULL,
				candle_date   DATE           NOT NULL,
				open          NUMERIC(20,8)  NOT NULL DEFAULT 0,
				high          NUMERIC(20,8)  NOT NULL DEFAULT 0,
				low           NUMERIC(20,8)  NOT NULL DEFAULT 0,
				close_price   NUMERIC(20,8)  NOT NULL DEFAULT 0,
				adj_close     NUMERIC(20,8)  NOT NULL DEFAULT 0,
				volume        BIGINT         NOT NULL DEFAULT 0,
				cash_dividend NUMERIC(20,8)  NOT NULL DEFAULT 0,
				split_factor  NUMERIC(10,6)  NOT NULL DEFAULT 1,
				currency      TEXT           NOT NULL DEFAULT '',
				created_at    TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
				UNIQUE (symbol, candle_date)
			);
			CREATE INDEX IF NOT EXISTS idx_eod_candles_symbol_date
				ON eod_candles (symbol, candle_date);
		`,
	},
}

// Bootstrap runs migrations, seeds defaults, and validates required settings.
func Bootstrap(ctx context.Context, sqlDB *sql.DB) (*Service, error) {
	err := RunMigrations(ctx, sqlDB)
	if err != nil {
		return nil, err
	}

	repo := NewPostgresRepository(sqlDB)

	err = SeedDefaultsIfEmpty(ctx, repo)
	if err != nil {
		return nil, err
	}

	svc := NewService(repo)

	err = svc.ValidateRequired(ctx)
	if err != nil {
		return nil, err
	}

	return svc, nil
}

// RunMigrations executes all known schema migrations exactly once.
func RunMigrations(ctx context.Context, sqlDB *sql.DB) error {
	_, err := sqlDB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, mig := range migrations {
		var count int

		err = sqlDB.QueryRowContext(
			ctx,
			`SELECT COUNT(*) FROM schema_migrations WHERE version = $1`,
			mig.version,
		).Scan(&count)
		if err != nil {
			return fmt.Errorf("check migration version %d: %w", mig.version, err)
		}

		if count > 0 {
			continue
		}

		err = applyMigration(ctx, sqlDB, mig)
		if err != nil {
			return err
		}
	}

	return nil
}

// SeedDefaultsIfEmpty inserts default settings only when the table is empty.
func SeedDefaultsIfEmpty(ctx context.Context, repo Repository) error {
	count, err := repo.Count(ctx)
	if err != nil {
		return fmt.Errorf("count app settings: %w", err)
	}

	if count > 0 {
		return nil
	}

	for key, defaultSetting := range defaultSettings {
		valueJSON, err := json.Marshal(defaultSetting.value)
		if err != nil {
			return fmt.Errorf("marshal default setting %s: %w", key, err)
		}

		err = repo.Set(ctx, Setting{
			Key:         key,
			Scope:       defaultSetting.scope,
			Description: defaultSetting.description,
			ValueJSON:   valueJSON,
			CreatedAt:   time.Time{},
			UpdatedAt:   time.Time{},
		})
		if err != nil {
			return fmt.Errorf("seed default setting %s: %w", key, err)
		}
	}

	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, mig migration) error {
	sqlTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start migration transaction %d: %w", mig.version, err)
	}

	_, err = sqlTx.ExecContext(ctx, mig.sql)
	if err != nil {
		rErr := sqlTx.Rollback()
		if rErr != nil {
			err = errors.Join(err, rErr)
		}

		return fmt.Errorf("apply migration %d (%s): %w", mig.version, mig.name, err)
	}

	_, err = sqlTx.ExecContext(
		ctx,
		`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
		mig.version,
		mig.name,
	)
	if err != nil {
		rErr := sqlTx.Rollback()
		if rErr != nil {
			err = errors.Join(err, rErr)
		}

		return fmt.Errorf("record migration %d (%s): %w", mig.version, mig.name, err)
	}

	err = sqlTx.Commit()
	if err != nil {
		return fmt.Errorf("commit migration %d (%s): %w", mig.version, mig.name, err)
	}

	return nil
}
