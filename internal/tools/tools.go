// Package tools implements the ops-* tools over easyops.v1.OpsService.
//
// Two guards apply to every tool:
//   - namespace: a request may only target the configured namespace;
//   - allow-write: every state-changing tool refuses unless writes are enabled.
//
// Secret VALUES are never returned: GetSecret/ListSecrets report the key names
// and a value-set marker, never the data.
package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	easybus "github.com/easydev-org/easybus-sdk"
	"github.com/easydev-org/easybus-sdk/extension"

	opsv1 "github.com/easydev-org/easyagent-ops-ext/gen/easyops/v1"
	"github.com/easydev-org/easyagent-ops-ext/internal/config"
	"github.com/easydev-org/easyagent-ops-ext/internal/i18n"
	"github.com/easydev-org/easyagent-ops-ext/internal/ops"
)

// Deps is what the tools need from the extension host.
type Deps struct {
	GetConfig func(name, session, tenant string) any
	Locale    func(session, tenant string) string
	Cache     *ops.ClientCache
}

// Handler is one tool implementation.
type Handler func(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error)

// Registry builds the read-only + build handlers (block 1).
func Registry(d Deps) map[string]Handler {
	t := &toolset{deps: d}
	return map[string]Handler{
		// services (read)
		"ops-list-services":    t.listServices,
		"ops-get-service":      t.getService,
		"ops-service-logs":     t.serviceLogs,
		"ops-service-manifest": t.serviceManifest,
		// services (write: upgrade / patch / rollback / scale / pause)
		"ops-deploy-service":   t.deployService,
		"ops-apply-manifest":   t.applyManifest,
		"ops-rollback-service": t.rollbackService,
		"ops-scale-service":    t.scaleService,
		"ops-pause-service":    t.pauseService,
		"ops-resume-service":   t.resumeService,
		"ops-delete-service":   t.deleteService,
		// image build
		"ops-build-image":        t.buildImage,
		"ops-build-image-stream": t.buildImageStream,
		"ops-import-image":       t.importImage,
		// asynchronous builds (id + history + cancel)
		"ops-create-build": t.createBuild,
		"ops-list-builds":  t.listBuilds,
		"ops-get-build":    t.getBuild,
		"ops-cancel-build": t.cancelBuild,
		// storage / config (read)
		"ops-list-pvcs":       t.listPVCs,
		"ops-get-pvc":         t.getPVC,
		"ops-list-configmaps": t.listConfigMaps,
		"ops-get-configmap":   t.getConfigMap,
		// config maps (write) — values ARE readable (they are not secrets)
		"ops-get-configmap-values": t.getConfigMapValues,
		"ops-put-configmap":        t.putConfigMap,
		"ops-delete-configmap":     t.deleteConfigMap,
		"ops-list-secrets":         t.listSecrets,
		"ops-get-secret":           t.getSecret,
		// jobs (block 2)
		"ops-create-job": t.createJob,
		"ops-list-jobs":  t.listJobs,
		"ops-get-job":    t.getJob,
		"ops-job-logs":   t.jobLogs,
		"ops-delete-job": t.deleteJob,
		// sandboxes (lifecycle; sandbox-ext only works INSIDE a running one)
		"ops-create-sandbox":  t.createSandbox,
		"ops-list-sandboxes":  t.listSandboxes,
		"ops-get-sandbox":     t.getSandbox,
		"ops-resolve-sandbox": t.resolveSandbox,
		"ops-delete-sandbox":  t.deleteSandbox,
		"ops-sandbox-logs":    t.sandboxLogs,
		// storage + secrets (write)
		"ops-create-pvc":        t.createPVC,
		"ops-delete-pvc":        t.deletePVC,
		"ops-put-secret":        t.putSecret,
		"ops-delete-secret":     t.deleteSecret,
		"ops-get-secret-values": t.getSecretValues,
	}
}

type toolset struct{ deps Deps }

func (t *toolset) locale(session, tenant string) string {
	if t.deps.Locale != nil {
		return t.deps.Locale(session, tenant)
	}
	return ""
}

// cfg parses the caller's config.
func (t *toolset) cfg(session, tenant string) config.Config {
	// Knob values arrive typed: a `type: boolean` knob is a bool, not a string.
	// Stringify everything so one reader handles every declared type (an
	// allow-write bool silently read as "" would disable the guard's OFF state
	// in an undebuggable way).
	get := func(name string) string {
		if t.deps.GetConfig == nil {
			return ""
		}
		switch v := t.deps.GetConfig(name, session, tenant).(type) {
		case string:
			return v
		case bool:
			if v {
				return "true"
			}
			return "false"
		case float64:
			return fmt.Sprintf("%v", v)
		case nil:
			return ""
		default:
			return fmt.Sprintf("%v", v)
		}
	}
	return config.Parse(get)
}

