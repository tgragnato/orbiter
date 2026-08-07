// Package taa implements the Tactical Asset Allocation signal engine.
// It enforces two per-holding constraints before emitting signals:
//
//  1. Core PMC Floor: emit CORE_PMC_FLOOR_ALERT when market price ≤ PMC.
//
//  2. Satellite Friction Gate: emit REBALANCE or ENTRY only when the ML
//     conviction exceeds the combined friction of broker fees, capital-gains
//     tax, and a configurable safety buffer.
package taa

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tgragnato/orbiter/internal/portfolio"
	"github.com/tgragnato/orbiter/internal/signal"
)

// PMCReader returns the weighted average purchase cost for a given symbol.
// An error means no cost data is available; in that case the floor check is skipped.
type PMCReader interface {
	PMC(ctx context.Context, symbol string) (float64, error)
}

// ConvictionProvider returns the current ML conviction score in [-1,+1] for a
// given holding symbol.
type ConvictionProvider interface {
	Conviction(symbol string) float64
}

// Config controls the friction gate parameters.
type Config struct {
	// TaxRate is the effective capital-gains tax rate (e.g. 0.26 for 26%).
	TaxRate float64
	// BrokerFeePercent is the broker transaction cost as a fraction (e.g. 0.0019).
	BrokerFeePercent float64
	// MaxBrokerFeeEUR caps the broker fee in absolute currency units. When > 0
	// the effective fee rate is min(BrokerFeePercent, MaxBrokerFeeEUR/positionValue).
	MaxBrokerFeeEUR float64
	// Buffer is an additional threshold above taxes+fees required to trade.
	Buffer float64
}

// Engine evaluates all holdings in the store and emits TAA signals when
// constraints are met.
type Engine struct {
	store      portfolio.HoldingsStore
	pmc        PMCReader
	conviction ConvictionProvider
	symbols    SymbolProvider // optional; nil disables entry signal evaluation
	dispatcher signal.Dispatcher
	cfg        Config
}

// NewEngine creates a TAA signal engine.
func NewEngine(
	store portfolio.HoldingsStore,
	pmc PMCReader,
	conviction ConvictionProvider,
	symbols SymbolProvider,
	dispatcher signal.Dispatcher,
	cfg Config,
) *Engine {
	return &Engine{
		store:      store,
		pmc:        pmc,
		conviction: conviction,
		symbols:    symbols,
		dispatcher: dispatcher,
		cfg:        cfg,
	}
}

// Evaluate loads all holdings and dispatches appropriate TAA signals.
// It is safe to call repeatedly (e.g. on an EOD timer).
func (e *Engine) Evaluate(ctx context.Context) error {
	holdings, err := e.store.ListHoldings(ctx)
	if err != nil {
		return fmt.Errorf("taa.Evaluate: list holdings: %w", err)
	}

	now := time.Now().UTC()

	// Core holdings: per-holding PMC floor check.
	for i := range holdings {
		if !holdings[i].TAAEnabled || holdings[i].Quantity <= 0 || holdings[i].AllocationType != portfolio.AllocationCore {
			continue
		}
		e.evaluateCore(ctx, holdings[i], now)
	}

	// Satellite holdings: portfolio-level conviction-weighted optimizer.
	msgs := optimizeSatellite(holdings, e.conviction, e.cfg, now)
	for i := range msgs {
		msg := msgs[i]
		if err := e.dispatcher.Dispatch(msg); err != nil {
			slog.Error("dispatch rebalance signal", "symbol", msg.Instrument, "error", err)
		} else {
			slog.Info("rebalance signal dispatched",
				"symbol", msg.Instrument,
				"current_weight", msg.CurrentWeight,
				"target_weight", msg.TargetWeight,
				"delta_eur", msg.DeltaEUR,
			)
		}
	}

	// Entry signals: tracked symbols not yet held with strong conviction.
	if e.symbols != nil {
		entryMsgs := evaluateEntries(holdings, e.symbols.Symbols(), e.conviction, e.cfg, now)
		for i := range entryMsgs {
			msg := entryMsgs[i]
			if err := e.dispatcher.Dispatch(msg); err != nil {
				slog.Error("dispatch entry signal", "symbol", msg.Instrument, "error", err)
			} else {
				slog.Info("entry signal dispatched",
					"symbol", msg.Instrument,
					"target_weight", msg.TargetWeight,
					"delta_eur", msg.DeltaEUR,
				)
			}
		}
	}

	return nil
}

func (e *Engine) evaluateCore(ctx context.Context, h portfolio.Holding, now time.Time) {
	if h.MarketPrice <= 0 {
		return
	}

	// Prefer PMC embedded in the holding (computed from transaction history);
	// fall back to the PMCReader interface (e.g. tax-lot records).
	pmc := h.PMC
	if pmc <= 0 {
		var err error
		pmc, err = e.pmc.PMC(ctx, h.Symbol)
		if err != nil || pmc <= 0 {
			return
		}
	}

	// Floor triggered: current price ≤ PMC → emit alert.
	if h.MarketPrice <= pmc {
		msg := signal.NewCorePMCFloorAlert(now, h.Symbol, h.MarketPrice, pmc)
		if err := e.dispatcher.Dispatch(msg); err != nil {
			slog.Error("dispatch core PMC floor alert", "symbol", h.Symbol, "error", err)
		} else {
			slog.Info("core PMC floor alert dispatched", "symbol", h.Symbol,
				"market_price", h.MarketPrice, "pmc", pmc)
		}
	}
}


func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// NullPMCReader returns an error for every symbol so the Core floor check is
// skipped when no accounting data is wired up.
type NullPMCReader struct{}

func (NullPMCReader) PMC(_ context.Context, _ string) (float64, error) {
	return 0, fmt.Errorf("no PMC reader configured")
}

