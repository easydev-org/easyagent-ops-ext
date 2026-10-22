package tools

import (
	"bytes"
	"os"
	"testing"
)

// TestEmbeddedManifestMatchesRoot is a guard against a duplicated file drifting.

// `//go:embed manifest.yaml` in cmd/ops-ext resolves relative to THAT directory, so
// the effective manifest is cmd/ops-ext/manifest.yaml — not the one at the repo
// root that everyone edits. They drifted once: the root copy lost the
// `allow-write` knob and the embedded one kept it, so the built image declared a
// knob the source no longer had. Which file wins is invisible at runtime; only the
// binary knows.

// Until the duplicate is removed, assert they are identical.
func TestEmbeddedManifestMatchesRoot(t *testing.T) {
	root, err := os.ReadFile("../../manifest.yaml")
	if err != nil {
		t.Fatalf("read root manifest: %v", err)
	}
	embedded, err := os.ReadFile("../../cmd/ops-ext/manifest.yaml")
	if err != nil {
		t.Fatalf("read embedded manifest: %v", err)
	}
	if !bytes.Equal(root, embedded) {
		t.Fatal("manifest.yaml and cmd/ops-ext/manifest.yaml differ; the embedded copy is what ships, " +
			"so edits to the root file have no effect on the built extension")
	}
}
