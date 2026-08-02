package accounting

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/tgragnato/orbiter/internal/configuration"
)

func TestCalculatorsDeterministicVectors(t *testing.T) {
	t.Parallel()

	lots := []TaxLot{
		{ID: 1, Symbol: "VWCE.DE", AcquiredAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), QuantityInitial: 10, QuantityRemaining: 10, UnitCost: 100},
		{ID: 2, Symbol: "VWCE.DE", AcquiredAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), QuantityInitial: 5, QuantityRemaining: 5, UnitCost: 120},
	}
	sell := SellTransaction{Symbol: "VWCE.DE", SoldAt: time.Now().UTC(), Quantity: 6, UnitPrice: 130}

	pmcResult, pmcUpdated, err := (PMCCalculator{}).Calculate(lots, sell)
	if err != nil {
		t.Fatalf("PMC Calculate() error = %v", err)
	}
	assertNear(t, pmcResult.TotalCost, 640)
	assertNear(t, pmcResult.RealizedPnL, 140)
	assertNear(t, pmcUpdated[0].QuantityRemaining, 6)
	assertNear(t, pmcUpdated[1].QuantityRemaining, 3)

	fifoResult, fifoUpdated, err := (FIFOCalculator{}).Calculate(lots, sell)
	if err != nil {
		t.Fatalf("FIFO Calculate() error = %v", err)
	}
	assertNear(t, fifoResult.TotalCost, 600)
	assertNear(t, fifoResult.RealizedPnL, 180)
	assertNear(t, fifoUpdated[0].QuantityRemaining, 4)
	assertNear(t, fifoUpdated[1].QuantityRemaining, 5)

	lifoResult, lifoUpdated, err := (LIFOCalculator{}).Calculate(lots, sell)
	if err != nil {
		t.Fatalf("LIFO Calculate() error = %v", err)
	}
	assertNear(t, lifoResult.TotalCost, 700)
	assertNear(t, lifoResult.RealizedPnL, 80)
	byID := map[int64]float64{}
	for _, lot := range lifoUpdated {
		byID[lot.ID] = lot.QuantityRemaining
	}
	assertNear(t, byID[1], 9)
	assertNear(t, byID[2], 0)
}

func TestCalculatorsInsufficientQuantity(t *testing.T) {
	t.Parallel()

	lots := []TaxLot{{ID: 1, Symbol: "VWCE.DE", QuantityInitial: 1, QuantityRemaining: 1, UnitCost: 100}}
	sell := SellTransaction{Symbol: "VWCE.DE", SoldAt: time.Now().UTC(), Quantity: 2, UnitPrice: 120}

	if _, _, err := (PMCCalculator{}).Calculate(lots, sell); err == nil {
		t.Fatalf("PMC expected insufficient quantity error")
	}
	if _, _, err := (FIFOCalculator{}).Calculate(lots, sell); err == nil {
		t.Fatalf("FIFO expected insufficient quantity error")
	}
	if _, _, err := (LIFOCalculator{}).Calculate(lots, sell); err == nil {
		t.Fatalf("LIFO expected insufficient quantity error")
	}
}

