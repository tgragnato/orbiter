//nolint:testpackage // accesses unexported tui symbols (LogsTabModel fields, LogEntry)
package tui

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// makeEntry builds a LogEntry for test use.
func makeEntry(level slog.Level, msg string) LogEntry {
	return LogEntry{Time: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), Level: level, Message: msg}
}

func TestLogsTabInitNilChannel(t *testing.T) {
	t.Parallel()

	m := NewLogsTabModel(nil)
	if cmd := m.Init(); cmd != nil {
		t.Fatalf("Init() with nil channel = non-nil cmd, want nil")
	}
}

func TestLogsTabInitWithChannel(t *testing.T) {
	t.Parallel()

	ch := NewLogChannel()
	m := NewLogsTabModel(ch)
	if cmd := m.Init(); cmd == nil {
		t.Fatalf("Init() with channel = nil, want non-nil")
	}
}

func TestLogsTabViewNilChannelPlaceholder(t *testing.T) {
	t.Parallel()

	m := NewLogsTabModel(nil)
	if got := m.View(); !strings.Contains(got, "not configured") {
		t.Errorf("View() = %q, want to contain 'not configured'", got)
	}
}

func TestLogsTabViewEmptyEntries(t *testing.T) {
	t.Parallel()

	m := NewLogsTabModel(NewLogChannel())
	if got := m.View(); !strings.Contains(got, "no log entries") {
		t.Errorf("View() = %q, want 'no log entries'", got)
	}
}

func TestLogsTabUpdateLogEntryAppends(t *testing.T) {
	t.Parallel()

	ch := NewLogChannel()
	m := NewLogsTabModel(ch)

	entry := makeEntry(slog.LevelInfo, "first entry")
	updated, cmd := m.Update(entry)
	m = updated.(LogsTabModel)

	if len(m.entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(m.entries))
	}
	if cmd == nil {
		t.Fatal("Update(LogEntry) cmd = nil, want waitForLogEntry cmd")
	}
}

func TestLogsTabUpdateCapsAtMaxEntries(t *testing.T) {
	t.Parallel()

	ch := NewLogChannel()
	m := NewLogsTabModel(ch)
	m.height = 1000 // ensure visibleLines doesn't cap

	for range maxLogEntries + 10 {
		updated, _ := m.Update(makeEntry(slog.LevelDebug, "msg"))
		m = updated.(LogsTabModel)
	}

	if len(m.entries) != maxLogEntries {
		t.Fatalf("entries len = %d, want %d", len(m.entries), maxLogEntries)
	}
}

func TestLogsTabScrollUpAndDown(t *testing.T) {
	t.Parallel()

	ch := NewLogChannel()
	m := NewLogsTabModel(ch)
	m.height = 10

	// Fill entries beyond visible range.
	for range 30 {
		updated, _ := m.Update(makeEntry(slog.LevelInfo, "msg"))
		m = updated.(LogsTabModel)
	}

	// Scroll up.
	up, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	mu := up.(LogsTabModel)
	if mu.offset != 1 {
		t.Errorf("after up, offset = %d, want 1", mu.offset)
	}

	// Scroll up via k.
	upK, _ := mu.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	mk := upK.(LogsTabModel)
	if mk.offset != 2 {
		t.Errorf("after k, offset = %d, want 2", mk.offset)
	}

	// Scroll down.
	down, _ := mk.Update(tea.KeyMsg{Type: tea.KeyDown})
	md := down.(LogsTabModel)
	if md.offset != 1 {
		t.Errorf("after down, offset = %d, want 1", md.offset)
	}

	// Scroll down via j.
	downJ, _ := md.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	mj := downJ.(LogsTabModel)
	if mj.offset != 0 {
		t.Errorf("after j, offset = %d, want 0", mj.offset)
	}
}

func TestLogsTabScrollToTopAndBottom(t *testing.T) {
	t.Parallel()

	ch := NewLogChannel()
	m := NewLogsTabModel(ch)
	m.height = 10

	for range 30 {
		updated, _ := m.Update(makeEntry(slog.LevelInfo, "msg"))
		m = updated.(LogsTabModel)
	}

	// g — jump to top (oldest).
	topM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	mt := topM.(LogsTabModel)
	if mt.offset == 0 {
		t.Errorf("after g, offset = 0, expected > 0 for many entries")
	}

	// G — jump back to bottom.
	botM, _ := mt.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	mb := botM.(LogsTabModel)
	if mb.offset != 0 {
		t.Errorf("after G, offset = %d, want 0", mb.offset)
	}
}

func TestLogsTabWindowSizeMsg(t *testing.T) {
	t.Parallel()

	m := NewLogsTabModel(NewLogChannel())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	mu := updated.(LogsTabModel)
	if mu.width != 120 || mu.height != 40 {
		t.Errorf("width=%d height=%d, want 120/40", mu.width, mu.height)
	}
}

func TestLogsTabVisibleLinesFloor(t *testing.T) {
	t.Parallel()

	m := NewLogsTabModel(NewLogChannel())
	m.height = 0 // triggers the floor
	if got := m.visibleLines(); got != 20 {
		t.Errorf("visibleLines() with height=0 = %d, want 20", got)
	}
}

func TestLogsTabRenderEntryAllLevels(t *testing.T) {
	t.Parallel()

	m := NewLogsTabModel(NewLogChannel())
	cases := []struct {
		level slog.Level
		tag   string
	}{
		{slog.LevelDebug, "DBG"},
		{slog.LevelInfo, "INF"},
		{slog.LevelWarn, "WRN"},
		{slog.LevelError, "ERR"},
	}
	for _, tc := range cases {
		rendered := m.renderEntry(makeEntry(tc.level, "test"))
		if !strings.Contains(rendered, tc.tag) {
			t.Errorf("renderEntry(%v) = %q, want to contain %q", tc.level, rendered, tc.tag)
		}
	}
}

func TestLogsTabRenderEntryCustomLevel(t *testing.T) {
	t.Parallel()

	m := NewLogsTabModel(NewLogChannel())
	e := LogEntry{Time: time.Now(), Level: slog.Level(42), Message: "custom"}
	rendered := m.renderEntry(e)
	if rendered == "" {
		t.Fatal("renderEntry() for custom level returned empty string")
	}
}

func TestLogsTabRenderEntryWithAttrs(t *testing.T) {
	t.Parallel()

	m := NewLogsTabModel(NewLogChannel())
	e := LogEntry{
		Time:    time.Now(),
		Level:   slog.LevelInfo,
		Message: "with attrs",
		Attrs:   []slog.Attr{slog.String("key", "val"), slog.Int("n", 1)},
	}
	rendered := m.renderEntry(e)
	if !strings.Contains(rendered, "key") {
		t.Errorf("renderEntry() = %q, want to contain attr key", rendered)
	}
}

func TestLogsTabViewShowsScrollHint(t *testing.T) {
	t.Parallel()

	ch := NewLogChannel()
	m := NewLogsTabModel(ch)
	m.height = 10

	for range 30 {
		updated, _ := m.Update(makeEntry(slog.LevelInfo, "msg"))
		m = updated.(LogsTabModel)
	}

	// Scroll up so offset > 0.
	up, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = up.(LogsTabModel)

	// The scroll hint is surfaced via NavHint() (merged into the root help line),
	// not in View() itself — the log body fills the full available height.
	hint := m.NavHint()
	if !strings.Contains(hint, "more below") {
		t.Errorf("NavHint() = %q, want scroll hint 'more below'", hint)
	}
}
