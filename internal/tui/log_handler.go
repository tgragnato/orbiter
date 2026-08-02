package tui

import (
	"context"
	"log/slog"
	"time"
)

// LogEntry is a single captured slog record.
type LogEntry struct {
	Time    time.Time
	Level   slog.Level
	Message string
	Attrs   []slog.Attr
}

// LogChannel streams log entries into the TUI.
type LogChannel chan LogEntry

// NewLogChannel creates a buffered channel for log entries.
func NewLogChannel() LogChannel {
	return make(LogChannel, 256)
}

// TUIHandler is a slog.Handler that forwards records to a LogChannel.
// Drops entries silently when the buffer is full so callers never block.
type TUIHandler struct {
	ch    LogChannel
	attrs []slog.Attr
	group string
}

// NewTUIHandler returns a handler that sends records to ch.
func NewTUIHandler(ch LogChannel) *TUIHandler {
	return &TUIHandler{ch: ch}
}

func (h *TUIHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *TUIHandler) Handle(_ context.Context, r slog.Record) error {
	entry := LogEntry{
		Time:    r.Time,
		Level:   r.Level,
		Message: r.Message,
	}
	r.Attrs(func(a slog.Attr) bool {
		entry.Attrs = append(entry.Attrs, a)
		return true
	})
	select {
	case h.ch <- entry:
	default:
	}
	return nil
}

func (h *TUIHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := &TUIHandler{ch: h.ch, group: h.group}
	next.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return next
}

func (h *TUIHandler) WithGroup(name string) slog.Handler {
	return &TUIHandler{ch: h.ch, attrs: h.attrs, group: name}
}
