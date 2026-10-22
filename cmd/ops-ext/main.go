// Command ops-ext is the easyagent ops extension server: Kubernetes operations
// (services / PVC / config / image build / jobs) through the easyops
// OpsService, over the easybus protocol.
//
// It holds NO cluster credentials: every call is a Connect RPC to easyops,
// which is the only component with k8s RBAC.
package main

import (
	"context"
	_ "embed"
	"log/slog"
	"os"

	easybus "github.com/easydev-org/easybus-sdk"
	"github.com/easydev-org/easybus-sdk/extension"
	"github.com/easydev-org/easybus-sdk/manifest"
	"github.com/easydev-org/easybus-sdk/transport/nats"

	"github.com/easydev-org/easyagent-ops-ext/internal/ops"
	"github.com/easydev-org/easyagent-ops-ext/internal/tools"
)

//go:embed manifest.yaml
var manifestYAML []byte

func main() {
	log := slog.Default().With("svc", "ops-ext")

	natsURL := envOr("NATS_URL", "nats://127.0.0.1:4222")
	bus, err := nats.Connect(natsURL)
	if err != nil {
		log.Error("nats connect failed", "err", err)
		os.Exit(1)
	}

	m, err := manifest.ParseManifest(manifestYAML)
	if err != nil {
		log.Error("load manifest failed", "err", err)
		os.Exit(1)
	}

	var ext *extension.Extension
	cache := ops.NewClientCache(8)
	handlers := tools.Registry(tools.Deps{
		Cache: cache,
		GetConfig: func(name, session, tenant string) any {
			if ext == nil {
				return nil
			}
			return ext.GetConfig(tenant, name, session)
		},
		Locale: func(session, tenant string) string {
			if ext == nil {
				return ""
			}
			return ext.GetSessionVariable(context.Background(), tenant, "agent", session, "locale", "")
		},
	})

	specs := make(map[string]extension.ToolSpec, len(handlers))
	for name, h := range handlers {
		h := h
		specs[name] = extension.ToolSpec{
			Execute: func(ctx context.Context, args map[string]any, callID, session, tenant string) (extension.ToolResultData, error) {
				// Install a progress sink for THIS call.
				//
				// The tenant and call id are only known here, so the sink cannot be
				// built once in Deps. Without it a build/rollout runs for minutes
				// with the console showing nothing but "running".
				cctx := tools.WithProgress(ctx, func(pctx context.Context, p *easybus.ToolProgress) {
					if ext != nil {
						// A dropped UI update must never fail the operation: the work
						// already happened.
						_ = ext.ReportProgress(pctx, tenant, callID, p)
					}
				})
				return h(cctx, args, session, tenant)
			},
		}
	}

	cfg := m.BuildConfig(manifest.Bindings{
		Handlers: specs,
		OnLifecycle: func(ctx context.Context, ev *easybus.LifecycleEvent, tenant string) error {
			log.Info("lifecycle", "kind", easybus.LifecycleKindString(ev.Kind), "tenant", tenant, "session", ev.SessionName)
			return nil
		},
	})
	ext = extension.New(bus, cfg)

	if err := extension.Serve(ext, extension.ServeOptions{
		Run: func(ctx context.Context, _ *extension.Extension) {
			log.Info("listening", "nats", natsURL)
		},
	}); err != nil {
		log.Error("serve failed", "err", err)
		os.Exit(1)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
