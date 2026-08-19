package indicator

import (
	"errors"

	"github.com/tgragnato/orbiter/pkg/ohlc"
)

// Indicator is the common interface implemented by all technical indicators.
type Indicator interface {
	Insert(o *ohlc.OHLC)
	ValueResultKeys() []string
	Value() (map[string]float64, error)
}

// ErrNotEnoughData is returned when an indicator does not yet have sufficient
// historical data to produce a meaningful result.
var ErrNotEnoughData = errors.New("not enough data to calculate indicator")
