package corporate

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/tgragnato/orbiter/internal/portfolio/data"
)

func TestAdjustPMCForSplit(t *testing.T) {
	t.Parallel()

	newQty, newCost, err := AdjustPMCForSplit(10, 100, 2)
	if err != nil {
		t.Fatalf("AdjustPMCForSplit() error = %v", err)
	}
	if newQty != 20 {
		t.Fatalf("newQty = %f, want 20", newQty)
	}
	if newCost != 50 {
		t.Fatalf("newCost = %f, want 50", newCost)
	}
	if newQty*newCost != 10*100 {
		t.Fatalf("cost basis mark changed")
	}

	if _, _, err := AdjustPMCForSplit(10, 100, 0); err == nil {
		t.Fatalf("expected error for split factor 0")
	}
}

type fakeRepo struct {
	actions   []CorporateAction
	dividends []DividendIncome
	openQty   float64
	err       error
	splitOps  int
}

func (f *fakeRepo) RecordCorporateAction(_ context.Context, action CorporateAction) error {
	if f.err != nil {
		return f.err
	}
	f.actions = append(f.actions, action)
	return nil
}

func (f *fakeRepo) ApplySplitToLots(context.Context, string, float64) error {
	if f.err != nil {
		return f.err
	}
	f.splitOps++
	return nil
}

func (f *fakeRepo) OpenQuantity(context.Context, string) (float64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.openQty, nil
}

func (f *fakeRepo) RecordDividendIncome(_ context.Context, dividend DividendIncome) error {
	if f.err != nil {
		return f.err
	}
	f.dividends = append(f.dividends, dividend)
	return nil
}

func TestServiceProcessCandles(t *testing.T) {
	t.Parallel()

	repo := &fakeRepo{openQty: 12}
	svc := NewService(repo)
	candles := []data.Candle{
		{Ticker: "VWCE.DE", Time: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), SplitFactor: 2},
		{Ticker: "VWCE.DE", Time: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), CashDividend: 1.5},
	}

	summary, err := svc.ProcessCandles(context.Background(), "VWCE.DE", 10, 100, candles)
	if err != nil {
		t.Fatalf("ProcessCandles() error = %v", err)
	}
	if summary.SplitEvents != 1 {
		t.Fatalf("SplitEvents = %d, want 1", summary.SplitEvents)
	}
	if summary.DividendEvents != 1 {
		t.Fatalf("DividendEvents = %d, want 1", summary.DividendEvents)
	}
	if summary.TotalDividendIncome != 18 {
		t.Fatalf("TotalDividendIncome = %f, want 18", summary.TotalDividendIncome)
	}
	if summary.UpdatedTotalQuantity != 20 || summary.UpdatedAverageCost != 50 {
		t.Fatalf("split adjustments unexpected: qty=%f cost=%f", summary.UpdatedTotalQuantity, summary.UpdatedAverageCost)
	}
	if len(repo.actions) != 2 {
		t.Fatalf("actions len = %d, want 2", len(repo.actions))
	}
	if repo.splitOps != 1 {
		t.Fatalf("splitOps = %d, want 1", repo.splitOps)
	}
	if len(repo.dividends) != 1 {
		t.Fatalf("dividends len = %d, want 1", len(repo.dividends))
	}
}

func TestServiceProcessCandlesErrors(t *testing.T) {
	t.Parallel()

	repo := &fakeRepo{err: errors.New("boom")}
	svc := NewService(repo)
	_, err := svc.ProcessCandles(context.Background(), "VWCE.DE", 10, 100, []data.Candle{{Time: time.Now(), SplitFactor: 2}})
	if err == nil {
		t.Fatalf("expected processing error")
	}
}

func TestPostgresRepositoryMethods(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)

	mock.ExpectExec("INSERT INTO corporate_actions").
		WithArgs("VWCE.DE", string(ActionTypeSplit), now, nil, 2.0, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.RecordCorporateAction(ctx, CorporateAction{Symbol: "VWCE.DE", Type: ActionTypeSplit, ExDate: now, SplitFactor: 2}); err != nil {
		t.Fatalf("RecordCorporateAction() split error = %v", err)
	}

	mock.ExpectExec("UPDATE tax_lot_records").WithArgs(2.0, "VWCE.DE").WillReturnResult(sqlmock.NewResult(0, 2))
	if err := repo.ApplySplitToLots(ctx, "VWCE.DE", 2); err != nil {
		t.Fatalf("ApplySplitToLots() error = %v", err)
	}
	if err := repo.ApplySplitToLots(ctx, "VWCE.DE", 0); err == nil {
		t.Fatalf("ApplySplitToLots() expected factor validation error")
	}

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT COALESCE(SUM(quantity_remaining), 0)
		FROM tax_lot_records
		WHERE symbol = $1
	`)).WithArgs("VWCE.DE").WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(12.5))
	qty, err := repo.OpenQuantity(ctx, "VWCE.DE")
	if err != nil {
		t.Fatalf("OpenQuantity() error = %v", err)
	}
	if qty != 12.5 {
		t.Fatalf("OpenQuantity() = %f, want 12.5", qty)
	}

	mock.ExpectExec("INSERT INTO dividend_income_records").
		WithArgs("VWCE.DE", now, nil, 12.5, 1.2, 15.0).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.RecordDividendIncome(ctx, DividendIncome{Symbol: "VWCE.DE", ExDate: now, Quantity: 12.5, CashDividendPerShare: 1.2, IncomeAmount: 15.0}); err != nil {
		t.Fatalf("RecordDividendIncome() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
