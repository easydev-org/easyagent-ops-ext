package tools

import (
	"context"
	"strings"
	"testing"

	easybus "github.com/easydev-org/easybus-sdk"
)

// collect installs a sink and returns the frames it receives.
//
// Frames are kept BY POINTER: ToolProgress embeds protoimpl.MessageState, which
// contains a mutex, so copying one (even into a slice) copies a lock — `go vet`
// rejects it and rightly so.
func collect() (context.Context, *[]*easybus.ToolProgress) {
	var got []*easybus.ToolProgress
	ctx := WithProgress(context.Background(), func(_ context.Context, p *easybus.ToolProgress) {
		got = append(got, p)
	})
	return ctx, &got
}

func text(p *easybus.ToolProgress) string {
	if p.Text == nil {
		return ""
	}
	return *p.Text
}

// TestProgressForwardsBuildChunks is the regression test for a long build that
// showed nothing until it finished.
//
// The chunk stream was already in place — buildkit's log was consumed chunk by
// chunk — but every chunk went only into the final result. A five-minute build
// therefore looked like a hung tool, then produced a wall of text.
func TestProgressForwardsBuildChunks(t *testing.T) {
	ctx, got := collect()

	p := NewProgress()
	p.Add(ctx, "step 1/3 : FROM golang\n")
	// Force the batch out: the reporter throttles, and a test must not sleep.
	p.Flush(ctx)
	p.Add(ctx, "step 2/3 : COPY . .\n")
	p.Flush(ctx)

	if len(*got) != 2 {
		t.Fatalf("emitted %d frames, want 2", len(*got))
	}
	if !strings.Contains(text((*got)[0]), "step 1/3") {
		t.Errorf("frame 0 = %q, want the first build line", text((*got)[0]))
	}
	if !strings.Contains(text((*got)[1]), "step 2/3") {
		t.Errorf("frame 1 = %q, want the second build line", text((*got)[1]))
	}
}

// TestProgressSplitsMultiLineChunks: a buildkit chunk is an arbitrary byte slice,
// not a line. Forwarding it verbatim would glue unrelated lines into one.
func TestProgressSplitsMultiLineChunks(t *testing.T) {
	ctx, got := collect()
	p := NewProgress()
	p.Add(ctx, "alpha\nbeta\n")
	p.Flush(ctx)

	if len(*got) != 1 {
		t.Fatalf("emitted %d frames, want 1", len(*got))
	}
	frame := text((*got)[0])
	if !strings.Contains(frame, "alpha") || !strings.Contains(frame, "beta") {
		t.Fatalf("frame = %q, want both lines", frame)
	}
	if strings.Contains(frame, "alphabeta") {
		t.Fatalf("frame = %q: lines were glued together", frame)
	}
}

// TestPhaseIsImmediate: a status change must not wait behind the batch timer —
// "waiting for rollout" is only useful while it is still true.
func TestPhaseIsImmediate(t *testing.T) {
	ctx, got := collect()
	p := NewProgress()
	p.Phase(ctx, "running", "deploying api (waiting for rollout)")

	if len(*got) != 1 {
		t.Fatalf("phase emitted %d frames, want 1 (immediately)", len(*got))
	}
	if !strings.Contains(text((*got)[0]), "waiting for rollout") {
		t.Errorf("frame = %q", text((*got)[0]))
	}
}

// TestNoSinkIsSafe: a tool called outside a turn (a test, a probe) must not panic.
func TestNoSinkIsSafe(t *testing.T) {
	p := NewProgress()
	p.Add(context.Background(), "no sink here")
	p.Flush(context.Background())
	p.Phase(context.Background(), "running", "still fine")
}