// client resolves the easyops client, ensuring the URL is configured.
func (t *toolset) client(cfg config.Config) (ops.Client, error) {
	if cfg.URL == "" {
		return nil, &extension.TypedError{
			Code:    easybus.ToolResultErrorCodeInvalidArgument,
			Message: i18n.T("", i18n.MsgNotConfigured, i18n.Params{"name": config.KnobURL}),
		}
	}
	return t.deps.Cache.Get(ops.Endpoint{URL: cfg.URL, Token: cfg.Token}), nil
}

// guard enforces the namespace allow-list and (for writes) allow-write.
func (t *toolset) guard(cfg config.Config, ns string, write bool, locale string) error {
	if !cfg.NamespaceAllowed(ns) {
		return &extension.TypedError{
			Code:    easybus.ToolResultErrorCodePermissionDenied,
			Message: i18n.T(locale, i18n.MsgNamespaceDenied, i18n.Params{"ns": ns, "allowed": cfg.Namespace}),
		}
	}
	if write && !cfg.AllowWrite {
		return &extension.TypedError{
			Code:    easybus.ToolResultErrorCodePermissionDenied,
			Message: i18n.T(locale, i18n.MsgWriteDisabled, nil),
		}
	}
	return nil
}

// ---- services (read) --------------------------------------------------------

func (t *toolset) listServices(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	res, err := cl.ListServices(ctx, &opsv1.ListServicesRequest{Owner: tenant})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	rows := make([]map[string]any, 0, len(res.GetServices()))
	lines := make([]string, 0, len(res.GetServices()))
	for _, s := range res.GetServices() {
		rows = append(rows, map[string]any{
			"name": s.GetName(), "image": s.GetImage(), "phase": s.GetPhase(),
			"ready": s.GetReady(), "replicas": s.GetReplicas(),
			"readyReplicas": s.GetReadyReplicas(), "paused": s.GetPaused(),
			"url": s.GetUrl(), "message": s.GetMessage(), "restarts": s.GetRestarts(),
			"createdAt": s.GetCreatedAt(),
		})
		mark := " "
		switch {
		case s.GetReplicas() > 0 && s.GetReadyReplicas() == s.GetReplicas():
			mark = "*"
		case s.GetReadyReplicas() > 0:
			mark = "~" // partially ready: a rollout is still converging
		}
		paused := ""
		if s.GetPaused() {
			paused = " (paused)"
		}
		lines = append(lines, fmt.Sprintf("%s %-24s %-12s %d/%d ready%s %s",
			mark, s.GetName(), s.GetPhase(), s.GetReadyReplicas(), s.GetReplicas(), paused, s.GetImage()))
	}
	if len(lines) == 0 {
		return extension.ToolResultData{Content: i18n.T(locale, i18n.MsgNoServices, nil)}, nil
	}
	return extension.ToolResultData{Content: strings.Join(lines, "\n"), Data: map[string]any{"services": rows}}, nil
}

func (t *toolset) getService(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	name := argStr(args, "name")
	if name == "" {
		return extension.ToolResultData{}, errArg("name")
	}
	res, err := cl.GetService(ctx, &opsv1.GetServiceRequest{Owner: tenant, Name: name})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{Content: describeService(res.GetService()), Data: serviceMap(res.GetService())}, nil
}

func (t *toolset) serviceLogs(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	name := argStr(args, "name")
	if name == "" {
		return extension.ToolResultData{}, errArg("name")
	}
	res, err := cl.ServiceLogs(ctx, &opsv1.ServiceLogsRequest{
		Owner: tenant, Name: name,
		TailLines: int64(argInt(args, "tail", 200)),
		// previous=true reads the PREVIOUS container instance, which is the only
		// way to see why a CrashLoopBackOff pod died (the current container may
		// have produced no output at all).
		Previous: argBool(args, "previous", false),
	})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{
		Content: explainServiceLogs(res, argBool(args, "previous", false)),
		Data: map[string]any{
			"lines": res.GetLines(), "podPhase": res.GetPodPhase(),
			"restarts": res.GetRestarts(), "message": res.GetMessage(),
			"available": res.GetAvailable(),
		},
	}, nil
}

