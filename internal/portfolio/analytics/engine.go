package analytics

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for analytics validation.
var (
	// ErrNAVMustBePositive is returned when NAV is not positive.
	ErrNAVMustBePositive = errors.New("nav must be > 0")
	// ErrCashFlowAmountPositive is returned when cash flow amount is not positive.
	ErrCashFlowAmountPositive = errors.New("cash flow amount must be > 0")
	// ErrNavBeforeFlowPositive is returned when nav_before_flow is not positive.
	ErrNavBeforeFlowPositive = errors.New("nav_before_flow must be > 0")
	// ErrUnsupportedCashFlowType is returned for unknown flow types.
	ErrUnsupportedCashFlowType = errors.New("unsupported cash flow type")
	// ErrInvalidTimeRange is returned when To is before From.
	ErrInvalidTimeRange = errors.New("invalid time range")
	// ErrSnapshotStartNAV is returned when a period starts with non-positive NAV.
	ErrSnapshotStartNAV = errors.New("snapshot start NAV must be > 0")
)

const (
	minSnapshotsForTWR = 2
	hoursPerDay        = 24
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
	PortfolioID  string
	BaseCurrency string // ISO 4217 currency of all NAV snapshots (empty = unknown)
	TotalReturn  float64
	Periods      []TWRPeriodDetails
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
	RecordCashFlowWithSnapshot(
		ctx context.Context,
		portfolioID string,
		flowType CashFlowType,
		amount float64,
		asset string,
		occurredAt time.Time,
		navBeforeFlow float64,
	) error
	ListSnapshots(ctx context.Context, portfolioID string, tr TimeRange) ([]NAVSnapshot, error)
	SignedCashFlowBetween(ctx context.Context, portfolioID string, from, to time.Time) (float64, error)
	// LastSnapshotAt returns the most recent snapshot timestamp for a portfolio.
	// Returns (zero, false, nil) when no snapshots exist yet.
	LastSnapshotAt(ctx context.Context, portfolioID string) (time.Time, bool, error)
	// BackfillTransactionFlows atomically replaces all auto-generated cash flows
	// (asset = 'AUTO') for the portfolio with the supplied set. Idempotent: safe
	// to call on every startup.
	BackfillTransactionFlows(ctx context.Context, portfolioID string, flows []TransactionFlow) error
	// ClearSnapshots removes all NAV snapshots for a portfolio (e.g. before recalculating in a new base currency).
	ClearSnapshots(ctx context.Context, portfolioID string) error
}

// TWREngine computes time-weighted return and captures snapshot/cash-flow events.
type TWREngine struct {
	repo Repository
}

// NewTWREngine creates a TWR engine.
func NewTWREngine(repo Repository) *TWREngine {
	return &TWREngine{repo: repo}
}

// BackfillTransactionFlows atomically replaces all auto-generated cash flows
// for the portfolio. Each BUY transaction should become a DEPOSIT and each
// SELL a WITHDRAWAL so the TWR formula can isolate price returns from capital
// injections/withdrawals.
func (e *TWREngine) BackfillTransactionFlows(ctx context.Context, portfolioID string, flows []TransactionFlow) error {
	err := e.repo.BackfillTransactionFlows(ctx, portfolioID, flows)
	if err != nil {
		return fmt.Errorf("backfill transaction flows: %w", err)
	}

	return nil
}

// CalculateTWR computes sub-period returns and chains them geometrically.
func (e *TWREngine) CalculateTWR(ctx context.Context, portfolioID string, tr TimeRange) (TWRResult, error) {
	if tr.To.Before(tr.From) {
		return TWRResult{PortfolioID: "", BaseCurrency: "", TotalReturn: 0, Periods: nil}, ErrInvalidTimeRange
	}

	snapshots, err := e.repo.ListSnapshots(ctx, portfolioID, tr)
	if err != nil {
		return TWRResult{PortfolioID: "", BaseCurrency: "", TotalReturn: 0, Periods: nil},
			fmt.Errorf("list snapshots: %w", err)
	}

	if len(snapshots) < minSnapshotsForTWR {
		return TWRResult{PortfolioID: portfolioID, BaseCurrency: "", TotalReturn: 0, Periods: nil}, nil
	}

	periods := make([]TWRPeriodDetails, 0, len(snapshots)-1)
	product := 1.0

	for i := range len(snapshots) - 1 {
		start := snapshots[i]
		end := snapshots[i+1]

		if start.NAV <= 0 {
			return TWRResult{PortfolioID: "", BaseCurrency: "", TotalReturn: 0, Periods: nil}, ErrSnapshotStartNAV
		}

		netCashFlow, err := e.repo.SignedCashFlowBetween(ctx, portfolioID, start.SnapshotAt, end.SnapshotAt)
		if err != nil {
			return TWRResult{PortfolioID: "", BaseCurrency: "", TotalReturn: 0, Periods: nil},
				fmt.Errorf("signed cash flow between: %w", err)
		}

		periodReturn := (end.NAV - netCashFlow) / start.NAV
		periodReturn -= 1
		periods = append(periods, TWRPeriodDetails{
			StartAt:     start.SnapshotAt,
			EndAt:       end.SnapshotAt,
			StartNAV:    start.NAV,
			EndNAV:      end.NAV,
			NetCashFlow: netCashFlow,
			Return:      periodReturn,
		})
		product *= (1 + periodReturn)
	}

	return TWRResult{
		PortfolioID:  portfolioID,
		BaseCurrency: "",
		TotalReturn:  product - 1,
		Periods:      periods,
	}, nil
}

