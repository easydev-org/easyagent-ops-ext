package tools

import (
	"strings"
	"testing"

	opsv1 "github.com/easydev-org/easyagent-ops-ext/gen/easyops/v1"
)

// TestPendingPVCSaysWhy pins the point of the message field: Pending alone is
// ambiguous, so the rendering must carry the reason AND the age.
func TestPendingPVCSaysWhy(t *testing.T) {
	p := &opsv1.PVCInfo{
		Name: "data", Phase: "Pending", Size: "10Gi", StorageClass: "workspace-local",
		Message:    "WaitForFirstConsumer: waiting for first consumer to be created before binding",
		AgeSeconds: 35,
	}
	out := describePVC(p)
	for _, want := range []string{"phase=Pending", "10Gi requested", "workspace-local",
		"waiting for first consumer", "age       : 35s"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// TestPendingPVCFailureIsDistinguishable: a failing claim looks the same at the
// phase level as a waiting one — only the message tells them apart.
func TestPendingPVCFailureIsDistinguishable(t *testing.T) {
	wait := &opsv1.PVCInfo{Name: "a", Phase: "Pending", AgeSeconds: 5,
		Message: "WaitForFirstConsumer: waiting for first consumer"}
	fail := &opsv1.PVCInfo{Name: "b", Phase: "Pending", AgeSeconds: 720,
		Message: `ProvisioningFailed: failed to provision volume with StorageClass "local-path"`}

	hintA, hintB := pvcHint(wait), pvcHint(fail)
	if hintA == hintB {
		t.Errorf("a waiting and a failing Pending claim must read differently, both %q", hintA)
	}
	if !strings.Contains(hintB, "ProvisioningFailed") {
		t.Errorf("failure hint lost the reason: %q", hintB)
	}
	// The age makes "stuck" visible: 12 minutes vs 5 seconds.
	if !strings.Contains(hintB, "12m") {
		t.Errorf("failure hint lost the age: %q", hintB)
	}
	if !strings.Contains(hintA, "5s") {
		t.Errorf("waiting hint lost the age: %q", hintA)
	}
}

// TestBoundPVCShowsVolume: a bound claim reports the real volume and the
// ALLOCATED capacity (which a provisioner may have rounded up).
func TestBoundPVCShowsVolume(t *testing.T) {
	p := &opsv1.PVCInfo{
		Name: "data", Phase: "Bound", Size: "10Gi", Capacity: "10Gi",
		BoundVolume: "pvc-abc123", StorageClass: "workspace-local",
		MountedBy: []string{"agent"}, AgeSeconds: 90,
	}
	out := describePVC(p)
	for _, want := range []string{"volume    : pvc-abc123", "10Gi allocated",
		"mounted by: agent", "1m30s"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "message") {
		t.Errorf("a Bound claim needs no message:\n%s", out)
	}
}

// TestLostPVCIsLoud: Lost means the data is unreachable; it must not read like
// a normal state.
func TestLostPVCIsLoud(t *testing.T) {
	h := pvcHint(&opsv1.PVCInfo{Name: "x", Phase: "Lost"})
	if !strings.Contains(h, "LOST") {
		t.Errorf("Lost must be loud, got %q", h)
	}
}

func TestShortDuration(t *testing.T) {
	cases := map[int64]string{5: "5s", 90: "1m30s", 7200: "2h0m"}
	for in, want := range cases {
		if got := shortDuration(in); got != want {
			t.Errorf("shortDuration(%d) = %q, want %q", in, got, want)
		}
	}
}