func (t *toolset) serviceManifest(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	name := argStr(args, "name")
	if name == "" {
		return extension.ToolResultData{}, errArg("name")
	}
	res, err := cl.GetServiceManifest(ctx, &opsv1.GetServiceManifestRequest{Owner: tenant, Name: name})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{Content: res.GetYaml()}, nil
}

// ---- storage / config (read) -----------------------------------------------

func (t *toolset) listPVCs(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	// include_all widens the view from "claims easyops created" to "every claim
	// in the namespace". Default false keeps the tenant-safe view (the
	// platform's postgres-data / garage-data are not a tenant's business), but
	// it also means a namespace full of claims can legitimately list "none" —
	// so the flag is exposed rather than hidden.
	includeAll := argBool(args, "include_all", false)
	res, err := cl.ListPVCs(ctx, &opsv1.ListPVCsRequest{Owner: tenant, IncludeAll: includeAll})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	lines := make([]string, 0, len(res.GetPvcs()))
	for _, p := range res.GetPvcs() {
		lines = append(lines, fmt.Sprintf("%-28s %-9s %-8s %s",
			p.GetName(), p.GetPhase(), p.GetSize(), pvcHint(p)))
	}
	if len(lines) == 0 {
		// An empty list is ambiguous on its own: say which view produced it and
		// how to widen it.
		if !includeAll {
			return extension.ToolResultData{Content: i18n.T(locale, i18n.MsgNoPVCsManaged, nil)}, nil
		}
		return extension.ToolResultData{Content: i18n.T(locale, i18n.MsgNone, nil)}, nil
	}
	return extension.ToolResultData{
		Content: strings.Join(lines, "\n"),
		Data:    map[string]any{"includeAll": includeAll, "count": len(lines)},
	}, nil
}

func (t *toolset) getPVC(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	name := argStr(args, "name")
	if name == "" {
		return extension.ToolResultData{}, errArg("name")
	}
	res, err := cl.GetPVC(ctx, &opsv1.GetPVCRequest{Owner: tenant, Name: name})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	p := res.GetPvc()
	return extension.ToolResultData{
		Content: describePVC(p),
		Data: map[string]any{
			"name": p.GetName(), "phase": p.GetPhase(),
			"size": p.GetSize(), "capacity": p.GetCapacity(),
			"storageClass": p.GetStorageClass(), "namespace": p.GetNamespace(),
			"boundVolume": p.GetBoundVolume(), "message": p.GetMessage(),
			"ageSeconds": p.GetAgeSeconds(), "mountedBy": p.GetMountedBy(),
		},
	}, nil
}

func (t *toolset) listConfigMaps(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	res, err := cl.ListConfigMaps(ctx, &opsv1.ListConfigMapsRequest{Owner: tenant})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	names := make([]string, 0, len(res.GetConfigmaps()))
	for _, c := range res.GetConfigmaps() {
		names = append(names, c.GetName())
	}
	sort.Strings(names)
	if len(names) == 0 {
		return extension.ToolResultData{Content: i18n.T(locale, i18n.MsgNone, nil)}, nil
	}
	return extension.ToolResultData{Content: strings.Join(names, "\n")}, nil
}

func (t *toolset) getConfigMap(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	name := argStr(args, "name")
	if name == "" {
		return extension.ToolResultData{}, errArg("name")
	}
	res, err := cl.GetConfigMap(ctx, &opsv1.GetConfigMapRequest{Owner: tenant, Name: name})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	if !res.GetFound() || res.GetConfigmap() == nil {
		return extension.ToolResultData{}, errNotFound(name)
	}
	keys := sortedKeys(res.GetConfigmap().GetData())
	return extension.ToolResultData{
		Content: fmt.Sprintf("configmap %s (%d keys): %s", name, len(keys), strings.Join(keys, ", ")),
		Data:    map[string]any{"name": name, "keys": keys},
	}, nil
}

// listSecrets reports NAMES ONLY. Secret data never leaves the server.
func (t *toolset) listSecrets(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	res, err := cl.ListSecrets(ctx, &opsv1.ListSecretsRequest{Owner: tenant})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	lines := make([]string, 0, len(res.GetSecrets()))
	for _, s := range res.GetSecrets() {
		// Keys only — never the values.
		keys := sortedKeys(s.GetData())
		lines = append(lines, fmt.Sprintf("%-28s keys=%d [%s]", s.GetName(), len(keys), strings.Join(keys, ", ")))
	}
	if len(lines) == 0 {
		return extension.ToolResultData{Content: i18n.T(locale, i18n.MsgNone, nil)}, nil
	}
	return extension.ToolResultData{Content: strings.Join(lines, "\n")}, nil
}

