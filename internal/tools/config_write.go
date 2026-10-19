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

// deleteService removes a service and its Service object.
//
// It is the counterpart of ops-deploy-service: an agent that can create a
// service but never remove one leaves orphaned workloads behind, and the
// alternative (leave it to a human) is exactly the toil this extension exists
// to remove. It is gated by allow-write like every other mutation.
func (t *toolset) deleteService(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	res, err := cl.DeleteService(ctx, &opsv1.DeleteServiceRequest{Owner: tenant, Name: name})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	if !res.GetDeleted() {
		return extension.ToolResultData{
			Content: i18n.T(locale, i18n.MsgServiceNotFound, i18n.Params{"name": name}),
			Data:    map[string]any{"deleted": false, "name": name},
		}, nil
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgServiceDeleted, i18n.Params{"name": name}),
		Data:    map[string]any{"deleted": true, "name": name},
	}, nil
}

// putConfigMap creates or replaces a config map.
//
// `data` is REPLACED wholesale, not merged: a Kubernetes config map update is a
// full object write, so a key omitted from the request disappears. To change
// one key, read the current data with ops-get-configmap-keys... which does not
// exist, so the caller must supply the complete map. We say so explicitly here
// and in the manifest rather than silently dropping keys.
func (t *toolset) putConfigMap(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
			Message: "`data` is required: a config map write REPLACES the whole object, " +
				"so supply every key you want to keep",
		}
	}
	dryRun := argBool(args, "dry_run", false)
	res, err := cl.PutConfigMap(ctx, &opsv1.PutConfigMapRequest{
		Owner: tenant, Name: name, Namespace: ns, Data: data, DryRun: dryRun,
	})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	keys := sortedKeys(res.GetConfigmap().GetData())
	if dryRun {
		return extension.ToolResultData{
			Content: i18n.T(locale, i18n.MsgConfigMapDryRunOK, i18n.Params{
				"name": name, "keys": strings.Join(keys, ", ")}),
			Data: map[string]any{"dryRun": true, "name": name, "keys": keys},
		}, nil
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgConfigMapPut, i18n.Params{
			"name": name, "n": fmt.Sprintf("%d", len(keys)), "keys": strings.Join(keys, ", ")}),
		Data: map[string]any{"name": name, "keys": keys},
	}, nil
}

// deleteConfigMap removes a config map.
func (t *toolset) deleteConfigMap(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	res, err := cl.DeleteConfigMap(ctx, &opsv1.DeleteConfigMapRequest{Owner: tenant, Name: name})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgConfigMapDeleted, i18n.Params{"name": name}),
		Data:    map[string]any{"deleted": res.GetDeleted(), "name": name},
	}, nil
}

// getConfigMapValues returns a config map's FULL contents (keys AND values).
//
// ConfigMap data is not secret by definition (secrets live in Secret objects),
// and an agent that can only see the key NAMES cannot actually use a config
// map — it cannot read a config file it is about to patch. ops-get-configmap
// stays key-only for a quick inventory; this tool is the one that returns
// values.
func (t *toolset) getConfigMapValues(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	data := res.GetConfigmap().GetData()
	keys := sortedKeys(data)
	var b strings.Builder
	fmt.Fprintf(&b, "configmap %s (%d keys):\n", name, len(keys))
	for _, k := range keys {
		v := data[k]
		// Long values (an embedded config file) are shown in full: that is the
		// point of this tool.
		fmt.Fprintf(&b, "--- %s ---\n%s\n", k, strings.TrimRight(v, "\n"))
	}
	return extension.ToolResultData{
		Content: b.String(),
		Data:    map[string]any{"name": name, "data": data, "keys": keys},
	}, nil
}
