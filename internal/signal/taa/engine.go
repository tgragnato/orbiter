// Package taa implements the Tactical Asset Allocation signal engine.
// It enforces two portfolio-level constraints before emitting rebalance signals:
//
//  1. Core PMC Floor: never emit a sell/rebalance for Core holdings when
//     the current market price is at or below the purchase cost (PMC).
//     Instead, a CORE_PMC_FLOOR_ALERT signal is emitted.
//
//  2. Satellite Friction Gate: only emit a TypeRebalance signal for Satellite
//     holdings when the expected alpha exceeds the combined friction of capital
//     gains tax, broker fees, and a configurable buffer.
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

// CoreSatelliteTargets holds the target allocation ratios for the portfolio.
type CoreSatelliteTargets struct {
	CoreRatio      float64
	SatelliteRatio float64
}

// TargetReader provides live TAA parameters from persistent settings.
// Implement this with configuration.Service in production code.
type TargetReader interface {
	GetCoreSatelliteTargets(ctx context.Context) (CoreSatelliteTargets, error)
	// GetRebalanceThreshold returns the minimum absolute allocation drift (e.g.
	// 0.05 = 5%) required before rebalance signals are considered. When this
	// method returns an error or zero the engine falls back to Config.RebalanceThreshold.
	GetRebalanceThreshold(ctx context.Context) (float64, error)
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
	// RebalanceThreshold is the minimum absolute allocation drift (e.g. 0.05 = 5%)
	// required before any rebalance signal is considered.
	RebalanceThreshold float64
}

// Engine evaluates all holdings in the store and emits TAA signals when
// constraints are met.
type Engine struct {
	store      portfolio.HoldingsStore
	pmc        PMCReader
	conviction ConvictionProvider
	dispatcher signal.Dispatcher
	cfg        Config
	targets    TargetReader // optional; nil disables the portfolio-level drift gate
}

// NewEngine creates a TAA signal engine.
func NewEngine(
	store portfolio.HoldingsStore,
	pmc PMCReader,
	conviction ConvictionProvider,
	dispatcher signal.Dispatcher,
	cfg Config,
	targets TargetReader,
) *Engine {
	return &Engine{
		store:      store,
		pmc:        pmc,
		conviction: conviction,
		dispatcher: dispatcher,
		cfg:        cfg,
		targets:    targets,
	}
}

// Evaluate loads all holdings and dispatches appropriate TAA signals.
// It is safe to call repeatedly (e.g. on an EOD timer).
func (e *Engine) Evaluate(ctx context.Context) error {
	holdings, err := e.store.ListHoldings(ctx)
	if err != nil {
		return fmt.Errorf("taa.Evaluate: list holdings: %w", err)
	}

	// Portfolio-level drift gate: skip per-holding evaluation when the actual
	// core/satellite split is within the configured rebalance threshold.
	// Both the target ratios and the threshold are fetched live from settings
	// when a TargetReader is wired; the static Config values serve as fallback.
	if e.targets != nil && len(holdings) > 0 {
		tgt, err := e.targets.GetCoreSatelliteTargets(ctx)
		if err == nil {
			threshold := e.cfg.RebalanceThreshold
			if dynThreshold, thErr := e.targets.GetRebalanceThreshold(ctx); thErr == nil && dynThreshold > 0 {
				threshold = dynThreshold
			}

			totalNAV, coreNAV := 0.0, 0.0
			for _, h := range holdings {
				if !h.TAAEnabled || h.Quantity <= 0 {
					continue
				}
				nav := h.NAV()
				totalNAV += nav
				if h.AllocationType == portfolio.AllocationCore {
					coreNAV += nav
				}
			}
			if totalNAV > 0 {
				drift := abs(coreNAV/totalNAV - tgt.CoreRatio)
				if drift < threshold {
					slog.Debug("TAA drift within threshold; skipping evaluation",
						"drift", drift, "threshold", threshold)
					return nil
				}
			}
		}
	}

	now := time.Now().UTC()
	for _, h := range holdings {
		if !h.TAAEnabled || h.Quantity <= 0 {
			continue
		}
		switch h.AllocationType {
		case portfolio.AllocationCore:
			e.evaluateCore(ctx, h, now)
		case portfolio.AllocationSatellite:
			e.evaluateSatellite(ctx, h, now)
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

func (e *Engine) evaluateSatellite(ctx context.Context, h portfolio.Holding, now time.Time) {
	conviction := e.conviction.Conviction(h.Symbol)

	// Effective fee rate: BrokerFeePercent unless the absolute cap is cheaper.
	feeRate := e.cfg.BrokerFeePercent
	if e.cfg.MaxBrokerFeeEUR > 0 {
		posValue := h.Quantity * h.MarketPrice
		if posValue > 0 {
			if capped := e.cfg.MaxBrokerFeeEUR / posValue; capped < feeRate {
				feeRate = capped
			}
		}
	}

	// Friction: round-trip fee cost + tax on that gain + safety buffer.
	friction := feeRate*(1+e.cfg.TaxRate) + e.cfg.Buffer
	expectedAlpha := abs(conviction)

	if expectedAlpha <= friction {
		slog.Debug("satellite friction gate blocked rebalance",
			"symbol", h.Symbol,
			"conviction", conviction,
			"friction", friction,
		)
		return
	}

	// Emit rebalance signal only when conviction exceeds the threshold.
	direction := "increase"
	if conviction < 0 {
		direction = "decrease"
	}
	msg := signal.NewRebalanceMessage(now, h.Symbol, conviction, direction)
	if err := e.dispatcher.Dispatch(msg); err != nil {
		slog.Error("dispatch rebalance signal", "symbol", h.Symbol, "error", err)
	} else {
		slog.Info("rebalance signal dispatched", "symbol", h.Symbol,
			"conviction", conviction, "direction", direction)
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

// ConstantConviction returns the same conviction score for every symbol.
// Useful for testing and for wiring before the ML engine has trained.
type ConstantConviction struct{ V float64 }

func (c ConstantConviction) Conviction(_ string) float64 { return c.V }