// getSecret reports the KEY NAMES and whether each is set — never the value.
func (t *toolset) getSecret(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	name := argStr(args, "name")
	if name == "" {
		return extension.ToolResultData{}, errArg("name")
	}
	res, err := cl.GetSecret(ctx, &opsv1.GetSecretRequest{Owner: tenant, Name: name})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	if !res.GetFound() || res.GetSecret() == nil {
		return extension.ToolResultData{}, errNotFound(name)
	}
	keys := sortedKeys(res.GetSecret().GetData())
	rows := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, map[string]any{"key": k, "set": true})
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgSecretKeysOnly, i18n.Params{"name": name, "keys": strings.Join(keys, ", ")}),
		Data:    map[string]any{"name": name, "keys": rows, "valuesWithheld": true},
	}, nil
}

// ---- jobs -------------------------------------------------------------------

func (t *toolset) createJob(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	ns := argStr(args, "namespace")
	if err := t.guard(cfg, ns, true, locale); err != nil {
		return extension.ToolResultData{}, err
	}
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	name := argStr(args, "name")
	image := argStr(args, "image")
	if name == "" || image == "" {
		return extension.ToolResultData{}, &extension.TypedError{
			Code:    easybus.ToolResultErrorCodeInvalidArgument,
			Message: "name and image are required",
		}
	}
	req := &opsv1.CreateJobRequest{
		Owner: tenant, Session: session, Name: name, Namespace: ns, Image: image,
		Command: argStrList(args, "command"), Args: argStrList(args, "args"),
		Env: argMap(args, "env"), Cpu: argStr(args, "cpu"), Memory: argStr(args, "memory"),
		RestartPolicy:           argStr(args, "restart_policy"),
		BackoffLimit:            int32(argInt(args, "backoff_limit", 0)),
		ActiveDeadlineSeconds:   int32(argInt(args, "active_deadline_seconds", 0)),
		TtlSecondsAfterFinished: int32(argInt(args, "ttl_seconds_after_finished", 0)),
		NodeSelector:            argMap(args, "node_selector"),
	}
	for _, v := range argVolumeMounts(args) {
		req.Volumes = append(req.Volumes, v)
	}
	res, err := cl.CreateJob(ctx, req)
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	j := res.GetJob()
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgJobCreated, i18n.Params{
			"name": j.GetName(), "state": j.GetState(), "exit": fmt.Sprintf("%d", j.GetExitCode()),
		}) + "\n" + describeJob(j),
		Data: jobMap(j),
	}, nil
}

func (t *toolset) listJobs(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	res, err := cl.ListJobs(ctx, &opsv1.ListJobsRequest{Owner: tenant})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	lines := make([]string, 0, len(res.GetJobs()))
	for _, j := range res.GetJobs() {
		lines = append(lines, describeJob(j))
	}
	if len(lines) == 0 {
		return extension.ToolResultData{Content: i18n.T(locale, i18n.MsgNoJobs, nil)}, nil
	}
	return extension.ToolResultData{Content: strings.Join(lines, "\n")}, nil
}

func (t *toolset) getJob(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	name := argStr(args, "name")
	if name == "" {
		return extension.ToolResultData{}, errArg("name")
	}
	res, err := cl.GetJob(ctx, &opsv1.GetJobRequest{Owner: tenant, Name: name})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{Content: describeJob(res.GetJob()), Data: jobMap(res.GetJob())}, nil
}

func (t *toolset) jobLogs(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	name := argStr(args, "name")
	if name == "" {
		return extension.ToolResultData{}, errArg("name")
	}
	res, err := cl.JobLogs(ctx, &opsv1.JobLogsRequest{
		Owner: tenant, Name: name, Container: argStr(args, "container"),
		TailLines: int32(argInt(args, "tail", 200)),
	})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{Content: res.GetLog()}, nil
}

func (t *toolset) deleteJob(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	if err := t.guard(cfg, "", true, locale); err != nil {
		return extension.ToolResultData{}, err
	}
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	name := argStr(args, "name")
	if name == "" {
		return extension.ToolResultData{}, errArg("name")
	}
	if _, err := cl.DeleteJob(ctx, &opsv1.DeleteJobRequest{Owner: tenant, Name: name, Propagation: argStr(args, "propagation")}); err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{Content: i18n.T(locale, i18n.MsgJobDeleted, i18n.Params{"name": name})}, nil
}

