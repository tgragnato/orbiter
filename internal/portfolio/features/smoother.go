package features

// ExponentialSmoother applies exponential smoothing to a stream of values,
// suppressing intraday noise without introducing look-ahead bias. Each call to
// Update returns the smoothed estimate for the latest observation.
//
// The smoothing factor α ∈ (0,1] controls how quickly the smoother forgets old
// values: α = 1 means no smoothing (passthrough), α close to 0 means heavy
// smoothing.
type ExponentialSmoother struct {
	alpha   float64
	current float64
	seeded  bool
}

// NewExponentialSmoother creates a smoother with the given α. α is clamped to
// (0, 1].
func NewExponentialSmoother(alpha float64) *ExponentialSmoother {
	if alpha <= 0 {
		alpha = 0.01
	}
	if alpha > 1 {
		alpha = 1
	}
	return &ExponentialSmoother{alpha: alpha}
}

// Update feeds a new observation and returns the smoothed value.
func (s *ExponentialSmoother) Update(value float64) float64 {
	if !s.seeded {
		s.current = value
		s.seeded = true
		return value
	}
	s.current = s.alpha*value + (1-s.alpha)*s.current
	return s.current
}

// Value returns the last smoothed estimate (0 before the first observation).
func (s *ExponentialSmoother) Value() float64 {
	return s.current
}

// Reset clears internal state so the smoother can be reused.
func (s *ExponentialSmoother) Reset() {
	s.current = 0
	s.seeded = false
}

// SmoothedReturns applies exponential smoothing to a pre-computed return series
// and returns the smoothed series (same length as input).
func SmoothedReturns(returns []float64, alpha float64) []float64 {
	s := NewExponentialSmoother(alpha)
	out := make([]float64, len(returns))
	for i, r := range returns {
		out[i] = s.Update(r)
	}
	return out
}
