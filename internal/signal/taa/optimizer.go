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
// A holding is included in the output only when abs(conviction) exceeds the
// per-holding friction threshold, which accounts for broker fees, capital-gains
// tax, and the configured safety buffer.
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

	for _, h := range holdings {
		if !h.TAAEnabled || h.Quantity <= 0 || h.AllocationType != portfolio.AllocationSatellite {
			continue
		}
		c := conviction.Conviction(h.Symbol)
		raw := 1 + c
		if raw < 0 {
			raw = 0
		}
		candidates = append(candidates, candidate{h: h, conviction: c, rawWeight: raw})
		totalNAV += h.NAV()
	}

	if len(candidates) == 0 || totalNAV <= 0 {
		return nil
	}

	// Normalize raw weights to target allocations.
	totalRaw := 0.0
	for _, c := range candidates {
		totalRaw += c.rawWeight
	}

	// If every conviction is -1 the raw total is 0; fall back to equal weight.
	if totalRaw == 0 {
		for i := range candidates {
			candidates[i].rawWeight = 1
		}
		totalRaw = float64(len(candidates))
	}

	// Build results and apply per-holding friction gate.
	var msgs []signal.Message
	for _, cand := range candidates {
		targetWeight := cand.rawWeight / totalRaw
		currentWeight := cand.h.NAV() / totalNAV
		deltaEUR := (targetWeight - currentWeight) * totalNAV

		// Friction: round-trip fee + tax on gains + safety buffer.
		feeRate := cfg.BrokerFeePercent
		if cfg.MaxBrokerFeeEUR > 0 {
			posValue := cand.h.Quantity * cand.h.MarketPrice
			if posValue > 0 {
				if capped := cfg.MaxBrokerFeeEUR / posValue; capped < feeRate {
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
			deltaEUR,
		))
	}

	return msgs
}