// ---- helpers ----------------------------------------------------------------

func errArg(name string) error {
	return &extension.TypedError{
		Code:    easybus.ToolResultErrorCodeInvalidArgument,
		Message: name + " is required",
	}
}

func errNotFound(name string) error {
	return &extension.TypedError{
		Code:    easybus.ToolResultErrorCodeNotFound,
		Message: "not found: " + name,
	}
}

// wrapErr maps a Connect error onto the extension's typed error.
func wrapErr(err error) error {
	return &extension.TypedError{
		Code:    easybus.ToolResultErrorCodeBusiness,
		Message: err.Error(),
	}
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// fmtInt renders an int32 for a message placeholder.
func fmtInt(n int32) string { return fmt.Sprintf("%d", n) }

// nowMillis is the clock used for elapsed-time rendering (unix millis).
func nowMillis() int64 { return time.Now().UnixMilli() }

// pvcHint is the one-line explanation appended to a list row.
//
// It exists because a PVC has only three phases and `Pending` covers BOTH "the
// provisioner is still working" and "provisioning FAILED". A bare "Pending" is
// therefore ambiguous, and an operator (or a model) has no way to tell a claim
// that needs 10 more seconds from one that will never bind.
func pvcHint(p *opsv1.PVCInfo) string {
	switch p.GetPhase() {
	case "Bound":
		if m := p.GetMountedBy(); len(m) > 0 {
			return "mounted by: " + strings.Join(m, ", ")
		}
		return ""
	case "Lost":
		return "LOST: the bound volume is gone — data is not reachable"
	}
	// Pending (or unknown): say WHY, plus how long it has been that way.
	msg := p.GetMessage()
	if msg == "" {
		msg = "(no event yet)"
	}
	if age := p.GetAgeSeconds(); age > 0 {
		return fmt.Sprintf("%s [pending %s]", msg, shortDuration(age))
	}
	return msg
}

// describePVC renders one claim in detail.
func describePVC(p *opsv1.PVCInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "pvc %s: phase=%s\n", p.GetName(), p.GetPhase())
	fmt.Fprintf(&b, "  size      : %s requested", p.GetSize())
	if c := p.GetCapacity(); c != "" {
		fmt.Fprintf(&b, ", %s allocated", c)
	}
	b.WriteString("\n")
	if sc := p.GetStorageClass(); sc != "" {
		fmt.Fprintf(&b, "  class     : %s\n", sc)
	}
	if ns := p.GetNamespace(); ns != "" {
		fmt.Fprintf(&b, "  namespace : %s\n", ns)
	}
	if v := p.GetBoundVolume(); v != "" {
		fmt.Fprintf(&b, "  volume    : %s\n", v)
	}
	if m := p.GetMountedBy(); len(m) > 0 {
		fmt.Fprintf(&b, "  mounted by: %s\n", strings.Join(m, ", "))
	}
	if age := p.GetAgeSeconds(); age > 0 {
		fmt.Fprintf(&b, "  age       : %s\n", shortDuration(age))
	}
	// The message is the whole point when the claim is not Bound.
	if msg := p.GetMessage(); msg != "" {
		fmt.Fprintf(&b, "  message   : %s\n", msg)
	} else if p.GetPhase() == "Pending" {
		b.WriteString("  message   : (no event yet — the provisioner has not reported anything)\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// shortDuration renders an age as "12s" / "4m30s" / "2h5m".
func shortDuration(seconds int64) string {
	d := time.Duration(seconds) * time.Second
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// explainServiceLogs renders a ServiceLogsResponse for a human (or a model).
//
// The DIAGNOSTIC HEADER is ALWAYS emitted, never gated on whether the log body
// is empty. Logs and diagnostics are DIFFERENT evidence from DIFFERENT k8s
// APIs (the pod log stream vs the pod status object), and each answers a
// question the other cannot:
//
//   - logs say WHAT the process did ("fatal: missing DATABASE_URL");
//   - the header says HOW the pod is doing (phase, restart count, and the first
//     unhealthy container's reason: CrashLoopBackOff / OOMKilled /
//     ImagePullBackOff).
//
// Gating the header on "no logs" hid it in the case it matters most: a crash
// looping container still prints its message before each death, so lines is
// non-empty while restarts climbs and reason explains the loop. Always showing
// both costs one line of noise on a healthy pod ("Running, restarts=0") and
// removes a whole class of "why does it keep dying?" confusion.
func explainServiceLogs(res *opsv1.ServiceLogsResponse, previous bool) string {
	var b strings.Builder

	// diagnostic header (always)
	b.WriteString("pod status:\n")
	phase := res.GetPodPhase()
	if phase == "" {
		phase = "unknown"
	}
	fmt.Fprintf(&b, "  phase    : %s\n", phase)
	fmt.Fprintf(&b, "  restarts : %d", res.GetRestarts())
	if res.GetRestarts() > 0 && !previous {
		b.WriteString("  (the container has died and been restarted)")
	}
	b.WriteString("\n")
	if m := res.GetMessage(); m != "" {
		fmt.Fprintf(&b, "  reason   : %s\n", m)
	}

	// log body
	body := strings.Join(res.GetLines(), "\n")
	label := "logs"
	if previous {
		label = "logs (PREVIOUS container instance)"
	}
	if strings.TrimSpace(body) == "" {
		b.WriteString("\n" + label + ": (no output from this container instance)")
		if !res.GetAvailable() {
			b.WriteString("\n  the log stream was NOT available for this container")
		}
		if !previous {
			b.WriteString("\n  a container that died produced its output in the PREVIOUS instance, " +
				"so re-run with `previous: true`")
		}
	} else {
		b.WriteString("\n" + label + ":\n" + body)
	}
	return b.String()
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func argVolumeMounts(args map[string]any) []*opsv1.JobVolumeMount {
	raw, ok := args["volumes"].([]any)
	if !ok {
		return nil
	}
	out := make([]*opsv1.JobVolumeMount, 0, len(raw))
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		claim, _ := m["claim"].(string)
		mount, _ := m["mount_path"].(string)
		if claim == "" || mount == "" {
			continue
		}
		name, _ := m["name"].(string)
		sub, _ := m["sub_path"].(string)
		ro, _ := m["read_only"].(bool)
		out = append(out, &opsv1.JobVolumeMount{
			Name: name, Claim: claim, MountPath: mount, SubPath: sub, ReadOnly: ro,
		})
	}
	return out
}

func describeService(s *opsv1.ServiceInfo) string {
	if s == nil {
		return ""
	}
	// READY/TOTAL is the single most useful line: "0/2" says the rollout has not
	// converged even when the phase looks calm.
	paused := ""
	if s.GetPaused() {
		paused = " [PAUSED]"
	}
	return fmt.Sprintf("service %s: phase=%s replicas=%d/%d (ready/total)%s image=%s url=%s%s",
		s.GetName(), s.GetPhase(), s.GetReadyReplicas(), s.GetReplicas(), paused,
		s.GetImage(), s.GetUrl(), cond(s.GetMessage(), " message="))
}

func cond(s, prefix string) string {
	if s == "" {
		return ""
	}
	return prefix + s
}

func serviceMap(s *opsv1.ServiceInfo) map[string]any {
	if s == nil {
		return nil
	}
	return map[string]any{
		"name": s.GetName(), "image": s.GetImage(), "phase": s.GetPhase(),
		"ready": s.GetReady(), "replicas": s.GetReplicas(),
		"readyReplicas": s.GetReadyReplicas(), "paused": s.GetPaused(),
		"url": s.GetUrl(), "message": s.GetMessage(), "restarts": s.GetRestarts(),
		"createdAt": s.GetCreatedAt(),
	}
}

func describeJob(j *opsv1.JobInfo) string {
	if j == nil {
		return ""
	}
	extra := ""
	if j.GetReason() != "" {
		extra = " reason=" + j.GetReason()
	}
	if j.GetMessage() != "" {
		extra += " message=" + j.GetMessage()
	}
	exit := ""
	if j.GetDone() {
		exit = fmt.Sprintf(" exit=%d", j.GetExitCode())
	}
	return fmt.Sprintf("job %s: %s%s image=%s%s", j.GetName(), j.GetState(), exit, j.GetImage(), extra)
}

func jobMap(j *opsv1.JobInfo) map[string]any {
	if j == nil {
		return nil
	}
	return map[string]any{
		"name": j.GetName(), "namespace": j.GetNamespace(), "image": j.GetImage(),
		"state": j.GetState(), "done": j.GetDone(),
		"succeeded": j.GetSucceeded(), "failed": j.GetFailed(), "active": j.GetActive(),
		"reason": j.GetReason(), "message": j.GetMessage(), "exitCode": j.GetExitCode(),
		"createdAt": j.GetCreatedAt(), "finishedAt": j.GetFinishedAt(),
	}
}
