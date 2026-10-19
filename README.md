# easyagent-ops-ext

A standalone easyagent **extension server** that gives the agent Kubernetes
**operations** capability — deploy/observe services, inspect storage and
config, build images, and run run-to-completion Jobs — over the easybus
protocol.

It speaks **`easyops.v1.OpsService`** (the easyops Connect API). It carries
**no cluster credentials of its own**: easyops is the only component with
Kubernetes RBAC, so this extension is a thin, policy-controlled client.

## Tools (39)

### Sandboxes — lifecycle (6)

**`sandbox-ext` works INSIDE a running sandbox** (exec, files, jobs,
`computer-*`). It has no tool to create, list, resolve or delete one — that
half lives here.

| Tool | What it does |
|---|---|
| `ops-create-sandbox` | Create a worker sandbox (image, cpu/memory, env, kvm, gpu, volumes, golden_disk) |
| `ops-list-sandboxes` | List sandboxes with phase / readiness / image / URL |
| `ops-get-sandbox` | One sandbox |
| `ops-resolve-sandbox` | The in-cluster **URL + token** — what you need to talk to it |
| `ops-delete-sandbox` | Delete the Pod, Service and Secret |
| `ops-sandbox-logs` | The sandbox **pod's** logs — the only way to diagnose one that never started (sandbox-ext cannot enter a sandbox that is not up). Falls back to pod phase / restarts / reason |

### Services — read

4 tools.

| Tool | What it does |
|---|---|
| `ops-list-services` | List services with **ready/total replicas** (`1/2`), phase, image, URL. `*` = fully ready, `~` = partially ready (rollout converging), blank = none ready |
| `ops-get-service` | One service in detail: phase, `replicas=N/M (ready/total)`, URL, restarts, message |
| `ops-service-logs` | **Always** prints a diagnostic header (pod phase, restart count, failure reason) **then** the log body. `previous: true` reads the PREVIOUS container instance |
| `ops-service-manifest` | The live multi-document YAML — the thing you edit to upgrade |

### Services — write (require `allow-write`)

7 tools.

| Tool | What it does |
|---|---|
| `ops-deploy-service` | Create/replace a service from structured fields (image, replicas, ports, env, cpu/memory) |
| `ops-apply-manifest` | **The upgrade path**: PATCH from an edited YAML. Always `dry_run: true` first — it validates without changing anything |
| `ops-rollback-service` | `k8s rollout undo` to a prior revision. Omit `revision` for the previous one |
| `ops-scale-service` | Set the replica count (scale up **and** down) |
| `ops-pause-service` | Scale to zero, remembering the count |
| `ops-resume-service` | Restore the count Pause remembered |
| `ops-delete-service` | Delete the Deployment **and** its Service. Use `pause` to merely stop something |

### Image build (requires `allow-write`)

1 tool.

`ops-build-image` — build a repo tarball with the cluster buildkitd and push it
to the registry.

### Storage — read + write

4 tools: `ops-list-pvcs`, `ops-get-pvc`, **`ops-create-pvc`** (size, storage
class), **`ops-delete-pvc`** (⚠️ destroys the data).

**Reading a PVC.** A claim has exactly three phases — `Pending` / `Bound` /
`Lost` — and `Pending` covers **both** "the provisioner is still working" and
"provisioning FAILED". `ops-get-pvc` therefore also reports:

| Field | Why it matters |
|---|---|
| `message` | the failing event (`ProvisioningFailed: ...`) or the waiting event (`WaitForFirstConsumer: waiting for first consumer...`) — **the only way to tell the two Pending cases apart** |
| `age` | seconds = normal, hours = stuck |
| `volume` | the bound PersistentVolume (`Bound` only) |
| `size` | `10Gi requested, 10Gi allocated` |

`ops-list-pvcs` appends a one-line hint per row (`mounted by: X`, the pending
reason + `[pending 8h9m]`, or a loud `LOST: ...`).

**Which claims do you see?** By default only the ones **this deployment
created** — the platform's own `postgres-data` / `garage-data` are not a
tenant's business. Pass **`include_all: true`** to list every claim in the
namespace. If you expect claims and get "none", that is the reason; the tool
says so and names the flag.

### Config — config maps

5 tools: `ops-list-configmaps`, `ops-get-configmap`,
`ops-get-configmap-values`, **`ops-put-configmap`**, **`ops-delete-configmap`**

`ops-get-configmap` returns **key names only** (quick inventory).
`ops-get-configmap-values` returns the **full contents** — config map data is
not secret (secrets are their own object), and an agent cannot patch a map it
cannot read.

### Config — secrets

5 tools: `ops-list-secrets`, `ops-get-secret`, **`ops-get-secret-values`**,
**`ops-put-secret`**, **`ops-delete-secret`**

`ops-get-secret` is the default and returns **key names only** — reading secret
values is privileged, so `ops-get-secret-values` is a **separate** tool.
It exists because `ops-put-secret` REPLACES the whole object: writing without
reading first would silently destroy keys. `ops-put-secret` never echoes the
values back, refuses empty `data`, and supports `dry_run`.

> ⚠️ **Any `put-*` tool REPLACES the whole object.** A Kubernetes write is a
> full object update, so **a key you omit is DELETED**. Read first
> (`ops-get-configmap-values` / `ops-get-secret-values`), then send the
> complete map. Both refuse empty `data` and both support `dry_run`.

### Jobs — run-to-completion

5 tools.

`ops-create-job`, `ops-list-jobs`, `ops-get-job`, `ops-job-logs`,
`ops-delete-job` — a one-shot buildctl pod, a migration, a test run. `create`
waits up to 5 minutes for a terminal state; a longer job returns `active` and
you poll `ops-get-job`. `create`/`delete` require `allow-write`.

## Recipes

### Diagnose a service that is not healthy

```
ops-list-services                     # spot "0/2 ready" or "~"
ops-get-service name=X                # phase, restarts, message
ops-service-logs name=X               # header explains, body shows output
ops-service-logs name=X previous=true # the instance that DIED
```

### Upgrade a service

```
ops-service-manifest name=X           # 1. take the current YAML
                                      # 2. edit it (image tag, env, resources)
ops-apply-manifest name=X yaml=... dry_run=true    # 3. validate
ops-apply-manifest name=X yaml=... dry_run=false   # 4. apply
ops-get-service name=X                # 5. wait for N/N ready
```

### Roll back a bad release

```
ops-rollback-service name=X           # the previous revision
ops-get-service name=X                # confirm N/N ready
```

### Resize or stop a service

```
ops-scale-service  name=X replicas=3  # scale up / down
ops-pause-service  name=X             # to zero, remembers the count
ops-resume-service name=X             # back to that count
```

## Logs and diagnostics are different evidence

`ops-service-logs` reports both, and they come from different Kubernetes APIs:

| | Logs (`lines`) | Diagnostics (phase / restarts / reason) |
|---|---|---|
| Source | pod **log stream** (stdout/stderr) | pod **status object** |
| Answers | *what the process did* | *how the pod is doing* |

A **crash-looping** container prints its message before every death, so the log
body is non-empty **while** `restarts` climbs and `reason` says
`CrashLoopBackOff`. That is why the header is always shown rather than only
when the body is empty — hiding it would hide the loop. When a container dies
before producing any output (image pull failure, `RunContainerError`,
unschedulable), the header is the *only* explanation, and the body says so
explicitly.

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
