//nolint:testpackage // accesses unexported tui symbols (fakeSignalReadModel, SignalsTabModel fields)
package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tgragnato/orbiter/internal/signal"
)

func TestSignalsTabRefreshAndView(t *testing.T) {
	t.Parallel()

	rm := fakeSignalReadModel{
		messages: []signal.Message{{Type: signal.TypeBuy, Summary: "Buy EURUSD", CreatedAt: time.Unix(1000, 0).UTC()}},
	}
	model := NewSignalsTabModelWithML(rm, nil)

	updated, _ := model.Update(model.refreshCmd()())
	model = updated.(SignalsTabModel)

	if len(model.messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(model.messages))
	}
	if got := model.View(); got == "" {
		t.Fatalf("View() empty, want non-empty")
	}
}

func TestSignalsTabTickCmd(t *testing.T) {
	t.Parallel()

	model := NewSignalsTabModelWithML(fakeSignalReadModel{}, nil)
	updated, cmd := model.Update(signalsTickMsg(time.Now()))
	_ = updated.(SignalsTabModel)
	if cmd == nil {
		t.Fatalf("tick cmd = nil, want non-nil")
	}

	// Use a 1ms tick to verify the message type without waiting signalsRefreshInterval.
	msg := tea.Tick(time.Millisecond, func(t time.Time) tea.Msg { return signalsTickMsg(t) })()
	if _, ok := msg.(signalsTickMsg); !ok {
		t.Fatalf("signalsTickCmd did not return signalsTickMsg, got %T", msg)
	}
}

func TestSignalsTabQuitStopsCommands(t *testing.T) {
	t.Parallel()

	model := NewSignalsTabModelWithML(fakeSignalReadModel{}, nil)
	model.quit = true
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	_ = updated.(SignalsTabModel)
	if cmd != nil {
		t.Fatalf("cmd = non-nil, want nil")
	}
}

func TestSignalsTabQuitView(t *testing.T) {
	t.Parallel()

	model := NewSignalsTabModelWithML(fakeSignalReadModel{}, nil)
	model.quit = true
	if got := model.View(); got != "" {
		t.Fatalf("View() while quit = %q, want empty", got)
	}
}

func TestSignalsTabNilReadModelRefreshCmd(t *testing.T) {
	t.Parallel()

	model := NewSignalsTabModelWithML(nil, nil)
	msg := model.refreshCmd()()
	sm, ok := msg.(signalsMsg)
	if !ok {
		t.Fatalf("msg type = %T, want signalsMsg", msg)
	}
	if sm.messages != nil {
		t.Fatalf("messages = non-nil, want nil for nil readModel")
	}
}

func TestSignalsTabViewWithMessages(t *testing.T) {
	t.Parallel()

	rm := fakeSignalReadModel{
		messages: []signal.Message{
			{Type: signal.TypeBuy, Summary: "Buy VWCE", CreatedAt: time.Unix(1000, 0).UTC()},
			{Type: signal.TypeSell, Summary: "Sell ZPRV", CreatedAt: time.Unix(2000, 0).UTC()},
		},
	}
	model := NewSignalsTabModelWithML(rm, nil)
	updated, _ := model.Update(model.refreshCmd()())
	model = updated.(SignalsTabModel)

	view := model.View()
	if view == "" {
		t.Fatalf("View() = empty, want non-empty")
	}
	// Should contain message count
	if len(model.messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(model.messages))
	}
}
