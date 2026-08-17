package analytics

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPostgresRepositoryRecordSnapshot(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresRepository(db)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectExec("INSERT INTO nav_snapshots").
		WithArgs("MAIN", at, 1000.0).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.RecordSnapshot(context.Background(), "MAIN", at, 1000.0); err != nil {
		t.Fatalf("RecordSnapshot() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestPostgresRepositoryRecordSnapshotError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresRepository(db)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectExec("INSERT INTO nav_snapshots").
		WillReturnError(errors.New("db error"))

	if err := repo.RecordSnapshot(context.Background(), "MAIN", at, 1000.0); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresRepositoryRecordCashFlowWithSnapshot(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresRepository(db)
	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO cash_flows").
		WithArgs("MAIN", string(CashFlowDeposit), 500.0, "EUR", at).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
	mock.ExpectExec("INSERT INTO nav_snapshots").
		WithArgs("MAIN", at, 900.0, int64(7)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := repo.RecordCashFlowWithSnapshot(context.Background(), "MAIN", CashFlowDeposit, 500.0, "EUR", at, 900.0); err != nil {
		t.Fatalf("RecordCashFlowWithSnapshot() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestPostgresRepositoryRecordCashFlowWithSnapshotBeginError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresRepository(db)
	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin().WillReturnError(errors.New("begin failed"))

	if err := repo.RecordCashFlowWithSnapshot(context.Background(), "MAIN", CashFlowDeposit, 100.0, "EUR", at, 500.0); err == nil {
		t.Fatal("expected begin error")
	}
}

func TestPostgresRepositoryRecordCashFlowWithSnapshotInsertFlowError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresRepository(db)
	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO cash_flows").WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	if err := repo.RecordCashFlowWithSnapshot(context.Background(), "MAIN", CashFlowDeposit, 100.0, "EUR", at, 500.0); err == nil {
		t.Fatal("expected insert error")
	}
}

func TestPostgresRepositoryRecordCashFlowWithSnapshotInsertSnapshotError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresRepository(db)
	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO cash_flows").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(3)))
	mock.ExpectExec("INSERT INTO nav_snapshots").WillReturnError(errors.New("snapshot insert failed"))
	mock.ExpectRollback()

	if err := repo.RecordCashFlowWithSnapshot(context.Background(), "MAIN", CashFlowDeposit, 100.0, "EUR", at, 500.0); err == nil {
		t.Fatal("expected snapshot insert error")
	}
}

func TestPostgresRepositoryListSnapshots(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresRepository(db)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	at1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	at2 := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{"id", "portfolio_id", "snapshot_at", "nav", "related_cash_flow_id"}).
		AddRow(int64(1), "MAIN", at1, 1000.0, nil).
		AddRow(int64(2), "MAIN", at2, 1050.0, int64(5))

	mock.ExpectQuery(regexp.QuoteMeta("FROM nav_snapshots")).
		WithArgs("MAIN", from, to).
		WillReturnRows(rows)

	snaps, err := repo.ListSnapshots(context.Background(), "MAIN", TimeRange{From: from, To: to})
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("got %d snapshots, want 2", len(snaps))
	}
	if snaps[0].RelatedFlowID != nil {
		t.Fatal("expected nil RelatedFlowID for first snapshot")
	}
	if snaps[1].RelatedFlowID == nil || *snaps[1].RelatedFlowID != 5 {
		t.Fatalf("expected RelatedFlowID=5, got %v", snaps[1].RelatedFlowID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestPostgresRepositoryListSnapshotsError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresRepository(db)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("FROM nav_snapshots").WillReturnError(errors.New("query failed"))

	if _, err := repo.ListSnapshots(context.Background(), "MAIN", TimeRange{From: from, To: to}); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresRepositorySignedCashFlowBetween(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresRepository(db)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("FROM cash_flows").
		WithArgs("MAIN", from, to).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(250.0))

	total, err := repo.SignedCashFlowBetween(context.Background(), "MAIN", from, to)
	if err != nil {
		t.Fatalf("SignedCashFlowBetween() error = %v", err)
	}
	if total != 250.0 {
		t.Fatalf("got %v, want 250", total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestPostgresRepositorySignedCashFlowBetweenZeroResult(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresRepository(db)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	// COALESCE(SUM(...), 0) always returns 0 when there are no cash flows — never NULL.
	mock.ExpectQuery("FROM cash_flows").
		WithArgs("MAIN", from, to).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0.0))

	total, err := repo.SignedCashFlowBetween(context.Background(), "MAIN", from, to)
	if err != nil {
		t.Fatalf("SignedCashFlowBetween() error = %v", err)
	}
	if total != 0 {
		t.Fatalf("expected 0 for null result, got %v", total)
	}
}

func TestPostgresRepositorySignedCashFlowBetweenError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresRepository(db)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("FROM cash_flows").WillReturnError(errors.New("query failed"))

	if _, err := repo.SignedCashFlowBetween(context.Background(), "MAIN", from, to); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresRepositoryClearSnapshots(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	repo := NewPostgresRepository(db)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM nav_snapshots WHERE portfolio_id = $1")).
		WithArgs("MAIN").
		WillReturnResult(sqlmock.NewResult(0, 5))

	if err := repo.ClearSnapshots(context.Background(), "MAIN"); err != nil {
		t.Fatalf("ClearSnapshots() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}
