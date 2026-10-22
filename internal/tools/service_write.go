package tools

import (
	"context"

	easybus "github.com/easydev-org/easybus-sdk"
	"github.com/easydev-org/easybus-sdk/extension"

	opsv1 "github.com/easydev-org/easyagent-ops-ext/gen/easyops/v1"
	"github.com/easydev-org/easyagent-ops-ext/internal/i18n"
)

// deployService creates or replaces a service from structured fields.
func (t *toolset) deployService(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	req := &opsv1.DeployServiceRequest{
		Owner: tenant, Session: session, Name: name, Namespace: ns, Image: image,
		ContainerPort: int32(argInt(args, "container_port", 0)),
		ServicePort:   int32(argInt(args, "service_port", 0)),
		Replicas:      int32(argInt(args, "replicas", 1)),
		Env:           argMap(args, "env"),
		Cpu:           argStr(args, "cpu"),
		Memory:        argStr(args, "memory"),
		Command:       argStrList(args, "command"),
	}
	// DeployService blocks until the rollout settles, which for a real image is
	// tens of seconds to minutes. The RPC is a single call, so there is no stream
	// to forward — but the WAIT must still be visible, or the console shows
	// "running" with nothing behind it.
	EmitProgress(ctx, &easybus.ToolProgress{
		Phase: easybusStr("running"),
		Text:  easybusStr("deploying " + name + " (waiting for rollout)"),
	})
	res, err := cl.DeployService(ctx, req)
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	s := res.GetService()
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgDeployed, i18n.Params{"name": name}) + "\n" + describeService(s),
		Data:    serviceMap(s),
	}, nil
}

// applyManifest patches a service from an edited multi-document YAML. dry_run
// validates without changing anything — always do that first.
func (t *toolset) applyManifest(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	manifest := argStr(args, "yaml")
	if name == "" || manifest == "" {
		return extension.ToolResultData{}, &extension.TypedError{
			Code:    easybus.ToolResultErrorCodeInvalidArgument,
			Message: "name and yaml are required",
		}
	}
	dryRun := argBool(args, "dry_run", false)
	res, err := cl.ApplyServiceManifest(ctx, &opsv1.ApplyServiceManifestRequest{
		Owner: tenant, Name: name, Yaml: manifest, DryRun: dryRun,
	})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	if dryRun {
		return extension.ToolResultData{
			Content: i18n.T(locale, i18n.MsgManifestDryRunOK, i18n.Params{"name": name}) + "\n" + res.GetYaml(),
			Data:    map[string]any{"dryRun": true, "yaml": res.GetYaml()},
		}, nil
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgManifestApplied, i18n.Params{"name": name}) + "\n" + describeService(res.GetService()),
		Data:    serviceMap(res.GetService()),
	}, nil
}

// rollbackService undoes a Deployment to a prior revision (k8s `rollout undo`).
// revision=0 rolls back to the previous one.
func (t *toolset) rollbackService(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	rev := int32(argInt(args, "revision", 0))
	res, err := cl.RollbackService(ctx, &opsv1.RollbackServiceRequest{Owner: tenant, Name: name, Revision: rev})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	which := "previous revision"
	if rev > 0 {
		which = i18n.T(locale, i18n.MsgRevisionN, i18n.Params{"n": fmtInt(rev)})
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgRolledBack, i18n.Params{"name": name, "which": which}) + "\n" + describeService(res.GetService()),
		Data:    serviceMap(res.GetService()),
	}, nil
}

// scaleService sets the replica count.
func (t *toolset) scaleService(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	replicas := int32(argInt(args, "replicas", -1))
	if replicas < 0 {
		return extension.ToolResultData{}, errArg("replicas")
	}
	res, err := cl.ScaleService(ctx, &opsv1.ScaleServiceRequest{Owner: tenant, Name: name, Replicas: replicas})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgScaled, i18n.Params{"name": name, "n": fmtInt(replicas)}) + "\n" + describeService(res.GetService()),
		Data:    serviceMap(res.GetService()),
	}, nil
}

// pauseService scales to zero, remembering the count Resume restores.
func (t *toolset) pauseService(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	res, err := cl.PauseService(ctx, &opsv1.PauseServiceRequest{Owner: tenant, Name: name})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgPaused, i18n.Params{"name": name}) + "\n" + describeService(res.GetService()),
		Data:    serviceMap(res.GetService()),
	}, nil
}

// resumeService restores the replica count Pause remembered.
func (t *toolset) resumeService(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	res, err := cl.ResumeService(ctx, &opsv1.ResumeServiceRequest{Owner: tenant, Name: name})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgResumed, i18n.Params{"name": name}) + "\n" + describeService(res.GetService()),
		Data:    serviceMap(res.GetService()),
	}, nil
}
