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

// TransactionFlow is one cash-flow entry derived from a trade transaction.
// Used by BackfillTransactionFlows to bulk-populate the cash_flows table.
type TransactionFlow struct {
	Type       CashFlowType
	Amount     float64
	OccurredAt time.Time
}

// Repository persists and reads analytics state for the TWR engine.
type Repository interface {
	RecordSnapshot(ctx context.Context, portfolioID string, snapshotAt time.Time, nav float64) error
	RecordCashFlowWithSnapshot(ctx context.Context, portfolioID string, flowType CashFlowType, amount float64, asset string, occurredAt time.Time, navBeforeFlow float64) error
	ListSnapshots(ctx context.Context, portfolioID string, tr TimeRange) ([]NAVSnapshot, error)
	SignedCashFlowBetween(ctx context.Context, portfolioID string, from, to time.Time) (float64, error)
	// LastSnapshotAt returns the most recent snapshot timestamp for a portfolio.
	// Returns (zero, false, nil) when no snapshots exist yet.
	LastSnapshotAt(ctx context.Context, portfolioID string) (time.Time, bool, error)
	// BackfillTransactionFlows atomically replaces all auto-generated cash flows
	// (asset = 'AUTO') for the portfolio with the supplied set. Idempotent: safe
	// to call on every startup.
	BackfillTransactionFlows(ctx context.Context, portfolioID string, flows []TransactionFlow) error
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

// LastNAVSnapshotAt returns the timestamp of the most recent NAV snapshot for
// the portfolio. Returns (zero, false, nil) when the table is empty.
func (e *TWREngine) LastNAVSnapshotAt(ctx context.Context, portfolioID string) (time.Time, bool, error) {
	return e.repo.LastSnapshotAt(ctx, portfolioID)
}

// BackfillTransactionFlows atomically replaces all auto-generated cash flows
// for the portfolio. Each BUY transaction should become a DEPOSIT and each
// SELL a WITHDRAWAL so the TWR formula can isolate price returns from capital
// injections/withdrawals.
func (e *TWREngine) BackfillTransactionFlows(ctx context.Context, portfolioID string, flows []TransactionFlow) error {
	return e.repo.BackfillTransactionFlows(ctx, portfolioID, flows)
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

// RecordSnapshot upserts a NAV snapshot for a portfolio day. The timestamp is
// truncated to UTC midnight so the partial unique index on
// (portfolio_id, snapshot_at) WHERE related_cash_flow_id IS NULL (migration v8)
// enforces at most one regular snapshot per portfolio per day.
//
// On conflict the existing row is updated (not skipped) so that each app restart
// refreshes historical NAVs with the latest Yahoo-adjusted prices. This is
// required for TWR correctness: cash flows are always recomputed from current
// adjusted prices (delete-then-insert), and the NAV snapshots they pair against
// must use the same price basis — otherwise retroactive dividend adjustments
// create permanent mismatches that manifest as spurious negative TWR in any
// period containing a purchase.
func (r *PostgresRepository) RecordSnapshot(ctx context.Context, portfolioID string, snapshotAt time.Time, nav float64) error {
	dayStart := snapshotAt.UTC().Truncate(24 * time.Hour)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO nav_snapshots (portfolio_id, snapshot_at, nav)
		VALUES ($1, $2, $3)
		ON CONFLICT (portfolio_id, snapshot_at) WHERE related_cash_flow_id IS NULL
		DO UPDATE SET nav = EXCLUDED.nav
	`, portfolioID, dayStart, nav)
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

	if err := tx.Commit(); err != nil {
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
	defer func() { _ = rows.Close() }()

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

// BackfillTransactionFlows atomically replaces all auto-generated cash flows
// (asset = 'AUTO') for the portfolio with the provided set. It runs inside a
// single transaction so a mid-run crash leaves the previous state intact.
func (r *PostgresRepository) BackfillTransactionFlows(ctx context.Context, portfolioID string, flows []TransactionFlow) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx,
		`DELETE FROM cash_flows WHERE portfolio_id = $1 AND asset = 'AUTO'`,
		portfolioID,
	); err != nil {
		return err
	}

	for _, f := range flows {
		if f.Amount <= 0 {
			continue
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO cash_flows (portfolio_id, flow_type, amount, asset, occurred_at)
			VALUES ($1, $2, $3, 'AUTO', $4)
		`, portfolioID, string(f.Type), f.Amount, f.OccurredAt); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

// LastSnapshotAt returns the most recent snapshot timestamp for a portfolio.
// Returns (zero, false, nil) when the table is empty.
func (r *PostgresRepository) LastSnapshotAt(ctx context.Context, portfolioID string) (time.Time, bool, error) {
	var t sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT MAX(snapshot_at) FROM nav_snapshots WHERE portfolio_id = $1`,
		portfolioID,
	).Scan(&t)
	if err != nil {
		return time.Time{}, false, err
	}
	if !t.Valid {
		return time.Time{}, false, nil
	}
	return t.Time.UTC(), true, nil
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
