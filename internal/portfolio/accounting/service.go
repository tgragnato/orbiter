package accounting

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tgragnato/orbiter/internal/configuration"
)

// MethodResolver loads cost basis preference from persistence.
type MethodResolver interface {
	ResolveCostBasisMethod(ctx context.Context) (configuration.CostBasisMethod, error)
}

// LedgerRepository manages tax lots and realized PnL persistence.
type LedgerRepository interface {
	ListOpenLots(ctx context.Context, symbol string) ([]TaxLot, error)
	AddTaxLot(ctx context.Context, lot TaxLot) (int64, error)
	PersistRealization(ctx context.Context, sell SellTransaction, method configuration.CostBasisMethod, result RealizedPnLResult, updatedLots []TaxLot) error
}

// Service provides realized PnL calculations with configurable cost basis methods.
type Service struct {
	repo        LedgerRepository
	resolver    MethodResolver
	calculators map[configuration.CostBasisMethod]CostBasisCalculator
}

// NewService creates a realized PnL service.
func NewService(repo LedgerRepository, resolver MethodResolver) *Service {
	return &Service{
		repo:     repo,
		resolver: resolver,
		calculators: map[configuration.CostBasisMethod]CostBasisCalculator{
			configuration.CostBasisPMC:  PMCCalculator{},
			configuration.CostBasisFIFO: FIFOCalculator{},
			configuration.CostBasisLIFO: LIFOCalculator{},
		},
	}
}

// CalculateRealizedPnL computes and persists one realized PnL event.
func (s *Service) CalculateRealizedPnL(ctx context.Context, sell SellTransaction) (RealizedPnLResult, error) {
	method, err := s.resolver.ResolveCostBasisMethod(ctx)
	if err != nil {
		return RealizedPnLResult{}, err
	}
	calculator, ok := s.calculators[method]
	if !ok {
		calculator = s.calculators[configuration.CostBasisPMC]
		method = configuration.CostBasisPMC
	}

	lots, err := s.repo.ListOpenLots(ctx, sell.Symbol)
	if err != nil {
		return RealizedPnLResult{}, err
	}

	result, updatedLots, err := calculator.Calculate(lots, sell)
	if err != nil {
		return RealizedPnLResult{}, err
	}

	if err := s.repo.PersistRealization(ctx, sell, method, result, updatedLots); err != nil {
		return RealizedPnLResult{}, err
	}

	return result, nil
}

// PostgresMethodResolver resolves cost basis mode from app_settings.
type PostgresMethodResolver struct {
	db *sql.DB
}

// NewPostgresMethodResolver creates a DB-backed cost basis resolver.
func NewPostgresMethodResolver(db *sql.DB) *PostgresMethodResolver {
	return &PostgresMethodResolver{db: db}
}

// ResolveCostBasisMethod loads configured method and defaults to PMC on missing/invalid values.
func (r *PostgresMethodResolver) ResolveCostBasisMethod(ctx context.Context) (configuration.CostBasisMethod, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, `SELECT value_json FROM app_settings WHERE key = $1`, configuration.KeyCostBasisMethod).Scan(&raw)
	if err == sql.ErrNoRows {
		return configuration.CostBasisPMC, nil
	}
	if err != nil {
		return "", err
	}

	var setting configuration.CostBasisSetting
	if err := json.Unmarshal(raw, &setting); err != nil {
		return configuration.CostBasisPMC, nil
	}

	switch strings.ToUpper(string(setting.Method)) {
	case string(configuration.CostBasisFIFO):
		return configuration.CostBasisFIFO, nil
	case string(configuration.CostBasisLIFO):
		return configuration.CostBasisLIFO, nil
	case string(configuration.CostBasisPMC):
		return configuration.CostBasisPMC, nil
	default:
		return configuration.CostBasisPMC, nil
	}
}

// PostgresLedgerRepository persists tax lot and realized PnL entries.
type PostgresLedgerRepository struct {
	db *sql.DB
}

// NewPostgresLedgerRepository creates a DB-backed ledger repository.
func NewPostgresLedgerRepository(db *sql.DB) *PostgresLedgerRepository {
	return &PostgresLedgerRepository{db: db}
}

// ListOpenLots returns lots with remaining quantity for one symbol.
func (r *PostgresLedgerRepository) ListOpenLots(ctx context.Context, symbol string) ([]TaxLot, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, symbol, acquired_at, quantity_initial, quantity_remaining, unit_cost
		FROM tax_lot_records
		WHERE symbol = $1 AND quantity_remaining > 0
		ORDER BY acquired_at, id
	`, symbol)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lots := make([]TaxLot, 0)
	for rows.Next() {
		var lot TaxLot
		if err := rows.Scan(&lot.ID, &lot.Symbol, &lot.AcquiredAt, &lot.QuantityInitial, &lot.QuantityRemaining, &lot.UnitCost); err != nil {
			return nil, err
		}
		lots = append(lots, lot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return lots, nil
}

// AddTaxLot inserts a new buy lot.
func (r *PostgresLedgerRepository) AddTaxLot(ctx context.Context, lot TaxLot) (int64, error) {
	if lot.QuantityInitial <= 0 {
		return 0, fmt.Errorf("quantity_initial must be > 0")
	}
	remaining := lot.QuantityRemaining
	if remaining <= 0 {
		remaining = lot.QuantityInitial
	}
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO tax_lot_records (symbol, acquired_at, quantity_initial, quantity_remaining, unit_cost)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, lot.Symbol, lot.AcquiredAt, lot.QuantityInitial, remaining, lot.UnitCost).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// PersistRealization stores realized PnL event, lot links, and updated lot balances.
func (r *PostgresLedgerRepository) PersistRealization(ctx context.Context, sell SellTransaction, method configuration.CostBasisMethod, result RealizedPnLResult, updatedLots []TaxLot) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, lot := range updatedLots {
		if _, err = tx.ExecContext(ctx, `
			UPDATE tax_lot_records
			SET quantity_remaining = $1, unit_cost = $2, updated_at = NOW()
			WHERE id = $3
		`, lot.QuantityRemaining, lot.UnitCost, lot.ID); err != nil {
			return err
		}
	}

	var realizedID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO realized_pnl (symbol, sold_at, sell_quantity, sell_unit_price, cost_basis_method, realized_pnl_amount)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, sell.Symbol, sell.SoldAt, sell.Quantity, sell.UnitPrice, string(method), result.RealizedPnL).Scan(&realizedID)
	if err != nil {
		return err
	}

	for _, link := range result.Consumptions {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO realized_pnl_lot_links (realized_pnl_id, tax_lot_id, quantity, unit_cost, cost_amount)
			VALUES ($1, $2, $3, $4, $5)
		`, realizedID, link.TaxLotID, link.Quantity, link.UnitCost, link.CostAmount); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

// TotalRealizedPnL returns the cumulative realized PnL amount.
func (r *PostgresLedgerRepository) TotalRealizedPnL(ctx context.Context) (float64, error) {
	var total sql.NullFloat64
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(realized_pnl_amount), 0) FROM realized_pnl`).Scan(&total)
	if err != nil {
		return 0, err
	}
	if !total.Valid {
		return 0, nil
	}
	return total.Float64, nil
}
