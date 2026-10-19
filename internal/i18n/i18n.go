// Package i18n localises the ops-ext RUNTIME tool text using the easybus SDK's
// catalog core.
package i18n

import (
	easybusi18n "github.com/easydev-org/easybus-sdk/i18n"
)

// Message keys (typed; a typo is a compile error at the call site).
const (
	MsgNotConfigured    = "notConfigured"
	MsgNamespaceDenied  = "namespaceDenied"
	MsgWriteDisabled    = "writeDisabled"
	MsgNoServices       = "noServices"
	MsgNoJobs           = "noJobs"
	MsgNone             = "none"
	MsgImageBuilt       = "imageBuilt"
	MsgArchiveRequired  = "archiveRequired"
	MsgJobCreated       = "jobCreated"
	MsgJobDeleted       = "jobDeleted"
	MsgSecretKeysOnly   = "secretKeysOnly"
	MsgDeployed         = "deployed"
	MsgManifestApplied  = "manifestApplied"
	MsgManifestDryRunOK = "manifestDryRunOK"
	MsgRolledBack       = "rolledBack"
	MsgRevisionN        = "revisionN"
	MsgScaled           = "scaled"
	MsgPaused           = "paused"
	MsgResumed          = "resumed"
	MsgBuildDone        = "buildDone"
	MsgBuildFailed      = "buildFailed"
	MsgImageImported    = "imageImported"
)

// Catalog is the message table (open locale set: add a column per language).
var Catalog = easybusi18n.Catalog{
	MsgNotConfigured: {
		"en": "the `{name}` config is not set",
		"zh": "尚未设置 `{name}` 配置。",
	},
	MsgNamespaceDenied: {
		"en": "namespace {ns} is not allowed (this extension may only touch `{allowed}`)",
		"zh": "不允许访问命名空间 {ns}（本扩展只允许操作 `{allowed}`）。",
	},
	MsgWriteDisabled: {
		"en": "write operations are disabled (set `allow-write=true` to enable them)",
		"zh": "写操作已禁用（设置 `allow-write=true` 以启用）。",
	},
	MsgNoServices: {
		"en": "no services",
		"zh": "没有服务",
	},
	MsgNoJobs: {
		"en": "no jobs",
		"zh": "没有任务",
	},
	MsgNone: {
		"en": "none",
		"zh": "无",
	},
	MsgImageBuilt: {
		"en": "built {ref}",
		"zh": "已构建 {ref}",
	},
	MsgArchiveRequired: {
		"en": "`archive` is required: the repo tarball (tar.gz) to build from",
		"zh": "需要 `archive`：用于构建的仓库 tarball（tar.gz）。",
	},
	MsgJobCreated: {
		"en": "job {name}: {state} (exit {exit})",
		"zh": "任务 {name}：{state}（退出码 {exit}）",
	},
	MsgJobDeleted: {
		"en": "deleted job {name}",
		"zh": "已删除任务 {name}",
	},
	MsgSecretKeysOnly: {
		"en": "secret {name}: keys only (values are never returned): {keys}",
		"zh": "机密 {name}：仅键名（永不返回值）：{keys}",
	},
	MsgDeployed: {
		"en": "deployed {name}",
		"zh": "已部署 {name}",
	},
	MsgManifestApplied: {
		"en": "applied manifest to {name}",
		"zh": "已将 manifest 应用到 {name}",
	},
	MsgManifestDryRunOK: {
		"en": "manifest for {name} is VALID (dry run, nothing changed)",
		"zh": "{name} 的 manifest 校验通过（dry run，未做任何改动）",
	},
	MsgRolledBack: {
		"en": "rolled {name} back to {which}",
		"zh": "已将 {name} 回滚到 {which}",
	},
	MsgRevisionN: {
		"en": "revision {n}",
		"zh": "版本 {n}",
	},
	MsgScaled: {
		"en": "scaled {name} to {n} replica(s)",
		"zh": "已将 {name} 扩缩到 {n} 个副本",
	},
	MsgPaused: {
		"en": "paused {name}",
		"zh": "已暂停 {name}",
	},
	MsgResumed: {
		"en": "resumed {name}",
		"zh": "已恢复 {name}",
	},
	MsgBuildDone: {
		"en": "build FINISHED in {elapsed}: {ref}",
		"zh": "构建完成，耗时 {elapsed}：{ref}",
	},
	MsgBuildFailed: {
		"en": "build FAILED: {err}",
		"zh": "构建失败：{err}",
	},
	MsgImageImported: {
		"en": "imported {ref}",
		"zh": "已导入 {ref}",
	},
}

// Params are the `{placeholder}` interpolation values.
type Params = easybusi18n.Params

var tr = easybusi18n.New(Catalog, "")

// T resolves a message key for a locale (” => the base locale).
func T(locale, key string, params Params) string {
	return tr.T(locale, key, params)
}
