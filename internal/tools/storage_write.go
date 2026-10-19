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

// ---- PVC ----

// createPVC provisions a persistent volume claim.
func (t *toolset) createPVC(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	if name == "" {
		return extension.ToolResultData{}, errArg("name")
	}
	res, err := cl.CreatePVC(ctx, &opsv1.CreatePVCRequest{
		Owner: tenant, Name: name, Namespace: ns,
		Size:         orDefault(argStr(args, "size"), ""),
		StorageClass: argStr(args, "storage_class"),
	})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	p := res.GetPvc()
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgPVCCreated, i18n.Params{
			"name": name, "size": p.GetSize(), "phase": p.GetPhase()}),
		Data: map[string]any{
			"name": p.GetName(), "size": p.GetSize(),
			"phase": p.GetPhase(), "storageClass": p.GetStorageClass(),
		},
	}, nil
}

// deletePVC removes a persistent volume claim. This DESTROYS the data on it.
func (t *toolset) deletePVC(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	res, err := cl.DeletePVC(ctx, &opsv1.DeletePVCRequest{Owner: tenant, Name: name})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgPVCDeleted, i18n.Params{"name": name}),
		Data:    map[string]any{"name": name, "deleted": res.GetDeleted()},
	}, nil
}

// ---- Secrets ----

// getSecretValues returns a secret's FULL contents.
//
// This is deliberately separate from ops-get-secret, which is key-only and
// stays the default: reading secret values is a privileged act. It exists
// because without it an agent cannot perform the operations that need a
// credential (inject a token into a job, copy a secret into a new namespace) —
// and ops-put-secret REPLACES the object, so writing blind would destroy keys.
// The tool says plainly that it is returning secrets.
func (t *toolset) getSecretValues(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	data := res.GetSecret().GetData()
	keys := sortedKeys(data)
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%d keys)\n", i18n.T(locale, i18n.MsgSecretValuesReturned, i18n.Params{"name": name, "n": fmt.Sprintf("%d", len(keys))}), len(keys))
	for _, k := range keys {
		fmt.Fprintf(&b, "--- %s ---\n%s\n", k, data[k])
	}
	return extension.ToolResultData{
		Content: b.String(),
		Data:    map[string]any{"name": name, "data": data, "keys": keys, "containsSecrets": true},
	}, nil
}

// putSecret creates or REPLACES a secret.
func (t *toolset) putSecret(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	if name == "" {
		return extension.ToolResultData{}, errArg("name")
	}
	data := argMap(args, "data")
	if len(data) == 0 {
		return extension.ToolResultData{}, &extension.TypedError{
			Code: easybus.ToolResultErrorCodeInvalidArgument,
			Message: "`data` is required: a secret write REPLACES the whole object, " +
				"so supply every key you want to keep",
		}
	}
	dryRun := argBool(args, "dry_run", false)
	res, err := cl.PutSecret(ctx, &opsv1.PutSecretRequest{
		Owner: tenant, Name: name, Namespace: ns, Data: data, DryRun: dryRun,
	})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	// Report KEY NAMES only — never echo the values back.
	keys := sortedKeys(res.GetSecret().GetData())
	if dryRun {
		return extension.ToolResultData{
			Content: i18n.T(locale, i18n.MsgSecretDryRunOK, i18n.Params{
				"name": name, "keys": strings.Join(keys, ", ")}),
			Data: map[string]any{"dryRun": true, "name": name, "keys": keys},
		}, nil
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgSecretPut, i18n.Params{
			"name": name, "keys": strings.Join(keys, ", ")}),
		Data: map[string]any{"name": name, "keys": keys},
	}, nil
}

// deleteSecret removes a secret.
func (t *toolset) deleteSecret(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	res, err := cl.DeleteSecret(ctx, &opsv1.DeleteSecretRequest{Owner: tenant, Name: name})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgSecretDeleted, i18n.Params{"name": name}),
		Data:    map[string]any{"name": name, "deleted": res.GetDeleted()},
	}, nil
}

// errArg is reused from tools.go; keep this file's imports honest.
var _ = easybus.ToolResultErrorCodeInvalidArgument
