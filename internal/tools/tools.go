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
		// image build
		"ops-build-image": t.buildImage,
		// storage / config (read)
		"ops-list-pvcs":       t.listPVCs,
		"ops-get-pvc":         t.getPVC,
		"ops-list-configmaps": t.listConfigMaps,
		"ops-get-configmap":   t.getConfigMap,
		"ops-list-secrets":    t.listSecrets,
		"ops-get-secret":      t.getSecret,
		// jobs (block 2)
		"ops-create-job": t.createJob,
		"ops-list-jobs":  t.listJobs,
		"ops-get-job":    t.getJob,
		"ops-job-logs":   t.jobLogs,
		"ops-delete-job": t.deleteJob,
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
	get := func(name string) string {
		if t.deps.GetConfig == nil {
			return ""
		}
		if s, ok := t.deps.GetConfig(name, session, tenant).(string); ok {
			return s
		}
		return ""
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
		if s.GetReady() {
			mark = "*"
		}
		paused := ""
		if s.GetPaused() {
			paused = " (paused)"
		}
		lines = append(lines, fmt.Sprintf("%s %-24s %s %d/%d%s %s",
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
	})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{Content: strings.Join(res.GetLines(), "\n")}, nil
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

// ---- image build ------------------------------------------------------------

func (t *toolset) buildImage(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	if err := t.guard(cfg, "", true, locale); err != nil {
		return extension.ToolResultData{}, err
	}
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	repo := argStr(args, "repo")
	if repo == "" {
		return extension.ToolResultData{}, errArg("repo")
	}
	tag := argStr(args, "tag")
	archive := argStr(args, "archive")
	if archive == "" {
		return extension.ToolResultData{}, &extension.TypedError{
			Code:    easybus.ToolResultErrorCodeInvalidArgument,
			Message: i18n.T(locale, i18n.MsgArchiveRequired, nil),
		}
	}
	res, err := cl.BuildImage(ctx, &opsv1.BuildImageRequest{
		Owner: tenant, Archive: []byte(archive), Dockerfile: orDefault(argStr(args, "dockerfile"), "Dockerfile"),
		Context: argStr(args, "context"), Repo: repo, Tag: tag, BuildArgs: argMap(args, "build_args"),
	})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgImageBuilt, i18n.Params{"ref": res.GetImageRef()}) + "\n" + res.GetLog(),
		Data:    map[string]any{"imageRef": res.GetImageRef()},
	}, nil
}

// ---- storage / config (read) -----------------------------------------------

func (t *toolset) listPVCs(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	res, err := cl.ListPVCs(ctx, &opsv1.ListPVCsRequest{Owner: tenant})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	lines := make([]string, 0, len(res.GetPvcs()))
	for _, p := range res.GetPvcs() {
		lines = append(lines, fmt.Sprintf("%-28s %-10s %s", p.GetName(), p.GetPhase(), p.GetSize()))
	}
	if len(lines) == 0 {
		return extension.ToolResultData{Content: i18n.T(locale, i18n.MsgNone, nil)}, nil
	}
	return extension.ToolResultData{Content: strings.Join(lines, "\n")}, nil
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
		Content: fmt.Sprintf("name=%s phase=%s size=%s class=%s mountedBy=%v",
			p.GetName(), p.GetPhase(), p.GetSize(), p.GetStorageClass(), p.GetMountedBy()),
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
	return fmt.Sprintf("service %s: phase=%s ready=%v %d/%d image=%s url=%s%s",
		s.GetName(), s.GetPhase(), s.GetReady(), s.GetReadyReplicas(), s.GetReplicas(),
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
