// Package i18n localises the ops-ext RUNTIME tool text using the easybus SDK's
// catalog core.
package i18n

import (
	easybusi18n "github.com/easydev-org/easybus-sdk/i18n"
)

// Message keys (typed; a typo is a compile error at the call site).
const (
	MsgNotConfigured   = "notConfigured"
	MsgNamespaceDenied = "namespaceDenied"
	MsgWriteDisabled   = "writeDisabled"
	MsgNoServices      = "noServices"
	MsgNoJobs          = "noJobs"
	MsgNone            = "none"
	MsgImageBuilt      = "imageBuilt"
	MsgArchiveRequired = "archiveRequired"
	MsgJobCreated      = "jobCreated"
	MsgJobDeleted      = "jobDeleted"
	MsgSecretKeysOnly  = "secretKeysOnly"
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
}

// Params are the `{placeholder}` interpolation values.
type Params = easybusi18n.Params

var tr = easybusi18n.New(Catalog, "")

// T resolves a message key for a locale (” => the base locale).
func T(locale, key string, params Params) string {
	return tr.T(locale, key, params)
}
