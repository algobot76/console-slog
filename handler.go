package console

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

var bufferPool = &sync.Pool{
	New: func() any { return new(buffer) },
}

var cwd, _ = os.Getwd()

// HandlerOptions are options for a ConsoleHandler.
// A zero HandlerOptions consists entirely of default values.
type HandlerOptions struct {
	// AddSource causes the handler to compute the source code position
	// of the log statement and add a SourceKey attribute to the output.
	AddSource bool

	// Level reports the minimum record level that will be logged.
	// The handler discards records with lower levels.
	// If Level is nil, the handler assumes LevelInfo.
	// The handler calls Level.Level for each record processed;
	// to adjust the minimum level dynamically, use a LevelVar.
	Level slog.Leveler

	// Disable colorized output
	NoColor bool
}

type Handler struct {
	opts    *HandlerOptions
	out     io.Writer
	group   string
	context buffer
	enc     *encoder
}

var _ slog.Handler = (*Handler)(nil)

// NewHandler creates a Handler that writes to w,
// using the given options.
// If opts is nil, the default options are used.
func NewHandler(out io.Writer, opts *HandlerOptions) *Handler {
	if opts == nil {
		opts = new(HandlerOptions)
	}
	if opts.Level == nil {
		opts.Level = slog.LevelInfo
	}
	opt := *opts // Copy struct
	return &Handler{
		opts:    &opt,
		out:     out,
		group:   "",
		context: nil,
		enc:     &encoder{nocolor: opt.NoColor},
	}
}

// Enabled implements slog.Handler.
func (h *Handler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.opts.Level.Level()
}

// Handle implements slog.Handler.
func (h *Handler) Handle(_ context.Context, rec slog.Record) error {
	buf := bufferPool.Get().(*buffer)

	h.enc.writeTimestamp(buf, rec.Time)
	h.enc.writeLevel(buf, rec.Level)
	if h.opts.AddSource && rec.PC > 0 {
		h.enc.writeSource(buf, rec.PC, cwd)
	}
	h.enc.writeMessage(buf, rec.Message)
	buf.copy(&h.context)
	rec.Attrs(func(a slog.Attr) bool {
		h.enc.writeAttr(buf, a, h.group)
		return true
	})
	h.enc.NewLine(buf)
	if _, err := buf.WriteTo(h.out); err != nil {
		buf.Reset()
		bufferPool.Put(buf)
		return err
	}
	bufferPool.Put(buf)
	return nil
}

// WithAttrs implements slog.Handler.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newCtx := h.context
	for _, a := range attrs {
		h.enc.writeAttr(&newCtx, a, h.group)
	}
	newCtx.Clip()
	return &Handler{
		opts:    h.opts,
		out:     h.out,
		group:   h.group,
		context: newCtx,
		enc:     h.enc,
	}
}

// WithGroup implements slog.Handler.
func (h *Handler) WithGroup(name string) slog.Handler {
	name = strings.TrimSpace(name)
	if h.group != "" {
		name = h.group + "." + name
	}
	return &Handler{
		opts:    h.opts,
		out:     h.out,
		group:   name,
		context: h.context,
		enc:     h.enc,
	}
}
