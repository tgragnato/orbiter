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

// File is the top-level structure written to / read from the JSON backup.
type File struct {
	Version      string   `json:"version"`
	CreatedAt    string   `json:"created_at"` // RFC3339
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
	AllocationType string  `json:"allocation_type"`
	ExecutedAt     string  `json:"executed_at"` // RFC3339
}

// RunBackup is the entry point for `orbiter backup …`.
func RunBackup(ctx context.Context, args []string, lookupEnv func(string) string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	dsn := fs.String("dsn", "", "PostgreSQL DSN")
	output := fs.String("output", "transactions.json", "output file path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *dsn == "" {
		*dsn = lookupEnv("DATABASE_URL")
	}
	if *dsn == "" {
		return errors.New("missing PostgreSQL DSN: provide --dsn or DATABASE_URL")
	}

	db, err := openDB(ctx, *dsn)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close database: %v\n", err)
		}
	}()

	store := portfolio.NewPostgresStore(db)
	txs, err := store.ListTransactions(ctx, "")
	if err != nil {
		return fmt.Errorf("list transactions: %w", err)
	}

	records := make([]Record, len(txs))
	for i := range txs {
		records[i] = Record{
			Symbol:         txs[i].Symbol,
			Type:           string(txs[i].Type),
			Quantity:       txs[i].Quantity,
			Price:          txs[i].Price,
			Fee:            txs[i].Fee,
			AllocationType: string(txs[i].AllocationType),
			ExecutedAt:     txs[i].ExecutedAt.UTC().Format(time.RFC3339),
		}
	}

	bf := File{
		Version:      backupVersion,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		Transactions: records,
	}

	f, err := os.Create(*output)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close output file: %v\n", err)
		}
	}()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(bf); err != nil {
		return fmt.Errorf("encode backup: %w", err)
	}

	fmt.Printf("backup: wrote %d transactions to %s\n", len(records), *output)
	return nil
}

func openDB(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		if err := db.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close database: %v\n", err)
		}
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return db, nil
}
