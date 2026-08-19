package taa

import (
	"time"

	"github.com/tgragnato/orbiter/internal/portfolio"
	"github.com/tgragnato/orbiter/internal/signal"
)

// optimizeSatellite computes conviction-weighted target allocations for all satellite
// holdings and returns one result per holding that clears the friction gate.
//
// Target weight formula:
//
//	rawWeight[i] = max(0, 1 + conviction[i])
//	  conviction = +1 → 2× (overweight)
//	  conviction =  0 → 1× (equal weight)
//	  conviction = -1 → 0  (exit position)
//
// Normalized: targetWeight[i] = rawWeight[i] / Σ rawWeights
//
// Holdings with conviction = -1 (rawWeight = 0) are emitted as TypeSell signals.
// All other holdings are normalized among themselves and emitted as TypeRebalance signals.
// A holding is included in the output only when abs(conviction) exceeds the
// per-holding friction threshold, which accounts for broker fees, capital-gains
// tax, and the configured safety buffer.
//
//nolint:cyclop,gocognit,nestif,funlen // inherent complexity of conviction-weighted satellite optimizer
func optimizeSatellite(
	holdings []portfolio.Holding,
	conviction ConvictionProvider,
	cfg Config,
	now time.Time,
) []signal.Message {
	// Collect satellite holdings that are eligible for TAA evaluation.
	type candidate struct {
		h          portfolio.Holding
		conviction float64
		rawWeight  float64
	}

	var candidates []candidate

	totalNAV := 0.0

	for _, holding := range holdings {
		if !holding.TAAEnabled || holding.Quantity <= 0 || holding.AllocationType != portfolio.AllocationSatellite {
			continue
		}

		conv := conviction.Conviction(holding.Symbol)
		raw := 1 + conv

		if raw < 0 {
			raw = 0
		}

		candidates = append(candidates, candidate{h: holding, conviction: conv, rawWeight: raw})
		totalNAV += holding.NAV()
	}

	if len(candidates) == 0 || totalNAV <= 0 {
		return nil
	}

	// Separate candidates into exits (rawWeight=0, conviction=-1) and keeps.
	// Exits are emitted as TypeSell; keeps are normalized and emitted as TypeRebalance.
	var exits, keeps []candidate

	for _, cand := range candidates {
		if cand.rawWeight == 0 {
			exits = append(exits, cand)
		} else {
			keeps = append(keeps, cand)
		}
	}

	var msgs []signal.Message

	// Emit exit signals for holdings with conviction = -1.
	for _, cand := range exits {
		feeRate := cfg.BrokerFeePercent
		if cfg.MaxBrokerFee > 0 {
			posValue := cand.h.Quantity * cand.h.MarketPrice
			if posValue > 0 {
				if capped := cfg.MaxBrokerFee / posValue; capped < feeRate {
					feeRate = capped
				}
			}
		}

		friction := feeRate*(1+cfg.TaxRate) + cfg.Buffer
		if abs(cand.conviction) <= friction {
			continue
		}

		currentWeight := cand.h.NAV() / totalNAV
		delta := -currentWeight * totalNAV

		msgs = append(msgs, signal.NewSellMessage(now, cand.h.Symbol, cand.conviction, currentWeight, delta, cfg.Currency))
	}

	// Normalize keeps and emit rebalance signals.
	if len(keeps) > 0 {
		totalRaw := 0.0

		for _, cand := range keeps {
			totalRaw += cand.rawWeight
		}

		for _, cand := range keeps {
			targetWeight := cand.rawWeight / totalRaw
			currentWeight := cand.h.NAV() / totalNAV
			delta := (targetWeight - currentWeight) * totalNAV

			feeRate := cfg.BrokerFeePercent
			if cfg.MaxBrokerFee > 0 {
				posValue := cand.h.Quantity * cand.h.MarketPrice
				if posValue > 0 {
					if capped := cfg.MaxBrokerFee / posValue; capped < feeRate {
						feeRate = capped
					}
				}
			}

			friction := feeRate*(1+cfg.TaxRate) + cfg.Buffer
			if abs(cand.conviction) <= friction {
				continue
			}

			direction := "increase"
			if cand.conviction < 0 {
				direction = "decrease"
			}

			msgs = append(msgs, signal.NewRebalanceMessage(
				now,
				cand.h.Symbol,
				cand.conviction,
				direction,
				currentWeight,
				targetWeight,
				delta,
				cfg.Currency,
			))
		}
	}

	return msgs
}
