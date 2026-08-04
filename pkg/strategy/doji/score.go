package doji

import "github.com/tgragnato/orbiter/pkg/ohlc"

// Score returns +0.3 when the previous candle is a DOJI (potential breakout
// setup pending tick-level confirmation), 0 otherwise. The score is kept small
// because the actual directional signal only fires at tick level.
func (d *Doji) Score(_ []*ohlc.OHLC) float64 {
	if isDOJI(d.previousCandle) {
		return 0.3
	}
	return 0
}
