package tick_test

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/tick"
)

const (
	instrumentEURUSD = "EURUSD"
	instrumentZERO   = "ZERO"
)

func TestTick_Spread(t *testing.T) {
	t.Parallel()

	bid := 1.00
	ask := 1.50
	tk := tick.New(instrumentEURUSD, time.Now(), bid, ask)

	spread := tk.Spread()
	if spread != 0.50 {
		t.Fatalf("expected %v, got %v", 0.50, spread)
	}
}

func TestTick_SpreadInPercent(t *testing.T) {
	t.Parallel()

	bid := 0.80
	ask := 1.50
	tk := tick.New(instrumentEURUSD, time.Now(), bid, ask)

	spread := tk.SpreadInPercent()
	if spread != 87.50 {
		t.Fatalf("expected %v, got %v", 87.50, spread)
	}
}

//nolint:funlen // table-driven test with many string cases
func TestTick_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		instrument string
		datetime   time.Time
		bid        float64
		ask        float64
		want       string
	}{
		{
			name:       "Tick EURUSD Standard",
			instrument: instrumentEURUSD,
			datetime:   time.Date(2023, time.October, 27, 10, 30, 0, 0, time.UTC),
			bid:        1.2558,
			ask:        1.2560,
			want:       "{Datetime=2023-10-27 10:30:00 +0000 UTC Bid=1.2558 Ask=1.256}",
		},
		{
			name:       "Tick XAUUSD TightSpread",
			instrument: "XAUUSD",
			datetime:   time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
			bid:        1950.00,
			ask:        1950.01,
			want:       "{Datetime=2024-01-01 00:00:00 +0000 UTC Bid=1950 Ask=1950.01}",
		},
		{
			name:       "Tick BTCUSD WideSpread",
			instrument: "BTCUSD",
			datetime:   time.Date(2023, time.November, 15, 14, 45, 0, 0, time.UTC),
			bid:        30000.00,
			ask:        30100.00,
			want:       "{Datetime=2023-11-15 14:45:00 +0000 UTC Bid=30000 Ask=30100}",
		},
		{
			name:       "Tick ZeroBid",
			instrument: "TEST",
			datetime:   time.Date(2025, time.March, 1, 12, 0, 0, 0, time.UTC),
			bid:        0.0,
			ask:        1.0,
			want:       "{Datetime=2025-03-01 12:00:00 +0000 UTC Bid=0 Ask=1}",
		},
		{
			name:       "Tick EmptyBidAndAsk",
			instrument: instrumentZERO,
			datetime:   time.Date(2025, time.April, 1, 12, 0, 0, 0, time.UTC),
			bid:        0.0,
			ask:        0.0,
			want:       "{Datetime=2025-04-01 12:00:00 +0000 UTC Bid=0 Ask=0}",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ti := tick.New(testCase.instrument, testCase.datetime, testCase.bid, testCase.ask)

			got := ti.String()
			if got != testCase.want {
				t.Errorf("String() = %v, want %v", got, testCase.want)
			}
		})
	}
}

//nolint:funlen // table-driven test with many validation cases
func TestTick_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		instrument string
		datetime   time.Time
		bid        float64
		ask        float64
		wantErr    bool
	}{
		{
			name:       "Valid Tick",
			instrument: instrumentEURUSD,
			datetime:   time.Now(),
			bid:        1.2558,
			ask:        1.2560,
			wantErr:    false,
		},
		{
			name:       "Empty Instrument",
			instrument: "",
			datetime:   time.Now(),
			bid:        1.0,
			ask:        1.1,
			wantErr:    true,
		},
		{
			name:       "Zero Datetime",
			instrument: instrumentEURUSD,
			datetime:   time.Time{},
			bid:        1.0,
			ask:        1.1,
			wantErr:    true,
		},
		{
			name:       "Negative Bid",
			instrument: instrumentEURUSD,
			datetime:   time.Now(),
			bid:        -1.0,
			ask:        1.1,
			wantErr:    false,
		},
		{
			name:       "Negative Ask",
			instrument: instrumentEURUSD,
			datetime:   time.Now(),
			bid:        -2.0,
			ask:        -1.1,
			wantErr:    false,
		},
		{
			name:       "Ask Less Than Bid",
			instrument: instrumentEURUSD,
			datetime:   time.Now(),
			bid:        1.2560,
			ask:        1.2558,
			wantErr:    true,
		},
		{
			name:       "Zero Bid",
			instrument: instrumentZERO,
			datetime:   time.Now(),
			bid:        0.0,
			ask:        0.2,
			wantErr:    true,
		},
		{
			name:       "Zero Ask",
			instrument: instrumentZERO,
			datetime:   time.Now(),
			bid:        -0.2,
			ask:        0.0,
			wantErr:    true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ti := tick.New(testCase.instrument, testCase.datetime, testCase.bid, testCase.ask)

			gotErr := ti.Validate()
			if gotErr != nil {
				if !testCase.wantErr {
					t.Errorf("Validate() failed: %v", gotErr)
				}

				return
			}

			if testCase.wantErr {
				t.Fatal("Validate() succeeded unexpectedly")
			}
		})
	}
}
