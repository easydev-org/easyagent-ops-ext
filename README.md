# easyagent-ops-ext

A standalone easyagent **extension server** that gives the agent Kubernetes
**operations** capability — deploy/observe services, inspect storage and
config, build images, and run run-to-completion Jobs — over the easybus
protocol.

It speaks **`easyops.v1.OpsService`** (the easyops Connect API). It carries
**no cluster credentials of its own**: easyops is the only component with
Kubernetes RBAC, so this extension is a thin, policy-controlled client.

## Tools

**Services (read)**
`ops-list-services`, `ops-get-service`, `ops-service-logs`,
`ops-service-manifest`

**Image build**
`ops-build-image` — build a repo tarball with the cluster buildkitd and push
it to the registry

**Storage / config (read)**
`ops-list-pvcs`, `ops-get-pvc`, `ops-list-configmaps`, `ops-get-configmap`,
`ops-list-secrets`, `ops-get-secret`

**Jobs (run-to-completion)**
`ops-create-job`, `ops-list-jobs`, `ops-get-job`, `ops-job-logs`,
`ops-delete-job` — a one-shot buildctl pod, a migration, a test run. `create`
waits up to 5 minutes for a terminal state; a longer job returns `active` and
you poll `ops-get-job`.

## Two guards on every call

| Knob | Effect |
|---|---|
| **`namespace`** | The ONLY namespace this extension may touch. A request whose `namespace` differs is refused — a model cannot reach another tenant's workloads. Empty disables the check. |
| **`allow-write`** | Gates **every** state-changing tool (`ops-build-image`, `ops-create-job`, `ops-delete-job`). **Off by default**: the agent can look, but not change anything until an operator enables it. |

## Secrets are never returned

`ops-get-secret` / `ops-list-secrets` report the **key names** and whether each
is set — never the values. `easyops-token` itself is a `kind: secret` config
knob, so the agent withholds it from every client.

## Configuration

```yaml
config:
  - name: easyops-url        # http://easyops.worker.svc.cluster.local
  - name: easyops-token      # kind: secret
  - name: namespace          # default: dev
  - name: allow-write        # default: false
```

## Wire contract

The easyops proto is **vendored** under `proto/easyops/v1/` and the Go client
stubs are generated with `buf` (connect-go v2).

## Build / test

```bash
go build ./cmd/ops-ext
go test ./...
```
