package accounting

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/tgragnato/orbiter/internal/configuration"
)

func TestPostgresLedgerRepositoryListOpenLotsAndAdd(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresLedgerRepository(db)
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{"id", "symbol", "acquired_at", "quantity_initial", "quantity_remaining", "unit_cost"}).
		AddRow(1, "VWCE.DE", now, 10.0, 7.0, 100.0)
	mock.ExpectQuery("FROM tax_lot_records").WithArgs("VWCE.DE").WillReturnRows(rows)

	lots, err := repo.ListOpenLots(context.Background(), "VWCE.DE")
	if err != nil {
		t.Fatalf("ListOpenLots() error = %v", err)
	}
	if len(lots) != 1 {
		t.Fatalf("len(lots) = %d, want 1", len(lots))
	}

	mock.ExpectQuery("INSERT INTO tax_lot_records").
		WithArgs("VWCE.DE", now, 10.0, 10.0, 95.0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))

	id, err := repo.AddTaxLot(context.Background(), TaxLot{Symbol: "VWCE.DE", AcquiredAt: now, QuantityInitial: 10, UnitCost: 95})
	if err != nil {
		t.Fatalf("AddTaxLot() error = %v", err)
	}
	if id != 42 {
		t.Fatalf("id = %d, want 42", id)
	}
}

func TestPostgresLedgerRepositoryPersistRealization(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresLedgerRepository(db)
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	sell := SellTransaction{Symbol: "VWCE.DE", SoldAt: now, Quantity: 2, UnitPrice: 120}
	result := RealizedPnLResult{
		RealizedPnL: 30,
		Consumptions: []LotConsumption{
			{TaxLotID: 1, Quantity: 2, UnitCost: 105, CostAmount: 210},
		},
	}
	updated := []TaxLot{{ID: 1, QuantityRemaining: 8, UnitCost: 105}}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`
			UPDATE tax_lot_records
			SET quantity_remaining = $1, unit_cost = $2, updated_at = NOW()
			WHERE id = $3
		`)).WithArgs(8.0, 105.0, int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO realized_pnl").
		WithArgs("VWCE.DE", now, 2.0, 120.0, string(configuration.CostBasisPMC), 30.0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(99))
	mock.ExpectExec("INSERT INTO realized_pnl_lot_links").
		WithArgs(int64(99), int64(1), 2.0, 105.0, 210.0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.PersistRealization(context.Background(), sell, configuration.CostBasisPMC, result, updated); err != nil {
		t.Fatalf("PersistRealization() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPostgresLedgerRepositoryAddTaxLotValidation(t *testing.T) {
	t.Parallel()

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresLedgerRepository(db)
	_, err = repo.AddTaxLot(context.Background(), TaxLot{Symbol: "VWCE.DE", AcquiredAt: time.Now(), QuantityInitial: 0, UnitCost: 10})
	if err == nil {
		t.Fatalf("AddTaxLot() expected validation error")
	}
}

func TestPostgresLedgerRepositoryTotalRealizedPnL(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresLedgerRepository(db)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(SUM(realized_pnl_amount), 0) FROM realized_pnl`)).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(123.45))

	total, err := repo.TotalRealizedPnL(context.Background())
	if err != nil {
		t.Fatalf("TotalRealizedPnL() error = %v", err)
	}
	if total != 123.45 {
		t.Fatalf("total = %f, want 123.45", total)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