// ClearSnapshots removes all NAV snapshots for the given portfolio.
func (e *TWREngine) ClearSnapshots(ctx context.Context, portfolioID string) error {
	err := e.repo.ClearSnapshots(ctx, portfolioID)
	if err != nil {
		return fmt.Errorf("clear snapshots: %w", err)
	}

	return nil
}

// LastNAVSnapshotAt returns the timestamp of the most recent NAV snapshot for
// the portfolio. Returns (zero, false, nil) when the table is empty.
func (e *TWREngine) LastNAVSnapshotAt(ctx context.Context, portfolioID string) (time.Time, bool, error) {
	snapshotTime, found, err := e.repo.LastSnapshotAt(ctx, portfolioID)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("last snapshot at: %w", err)
	}

	return snapshotTime, found, nil
}

// RecordCashFlow captures a pre-flow NAV snapshot and persists the cash flow atomically.
func (e *TWREngine) RecordCashFlow(
	ctx context.Context,
	portfolioID string,
	flowType CashFlowType,
	amount float64,
	asset string,
	occurredAt time.Time,
	navBeforeFlow float64,
) error {
	if amount <= 0 {
		return ErrCashFlowAmountPositive
	}

	if navBeforeFlow <= 0 {
		return ErrNavBeforeFlowPositive
	}

	if flowType != CashFlowDeposit && flowType != CashFlowWithdrawal {
		return fmt.Errorf("%w: %q", ErrUnsupportedCashFlowType, flowType)
	}

	err := e.repo.RecordCashFlowWithSnapshot(ctx, portfolioID, flowType, amount, asset, occurredAt, navBeforeFlow)
	if err != nil {
		return fmt.Errorf("record cash flow with snapshot: %w", err)
	}

	return nil
}

// RecordNAVSnapshot captures an explicit NAV checkpoint.
func (e *TWREngine) RecordNAVSnapshot(
	ctx context.Context, portfolioID string, snapshotAt time.Time, nav float64,
) error {
	if nav <= 0 {
		return ErrNAVMustBePositive
	}

	err := e.repo.RecordSnapshot(ctx, portfolioID, snapshotAt, nav)
	if err != nil {
		return fmt.Errorf("record snapshot: %w", err)
	}

	return nil
}

// PostgresRepository persists analytics events in PostgreSQL.
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository creates a PostgreSQL repository for TWR analytics.
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// BackfillTransactionFlows atomically replaces all auto-generated cash flows
// (asset = 'AUTO') for the portfolio with the provided set. It runs inside a
// single transaction so a mid-run crash leaves the previous state intact.
func (r *PostgresRepository) BackfillTransactionFlows(
	ctx context.Context, portfolioID string, flows []TransactionFlow,
) error {
	sqlTx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			_ = sqlTx.Rollback()
		}
	}()

	_, err = sqlTx.ExecContext(ctx,
		`DELETE FROM cash_flows WHERE portfolio_id = $1 AND asset = 'AUTO'`,
		portfolioID,
	)
	if err != nil {
		return fmt.Errorf("delete auto cash flows: %w", err)
	}

	for _, flow := range flows {
		if flow.Amount <= 0 {
			continue
		}

		_, err = sqlTx.ExecContext(ctx, `
			INSERT INTO cash_flows (portfolio_id, flow_type, amount, asset, occurred_at)
			VALUES ($1, $2, $3, 'AUTO', $4)
		`, portfolioID, string(flow.Type), flow.Amount, flow.OccurredAt)
		if err != nil {
			return fmt.Errorf("insert cash flow: %w", err)
		}
	}

	err = sqlTx.Commit()
	if err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// ClearSnapshots removes all NAV snapshots for a portfolio.
