package corporate

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/tgragnato/orbiter/internal/portfolio/data"
)

// ActionType classifies supported corporate action types.
type ActionType string

const (
	// ActionTypeSplit is a stock split event.
	ActionTypeSplit ActionType = "SPLIT"
	// ActionTypeDividend is a cash dividend event.
	ActionTypeDividend ActionType = "DIVIDEND"
)

// CorporateAction is one persisted event with normalized fields.
type CorporateAction struct {
	Symbol               string
	Type                 ActionType
	ExDate               time.Time
	PaymentDate          *time.Time
	SplitFactor          float64
	CashDividendPerShare float64
}

// DividendIncome is one realized cash distribution record.
type DividendIncome struct {
	Symbol               string
	ExDate               time.Time
	PaymentDate          *time.Time
	Quantity             float64
	CashDividendPerShare float64
	IncomeAmount         float64
}

// ProcessSummary aggregates processor output over one action batch.
type ProcessSummary struct {
	SplitEvents            int
	DividendEvents         int
	TotalDividendIncome    float64
	UpdatedTotalQuantity   float64
	UpdatedAverageCost     float64
	PreservedCostBasisMark float64
}

// Repository persists action records and applies lot updates.
type Repository interface {
	RecordCorporateAction(ctx context.Context, action CorporateAction) error
	ApplySplitToLots(ctx context.Context, symbol string, factor float64) error
	OpenQuantity(ctx context.Context, symbol string) (float64, error)
	RecordDividendIncome(ctx context.Context, dividend DividendIncome) error
}

// Service processes split and dividend events from market data.
type Service struct {
	repo Repository
}

// NewService creates a corporate action processing service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// AdjustPMCForSplit applies split math to quantity and average cost.
func AdjustPMCForSplit(quantity, averageCost, splitFactor float64) (float64, float64, error) {
	if splitFactor <= 0 {
		return 0, 0, fmt.Errorf("split factor must be > 0")
	}
	return quantity * splitFactor, averageCost / splitFactor, nil
}

// ProcessCandles applies split/dividend events encoded in EOD candles.
func (s *Service) ProcessCandles(ctx context.Context, symbol string, quantity, avgCost float64, candles []data.Candle) (ProcessSummary, error) {
	summary := ProcessSummary{UpdatedTotalQuantity: quantity, UpdatedAverageCost: avgCost}
	summary.PreservedCostBasisMark = quantity * avgCost

	for _, candle := range candles {
		exDate := candle.Time.UTC()

		if candle.SplitFactor > 0 && candle.SplitFactor != 1 {
			action := CorporateAction{Symbol: symbol, Type: ActionTypeSplit, ExDate: exDate, SplitFactor: candle.SplitFactor}
			if err := s.repo.RecordCorporateAction(ctx, action); err != nil {
				return ProcessSummary{}, err
			}
			if err := s.repo.ApplySplitToLots(ctx, symbol, candle.SplitFactor); err != nil {
				return ProcessSummary{}, err
			}
			newQty, newCost, err := AdjustPMCForSplit(summary.UpdatedTotalQuantity, summary.UpdatedAverageCost, candle.SplitFactor)
			if err != nil {
				return ProcessSummary{}, err
			}
			summary.UpdatedTotalQuantity = newQty
			summary.UpdatedAverageCost = newCost
			summary.SplitEvents++
		}

		if candle.CashDividend > 0 {
			action := CorporateAction{Symbol: symbol, Type: ActionTypeDividend, ExDate: exDate, CashDividendPerShare: candle.CashDividend}
			if err := s.repo.RecordCorporateAction(ctx, action); err != nil {
				return ProcessSummary{}, err
			}
			openQty, err := s.repo.OpenQuantity(ctx, symbol)
			if err != nil {
				return ProcessSummary{}, err
			}
			income := openQty * candle.CashDividend
			record := DividendIncome{
				Symbol:               symbol,
				ExDate:               exDate,
				Quantity:             openQty,
				CashDividendPerShare: candle.CashDividend,
				IncomeAmount:         income,
			}
			if err := s.repo.RecordDividendIncome(ctx, record); err != nil {
				return ProcessSummary{}, err
			}
			summary.DividendEvents++
			summary.TotalDividendIncome += income
		}
	}

	return summary, nil
}

// PostgresRepository persists corporate actions and lot/dividend side effects.
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository creates a DB-backed corporate repository.
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// RecordCorporateAction inserts one normalized action event.
func (r *PostgresRepository) RecordCorporateAction(ctx context.Context, action CorporateAction) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO corporate_actions (symbol, action_type, ex_date, payment_date, split_factor, cash_dividend_per_share)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (symbol, action_type, ex_date) DO NOTHING
	`, action.Symbol, string(action.Type), action.ExDate, action.PaymentDate, nullIfZero(action.SplitFactor), nullIfZero(action.CashDividendPerShare))
	return err
}

// ApplySplitToLots adjusts lot quantities and unit costs for stock splits.
func (r *PostgresRepository) ApplySplitToLots(ctx context.Context, symbol string, factor float64) error {
	if factor <= 0 {
		return fmt.Errorf("split factor must be > 0")
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE tax_lot_records
		SET quantity_initial = quantity_initial * $1,
			quantity_remaining = quantity_remaining * $1,
			unit_cost = unit_cost / $1,
			updated_at = NOW()
		WHERE symbol = $2
	`, factor, symbol)
	return err
}

// OpenQuantity returns total currently open quantity for one symbol.
func (r *PostgresRepository) OpenQuantity(ctx context.Context, symbol string) (float64, error) {
	var qty sql.NullFloat64
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(quantity_remaining), 0)
		FROM tax_lot_records
		WHERE symbol = $1
	`, symbol).Scan(&qty)
	if err != nil {
		return 0, err
	}
	if !qty.Valid {
		return 0, nil
	}
	return qty.Float64, nil
}

// RecordDividendIncome inserts one cash dividend attribution record.
func (r *PostgresRepository) RecordDividendIncome(ctx context.Context, dividend DividendIncome) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO dividend_income_records (symbol, ex_date, payment_date, quantity, cash_dividend_per_share, income_amount)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (symbol, ex_date)
		DO UPDATE SET
			payment_date = EXCLUDED.payment_date,
			quantity = EXCLUDED.quantity,
			cash_dividend_per_share = EXCLUDED.cash_dividend_per_share,
			income_amount = EXCLUDED.income_amount
	`, dividend.Symbol, dividend.ExDate, dividend.PaymentDate, dividend.Quantity, dividend.CashDividendPerShare, dividend.IncomeAmount)
	return err
}

func nullIfZero(v float64) any {
	if v == 0 {
		return nil
	}
	return v
}
