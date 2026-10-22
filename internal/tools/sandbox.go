package tools

import (
	"context"
	"fmt"
	"strings"

	easybus "github.com/easydev-org/easybus-sdk"
	"github.com/easydev-org/easybus-sdk/extension"

	opsv1 "github.com/easydev-org/easyagent-ops-ext/gen/easyops/v1"
	"github.com/easydev-org/easyagent-ops-ext/internal/i18n"
)

// Sandbox lifecycle through easyops.
//
// NOTE the division of labour with sandbox-ext: sandbox-ext works INSIDE an
// already-running sandbox (exec, files, jobs, computer-*). It has no tool to
// CREATE, LIST, RESOLVE or DELETE a sandbox — that is easyops's job, and it was
// the missing half of "give the agent a working machine".

func sandboxVolumes(args map[string]any) []*opsv1.VolumeMountSpec {
	raw, ok := args["volumes"].([]any)
	if !ok {
		return nil
	}
	out := make([]*opsv1.VolumeMountSpec, 0, len(raw))
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		pvc, _ := m["pvc"].(string)
		mount, _ := m["mount_path"].(string)
		if pvc == "" || mount == "" {
			continue
		}
		sub, _ := m["sub_path"].(string)
		ro, _ := m["read_only"].(bool)
		out = append(out, &opsv1.VolumeMountSpec{Pvc: pvc, MountPath: mount, ReadOnly: ro, SubPath: sub})
	}
	return out
}

func describeSandbox(s *opsv1.Sandbox) string {
	if s == nil {
		return ""
	}
	ready := ""
	if s.GetReady() {
		ready = " [READY]"
	}
	return fmt.Sprintf("sandbox %s: phase=%s%s image=%s url=%s",
		s.GetName(), s.GetPhase(), ready, s.GetImage(), s.GetUrl())
}

// createSandbox starts a worker sandbox and returns its URL.
func (t *toolset) createSandbox(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	// Creating a sandbox is Pod + Service + Secret and then waiting for the Pod to
	// become ready — up to ready_timeout_seconds. Show the wait.
	EmitProgress(ctx, &easybus.ToolProgress{
		Phase: easybusStr("running"),
		Text:  easybusStr("creating sandbox " + name + " and waiting for it to become ready"),
	})
	res, err := cl.CreateSandbox(ctx, &opsv1.CreateSandboxRequest{
		Owner: tenant, Session: session, Name: name,
		Image:               argStr(args, "image"),
		Cpu:                 argStr(args, "cpu"),
		Memory:              argStr(args, "memory"),
		Env:                 argMap(args, "env"),
		Kvm:                 argBool(args, "kvm", false),
		GpuCount:            int32(argInt(args, "gpu_count", 0)),
		ReadyTimeoutSeconds: int32(argInt(args, "ready_timeout_seconds", 0)),
		GoldenDisk:          argBool(args, "golden_disk", false),
		Volumes:             sandboxVolumes(args),
	})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	s := res.GetSandbox()
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgSandboxCreated, i18n.Params{"name": name}) + "\n" + describeSandbox(s),
		Data:    sandboxMap(s),
	}, nil
}

// listSandboxes lists the caller's sandboxes.
func (t *toolset) listSandboxes(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	res, err := cl.ListSandboxes(ctx, &opsv1.ListSandboxesRequest{Owner: tenant})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	if len(res.GetSandboxes()) == 0 {
		return extension.ToolResultData{Content: i18n.T(locale, i18n.MsgNoSandboxes, nil)}, nil
	}
	lines := make([]string, 0, len(res.GetSandboxes()))
	for _, s := range res.GetSandboxes() {
		lines = append(lines, describeSandbox(s))
	}
	return extension.ToolResultData{Content: strings.Join(lines, "\n")}, nil
}

// getSandbox shows one sandbox.
func (t *toolset) getSandbox(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	name := argStr(args, "name")
	if name == "" {
		return extension.ToolResultData{}, errArg("name")
	}
	res, err := cl.GetSandbox(ctx, &opsv1.GetSandboxRequest{Owner: tenant, Name: name})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{Content: describeSandbox(res.GetSandbox()), Data: sandboxMap(res.GetSandbox())}, nil
}

// resolveSandbox returns the sandbox's in-cluster URL + token, i.e. what you
// need to actually talk to it.
func (t *toolset) resolveSandbox(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	res, err := cl.ResolveSandbox(ctx, &opsv1.ResolveSandboxRequest{Owner: tenant, Name: name})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgSandboxResolved, i18n.Params{"name": name, "url": res.GetUrl()}),
		// The token is a sandbox-scoped credential, not a cluster secret; without
		// it the URL is unusable.
		Data: map[string]any{"name": name, "url": res.GetUrl(), "token": res.GetToken()},
	}, nil
}

// deleteSandbox stops and removes a sandbox.
func (t *toolset) deleteSandbox(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	if _, err := cl.DeleteSandbox(ctx, &opsv1.DeleteSandboxRequest{Owner: tenant, Name: name}); err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgSandboxDeleted, i18n.Params{"name": name}),
		Data:    map[string]any{"name": name, "deleted": true},
	}, nil
}

// sandboxLogs reads a sandbox's pod logs. Unlike sandbox-ext (which works
// INSIDE the worker), this is the only way to see why a sandbox FAILED to start.
func (t *toolset) sandboxLogs(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	name := argStr(args, "name")
	if name == "" {
		return extension.ToolResultData{}, errArg("name")
	}
	res, err := cl.SandboxLogs(ctx, &opsv1.SandboxLogsRequest{
		Owner: tenant, Name: name,
		TailLines: int64(argInt(args, "tail", 200)),
		Previous:  argBool(args, "previous", false),
	})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	// Logs and diagnostics are different evidence (see explainServiceLogs): a
	// sandbox that never started has no stdout, and only the pod status
	// explains why.
	body := strings.Join(res.GetLines(), "\n")
	if strings.TrimSpace(body) == "" {
		body = fmt.Sprintf("(no log output)\n  pod phase: %s\n  restarts : %d",
			res.GetPodPhase(), res.GetRestarts())
		if m := res.GetMessage(); m != "" {
			body += "\n  reason   : " + m
		}
	}
	return extension.ToolResultData{
		Content: body,
		Data: map[string]any{
			"lines": res.GetLines(), "podPhase": res.GetPodPhase(),
			"restarts": res.GetRestarts(), "message": res.GetMessage(),
			"available": res.GetAvailable(),
		},
	}, nil
}

func sandboxMap(s *opsv1.Sandbox) map[string]any {
	if s == nil {
		return nil
	}
	return map[string]any{
		"name": s.GetName(), "image": s.GetImage(), "phase": s.GetPhase(),
		"ready": s.GetReady(), "url": s.GetUrl(),
	}
}
