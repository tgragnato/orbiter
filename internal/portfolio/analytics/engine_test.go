package analytics

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

type fakeRepository struct {
	snapshots     []NAVSnapshot
	cashFlows     map[string]float64
	recordSnapErr error
	recordFlowErr error
	listErr       error
	flowErr       error

	recordedSnapshots []NAVSnapshot
	recordedFlows     []recordedFlow
}

type recordedFlow struct {
	PortfolioID   string
	FlowType      CashFlowType
	Amount        float64
	Asset         string
	OccurredAt    time.Time
	NAVBeforeFlow float64
}

func (f *fakeRepository) RecordSnapshot(_ context.Context, portfolioID string, snapshotAt time.Time, nav float64) error {
	if f.recordSnapErr != nil {
		return f.recordSnapErr
	}
	f.recordedSnapshots = append(f.recordedSnapshots, NAVSnapshot{
		PortfolioID: portfolioID,
		SnapshotAt:  snapshotAt,
		NAV:         nav,
	})
	return nil
}

func (f *fakeRepository) RecordCashFlowWithSnapshot(_ context.Context, portfolioID string, flowType CashFlowType, amount float64, asset string, occurredAt time.Time, navBeforeFlow float64) error {
	if f.recordFlowErr != nil {
		return f.recordFlowErr
	}
	f.recordedFlows = append(f.recordedFlows, recordedFlow{
		PortfolioID:   portfolioID,
		FlowType:      flowType,
		Amount:        amount,
		Asset:         asset,
		OccurredAt:    occurredAt,
		NAVBeforeFlow: navBeforeFlow,
	})
	return nil
}

func (f *fakeRepository) ListSnapshots(_ context.Context, _ string, _ TimeRange) ([]NAVSnapshot, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]NAVSnapshot, len(f.snapshots))
	copy(out, f.snapshots)
	return out, nil
}

func (f *fakeRepository) SignedCashFlowBetween(_ context.Context, _ string, from, to time.Time) (float64, error) {
	if f.flowErr != nil {
		return 0, f.flowErr
	}
	key := from.UTC().Format(time.RFC3339) + "->" + to.UTC().Format(time.RFC3339)
	return f.cashFlows[key], nil
}

func (f *fakeRepository) BackfillTransactionFlows(_ context.Context, _ string, _ []TransactionFlow) error {
	return nil
}

func (f *fakeRepository) ClearSnapshots(_ context.Context, _ string) error {
	f.snapshots = nil
	return nil
}

func (f *fakeRepository) LastSnapshotAt(_ context.Context, _ string) (time.Time, bool, error) {
	if len(f.snapshots) == 0 {
		return time.Time{}, false, nil
	}
	last := f.snapshots[0].SnapshotAt
	for _, s := range f.snapshots[1:] {
		if s.SnapshotAt.After(last) {
			last = s.SnapshotAt
		}
	}
	return last, true, nil
}

func TestCalculateTWRPACStyleFlows(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(24 * time.Hour)
	t2 := t1.Add(24 * time.Hour)
	t3 := t2.Add(24 * time.Hour)

	repo := &fakeRepository{
		snapshots: []NAVSnapshot{
			{PortfolioID: "MAIN", SnapshotAt: t0, NAV: 1000},
			{PortfolioID: "MAIN", SnapshotAt: t1, NAV: 1120},
			{PortfolioID: "MAIN", SnapshotAt: t2, NAV: 1265},
			{PortfolioID: "MAIN", SnapshotAt: t3, NAV: 1330},
		},
		cashFlows: map[string]float64{
			t0.Format(time.RFC3339) + "->" + t1.Format(time.RFC3339): 100, // R1 = (1120-100)/1000 - 1 = 2%
			t1.Format(time.RFC3339) + "->" + t2.Format(time.RFC3339): 50,  // R2 = (1265-50)/1120 - 1 = 8.4821%
			t2.Format(time.RFC3339) + "->" + t3.Format(time.RFC3339): -20, // R3 = (1330-(-20))/1265 - 1 = 6.7194%
		},
	}

	engine := NewTWREngine(repo)
	result, err := engine.CalculateTWR(context.Background(), "MAIN", TimeRange{From: t0, To: t3})
	if err != nil {
		t.Fatalf("CalculateTWR() error = %v", err)
	}

	if len(result.Periods) != 3 {
		t.Fatalf("period count = %d, want 3", len(result.Periods))
	}

	wantPeriod := []float64{0.02, 0.0848214285714286, 0.0671936758893282}
	for i := range wantPeriod {
		assertNear(t, result.Periods[i].Return, wantPeriod[i], 1e-9)
	}

	wantTotal := (1+wantPeriod[0])*(1+wantPeriod[1])*(1+wantPeriod[2]) - 1
	assertNear(t, result.TotalReturn, wantTotal, 1e-9)
}

func TestCalculateTWRLargeWithdrawal(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(24 * time.Hour)

	repo := &fakeRepository{
		snapshots: []NAVSnapshot{
			{PortfolioID: "MAIN", SnapshotAt: t0, NAV: 1000},
			{PortfolioID: "MAIN", SnapshotAt: t1, NAV: 220},
		},
		cashFlows: map[string]float64{
			t0.Format(time.RFC3339) + "->" + t1.Format(time.RFC3339): -800,
		},
	}

	engine := NewTWREngine(repo)
	result, err := engine.CalculateTWR(context.Background(), "MAIN", TimeRange{From: t0, To: t1})
	if err != nil {
		t.Fatalf("CalculateTWR() error = %v", err)
	}

	// (220 - (-800))/1000 - 1 = 2%
	assertNear(t, result.TotalReturn, 0.02, 1e-9)
}

