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

// TestWriteToolsAreNotGatedByAllowWrite: a state-changing tool must reach the
// backend regardless of `allow-write`.
//
// That knob used to refuse every write before it left the extension. It was a
// second switch layered under the NAMESPACE scope, and its only effect in practice
// was to block legitimate work until someone remembered to set it — with an error
// naming a knob the caller could not set. The namespace is the real blast-radius
// limit, and it is still enforced (see the namespace test below).
//
// The backend here is deliberately unroutable, so the assertion is precise: the
// call gets far enough to fail on the NETWORK, which can only happen if the
// allow-write gate let it through.
func TestWriteToolsAreNotGatedByAllowWrite(t *testing.T) {
	r := reg(map[string]string{
		config.KnobURL:       "http://127.0.0.1:1", // unroutable on purpose
		config.KnobNamespace: "dev",
		// allow-write deliberately ABSENT: it must not matter.
	})
	writes := []struct {
		tool string
		args map[string]any
	}{
		{"ops-build-image", map[string]any{"repo": "a/b", "archive": "x"}},
		{"ops-create-job", map[string]any{"name": "j", "image": "i"}},
		{"ops-delete-job", map[string]any{"name": "j"}},
		{"ops-deploy-service", map[string]any{"name": "s", "image": "i"}},
		{"ops-apply-manifest", map[string]any{"name": "s", "yaml": "x"}},
		{"ops-rollback-service", map[string]any{"name": "s"}},
		{"ops-scale-service", map[string]any{"name": "s", "replicas": float64(2)}},
		{"ops-pause-service", map[string]any{"name": "s"}},
		{"ops-resume-service", map[string]any{"name": "s"}},
		{"ops-delete-service", map[string]any{"name": "s"}},
		{"ops-put-configmap", map[string]any{"name": "c", "data": map[string]any{"k": "v"}}},
		{"ops-delete-configmap", map[string]any{"name": "c"}},
		{"ops-create-sandbox", map[string]any{"name": "sb"}},
		{"ops-delete-sandbox", map[string]any{"name": "sb"}},
		{"ops-create-pvc", map[string]any{"name": "p"}},
		{"ops-delete-pvc", map[string]any{"name": "p"}},
		{"ops-put-secret", map[string]any{"name": "sec", "data": map[string]any{"k": "v"}}},
		{"ops-delete-secret", map[string]any{"name": "sec"}},
		{"ops-create-build", map[string]any{"repo": "a/b", "archive": "x"}},
		{"ops-cancel-build", map[string]any{"id": "b1"}},
		{"ops-build-files", map[string]any{"archive": "x"}},
	}
	for _, w := range writes {
		_, err := r[w.tool](context.Background(), w.args, "s", "t")
		if err == nil {
			continue // reached the backend and somehow succeeded: fine
		}
		if strings.Contains(err.Error(), "allow-write") {
			t.Errorf("%s: still gated by allow-write: %v", w.tool, err)
		}
		// Any OTHER error (connection refused) proves it was let through.
	}
}

// TestNamespaceGuardOnCreateJob: a namespace outside the allow-list is refused.
//
// This is the boundary that MATTERS: it keeps one extension from touching another
// team's namespace. (The old `allow-write` switch only toggled writes on and off
// wholesale, which is not a blast-radius limit at all.)
func TestNamespaceGuardOnCreateJob(t *testing.T) {
	r := reg(map[string]string{
		config.KnobURL:       "http://127.0.0.1:1",
		config.KnobNamespace: "dev",
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