func TestPostgresMethodResolver(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	resolver := NewPostgresMethodResolver(db)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value_json FROM app_settings WHERE key = $1`)).
		WithArgs(configuration.KeyCostBasisMethod).
		WillReturnRows(sqlmock.NewRows([]string{"value_json"}).AddRow([]byte(`{"method":"FIFO"}`)))

	method, err := resolver.ResolveCostBasisMethod(context.Background())
	if err != nil {
		t.Fatalf("ResolveCostBasisMethod() error = %v", err)
	}
	if method != configuration.CostBasisFIFO {
		t.Fatalf("method = %q, want FIFO", method)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value_json FROM app_settings WHERE key = $1`)).
		WithArgs(configuration.KeyCostBasisMethod).
		WillReturnError(sql.ErrNoRows)

	method, err = resolver.ResolveCostBasisMethod(context.Background())
	if err != nil {
		t.Fatalf("ResolveCostBasisMethod() fallback error = %v", err)
	}
	if method != configuration.CostBasisPMC {
		t.Fatalf("method = %q, want PMC fallback", method)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value_json FROM app_settings WHERE key = $1`)).
		WithArgs(configuration.KeyCostBasisMethod).
		WillReturnRows(sqlmock.NewRows([]string{"value_json"}).AddRow([]byte(`{"method":"WAT"}`)))

	method, err = resolver.ResolveCostBasisMethod(context.Background())
	if err != nil {
		t.Fatalf("ResolveCostBasisMethod() invalid method fallback error = %v", err)
	}
	if method != configuration.CostBasisPMC {
		t.Fatalf("method = %q, want PMC fallback on invalid method", method)
	}
}

type fakeMethodResolver struct {
	method configuration.CostBasisMethod
}

func (f fakeMethodResolver) ResolveCostBasisMethod(context.Context) (configuration.CostBasisMethod, error) {
	return f.method, nil
}

type fakeLedgerRepo struct {
	lots        []TaxLot
	persisted   bool
	lastMethod  configuration.CostBasisMethod
	lastResult  RealizedPnLResult
	lastUpdated []TaxLot
}

func (f *fakeLedgerRepo) ListOpenLots(context.Context, string) ([]TaxLot, error) {
	out := make([]TaxLot, len(f.lots))
	copy(out, f.lots)
	return out, nil
}

func (f *fakeLedgerRepo) AddTaxLot(context.Context, TaxLot) (int64, error) {
	return 1, nil
}

func (f *fakeLedgerRepo) PersistRealization(_ context.Context, _ SellTransaction, method configuration.CostBasisMethod, result RealizedPnLResult, updatedLots []TaxLot) error {
	f.persisted = true
	f.lastMethod = method
	f.lastResult = result
	f.lastUpdated = updatedLots
	return nil
}

func TestServiceCalculateRealizedPnLUsesConfiguredMethod(t *testing.T) {
	t.Parallel()

	repo := &fakeLedgerRepo{lots: []TaxLot{
		{ID: 1, Symbol: "VWCE.DE", AcquiredAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), QuantityInitial: 10, QuantityRemaining: 10, UnitCost: 100},
		{ID: 2, Symbol: "VWCE.DE", AcquiredAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), QuantityInitial: 5, QuantityRemaining: 5, UnitCost: 120},
	}}
	svc := NewService(repo, fakeMethodResolver{method: configuration.CostBasisLIFO})

	result, err := svc.CalculateRealizedPnL(context.Background(), SellTransaction{Symbol: "VWCE.DE", SoldAt: time.Now().UTC(), Quantity: 6, UnitPrice: 130})
	if err != nil {
		t.Fatalf("CalculateRealizedPnL() error = %v", err)
	}
	if !repo.persisted {
		t.Fatalf("persist was not called")
	}
	if repo.lastMethod != configuration.CostBasisLIFO {
		t.Fatalf("method = %q, want LIFO", repo.lastMethod)
	}
	assertNear(t, result.RealizedPnL, 80)
}

func TestServiceUnknownMethodFallsBackToPMC(t *testing.T) {
	t.Parallel()

	repo := &fakeLedgerRepo{lots: []TaxLot{{ID: 1, Symbol: "VWCE.DE", AcquiredAt: time.Now().UTC(), QuantityInitial: 10, QuantityRemaining: 10, UnitCost: 100}}}
	svc := NewService(repo, fakeMethodResolver{method: configuration.CostBasisMethod("UNKNOWN")})

	result, err := svc.CalculateRealizedPnL(context.Background(), SellTransaction{Symbol: "VWCE.DE", SoldAt: time.Now().UTC(), Quantity: 2, UnitPrice: 130})
	if err != nil {
		t.Fatalf("CalculateRealizedPnL() error = %v", err)
	}
	if !repo.persisted {
		t.Fatalf("persist was not called")
	}
	if repo.lastMethod != configuration.CostBasisPMC {
		t.Fatalf("method = %q, want PMC fallback", repo.lastMethod)
	}
	assertNear(t, result.RealizedPnL, 60)
}

func assertNear(t *testing.T, got, want float64) {
	t.Helper()
	const eps = 1e-6
	if got > want+eps || got < want-eps {
		t.Fatalf("value = %.10f, want %.10f", got, want)
	}
}