func TestCalculateTWRFlatMarket(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(24 * time.Hour)
	t2 := t1.Add(24 * time.Hour)

	repo := &fakeRepository{
		snapshots: []NAVSnapshot{
			{PortfolioID: "MAIN", SnapshotAt: t0, NAV: 1000},
			{PortfolioID: "MAIN", SnapshotAt: t1, NAV: 1200},
			{PortfolioID: "MAIN", SnapshotAt: t2, NAV: 1000},
		},
		cashFlows: map[string]float64{
			t0.Format(time.RFC3339) + "->" + t1.Format(time.RFC3339): 200,
			t1.Format(time.RFC3339) + "->" + t2.Format(time.RFC3339): -200,
		},
	}

	engine := NewTWREngine(repo)
	result, err := engine.CalculateTWR(context.Background(), "MAIN", TimeRange{From: t0, To: t2})
	if err != nil {
		t.Fatalf("CalculateTWR() error = %v", err)
	}

	assertNear(t, result.TotalReturn, 0, 1e-9)
}

func TestCalculateTWREdges(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	engine := NewTWREngine(&fakeRepository{})

	_, err := engine.CalculateTWR(context.Background(), "MAIN", TimeRange{From: now, To: now.Add(-time.Hour)})
	if err == nil {
		t.Fatalf("expected invalid range error")
	}

	repo := &fakeRepository{snapshots: []NAVSnapshot{{PortfolioID: "MAIN", SnapshotAt: now, NAV: 100}}}
	engine = NewTWREngine(repo)
	result, err := engine.CalculateTWR(context.Background(), "MAIN", TimeRange{From: now.Add(-time.Hour), To: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("CalculateTWR() error = %v", err)
	}
	if result.TotalReturn != 0 || len(result.Periods) != 0 {
		t.Fatalf("unexpected non-empty result: %+v", result)
	}

	repo = &fakeRepository{snapshots: []NAVSnapshot{{PortfolioID: "MAIN", SnapshotAt: now, NAV: 0}, {PortfolioID: "MAIN", SnapshotAt: now.Add(time.Hour), NAV: 10}}}
	engine = NewTWREngine(repo)
	_, err = engine.CalculateTWR(context.Background(), "MAIN", TimeRange{From: now.Add(-time.Hour), To: now.Add(2 * time.Hour)})
	if err == nil {
		t.Fatalf("expected start NAV validation error")
	}
}

func TestRecordMethodsValidationAndDelegation(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	repo := &fakeRepository{}
	engine := NewTWREngine(repo)

	if err := engine.RecordNAVSnapshot(context.Background(), "MAIN", now, 0); err == nil {
		t.Fatalf("expected nav validation error")
	}
	if err := engine.RecordNAVSnapshot(context.Background(), "MAIN", now, 10); err != nil {
		t.Fatalf("RecordNAVSnapshot() error = %v", err)
	}
	if len(repo.recordedSnapshots) != 1 {
		t.Fatalf("recorded snapshots = %d, want 1", len(repo.recordedSnapshots))
	}

	if err := engine.RecordCashFlow(context.Background(), "MAIN", CashFlowDeposit, 0, "EUR", now, 100); err == nil {
		t.Fatalf("expected amount validation error")
	}
	if err := engine.RecordCashFlow(context.Background(), "MAIN", CashFlowType("BAD"), 10, "EUR", now, 100); err == nil {
		t.Fatalf("expected flow type validation error")
	}
	if err := engine.RecordCashFlow(context.Background(), "MAIN", CashFlowDeposit, 10, "EUR", now, 0); err == nil {
		t.Fatalf("expected nav_before_flow validation error")
	}
	if err := engine.RecordCashFlow(context.Background(), "MAIN", CashFlowWithdrawal, 10, "EUR", now, 1000); err != nil {
		t.Fatalf("RecordCashFlow() error = %v", err)
	}
	if len(repo.recordedFlows) != 1 {
		t.Fatalf("recorded flows = %d, want 1", len(repo.recordedFlows))
	}
}

func TestCalculateTWRErrorsFromRepository(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	engine := NewTWREngine(&fakeRepository{listErr: errors.New("list failed")})
	_, err := engine.CalculateTWR(context.Background(), "MAIN", TimeRange{From: now.Add(-time.Hour), To: now})
	if err == nil {
		t.Fatalf("expected list error")
	}

	repo := &fakeRepository{
		snapshots: []NAVSnapshot{{PortfolioID: "MAIN", SnapshotAt: now.Add(-time.Hour), NAV: 100}, {PortfolioID: "MAIN", SnapshotAt: now, NAV: 110}},
		flowErr:   errors.New("flow failed"),
	}
	engine = NewTWREngine(repo)
	_, err = engine.CalculateTWR(context.Background(), "MAIN", TimeRange{From: now.Add(-2 * time.Hour), To: now})
	if err == nil {
		t.Fatalf("expected flow error")
	}
}

func assertNear(t *testing.T, got, want, eps float64) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Fatalf("got %.12f, want %.12f (eps=%.12f)", got, want, eps)
	}
}
