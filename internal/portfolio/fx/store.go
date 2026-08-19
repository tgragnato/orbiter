package fx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Store persists and retrieves exchange rates from durable storage.
type Store interface {
	// UpsertRates inserts or updates a batch of rates.
	// The unique key is (base_currency, quote_currency, rate_date).
	UpsertRates(ctx context.Context, rates []Rate) error
	// GetRate returns the rate for a specific pair and exact date.
	// Returns (zero, false, nil) when no row exists for that date.
	GetRate(ctx context.Context, base, quote string, date time.Time) (float64, bool, error)
	// GetRateOnOrBefore returns the most recent rate whose date is ≤ date.
	// Used to forward-fill over weekends and public holidays.
	// Returns (zero, false, nil) when no row exists at all for the pair.
	GetRateOnOrBefore(ctx context.Context, base, quote string, date time.Time) (float64, bool, error)
}

// PostgresStore implements Store backed by the fx_rates table.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore creates an FX rate store backed by PostgreSQL.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// UpsertRates inserts or updates each rate row, keyed on (base, quote, date).
func (s *PostgresStore) UpsertRates(ctx context.Context, rates []Rate) error {
	for _, fxRate := range rates {
		if fxRate.Rate <= 0 {
			continue
		}

		day := fxRate.Date.UTC().Truncate(hoursPerDay * time.Hour)

		_, err := s.db.ExecContext(ctx, `
			INSERT INTO fx_rates (base_currency, quote_currency, rate_date, rate)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (base_currency, quote_currency, rate_date)
			DO UPDATE SET rate = EXCLUDED.rate
		`, fxRate.BaseCurrency, fxRate.QuoteCurrency, day, fxRate.Rate)
		if err != nil {
			return fmt.Errorf("fx: upsert rate %s/%s %s: %w",
				fxRate.BaseCurrency, fxRate.QuoteCurrency, day.Format("2006-01-02"), err)
		}
	}

	return nil
}

// GetRate returns the exact-date rate for a currency pair.
// Returns (zero, false, nil) when not found.
//
func (s *PostgresStore) GetRate(ctx context.Context, base, quote string, date time.Time) (float64, bool, error) {
	day := date.UTC().Truncate(hoursPerDay * time.Hour)

	var rate float64

	err := s.db.QueryRowContext(ctx, `
		SELECT rate FROM fx_rates
		WHERE base_currency = $1 AND quote_currency = $2 AND rate_date = $3
	`, base, quote, day).Scan(&rate)

	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}

	if err != nil {
		return 0, false, fmt.Errorf("fx: get rate %s/%s %s: %w",
			base, quote, day.Format("2006-01-02"), err)
	}

	return rate, true, nil
}

// GetRateOnOrBefore returns the most recent rate whose rate_date ≤ date.
// Returns (zero, false, nil) when no row exists for the pair at all.
//
func (s *PostgresStore) GetRateOnOrBefore(
	ctx context.Context, base, quote string, date time.Time,
) (float64, bool, error) {
	day := date.UTC().Truncate(hoursPerDay * time.Hour)

	var rate float64

	err := s.db.QueryRowContext(ctx, `
		SELECT rate FROM fx_rates
		WHERE base_currency = $1 AND quote_currency = $2 AND rate_date <= $3
		ORDER BY rate_date DESC
		LIMIT 1
	`, base, quote, day).Scan(&rate)

	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}

	if err != nil {
		return 0, false, fmt.Errorf("fx: get rate on-or-before %s/%s %s: %w",
			base, quote, day.Format("2006-01-02"), err)
	}

	return rate, true, nil
}
