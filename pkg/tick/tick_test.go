package tick

import (
	"testing"
	"time"
)

func TestTick_Spread(t *testing.T) {
	t.Parallel()

	bid := 1.00
	ask := 1.50
	tick := New("EURUSD", time.Now(), bid, ask)
	spread := tick.Spread()
	if spread != 0.50 {
		t.Fatalf("expected %v, got %v", 0.50, spread)
	}
}

func TestTick_SpreadInPercent(t *testing.T) {
	t.Parallel()

	bid := 0.80
	ask := 1.50
	tick := New("EURUSD", time.Now(), bid, ask)
	spread := tick.SpreadInPercent()
	if spread != 87.50 {
		t.Fatalf("expected %v, got %v", 87.50, spread)
	}
}

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
			instrument: "EURUSD",
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
			instrument: "ZERO",
			datetime:   time.Date(2025, time.April, 1, 12, 0, 0, 0, time.UTC),
			bid:        0.0,
			ask:        0.0,
			want:       "{Datetime=2025-04-01 12:00:00 +0000 UTC Bid=0 Ask=0}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ti := New(tt.instrument, tt.datetime, tt.bid, tt.ask)
			got := ti.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

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
			instrument: "EURUSD",
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
			instrument: "EURUSD",
			datetime:   time.Time{},
			bid:        1.0,
			ask:        1.1,
			wantErr:    true,
		},
		{
			name:       "Negative Bid",
			instrument: "EURUSD",
			datetime:   time.Now(),
			bid:        -1.0,
			ask:        1.1,
			wantErr:    false,
		},
		{
			name:       "Negative Ask",
			instrument: "EURUSD",
			datetime:   time.Now(),
			bid:        -2.0,
			ask:        -1.1,
			wantErr:    false,
		},
		{
			name:       "Ask Less Than Bid",
			instrument: "EURUSD",
			datetime:   time.Now(),
			bid:        1.2560,
			ask:        1.2558,
			wantErr:    true,
		},
		{
			name:       "Zero Bid",
			instrument: "ZERO",
			datetime:   time.Now(),
			bid:        0.0,
			ask:        0.2,
			wantErr:    true,
		},
		{
			name:       "Zero Ask",
			instrument: "ZERO",
			datetime:   time.Now(),
			bid:        -0.2,
			ask:        0.0,
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ti := New(tt.instrument, tt.datetime, tt.bid, tt.ask)
			gotErr := ti.Validate()
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("Validate() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("Validate() succeeded unexpectedly")
			}
		})
	}
}
