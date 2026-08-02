package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CashFlowType classifies supported manual cash flow events.
type CashFlowType string

const (
	// CashFlowDeposit represents a positive cash contribution.
	CashFlowDeposit CashFlowType = "DEPOSIT"
	// CashFlowWithdrawal represents a cash removal from the portfolio.
	CashFlowWithdrawal CashFlowType = "WITHDRAWAL"
)

// TimeRange limits TWR calculations to a specific interval.
type TimeRange struct {
	From time.Time
	To   time.Time
}

// NAVSnapshot stores one valuation point used for sub-period return math.
type NAVSnapshot struct {
	ID            int64
	PortfolioID   string
	SnapshotAt    time.Time
	NAV           float64
	RelatedFlowID *int64
}

// TWRPeriodDetails is one sub-period return component.
type TWRPeriodDetails struct {
	StartAt     time.Time
	EndAt       time.Time
	StartNAV    float64
	EndNAV      float64
	NetCashFlow float64
	Return      float64
}

// TWRResult contains total chained TWR and per-period diagnostics.
type TWRResult struct {
	PortfolioID string
	TotalReturn float64
	Periods     []TWRPeriodDetails
}

// Repository persists and reads analytics state for the TWR engine.
type Repository interface {
	RecordSnapshot(ctx context.Context, portfolioID string, snapshotAt time.Time, nav float64) error
	RecordCashFlowWithSnapshot(ctx context.Context, portfolioID string, flowType CashFlowType, amount float64, asset string, occurredAt time.Time, navBeforeFlow float64) error
	ListSnapshots(ctx context.Context, portfolioID string, tr TimeRange) ([]NAVSnapshot, error)
	SignedCashFlowBetween(ctx context.Context, portfolioID string, from, to time.Time) (float64, error)
}

// TWREngine computes time-weighted return and captures snapshot/cash-flow events.
type TWREngine struct {
	repo Repository
}

// NewTWREngine creates a TWR engine.
func NewTWREngine(repo Repository) *TWREngine {
	return &TWREngine{repo: repo}
}

// RecordNAVSnapshot captures an explicit NAV checkpoint.
func (e *TWREngine) RecordNAVSnapshot(ctx context.Context, portfolioID string, snapshotAt time.Time, nav float64) error {
	if nav <= 0 {
		return fmt.Errorf("nav must be > 0")
	}
	return e.repo.RecordSnapshot(ctx, portfolioID, snapshotAt, nav)
}

// RecordCashFlow captures a pre-flow NAV snapshot and persists the cash flow atomically.
func (e *TWREngine) RecordCashFlow(ctx context.Context, portfolioID string, flowType CashFlowType, amount float64, asset string, occurredAt time.Time, navBeforeFlow float64) error {
	if amount <= 0 {
		return fmt.Errorf("cash flow amount must be > 0")
	}
	if navBeforeFlow <= 0 {
		return fmt.Errorf("nav_before_flow must be > 0")
	}
	if flowType != CashFlowDeposit && flowType != CashFlowWithdrawal {
		return fmt.Errorf("unsupported cash flow type %q", flowType)
	}
	return e.repo.RecordCashFlowWithSnapshot(ctx, portfolioID, flowType, amount, asset, occurredAt, navBeforeFlow)
}

