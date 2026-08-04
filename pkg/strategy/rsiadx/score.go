package rsiadx

import "github.com/tgragnato/orbiter/pkg/ohlc"

// Score returns a conviction in [-1.0, +1.0]. Without a strong ADX trend the
// score is zero. With ADX ≥ threshold the RSI conviction is scaled by the
// normalised ADX strength (capped at 1.0).
func (d *RSIADX) Score(_ []*ohlc.OHLC) float64 {
	adxVal, err := d.getADX()
	if err != nil || adxVal < adxThreshold {
		return 0
	}

	rsiVal, err := d.getRSI()
	if err != nil {
		return 0
	}
	upper, lower := d.eo.RSI()

	// Normalise ADX strength relative to threshold (100 as ceiling).
	adxScale := (adxVal - adxThreshold) / (100.0 - adxThreshold)
	if adxScale > 1 {
		adxScale = 1
	}

	const mid = 50.0
	var rsiConviction float64
	switch {
	case rsiVal <= lower:
		rsiConviction = 1.0
	case rsiVal >= upper:
		rsiConviction = -1.0
	case rsiVal < mid:
		rsiConviction = (mid - rsiVal) / (mid - lower)
	default:
		rsiConviction = -(rsiVal - mid) / (upper - mid)
	}

	return rsiConviction * adxScale
}
