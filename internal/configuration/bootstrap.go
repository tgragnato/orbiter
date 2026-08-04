package configuration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

type migration struct {
	version int
	name    string
	sql     string
}

var migrations = []migration{
	{
		version: 1,
		name:    "create_app_settings",
		sql: `
			CREATE TABLE IF NOT EXISTS app_settings (
				key TEXT PRIMARY KEY,
				scope TEXT NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				value_json JSONB NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)
		`,
	},
	{
		version: 2,
		name:    "create_holdings_and_allocation_type",
		sql: `
			CREATE TABLE IF NOT EXISTS holdings (
				id BIGSERIAL PRIMARY KEY,
				symbol TEXT NOT NULL,
				quantity NUMERIC(20,8) NOT NULL,
				market_price NUMERIC(20,8) NOT NULL,
				allocation_type TEXT NOT NULL DEFAULT 'SATELLITE' CHECK (allocation_type IN ('CORE','SATELLITE')),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_holdings_symbol ON holdings(symbol);
			ALTER TABLE IF EXISTS positions ADD COLUMN IF NOT EXISTS allocation_type TEXT NOT NULL DEFAULT 'SATELLITE';
		`,
	},
	{
		version: 3,
		name:    "create_corporate_actions_and_accounting_tables",
		sql: `
			CREATE TABLE IF NOT EXISTS corporate_actions (
				id BIGSERIAL PRIMARY KEY,
				symbol TEXT NOT NULL,
				action_type TEXT NOT NULL CHECK (action_type IN ('SPLIT','DIVIDEND')),
				ex_date DATE NOT NULL,
				payment_date DATE,
				split_factor NUMERIC(20,10),
				cash_dividend_per_share NUMERIC(20,10),
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(symbol, action_type, ex_date)
			);

			CREATE TABLE IF NOT EXISTS tax_lot_records (
				id BIGSERIAL PRIMARY KEY,
				symbol TEXT NOT NULL,
				acquired_at TIMESTAMPTZ NOT NULL,
				quantity_initial NUMERIC(20,8) NOT NULL,
				quantity_remaining NUMERIC(20,8) NOT NULL,
				unit_cost NUMERIC(20,10) NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			CREATE TABLE IF NOT EXISTS realized_pnl (
				id BIGSERIAL PRIMARY KEY,
				symbol TEXT NOT NULL,
				sold_at TIMESTAMPTZ NOT NULL,
				sell_quantity NUMERIC(20,8) NOT NULL,
				sell_unit_price NUMERIC(20,10) NOT NULL,
				cost_basis_method TEXT NOT NULL CHECK (cost_basis_method IN ('PMC','FIFO','LIFO')),
				realized_pnl_amount NUMERIC(20,10) NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			CREATE TABLE IF NOT EXISTS realized_pnl_lot_links (
				id BIGSERIAL PRIMARY KEY,
				realized_pnl_id BIGINT NOT NULL REFERENCES realized_pnl(id) ON DELETE CASCADE,
				tax_lot_id BIGINT NOT NULL REFERENCES tax_lot_records(id) ON DELETE RESTRICT,
				quantity NUMERIC(20,8) NOT NULL,
				unit_cost NUMERIC(20,10) NOT NULL,
				cost_amount NUMERIC(20,10) NOT NULL
			);

			CREATE TABLE IF NOT EXISTS dividend_income_records (
				id BIGSERIAL PRIMARY KEY,
				symbol TEXT NOT NULL,
				ex_date DATE NOT NULL,
				payment_date DATE,
				quantity NUMERIC(20,8) NOT NULL,
				cash_dividend_per_share NUMERIC(20,10) NOT NULL,
				income_amount NUMERIC(20,10) NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(symbol, ex_date)
			);

			CREATE INDEX IF NOT EXISTS idx_corporate_actions_symbol_ex_date ON corporate_actions(symbol, ex_date);
			CREATE INDEX IF NOT EXISTS idx_tax_lot_records_symbol_acquired_at ON tax_lot_records(symbol, acquired_at);
			CREATE INDEX IF NOT EXISTS idx_realized_pnl_symbol_sold_at ON realized_pnl(symbol, sold_at);
		`,
	},
	{
		version: 4,
		name:    "create_twr_analytics_tables",
		sql: `
			CREATE TABLE IF NOT EXISTS cash_flows (
				id BIGSERIAL PRIMARY KEY,
				portfolio_id TEXT NOT NULL,
				flow_type TEXT NOT NULL CHECK (flow_type IN ('DEPOSIT','WITHDRAWAL')),
				amount NUMERIC(20,10) NOT NULL CHECK (amount > 0),
				asset TEXT NOT NULL DEFAULT 'CASH',
				occurred_at TIMESTAMPTZ NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			CREATE TABLE IF NOT EXISTS nav_snapshots (
				id BIGSERIAL PRIMARY KEY,
				portfolio_id TEXT NOT NULL,
				snapshot_at TIMESTAMPTZ NOT NULL,
				nav NUMERIC(20,10) NOT NULL CHECK (nav > 0),
				related_cash_flow_id BIGINT REFERENCES cash_flows(id) ON DELETE SET NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			CREATE INDEX IF NOT EXISTS idx_cash_flows_portfolio_occurred_at ON cash_flows(portfolio_id, occurred_at);
			CREATE INDEX IF NOT EXISTS idx_nav_snapshots_portfolio_snapshot_at ON nav_snapshots(portfolio_id, snapshot_at);
		`,
	},
	{
		version: 5,
		name:    "create_ml_model_checkpoints",
		sql: `
			CREATE TABLE IF NOT EXISTS ml_model_checkpoints (
				id BIGSERIAL PRIMARY KEY,
				model_name TEXT NOT NULL,
				metrics_json JSONB NOT NULL DEFAULT '{}',
				model_data BYTEA NOT NULL,
				is_active BOOLEAN NOT NULL DEFAULT false,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			CREATE INDEX IF NOT EXISTS idx_ml_model_checkpoints_name_active
				ON ml_model_checkpoints (model_name, is_active, created_at DESC);
		`,
	},
	{
		version: 6,
		name:    "add_transactions_and_pmc",
		sql: `
			ALTER TABLE holdings ADD COLUMN IF NOT EXISTS pmc NUMERIC(20,10) NOT NULL DEFAULT 0;

			CREATE TABLE IF NOT EXISTS transactions (
				id               BIGSERIAL PRIMARY KEY,
				symbol           TEXT NOT NULL,
				transaction_type TEXT NOT NULL CHECK (transaction_type IN ('BUY','SELL')),
				quantity         NUMERIC(20,8) NOT NULL CHECK (quantity > 0),
				price            NUMERIC(20,10) NOT NULL CHECK (price > 0),
				fee              NUMERIC(20,10) NOT NULL DEFAULT 0 CHECK (fee >= 0),
				allocation_type  TEXT NOT NULL DEFAULT 'SATELLITE'
				                 CHECK (allocation_type IN ('CORE','SATELLITE')),
				executed_at      TIMESTAMPTZ NOT NULL,
				created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_transactions_symbol_executed_at
				ON transactions(symbol, executed_at);
		`,
	},
	{
		version: 7,
		name:    "add_taa_enabled_to_holdings",
		sql: `
			ALTER TABLE holdings ADD COLUMN IF NOT EXISTS taa_enabled BOOLEAN NOT NULL DEFAULT true;
		`,
	},
}

