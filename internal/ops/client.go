// Package ops wraps the easyops.v1.OpsService Connect client: bearer auth, a
// per-(url,token) LRU cache, and typed helpers the tools call.
package ops

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"

	opsv1connect "github.com/easydev-org/easyagent-ops-ext/gen/easyops/v1/opsv1connect"
)

// Endpoint is the easyops address + shared service token.
type Endpoint struct {
	URL   string
	Token string
}

// Client is the OpsService Connect client.
type Client = opsv1connect.OpsServiceClient

func normalizeURL(raw string) string {
	s := strings.TrimSpace(raw)
	for strings.HasSuffix(s, "/") {
		s = s[:len(s)-1]
	}
	return s
}

// bearerInterceptor attaches `Authorization: Bearer <token>` (the easyops
// shared service token).
func bearerInterceptor(token string) connect.ClientInterceptor {
	return func(next connect.ClientFunc) connect.ClientFunc {
		return func(ctx context.Context, spec connect.Spec) (connect.ClientStream, error) {
			if token != "" {
				var info *connect.CallInfo
				ctx, info = connect.NewClientContext(ctx)
				info.RequestHeader().Set("Authorization", "Bearer "+token)
			}
			return next(ctx, spec)
		}
	}
}

// Dial builds a bearer-authenticated OpsService client.
func Dial(ep Endpoint) Client {
	hc := &http.Client{}
	var ics []connect.ClientInterceptor
	if ep.Token != "" {
		ics = append(ics, bearerInterceptor(ep.Token))
	}
	return opsv1connect.NewOpsServiceClient(connect.NewClient(
		connecthttp.NewTransport(hc, normalizeURL(ep.URL), connecthttp.WithReadMaxBytes(0)),
		ics...,
	))
}

// ClientCache caches clients per (url, token), LRU-capped.
type ClientCache struct {
	mu    sync.Mutex
	max   int
	byKey map[string]Client
}

// NewClientCache builds a cache capped at max entries (default 8).
func NewClientCache(max int) *ClientCache {
	if max <= 0 {
		max = 8
	}
	return &ClientCache{max: max, byKey: map[string]Client{}}
}

func key(ep Endpoint) string { return normalizeURL(ep.URL) + "\x00" + ep.Token }

// Get returns a cached client for ep.
func (c *ClientCache) Get(ep Endpoint) Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := key(ep)
	if hit, ok := c.byKey[k]; ok {
		return hit
	}
	cl := Dial(ep)
	c.byKey[k] = cl
	if len(c.byKey) > c.max {
		for oldest := range c.byKey {
			if oldest != k {
				delete(c.byKey, oldest)
				break
			}
		}
	}
	return cl
}
