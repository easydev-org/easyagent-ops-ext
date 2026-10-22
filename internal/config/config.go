// Package config parses the ops-ext config knobs. Two are security-critical:
//
//   - `namespace`   — the ONLY tenant-internal namespace this extension may
//     touch. Every request that carries a namespace is checked against it, so
//     a model cannot reach another tenant's workloads.
//   - `allow-write` — gates every state-changing tool. Off by default: an
//     agent with ops access can still LOOK (list/get/logs) but not change
//     anything until an operator turns writes on.
package config

import "strings"

// Knob names — single source of truth for the manifest + readers.
const (
	KnobURL       = "easyops-url"
	KnobToken     = "easyops-token"
	KnobNamespace = "namespace"
)

// Config is the parsed ops-ext configuration.
type Config struct {
	URL       string
	Token     string
	Namespace string
}

// Parse builds a Config from the raw knob values.
func Parse(get func(name string) string) Config {
	return Config{
		URL:       strings.TrimSpace(get(KnobURL)),
		Token:     strings.TrimSpace(get(KnobToken)),
		Namespace: strings.TrimSpace(get(KnobNamespace)),
	}
}

// NamespaceAllowed reports whether a request may target `ns`. An empty ns means
// "the extension's configured namespace", so it is always allowed; otherwise
// the request must match the configured namespace exactly (an unset namespace
// disables the check).
func (c Config) NamespaceAllowed(ns string) bool {
	if ns == "" || c.Namespace == "" {
		return true
	}
	return ns == c.Namespace
}
