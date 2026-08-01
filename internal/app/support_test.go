package app

import (
	"testing"
	"time"

	"github.com/sklinkert/at/internal/strategy"
)

func TestBuildStrategy(t *testing.T) {
	tests := []struct {
		name     string
		strategy string
	}{
		{name: "doji", strategy: strategy.NameDOJI},
		{name: "heikinashi", strategy: strategy.NameHeikinAshi},
		{name: "scalper", strategy: strategy.NameScalper},
		{name: "stochrsi", strategy: strategy.NameStochRSI},
		{name: "lowcandle", strategy: strategy.NameLowCandle},
		{name: "harami", strategy: strategy.NameHarami},
		{name: "sma10", strategy: strategy.NameSMA10},
		{name: "engulfing", strategy: strategy.NameEngulfing},
		{name: "rsi", strategy: strategy.NameRSI},
		{name: "rsiadx", strategy: strategy.NameRSIADX},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildStrategy(tt.strategy, "EURUSD", time.Minute)
			if err != nil {
				t.Fatalf("buildStrategy() error = %v", err)
			}
			if got == nil {
				t.Fatalf("buildStrategy() returned nil strategy")
			}
			if got.Name() != tt.strategy {
				t.Fatalf("strategy.Name() = %q, want %q", got.Name(), tt.strategy)
			}
		})
	}
}

func TestBuildStrategyRejectsUnknown(t *testing.T) {
	if _, err := buildStrategy("unknown", "EURUSD", time.Minute); err == nil {
		t.Fatalf("buildStrategy() error = nil, want non-nil")
	}
}
