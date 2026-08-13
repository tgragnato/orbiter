package taa

import (
	"time"

	"github.com/tgragnato/orbiter/internal/portfolio"
	"github.com/tgragnato/orbiter/internal/signal"
)

// SymbolProvider returns all symbols the ML engine has conviction scores for,
// including those not currently held in the portfolio.
type SymbolProvider interface {
	Symbols() []string
}

// evaluateEntries suggests new satellite positions for tracked symbols that are
// not currently held but have strong ML conviction.
//
// The suggested allocation is computed by running the full conviction-weighted
// optimizer across both held satellites and candidate unowned symbols (treated as
// current weight = 0%). This gives a portfolio-consistent target: if you were to
// open this position, it should represent X% of your satellite NAV.
//
// Only symbols with abs(conviction) > friction are emitted as TypeBuy signals.
func evaluateEntries(
	holdings []portfolio.Holding,
	tracked []string,
	conviction ConvictionProvider,
	cfg Config,
	now time.Time,
) []signal.Message {
	if len(tracked) == 0 {
		return nil
	}

	// suppressed tracks symbols that must never receive entry signals:
	//   - TAAEnabled=false: user explicitly opted this symbol out of TAA
	//   - TAAEnabled=true, Quantity>0: already held; rebalance optimizer handles it
	suppressed := map[string]struct{}{}
	totalSatelliteNAV := 0.0
	for _, h := range holdings {
		if h.AllocationType != portfolio.AllocationSatellite || !h.TAAEnabled {
			suppressed[h.Symbol] = struct{}{}
			continue
		}
		if h.Quantity > 0 {
			suppressed[h.Symbol] = struct{}{}
			totalSatelliteNAV += h.NAV()
		}
	}

	// Collect unowned candidates with non-trivial conviction.
	type candidate struct {
		symbol     string
		conviction float64
		rawWeight  float64
	}

	var candidates []candidate
	for _, sym := range tracked {
		if _, ok := suppressed[sym]; ok {
			continue // already held; handled by the satellite rebalance optimizer
		}
		c := conviction.Conviction(sym)
		raw := 1 + c
		if raw < 0 {
			raw = 0
		}
		candidates = append(candidates, candidate{symbol: sym, conviction: c, rawWeight: raw})
	}

	if len(candidates) == 0 {
		return nil
	}

	// Also include held satellites in the weight pool so suggested entry %
	// reflects the actual portfolio context.
	type heldWeight struct {
		symbol    string
		rawWeight float64
	}
	var heldWeights []heldWeight
	for _, h := range holdings {
		if !h.TAAEnabled || h.Quantity <= 0 || h.AllocationType != portfolio.AllocationSatellite {
			continue
		}
		c := conviction.Conviction(h.Symbol)
		raw := 1 + c
		if raw < 0 {
			raw = 0
		}
		heldWeights = append(heldWeights, heldWeight{symbol: h.Symbol, rawWeight: raw})
	}

	totalRaw := 0.0
	for _, hw := range heldWeights {
		totalRaw += hw.rawWeight
	}
	for _, cand := range candidates {
		totalRaw += cand.rawWeight
	}
	if totalRaw == 0 {
		eq := 1.0 / float64(len(candidates)+len(heldWeights))
		for i := range candidates {
			candidates[i].rawWeight = eq
		}
		totalRaw = 1.0
	}

	var msgs []signal.Message
	for _, cand := range candidates {
		// Per-candidate friction: without a position value we fall back to
		// BrokerFeePercent (cannot apply the EUR cap without knowing order size).
		friction := cfg.BrokerFeePercent*(1+cfg.TaxRate) + cfg.Buffer
		if abs(cand.conviction) <= friction {
			continue
		}

		targetWeight := cand.rawWeight / totalRaw
		delta := targetWeight * totalSatelliteNAV

		msgs = append(msgs, signal.NewBuyMessage(now, cand.symbol, cand.conviction, targetWeight, delta, cfg.Currency))
	}

	return msgs
}
