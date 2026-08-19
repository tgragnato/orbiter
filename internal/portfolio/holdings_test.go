//nolint:testpackage // accesses unexported parseAllocationType
package portfolio

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

const (
	colAllocationType  = "allocation_type"
	colCreatedAt       = "created_at"
	colFactor          = "factor"
	colAllocType       = "alloc_type"
	colExDate          = "ex_date"
	colCurrency        = "currency"
	colPrice           = "price"
	colExecutedAt      = "executed_at"
	colSplitDate       = "split_date"
	colQuantity        = "quantity"
	colTransactionType = "transaction_type"
	colFee             = "fee"
	colSymbol     = "symbol"
	colTAAEnabled = "taa_enabled"
)

func TestBuildSummary(t *testing.T) {
	t.Parallel()

	holdings := []Holding{
		{
			ID: 1, Symbol: "VWCE.DE", Quantity: 2, MarketPrice: 100,
			AllocationType: AllocationCore, PMC: 0, TAAEnabled: false, Currency: "",
		},
		{
			ID: 2, Symbol: "ZPRV.DE", Quantity: 1, MarketPrice: 50,
			AllocationType: AllocationSatellite, PMC: 0, TAAEnabled: false, Currency: "",
		},
	}

	summary := BuildSummary(holdings)

	if summary.TotalNAV != 250 {
		t.Fatalf("TotalNAV = %f, want 250", summary.TotalNAV)
	}

	if summary.CoreNAV != 200 {
		t.Fatalf("CoreNAV = %f, want 200", summary.CoreNAV)
	}

	if summary.SatelliteNAV != 50 {
		t.Fatalf("SatelliteNAV = %f, want 50", summary.SatelliteNAV)
	}

	if summary.CorePercent != 80 {
		t.Fatalf("CorePercent = %f, want 80", summary.CorePercent)
	}

	if summary.SatellitePercent != 20 {
		t.Fatalf("SatellitePercent = %f, want 20", summary.SatellitePercent)
	}
}

