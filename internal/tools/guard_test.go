package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/easydev-org/easybus-sdk/extension"

	"github.com/easydev-org/easyagent-ops-ext/internal/config"
	"github.com/easydev-org/easyagent-ops-ext/internal/ops"
)

// reg builds a registry with the given config values.
func reg(values map[string]string) map[string]Handler {
	return Registry(Deps{
		Cache:  ops.NewClientCache(2),
		Locale: func(string, string) string { return "en" },
		GetConfig: func(name, _, _ string) any {
			return values[name]
		},
	})
}

// TestWriteToolsRequireAllowWrite: every state-changing tool must refuse when
// allow-write is off — BEFORE it reaches easyops.
func TestWriteToolsRequireAllowWrite(t *testing.T) {
	r := reg(map[string]string{
		config.KnobURL:       "http://127.0.0.1:1", // unroutable on purpose
		config.KnobNamespace: "dev",
		// allow-write deliberately absent
	})
	writes := []struct {
		tool string
		args map[string]any
	}{
		{"ops-build-image", map[string]any{"repo": "a/b", "archive": "x"}},
		{"ops-create-job", map[string]any{"name": "j", "image": "i"}},
		{"ops-delete-job", map[string]any{"name": "j"}},
	}
	for _, w := range writes {
		_, err := r[w.tool](context.Background(), w.args, "s", "t")
		if err == nil {
			t.Errorf("%s: must refuse when allow-write is off", w.tool)
			continue
		}
		var te *extension.TypedError
		if ok := asTyped(err, &te); !ok || !strings.Contains(te.Message, "allow-write") {
			t.Errorf("%s: error = %v, want the allow-write message", w.tool, err)
		}
	}
}

// TestNamespaceGuardOnCreateJob: a namespace outside the allow-list is refused
// even when writes are enabled.
func TestNamespaceGuardOnCreateJob(t *testing.T) {
	r := reg(map[string]string{
		config.KnobURL:        "http://127.0.0.1:1",
		config.KnobNamespace:  "dev",
		config.KnobAllowWrite: "true",
	})
	_, err := r["ops-create-job"](context.Background(), map[string]any{
		"name": "j", "image": "i", "namespace": "worker",
	}, "s", "t")
	if err == nil {
		t.Fatal("a foreign namespace must be refused")
	}
	var te *extension.TypedError
	if ok := asTyped(err, &te); !ok || !strings.Contains(te.Message, "worker") {
		t.Errorf("error = %v, want a namespace-denied message", err)
	}
}

// TestURLRequired: without easyops-url every tool explains the missing config.
func TestURLRequired(t *testing.T) {
	r := reg(map[string]string{})
	if _, err := r["ops-list-services"](context.Background(), map[string]any{}, "s", "t"); err == nil {
		t.Fatal("missing easyops-url must error")
	}
}

func asTyped(err error, out **extension.TypedError) bool {
	te, ok := err.(*extension.TypedError)
	if ok {
		*out = te
	}
	return ok
}
