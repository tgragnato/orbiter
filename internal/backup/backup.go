// Package backup provides CLI-invoked backup and restore of transaction history.
package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/tgragnato/orbiter/internal/portfolio"
)

const backupVersion = "1"

// ErrMissingDSN is returned when no PostgreSQL DSN is provided.
var ErrMissingDSN = errors.New("missing PostgreSQL DSN: provide --dsn or DATABASE_URL")

// File is the top-level structure written to / read from the JSON backup.
type File struct {
	Version      string   `json:"version"`
	CreatedAt    string   `json:"createdAt"`
	Transactions []Record `json:"transactions"`
}

// Record is one transaction as stored in the backup. The DB-assigned id is
// intentionally omitted so the file is portable across database instances.
type Record struct {
	Symbol         string  `json:"symbol"`
	Type           string  `json:"type"`
	Quantity       float64 `json:"quantity"`
	Price          float64 `json:"price"`
	Fee            float64 `json:"fee"`
	AllocationType string  `json:"allocationType"`
	ExecutedAt     string  `json:"executedAt"`
}

// RunBackup is the entry point for `orbiter backup …`.
//
//nolint:cyclop,funlen // complexity is inherent to the backup workflow
func RunBackup(ctx context.Context, args []string, lookupEnv func(string) string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	dsn := fs.String("dsn", "", "PostgreSQL DSN")
	output := fs.String("output", "transactions.json", "output file path")

	parseErr := fs.Parse(args)
	if parseErr != nil {
		return fmt.Errorf("parse flags: %w", parseErr)
	}

	if *dsn == "" {
		*dsn = lookupEnv("DATABASE_URL")
	}

	if *dsn == "" {
		return ErrMissingDSN
	}

	sqlDB, err := openDB(ctx, *dsn)
	if err != nil {
		return err
	}

	defer func() {
		closeErr := sqlDB.Close()
		if closeErr != nil {
			fmt.Fprintf(os.Stderr, "close database: %v\n", closeErr)
		}
	}()

	store := portfolio.NewPostgresStore(sqlDB)

	txs, err := store.ListTransactions(ctx, "")
	if err != nil {
		return fmt.Errorf("list transactions: %w", err)
	}

	records := make([]Record, 0, len(txs))

	for idx := range txs {
		records[idx] = Record{
			Symbol:         txs[idx].Symbol,
			Type:           string(txs[idx].Type),
			Quantity:       txs[idx].Quantity,
			Price:          txs[idx].Price,
			Fee:            txs[idx].Fee,
			AllocationType: string(txs[idx].AllocationType),
			ExecutedAt:     txs[idx].ExecutedAt.UTC().Format(time.RFC3339),
		}
	}

	backupFile := File{
		Version:      backupVersion,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		Transactions: records,
	}

	outputFile, err := os.Create(*output)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}

	defer func() {
		closeErr := outputFile.Close()
		if closeErr != nil {
			fmt.Fprintf(os.Stderr, "close output file: %v\n", closeErr)
		}
	}()

	enc := json.NewEncoder(outputFile)
	enc.SetIndent("", "  ")

	encodeErr := enc.Encode(backupFile)
	if encodeErr != nil {
		return fmt.Errorf("encode backup: %w", encodeErr)
	}

	fmt.Printf("backup: wrote %d transactions to %s\n", len(records), *output)

	return nil
}

func openDB(ctx context.Context, dsn string) (*sql.DB, error) {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	pingErr := sqlDB.PingContext(ctx)
	if pingErr != nil {
		closeErr := sqlDB.Close()
		if closeErr != nil {
			fmt.Fprintf(os.Stderr, "close database: %v\n", closeErr)
		}

		return nil, fmt.Errorf("ping database: %w", pingErr)
	}

	return sqlDB, nil
}
