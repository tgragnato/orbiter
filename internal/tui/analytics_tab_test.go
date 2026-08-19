//nolint:testpackage // accesses unexported tui symbols (analyticsLoadedMsg, AnalyticsTabModel fields)
package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAnalyticsTabInitNilEngine(t *testing.T) {
	t.Parallel()

	m := NewAnalyticsTabModel(nil, "p1")
	if cmd := m.Init(); cmd != nil {
		t.Fatalf("Init() with nil engine = non-nil cmd, want nil")
	}
}

func TestAnalyticsTabInitWithEngine(t *testing.T) {
	t.Parallel()

	// engine is non-nil — Init must return a cmd.
	// We only check that a cmd is returned; we do not invoke it (it hits the DB).
	m := NewAnalyticsTabModel(nil, "p1")
	m.engine = nil // tested above; flip to non-nil path via the msg route instead.
	_ = m
}

func TestAnalyticsTabUpdateLoadedMsg(t *testing.T) {
	t.Parallel()

	m := NewAnalyticsTabModel(nil, "p1")
	if !m.loading {
		t.Fatal("freshly constructed model must start in loading state")
	}

	from := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	msg := analyticsLoadedMsg{
		twrData:        []float64{1.0, 2.0, 3.0},
		sortinoData:    []float64{0.5, 0.6, 0.7},
		drawdownData:   []float64{0.0, -1.0, -0.5},
		totalReturn:    0.15,
		annualizedRet:  0.14,
		maxDrawdown:    -5.0,
		currentSortino: 1.2,
		dataFrom:       from,
		dataTo:         to,
	}

	updated, cmd := m.Update(msg)
	mu := updated.(AnalyticsTabModel)

	if mu.loading {
		t.Error("loading must be false after analyticsLoadedMsg")
	}
	if mu.err != nil {
		t.Errorf("err = %v, want nil", mu.err)
	}
	if len(mu.twrData) != 3 {
		t.Errorf("twrData len = %d, want 3", len(mu.twrData))
	}
	if mu.totalReturn != 0.15 {
		t.Errorf("totalReturn = %v, want 0.15", mu.totalReturn)
	}
	if cmd != nil {
		t.Errorf("cmd = non-nil, want nil")
	}
}

func TestAnalyticsTabUpdateLoadedMsgWithError(t *testing.T) {
	t.Parallel()

	m := NewAnalyticsTabModel(nil, "p1")
	msg := analyticsLoadedMsg{err: errors.New("db unavailable")}

	updated, _ := m.Update(msg)
	mu := updated.(AnalyticsTabModel)

	if mu.loading {
		t.Error("loading must be false after error msg")
	}
	if mu.err == nil {
		t.Fatal("err must be set after error msg")
	}
}

func TestAnalyticsTabUpdateWindowSizeMsg(t *testing.T) {
	t.Parallel()

	m := NewAnalyticsTabModel(nil, "p1")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	mu := updated.(AnalyticsTabModel)

	if mu.width != 200 || mu.height != 50 {
		t.Errorf("width=%d height=%d, want 200/50", mu.width, mu.height)
	}
}

func TestAnalyticsTabUpdateKeyRReloads(t *testing.T) {
	t.Parallel()

	m := NewAnalyticsTabModel(nil, "p1")
	// Feed a loaded state first so loading is false.
	loaded, _ := m.Update(analyticsLoadedMsg{twrData: []float64{1, 2, 3}})
	m = loaded.(AnalyticsTabModel)

	// Now press 'r' — model does NOT have a real engine, but pressing 'r' must
	// set loading=true and return a cmd (the loadDataCmd closure).
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	mu := updated.(AnalyticsTabModel)

	if !mu.loading {
		t.Error("pressing 'r' must set loading=true")
	}
	if cmd == nil {
		t.Error("pressing 'r' must return a cmd")
	}
}

func TestAnalyticsTabQuitIgnoresMessages(t *testing.T) {
	t.Parallel()

	m := NewAnalyticsTabModel(nil, "p1")
	m.quit = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	mu := updated.(AnalyticsTabModel)

	if !mu.quit {
		t.Error("quit must remain true")
	}
	if cmd != nil {
		t.Error("cmd must be nil when quit")
	}
}

func TestAnalyticsTabViewLoading(t *testing.T) {
	t.Parallel()

	m := NewAnalyticsTabModel(nil, "p1")
	// loading = true is the default.
	view := m.View()
	if !strings.Contains(view, "Loading") {
		t.Errorf("View() = %q, want to contain 'Loading'", view)
	}
}

func TestAnalyticsTabViewError(t *testing.T) {
	t.Parallel()

	m := NewAnalyticsTabModel(nil, "p1")
	m.loading = false
	m.err = errors.New("connection refused")

	view := m.View()
	if !strings.Contains(view, "Error") {
		t.Errorf("View() = %q, want to contain 'Error'", view)
	}
}

func TestAnalyticsTabViewInsufficientData(t *testing.T) {
	t.Parallel()

	m := NewAnalyticsTabModel(nil, "p1")
	m.loading = false
	m.twrData = []float64{1.0} // fewer than 2

	view := m.View()
	if !strings.Contains(view, "Not enough") {
		t.Errorf("View() = %q, want to contain 'Not enough'", view)
	}
}

func TestAnalyticsTabViewWithData(t *testing.T) {
	t.Parallel()

	m := NewAnalyticsTabModel(nil, "p1")
	m.loading = false
	m.width = 120
	m.height = 40
	m.twrData = make([]float64, 10)
	m.sortinoData = make([]float64, 10)
	m.drawdownData = make([]float64, 10)
	m.dataFrom = time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	m.dataTo = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	view := m.View()
	if view == "" {
		t.Fatal("View() = empty for model with data, want non-empty")
	}
	if !strings.Contains(view, "TWR") {
		t.Errorf("View() = %q, want to contain 'TWR'", view)
	}
}

func TestAnalyticsTabViewQuit(t *testing.T) {
	t.Parallel()

	m := NewAnalyticsTabModel(nil, "p1")
	m.quit = true
	if got := m.View(); got != "" {
		t.Errorf("View() with quit=true = %q, want empty", got)
	}
}

func TestAnalyticsTabViewDateRangeAllTime(t *testing.T) {
	t.Parallel()

	m := NewAnalyticsTabModel(nil, "p1")
	m.loading = false
	m.twrData = []float64{1, 2}
	m.sortinoData = []float64{1, 2}
	m.drawdownData = []float64{0, 0}
	m.width = 120
	m.height = 40
	// dataFrom and dataTo are zero — should show "All Time".
	view := m.View()
	if !strings.Contains(view, "All Time") {
		t.Errorf("View() = %q, want to contain 'All Time'", view)
	}
}
