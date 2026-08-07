package tui

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestNewLogChannelIsBuffered(t *testing.T) {
	t.Parallel()

	ch := NewLogChannel()
	if cap(ch) == 0 {
		t.Fatal("NewLogChannel() returned an unbuffered channel")
	}
}

func TestTUIHandlerEnabledAlwaysTrue(t *testing.T) {
	t.Parallel()

	h := NewTUIHandler(NewLogChannel())
	for _, lvl := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		if !h.Enabled(context.Background(), lvl) {
			t.Errorf("Enabled() = false for level %v, want true", lvl)
		}
	}
}

func TestTUIHandlerHandleForwardsRecord(t *testing.T) {
	t.Parallel()

	ch := NewLogChannel()
	h := NewTUIHandler(ch)

	ts := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	r := slog.NewRecord(ts, slog.LevelInfo, "hello world", 0)
	r.AddAttrs(slog.String("key", "value"))

	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	select {
	case entry := <-ch:
		if entry.Message != "hello world" {
			t.Errorf("Message = %q, want %q", entry.Message, "hello world")
		}
		if entry.Level != slog.LevelInfo {
			t.Errorf("Level = %v, want Info", entry.Level)
		}
		if entry.Time != ts {
			t.Errorf("Time = %v, want %v", entry.Time, ts)
		}
		if len(entry.Attrs) != 1 || entry.Attrs[0].Key != "key" {
			t.Errorf("Attrs = %v, want [{key value}]", entry.Attrs)
		}
	default:
		t.Fatal("no entry received on channel")
	}
}

func TestTUIHandlerHandleDropsWhenFull(t *testing.T) {
	t.Parallel()

	// Use a zero-buffer channel to force the drop path immediately.
	ch := make(LogChannel)
	h := NewTUIHandler(ch)

	r := slog.NewRecord(time.Now(), slog.LevelError, "dropped", 0)
	// Must not block or panic.
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle() on full channel returned error: %v", err)
	}
}

func TestTUIHandlerWithAttrsMergesAttrs(t *testing.T) {
	t.Parallel()

	ch := NewLogChannel()
	h := NewTUIHandler(ch)
	h2 := h.WithAttrs([]slog.Attr{slog.String("svc", "tui")})

	th, ok := h2.(*TUIHandler)
	if !ok {
		t.Fatalf("WithAttrs() returned %T, want *TUIHandler", h2)
	}
	if th.ch != ch {
		t.Error("WithAttrs() must share the original channel")
	}
	if len(th.attrs) != 1 || th.attrs[0].Key != "svc" {
		t.Errorf("attrs = %v, want [{svc tui}]", th.attrs)
	}
}

func TestTUIHandlerWithAttrsChainsAttrs(t *testing.T) {
	t.Parallel()

	ch := NewLogChannel()
	h := NewTUIHandler(ch)
	h2 := h.WithAttrs([]slog.Attr{slog.String("a", "1")})
	h3 := h2.WithAttrs([]slog.Attr{slog.String("b", "2")})

	th := h3.(*TUIHandler)
	if len(th.attrs) != 2 {
		t.Fatalf("chained attrs len = %d, want 2", len(th.attrs))
	}
}

func TestTUIHandlerWithGroupSetsGroup(t *testing.T) {
	t.Parallel()

	ch := NewLogChannel()
	h := NewTUIHandler(ch)
	h2 := h.WithGroup("mygroup")

	th, ok := h2.(*TUIHandler)
	if !ok {
		t.Fatalf("WithGroup() returned %T, want *TUIHandler", h2)
	}
	if th.group != "mygroup" {
		t.Errorf("group = %q, want %q", th.group, "mygroup")
	}
	if th.ch != ch {
		t.Error("WithGroup() must share the original channel")
	}
}
