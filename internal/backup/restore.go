package backup

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/tgragnato/orbiter/internal/configuration"
	"github.com/tgragnato/orbiter/internal/portfolio"
)

// ErrUnsupportedVersion is returned when the backup file version is not supported.
var ErrUnsupportedVersion = fmt.Errorf("unsupported backup version (expected %q)", backupVersion)

// RunRestore is the entry point for `orbiter restore …`.
// It inserts every transaction from the JSON file via AddTransaction, which
// recalculates holdings after each insert. The operation is additive — existing
// transactions are not removed. To restore into an empty database, truncate the
// transactions and holdings tables first.
//
//nolint:cyclop,funlen // complexity is inherent to the restore workflow
func RunRestore(ctx context.Context, args []string, lookupEnv func(string) string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	dsn := fs.String("dsn", "", "PostgreSQL DSN")
	input := fs.String("input", "transactions.json", "input file path")

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

	inputFile, err := os.Open(*input)
	if err != nil {
		return fmt.Errorf("open input file: %w", err)
	}

	defer func() {
		closeErr := inputFile.Close()
		if closeErr != nil {
			fmt.Fprintf(os.Stderr, "close input file: %v\n", closeErr)
		}
	}()

	var backupFile File

	decodeErr := json.NewDecoder(inputFile).Decode(&backupFile)
	if decodeErr != nil {
		return fmt.Errorf("decode backup file: %w", decodeErr)
	}

	if backupFile.Version != backupVersion {
		return fmt.Errorf("got %q: %w", backupFile.Version, ErrUnsupportedVersion)
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

	migrateErr := configuration.RunMigrations(ctx, sqlDB)
	if migrateErr != nil {
		return fmt.Errorf("run migrations: %w", migrateErr)
	}

	store := portfolio.NewPostgresStore(sqlDB)

	inserted, failed := 0, 0

	for _, rec := range backupFile.Transactions {
		executedAt, parseTimeErr := time.Parse(time.RFC3339, rec.ExecutedAt)
		if parseTimeErr != nil {
			fmt.Fprintf(os.Stderr, "restore: skip record %s %s — bad date %q: %v\n",
				rec.Symbol, rec.Type, rec.ExecutedAt, parseTimeErr)

			failed++

			continue
		}

		transaction := portfolio.Transaction{
			ID:             0,
			Symbol:         rec.Symbol,
			Type:           portfolio.TransactionType(rec.Type),
			Quantity:       rec.Quantity,
			Price:          rec.Price,
			Fee:            rec.Fee,
			AllocationType: portfolio.AllocationType(rec.AllocationType),
			Currency:       "",
			ExecutedAt:     executedAt.UTC(),
			CreatedAt:      time.Time{},
		}

		addErr := store.AddTransaction(ctx, transaction)
		if addErr != nil {
			fmt.Fprintf(os.Stderr, "restore: failed to insert %s %s %s: %v\n",
				rec.Symbol, rec.Type, rec.ExecutedAt, addErr)

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
