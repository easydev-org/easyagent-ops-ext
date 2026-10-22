package tools

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	easybus "github.com/easydev-org/easybus-sdk"
)

// ProgressFunc reports an in-flight update for the current tool call.
type ProgressFunc func(ctx context.Context, p *easybus.ToolProgress)

type progressKey struct{}

// WithProgress attaches a progress sink to ctx.
//
// Carried on the context rather than in Deps because the tenant and call id are
// only known inside the per-call closure in main, after those are in hand — and
// every handler already receives the ctx.
func WithProgress(ctx context.Context, fn ProgressFunc) context.Context {
	return context.WithValue(ctx, progressKey{}, fn)
}

// EmitProgress reports one update for the call this ctx belongs to. A nil sink
// (no UI attached, or a caller outside a turn) makes this a no-op.
func EmitProgress(ctx context.Context, p *easybus.ToolProgress) {
	if fn, ok := ctx.Value(progressKey{}).(ProgressFunc); ok && fn != nil {
		fn(ctx, p)
		return
	}
	// A frame with nowhere to go would otherwise vanish, which looks exactly like
	// "the extension reported nothing".
	slog.Warn("tool progress dropped: no sink on context")
}

// Progress is the helper the long-running tools use.
//
// These operations are already STREAMING — a build's log arrives chunk by chunk,
// a rollout's status changes as pods come up — but the chunks were only collected
// into the final result. A user watching a five-minute image build saw nothing
// until it finished. This forwards them.
//
// Batching matters: buildkit emits a line per millisecond at times, and relaying
// each one floods the bus and the browser.
type Progress struct {
	mu   sync.Mutex
	buf  []string
	last time.Time

	minInterval      time.Duration
	maxLinesPerEvent int
}

// NewProgress returns a batching reporter (~5 updates/second, 50 lines each).
func NewProgress() *Progress {
	return &Progress{minInterval: 200 * time.Millisecond, maxLinesPerEvent: 50}
}

// Line queues one line of output.
func (r *Progress) Line(ctx context.Context, line string) {
	r.Add(ctx, line)
}

// Add queues lines and flushes when enough time has passed.
func (r *Progress) Add(ctx context.Context, lines ...string) {
	r.mu.Lock()
	for _, l := range lines {
		// Buildkit chunks carry their own newlines and may contain several lines.
		for _, one := range strings.Split(strings.ReplaceAll(l, "\r\n", "\n"), "\n") {
			one = strings.TrimRight(one, "\r")
			if strings.TrimSpace(one) == "" {
				continue
			}
			r.buf = append(r.buf, one)
		}
	}
	due := time.Since(r.last) >= r.minInterval
	var batch []string
	if due && len(r.buf) > 0 {
		batch, r.buf = r.buf, nil
		r.last = time.Now()
	}
	r.mu.Unlock()
	if len(batch) > 0 {
		r.emit(ctx, batch)
	}
}

// Phase reports a STATUS line ("waiting for rollout", "1/3 ready"), which is not
// command output but is what the user is waiting to see. Sent immediately rather
// than batched: these are sparse and each one is meaningful.
func (r *Progress) Phase(ctx context.Context, phase, text string) {
	EmitProgress(ctx, &easybus.ToolProgress{
		Phase: easybusStr(phase),
		Text:  easybusStr(text),
	})
}

// Flush sends whatever is pending. Call it before returning so the tail is not
// lost.
func (r *Progress) Flush(ctx context.Context) {
	r.mu.Lock()
	batch := r.buf
	r.buf = nil
	r.last = time.Now()
	r.mu.Unlock()
	if len(batch) > 0 {
		r.emit(ctx, batch)
	}
}

func (r *Progress) emit(ctx context.Context, lines []string) {
	dropped := 0
	if len(lines) > r.maxLinesPerEvent {
		dropped = len(lines) - r.maxLinesPerEvent
		lines = lines[:r.maxLinesPerEvent]
	}
	text := strings.Join(lines, "\n")
	if dropped > 0 {
		// Say so rather than truncating silently: a gap with no explanation looks
		// like the operation hung.
		text += "\n… " + itoa(dropped) + " more line(s) in this batch"
	}
	EmitProgress(ctx, &easybus.ToolProgress{
		Phase: easybusStr("running"),
		Text:  easybusStr(text),
	})
}

func easybusStr(s string) *string { return &s }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
