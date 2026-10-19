package config

import "testing"

func TestParse(t *testing.T) {
	m := map[string]string{
		KnobURL:        "http://easyops:8080",
		KnobToken:      "tok",
		KnobNamespace:  "dev",
		KnobAllowWrite: "true",
	}
	c := Parse(func(k string) string { return m[k] })
	if c.URL != "http://easyops:8080" || c.Token != "tok" || c.Namespace != "dev" || !c.AllowWrite {
		t.Fatalf("parse = %+v", c)
	}
	// Defaults: no allow-write => false.
	c2 := Parse(func(string) string { return "" })
	if c2.AllowWrite {
		t.Error("allow-write must default to false")
	}
}

// TestNamespaceAllowed pins the allow-list: the configured namespace only, and
// an empty request namespace means "the configured one".
func TestNamespaceAllowed(t *testing.T) {
	c := Config{Namespace: "dev"}
	if !c.NamespaceAllowed("") {
		t.Error("empty ns must be allowed (it means the configured one)")
	}
	if !c.NamespaceAllowed("dev") {
		t.Error("the configured ns must be allowed")
	}
	if c.NamespaceAllowed("worker") {
		t.Error("another namespace must be DENIED")
	}
	if c.NamespaceAllowed("dev-other") {
		t.Error("a prefix-similar namespace must be DENIED")
	}
	// An unset configured namespace disables the check entirely.
	off := Config{}
	if !off.NamespaceAllowed("anything") {
		t.Error("unset namespace must disable the check")
	}
}
