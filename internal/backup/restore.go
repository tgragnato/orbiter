package backup

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/tgragnato/orbiter/internal/portfolio"
)

// RunRestore is the entry point for `orbiter restore …`.
// It inserts every transaction from the JSON file via AddTransaction, which
// recalculates holdings after each insert. The operation is additive — existing
// transactions are not removed. To restore into an empty database, truncate the
// transactions and holdings tables first.
func RunRestore(ctx context.Context, args []string, lookupEnv func(string) string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	dsn := fs.String("dsn", "", "PostgreSQL DSN")
	input := fs.String("input", "transactions.json", "input file path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *dsn == "" {
		*dsn = lookupEnv("DATABASE_URL")
	}
	if *dsn == "" {
		return errors.New("missing PostgreSQL DSN: provide --dsn or DATABASE_URL")
	}

	f, err := os.Open(*input)
	if err != nil {
		return fmt.Errorf("open input file: %w", err)
	}
	defer f.Close()

	var bf File
	if err := json.NewDecoder(f).Decode(&bf); err != nil {
		return fmt.Errorf("decode backup file: %w", err)
	}
	if bf.Version != backupVersion {
		return fmt.Errorf("unsupported backup version %q (expected %q)", bf.Version, backupVersion)
	}

	db, err := openDB(ctx, *dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	store := portfolio.NewPostgresStore(db)

	inserted, failed := 0, 0
	for _, r := range bf.Transactions {
		executedAt, err := time.Parse(time.RFC3339, r.ExecutedAt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "restore: skip record %s %s — bad date %q: %v\n",
				r.Symbol, r.Type, r.ExecutedAt, err)
			failed++
			continue
		}

		tx := portfolio.Transaction{
			Symbol:         r.Symbol,
			Type:           portfolio.TransactionType(r.Type),
			Quantity:       r.Quantity,
			Price:          r.Price,
			Fee:            r.Fee,
			AllocationType: portfolio.AllocationType(r.AllocationType),
			ExecutedAt:     executedAt.UTC(),
		}
		if err := store.AddTransaction(ctx, tx); err != nil {
			fmt.Fprintf(os.Stderr, "restore: failed to insert %s %s %s: %v\n",
				r.Symbol, r.Type, r.ExecutedAt, err)
			failed++
			continue
		}
		inserted++
	}

	fmt.Printf("restore: inserted %d transactions", inserted)
	if failed > 0 {
		fmt.Printf(", %d failed (see stderr)", failed)
	}
	fmt.Println()
	return nil
}