// Bootstrap runs migrations, seeds defaults, and validates required settings.
func Bootstrap(ctx context.Context, db *sql.DB) (*Service, error) {
	if err := RunMigrations(ctx, db); err != nil {
		return nil, err
	}

	repo := NewPostgresRepository(db)
	if err := SeedDefaultsIfEmpty(ctx, repo); err != nil {
		return nil, err
	}

	svc := NewService(repo)
	if err := svc.ValidateRequired(ctx); err != nil {
		return nil, err
	}

	return svc, nil
}

// RunMigrations executes all known schema migrations exactly once.
func RunMigrations(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, m := range migrations {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = $1`, m.version).Scan(&count); err != nil {
			return fmt.Errorf("check migration version %d: %w", m.version, err)
		}
		if count > 0 {
			continue
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("start migration transaction %d: %w", m.version, err)
		}

		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			if rErr := tx.Rollback(); rErr != nil {
				err = errors.Join(err, rErr)
			}
			return fmt.Errorf("apply migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.version, m.name); err != nil {
			if rErr := tx.Rollback(); rErr != nil {
				err = errors.Join(err, rErr)
			}
			return fmt.Errorf("record migration %d (%s): %w", m.version, m.name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d (%s): %w", m.version, m.name, err)
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

		if err := repo.Set(ctx, Setting{
			Key:         key,
			Scope:       defaultSetting.scope,
			Description: defaultSetting.description,
			ValueJSON:   valueJSON,
		}); err != nil {
			return fmt.Errorf("seed default setting %s: %w", key, err)
		}
	}

	return nil
}