func (r *PostgresRepository) ClearSnapshots(ctx context.Context, portfolioID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM nav_snapshots WHERE portfolio_id = $1`, portfolioID)
	if err != nil {
		return fmt.Errorf("delete nav snapshots: %w", err)
	}

	return nil
}

// LastSnapshotAt returns the most recent snapshot timestamp for a portfolio.
// Returns (zero, false, nil) when the table is empty.
func (r *PostgresRepository) LastSnapshotAt(ctx context.Context, portfolioID string) (time.Time, bool, error) {
	var nullTime sql.NullTime

	err := r.db.QueryRowContext(ctx,
		`SELECT MAX(snapshot_at) FROM nav_snapshots WHERE portfolio_id = $1`,
		portfolioID,
	).Scan(&nullTime)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("query last snapshot at: %w", err)
	}

	if !nullTime.Valid {
		return time.Time{}, false, nil
	}

	return nullTime.Time.UTC(), true, nil
}

// ListSnapshots returns ordered snapshots in the requested time range.
func (r *PostgresRepository) ListSnapshots(
	ctx context.Context, portfolioID string, timeRange TimeRange,
) ([]NAVSnapshot, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, portfolio_id, snapshot_at, nav, related_cash_flow_id
		FROM nav_snapshots
		WHERE portfolio_id = $1 AND snapshot_at >= $2 AND snapshot_at <= $3
		ORDER BY snapshot_at, id
	`, portfolioID, timeRange.From, timeRange.To)
	if err != nil {
		return nil, fmt.Errorf("query nav snapshots: %w", err)
	}

	defer func() { _ = rows.Close() }()

	out := make([]NAVSnapshot, 0)

	for rows.Next() {
		var snap NAVSnapshot

		var related sql.NullInt64

		err = rows.Scan(&snap.ID, &snap.PortfolioID, &snap.SnapshotAt, &snap.NAV, &related)
		if err != nil {
			return nil, fmt.Errorf("scan nav snapshot: %w", err)
		}

		if related.Valid {
			flowID := related.Int64
			snap.RelatedFlowID = &flowID
		}

		out = append(out, snap)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return out, nil
}

// RecordCashFlowWithSnapshot inserts flow plus pre-flow snapshot in one transaction.
func (r *PostgresRepository) RecordCashFlowWithSnapshot(
	ctx context.Context,
	portfolioID string,
	flowType CashFlowType,
	amount float64,
	asset string,
	occurredAt time.Time,
	navBeforeFlow float64,
) error {
	sqlTx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			_ = sqlTx.Rollback()
		}
	}()

	var flowID int64

	err = sqlTx.QueryRowContext(ctx, `
		INSERT INTO cash_flows (portfolio_id, flow_type, amount, asset, occurred_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, portfolioID, string(flowType), amount, asset, occurredAt).Scan(&flowID)
	if err != nil {
		return fmt.Errorf("insert cash flow: %w", err)
	}

	_, err = sqlTx.ExecContext(ctx, `
		INSERT INTO nav_snapshots (portfolio_id, snapshot_at, nav, related_cash_flow_id)
		VALUES ($1, $2, $3, $4)
	`, portfolioID, occurredAt, navBeforeFlow, flowID)
	if err != nil {
		return fmt.Errorf("insert nav snapshot: %w", err)
	}

	err = sqlTx.Commit()
	if err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
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
func (r *PostgresRepository) RecordSnapshot(
	ctx context.Context, portfolioID string, snapshotAt time.Time, nav float64,
) error {
	dayStart := snapshotAt.UTC().Truncate(hoursPerDay * time.Hour)

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO nav_snapshots (portfolio_id, snapshot_at, nav)
		VALUES ($1, $2, $3)
		ON CONFLICT (portfolio_id, snapshot_at) WHERE related_cash_flow_id IS NULL
		DO UPDATE SET nav = EXCLUDED.nav
	`, portfolioID, dayStart, nav)
	if err != nil {
		return fmt.Errorf("exec insert nav snapshot: %w", err)
	}

	return nil
}

// SignedCashFlowBetween returns deposits as positive and withdrawals as negative in (from, until].
func (r *PostgresRepository) SignedCashFlowBetween(
	ctx context.Context, portfolioID string, from, until time.Time,
) (float64, error) {
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
	`, portfolioID, from, until).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("query signed cash flow: %w", err)
	}

	if !total.Valid {
		return 0, nil
	}

	return total.Float64, nil
}