// CalculateTWR computes sub-period returns and chains them geometrically.
func (e *TWREngine) CalculateTWR(ctx context.Context, portfolioID string, tr TimeRange) (TWRResult, error) {
	if tr.To.Before(tr.From) {
		return TWRResult{}, fmt.Errorf("invalid time range")
	}

	snapshots, err := e.repo.ListSnapshots(ctx, portfolioID, tr)
	if err != nil {
		return TWRResult{}, err
	}
	if len(snapshots) < 2 {
		return TWRResult{PortfolioID: portfolioID, TotalReturn: 0, Periods: nil}, nil
	}

	periods := make([]TWRPeriodDetails, 0, len(snapshots)-1)
	product := 1.0

	for i := 0; i < len(snapshots)-1; i++ {
		start := snapshots[i]
		end := snapshots[i+1]
		if start.NAV <= 0 {
			return TWRResult{}, fmt.Errorf("snapshot start NAV must be > 0")
		}

		netCashFlow, err := e.repo.SignedCashFlowBetween(ctx, portfolioID, start.SnapshotAt, end.SnapshotAt)
		if err != nil {
			return TWRResult{}, err
		}

		r := (end.NAV - netCashFlow) / start.NAV
		r -= 1
		periods = append(periods, TWRPeriodDetails{
			StartAt:     start.SnapshotAt,
			EndAt:       end.SnapshotAt,
			StartNAV:    start.NAV,
			EndNAV:      end.NAV,
			NetCashFlow: netCashFlow,
			Return:      r,
		})
		product *= (1 + r)
	}

	return TWRResult{
		PortfolioID: portfolioID,
		TotalReturn: product - 1,
		Periods:     periods,
	}, nil
}

// PostgresRepository persists analytics events in PostgreSQL.
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository creates a PostgreSQL repository for TWR analytics.
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// RecordSnapshot inserts an explicit NAV snapshot.
func (r *PostgresRepository) RecordSnapshot(ctx context.Context, portfolioID string, snapshotAt time.Time, nav float64) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO nav_snapshots (portfolio_id, snapshot_at, nav)
		VALUES ($1, $2, $3)
	`, portfolioID, snapshotAt, nav)
	return err
}

// RecordCashFlowWithSnapshot inserts flow plus pre-flow snapshot in one transaction.
func (r *PostgresRepository) RecordCashFlowWithSnapshot(ctx context.Context, portfolioID string, flowType CashFlowType, amount float64, asset string, occurredAt time.Time, navBeforeFlow float64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var flowID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO cash_flows (portfolio_id, flow_type, amount, asset, occurred_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, portfolioID, string(flowType), amount, asset, occurredAt).Scan(&flowID)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO nav_snapshots (portfolio_id, snapshot_at, nav, related_cash_flow_id)
		VALUES ($1, $2, $3, $4)
	`, portfolioID, occurredAt, navBeforeFlow, flowID)
	if err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

// ListSnapshots returns ordered snapshots in the requested time range.
func (r *PostgresRepository) ListSnapshots(ctx context.Context, portfolioID string, tr TimeRange) ([]NAVSnapshot, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, portfolio_id, snapshot_at, nav, related_cash_flow_id
		FROM nav_snapshots
		WHERE portfolio_id = $1 AND snapshot_at >= $2 AND snapshot_at <= $3
		ORDER BY snapshot_at, id
	`, portfolioID, tr.From, tr.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]NAVSnapshot, 0)
	for rows.Next() {
		var snap NAVSnapshot
		var related sql.NullInt64
		if err := rows.Scan(&snap.ID, &snap.PortfolioID, &snap.SnapshotAt, &snap.NAV, &related); err != nil {
			return nil, err
		}
		if related.Valid {
			v := related.Int64
			snap.RelatedFlowID = &v
		}
		out = append(out, snap)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// SignedCashFlowBetween returns deposits as positive and withdrawals as negative in (from, to].
func (r *PostgresRepository) SignedCashFlowBetween(ctx context.Context, portfolioID string, from, to time.Time) (float64, error) {
	var total sql.NullFloat64
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(
			CASE flow_type
				WHEN 'DEPOSIT' THEN amount
				WHEN 'WITHDRAWAL' THEN -amount
				ELSE 0
			END
		), 0)
		FROM cash_flows
		WHERE portfolio_id = $1
		  AND occurred_at > $2
		  AND occurred_at <= $3
	`, portfolioID, from, to).Scan(&total)
	if err != nil {
		return 0, err
	}
	if !total.Valid {
		return 0, nil
	}
	return total.Float64, nil
}