func TestPostgresStoreListHoldingsAndToggle(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer sqlDB.Close()

	store := NewPostgresStore(sqlDB)

	rows := sqlmock.NewRows([]string{
		"id", colSymbol, colQuantity, "market_price", "pmc", colAllocationType, colTAAEnabled, colCurrency,
	}).
		AddRow(1, "VWCE.DE", 2.0, 100.0, 95.0, "CORE", true, "EUR").
		AddRow(2, "ZPRV.DE", 1.0, 50.0, 0.0, "SATELLITE", false, "USD")

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, symbol, quantity, market_price, pmc, allocation_type, taa_enabled, currency
		FROM holdings
		ORDER BY symbol, id
	`)).WillReturnRows(rows)

	holdings, err := store.ListHoldings(context.Background())
	if err != nil {
		t.Fatalf("ListHoldings() error = %v", err)
	}

	if len(holdings) != 2 {
		t.Fatalf("holdings len = %d, want 2", len(holdings))
	}

	if holdings[0].AllocationType != AllocationCore {
		t.Fatalf("allocation[0] = %q, want CORE", holdings[0].AllocationType)
	}

	mock.ExpectExec("UPDATE holdings").WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.ToggleAllocation(context.Background(), 2)
	if err != nil {
		t.Fatalf("ToggleAllocation() error = %v", err)
	}

	err = mock.ExpectationsWereMet()
	if err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPostgresStoreToggleNotFound(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer sqlDB.Close()

	store := NewPostgresStore(sqlDB)

	mock.ExpectExec("UPDATE holdings").WithArgs(int64(99)).WillReturnResult(sqlmock.NewResult(0, 0))

	err = store.ToggleAllocation(context.Background(), 99)
	if err == nil {
		t.Fatalf("ToggleAllocation() error = nil, want non-nil")
	}
}

func TestParseAllocationType(t *testing.T) {
	t.Parallel()

	if got := parseAllocationType("core"); got != AllocationCore {
		t.Fatalf("parseAllocationType(core) = %q, want CORE", got)
	}

	if got := parseAllocationType("unknown"); got != AllocationSatellite {
		t.Fatalf("parseAllocationType(unknown) = %q, want SATELLITE", got)
	}
}

//nolint:funlen // test sets up many mock expectations to cover full AddTransaction flow
func TestPostgresStoreAddTransactionRecalculatesHolding(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer sqlDB.Close()

	store := NewPostgresStore(sqlDB)
	executedAt := txBase

	// 1. INSERT INTO transactions (now includes currency column)
	mock.ExpectExec("INSERT INTO transactions").
		WithArgs(vwceMI, "BUY", 10.0, 100.0, 2.0, "SATELLITE", "", executedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// 2. ListTransactions(ctx, "VWCE.MI") -- called by recalculateSymbol
	txCols := []string{
		"id", colSymbol, colTransactionType, colQuantity, colPrice,
		colFee, colAllocationType, colCurrency, colExecutedAt, colCreatedAt,
	}
	txRows := sqlmock.NewRows(txCols).
		AddRow(1, vwceMI, "BUY", 10.0, 100.0, 2.0, "SATELLITE", "", executedAt, executedAt)
	mock.ExpectQuery("FROM transactions").WithArgs(vwceMI).WillReturnRows(txRows)

	// 3. listSplitsForSymbol -- no splits recorded for this symbol yet
	mock.ExpectQuery("FROM stock_splits").WithArgs(vwceMI).
		WillReturnRows(sqlmock.NewRows([]string{colSplitDate, colFactor}))

	// 4. BEGIN
	mock.ExpectBegin()

	// 5. Read existing market_price, taa_enabled, allocation_type, currency -- defaults for new holding
	mock.ExpectQuery("SELECT COALESCE").WithArgs(vwceMI).
		WillReturnRows(
			sqlmock.NewRows([]string{colPrice, colTAAEnabled, colAllocType, colCurrency}).
				AddRow(98.5, true, "SATELLITE", "EUR"),
		)

	// 5. DELETE existing rows
	mock.ExpectExec("DELETE FROM holdings").WithArgs(vwceMI).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// 6. INSERT new holding -- PMC = (10*100 + 2) / 10 = 100.2; preserve market_price 98.5
	mock.ExpectExec("INSERT INTO holdings").
		WithArgs(vwceMI, 10.0, 98.5, 100.2, "SATELLITE", true, "EUR", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// 7. COMMIT
	mock.ExpectCommit()

	// 8. cleanupStaleDividendRecords -- no existing dividend records for this symbol
	mock.ExpectQuery("SELECT ex_date FROM dividend_income_records").WithArgs(vwceMI).
		WillReturnRows(sqlmock.NewRows([]string{colExDate}))

	txn := Transaction{
		ID:             0,
		Symbol:         vwceMI,
		Type:           TransactionBuy,
		Quantity:       10,
		Price:          100,
		Fee:            2,
		AllocationType: AllocationSatellite,
		ExecutedAt:     executedAt,
		Currency:       "",
		CreatedAt:      time.Time{},
	}

	err = store.AddTransaction(context.Background(), txn)
	if err != nil {
		t.Fatalf("AddTransaction() error = %v", err)
	}

	err = mock.ExpectationsWereMet()
	if err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

//nolint:dupl,funlen // similar test structure for different scenarios (closes position vs preserves PMC)
func TestPostgresStoreAddTransactionClosesPosition(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer sqlDB.Close()

	store := NewPostgresStore(sqlDB)
	executedAt := txBase

	// SELL 10 on a position of 10 -> net qty = 0 -> no INSERT after DELETE.
	mock.ExpectExec("INSERT INTO transactions").
		WithArgs(vwceMI, "SELL", 10.0, 110.0, 0.0, "SATELLITE", "", executedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// ListTransactions returns one BUY + one SELL -> net qty 0
	txCols := []string{
		"id", colSymbol, colTransactionType, colQuantity, colPrice,
		colFee, colAllocationType, colCurrency, colExecutedAt, colCreatedAt,
	}
	txRows := sqlmock.NewRows(txCols).
		AddRow(
			1, vwceMI, "BUY", 10.0, 100.0, 0.0, "SATELLITE", "",
			executedAt.Add(-time.Hour), executedAt.Add(-time.Hour),
		).
		AddRow(2, vwceMI, "SELL", 10.0, 110.0, 0.0, "SATELLITE", "", executedAt, executedAt)
	mock.ExpectQuery("FROM transactions").WithArgs(vwceMI).WillReturnRows(txRows)

	// listSplitsForSymbol -- no splits recorded for this symbol yet
	mock.ExpectQuery("FROM stock_splits").WithArgs(vwceMI).
		WillReturnRows(sqlmock.NewRows([]string{colSplitDate, colFactor}))

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE").WithArgs(vwceMI).
		WillReturnRows(
			sqlmock.NewRows([]string{colPrice, colTAAEnabled, colAllocType, colCurrency}).
				AddRow(0, true, "SATELLITE", "EUR"),
		)
	mock.ExpectExec("DELETE FROM holdings").WithArgs(vwceMI).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// INSERT with qty=0 to preserve taa_enabled and allocation_type for history.
	mock.ExpectExec("INSERT INTO holdings").
		WithArgs(vwceMI, 0.0, 0.0, 0.0, "SATELLITE", true, "EUR", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// cleanupStaleDividendRecords -- no existing dividend records
	mock.ExpectQuery("SELECT ex_date FROM dividend_income_records").WithArgs(vwceMI).
		WillReturnRows(sqlmock.NewRows([]string{colExDate}))

	txn := Transaction{
		ID:             0,
		Symbol:         vwceMI,
		Type:           TransactionSell,
		Quantity:       10,
		Price:          110,
		Fee:            0,
		AllocationType: AllocationSatellite,
		ExecutedAt:     executedAt,
		Currency:       "",
		CreatedAt:      time.Time{},
	}

	err = store.AddTransaction(context.Background(), txn)
	if err != nil {
		t.Fatalf("AddTransaction() error = %v", err)
	}

	err = mock.ExpectationsWereMet()
	if err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

//nolint:dupl,funlen // similar test structure for different scenarios (preserves PMC vs closes position)
func TestPostgresStoreAddTransactionPartialSellPreservesPMC(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer sqlDB.Close()

	store := NewPostgresStore(sqlDB)
	executedAt := txBase

	// SELL 4 of a 10-unit position bought at 100 + 2 fee -> qty 6, PMC must stay 100.2.
	mock.ExpectExec("INSERT INTO transactions").
		WithArgs(vwceMI, "SELL", 4.0, 120.0, 0.0, "SATELLITE", "", executedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	txCols := []string{
		"id", colSymbol, colTransactionType, colQuantity, colPrice,
		colFee, colAllocationType, colCurrency, colExecutedAt, colCreatedAt,
	}
	txRows := sqlmock.NewRows(txCols).
		AddRow(
			1, vwceMI, "BUY", 10.0, 100.0, 2.0, "SATELLITE", "",
			executedAt.Add(-time.Hour), executedAt.Add(-time.Hour),
		).
		AddRow(2, vwceMI, "SELL", 4.0, 120.0, 0.0, "SATELLITE", "", executedAt, executedAt)
	mock.ExpectQuery("FROM transactions").WithArgs(vwceMI).WillReturnRows(txRows)

	// listSplitsForSymbol -- no splits recorded for this symbol yet
	mock.ExpectQuery("FROM stock_splits").WithArgs(vwceMI).
		WillReturnRows(sqlmock.NewRows([]string{colSplitDate, colFactor}))

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE").WithArgs(vwceMI).
		WillReturnRows(
			sqlmock.NewRows([]string{colPrice, colTAAEnabled, colAllocType, colCurrency}).
				AddRow(98.5, true, "SATELLITE", "EUR"),
		)
	mock.ExpectExec("DELETE FROM holdings").WithArgs(vwceMI).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// PMC = (10*100 + 2) / 10 = 100.2 from the BUY; partial SELL must NOT alter PMC.
	mock.ExpectExec("INSERT INTO holdings").
		WithArgs(vwceMI, 6.0, 98.5, 100.2, "SATELLITE", true, "EUR", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// cleanupStaleDividendRecords -- no existing dividend records
	mock.ExpectQuery("SELECT ex_date FROM dividend_income_records").WithArgs(vwceMI).
		WillReturnRows(sqlmock.NewRows([]string{colExDate}))

	txn := Transaction{
		ID:             0,
		Symbol:         vwceMI,
		Type:           TransactionSell,
		Quantity:       4,
		Price:          120,
		Fee:            0,
		AllocationType: AllocationSatellite,
		ExecutedAt:     executedAt,
		Currency:       "",
		CreatedAt:      time.Time{},
	}

	err = store.AddTransaction(context.Background(), txn)
	if err != nil {
		t.Fatalf("AddTransaction() error = %v", err)
	}

	err = mock.ExpectationsWereMet()
	if err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPostgresStoreToggleTAAEnabled(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer sqlDB.Close()

	store := NewPostgresStore(sqlDB)

	mock.ExpectExec("UPDATE holdings").WithArgs(vwceMI).WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.ToggleTAAEnabled(context.Background(), vwceMI)
	if err != nil {
		t.Fatalf("ToggleTAAEnabled() error = %v", err)
	}

	err = mock.ExpectationsWereMet()
	if err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPostgresStoreToggleTAAEnabledNotFound(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer sqlDB.Close()

	store := NewPostgresStore(sqlDB)

	mock.ExpectExec("UPDATE holdings").WithArgs("UNKNOWN").WillReturnResult(sqlmock.NewResult(0, 0))

	err = store.ToggleTAAEnabled(context.Background(), "UNKNOWN")
	if err == nil {
		t.Fatalf("ToggleTAAEnabled() error = nil, want non-nil for unknown symbol")
	}
}

func TestPostgresStoreListHoldingsQueryError(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer sqlDB.Close()

	store := NewPostgresStore(sqlDB)

	mock.ExpectQuery("FROM holdings").WillReturnError(errors.New("boom"))

	_, err = store.ListHoldings(context.Background())
	if err == nil {
		t.Fatalf("ListHoldings() error = nil, want non-nil")
	}
}
