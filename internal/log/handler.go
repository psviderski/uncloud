package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

// SlogTextHandler extends the standard [slog.TextHandler] to provide custom formatting that is very similar
// to the default slog handler. It prefixes each log entry with a right-padded level indicator and message,
// while excluding the default time, level, and message attributes from the structured output.
//
// The output format is:
// LEVEL MESSAGE key1=value1 key2=value2
type SlogTextHandler struct {
	*slog.TextHandler
	mu sync.Mutex
	w  io.Writer
}

func NewSlogTextHandler(w io.Writer, opts *slog.HandlerOptions) *SlogTextHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	// Remove time, level, and message from the default attributes.
	opts.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey || a.Key == slog.LevelKey || a.Key == slog.MessageKey {
			return slog.Attr{}
		}
		return a
	}

	return &SlogTextHandler{
		TextHandler: slog.NewTextHandler(w, opts),
		w:           w,
	}
}

func (h *SlogTextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.TextHandler.Enabled(ctx, level)
}

func (h *SlogTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h.TextHandler.WithAttrs(attrs)
}

func (h *SlogTextHandler) WithGroup(name string) slog.Handler {
	return h.TextHandler.WithGroup(name)
}

func (h *SlogTextHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	levelMsg := fmt.Sprintf("%-5s %s ", r.Level.String(), r.Message)
	if _, err := h.w.Write([]byte(levelMsg)); err != nil {
		return err
	}

	return h.TextHandler.Handle(ctx, r)
}
