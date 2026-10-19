package tools

import (
	"strings"
	"testing"

	opsv1 "github.com/easydev-org/easyagent-ops-ext/gen/easyops/v1"
)

// TestEmptyLogsStillExplain: an empty log body must NEVER render as "". That is
// exactly what a crashing container looks like, and an empty answer tells the
// caller nothing. We must surface the pod phase / restarts / reason instead.
func TestEmptyLogsStillExplain(t *testing.T) {
	res := &opsv1.ServiceLogsResponse{
		PodPhase: "Running", Restarts: 7,
		Message:   "unable to retrieve container logs",
		Available: false,
	}
	out := explainServiceLogs(res, false)
	if strings.TrimSpace(out) == "" {
		t.Fatal("empty logs rendered as an empty string")
	}
	for _, want := range []string{"pod status", "Running", "7", "unable to retrieve", "previous: true"} {
		if !strings.Contains(out, want) {
			t.Errorf("explanation missing %q:\n%s", want, out)
		}
	}
}

// TestEmptyPreviousLogsExplainWithoutNextStep: when reading the PREVIOUS
// instance there is nothing further to suggest, so the "call again with
// previous: true" hint must not appear (it would loop the caller).
func TestEmptyPreviousLogsExplainWithoutNextStep(t *testing.T) {
	res := &opsv1.ServiceLogsResponse{PodPhase: "Running", Restarts: 3, Available: true}
	out := explainServiceLogs(res, true)
	if !strings.Contains(out, "PREVIOUS") {
		t.Errorf("must name the previous instance:\n%s", out)
	}
	if strings.Contains(out, "next step") {
		t.Errorf("must not suggest previous again:\n%s", out)
	}
}

// TestHeaderAlwaysPresent pins the decision: the diagnostic header is emitted
// EVEN WHEN there are logs. A crash-looping container prints its message before
// every death, so gating the header on "no logs" would hide `restarts` and the
// reason exactly when they explain the loop.
func TestHeaderAlwaysPresent(t *testing.T) {
	res := &opsv1.ServiceLogsResponse{
		Lines:    []string{"fatal: missing DATABASE_URL"},
		PodPhase: "Running", Restarts: 5, Message: "CrashLoopBackOff",
		Available: true,
	}
	out := explainServiceLogs(res, false)
	for _, want := range []string{"pod status", "Running", "5", "CrashLoopBackOff", "fatal: missing DATABASE_URL"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Both must be present, and the log body must come AFTER the header.
	if strings.Index(out, "pod status") > strings.Index(out, "fatal: missing") {
		t.Errorf("header must precede the log body:\n%s", out)
	}
}
