// Package i18n localises the ops-ext RUNTIME tool text using the easybus SDK's
// catalog core.
package i18n

import (
	easybusi18n "github.com/easydev-org/easybus-sdk/i18n"
)

// Message keys (typed; a typo is a compile error at the call site).
const (
	MsgNotConfigured        = "notConfigured"
	MsgNamespaceDenied      = "namespaceDenied"
	MsgWriteDisabled        = "writeDisabled"
	MsgNoServices           = "noServices"
	MsgNoJobs               = "noJobs"
	MsgNone                 = "none"
	MsgImageBuilt           = "imageBuilt"
	MsgArchiveRequired      = "archiveRequired"
	MsgJobCreated           = "jobCreated"
	MsgJobDeleted           = "jobDeleted"
	MsgSecretKeysOnly       = "secretKeysOnly"
	MsgDeployed             = "deployed"
	MsgManifestApplied      = "manifestApplied"
	MsgManifestDryRunOK     = "manifestDryRunOK"
	MsgRolledBack           = "rolledBack"
	MsgRevisionN            = "revisionN"
	MsgScaled               = "scaled"
	MsgPaused               = "paused"
	MsgResumed              = "resumed"
	MsgBuildDone            = "buildDone"
	MsgBuildFailed          = "buildFailed"
	MsgImageImported        = "imageImported"
	MsgServiceDeleted       = "serviceDeleted"
	MsgServiceNotFound      = "serviceNotFound"
	MsgConfigMapPut         = "configMapPut"
	MsgConfigMapDeleted     = "configMapDeleted"
	MsgConfigMapDryRunOK    = "configMapDryRunOK"
	MsgPVCCreated           = "pvcCreated"
	MsgPVCDeleted           = "pvcDeleted"
	MsgSecretPut            = "secretPut"
	MsgSecretDeleted        = "secretDeleted"
	MsgSecretDryRunOK       = "secretDryRunOK"
	MsgSecretValuesReturned = "secretValuesReturned"
	MsgSandboxCreated       = "sandboxCreated"
	MsgSandboxDeleted       = "sandboxDeleted"
	MsgSandboxResolved      = "sandboxResolved"
	MsgNoSandboxes          = "noSandboxes"
	MsgNoPVCsManaged        = "noPVCsManaged"
	MsgBuildStarted         = "buildStarted"
	MsgBuildCanceled        = "buildCanceled"
	MsgBuildNotFound        = "buildNotFound"
	MsgNoBuilds             = "noBuilds"
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
	MsgServiceDeleted: {
		"en": "deleted service {name}",
		"zh": "已删除服务 {name}",
	},
	MsgServiceNotFound: {
		"en": "service {name} not found (nothing deleted)",
		"zh": "未找到服务 {name}（未删除任何东西）",
	},
	MsgConfigMapPut: {
		"en": "wrote configmap {name} ({n} keys: {keys}) — the object was REPLACED, so any key not listed is gone",
		"zh": "已写入 ConfigMap {name}（{n} 个键：{keys}）——该对象被整体替换，未列出的键已消失",
	},
	MsgConfigMapDeleted: {
		"en": "deleted configmap {name}",
		"zh": "已删除 ConfigMap {name}",
	},
	MsgConfigMapDryRunOK: {
		"en": "configmap {name} is VALID (dry run, nothing changed); keys: {keys}",
		"zh": "ConfigMap {name} 校验通过（dry run，未做改动）；键：{keys}",
	},
	MsgPVCCreated: {
		"en": "created PVC {name} ({size}, phase {phase})",
		"zh": "已创建 PVC {name}（{size}，状态 {phase}）",
	},
	MsgPVCDeleted: {
		"en": "deleted PVC {name} — its data is GONE",
		"zh": "已删除 PVC {name} —— 其数据已丢失",
	},
	MsgSecretPut: {
		"en": "wrote secret {name} ({keys}) — the object was REPLACED, so any key not listed is gone",
		"zh": "已写入机密 {name}（{keys}）——该对象被整体替换，未列出的键已消失",
	},
	MsgSecretDeleted: {
		"en": "deleted secret {name}",
		"zh": "已删除机密 {name}",
	},
	MsgSecretDryRunOK: {
		"en": "secret {name} is VALID (dry run, nothing changed); keys: {keys}",
		"zh": "机密 {name} 校验通过（dry run，未做改动）；键：{keys}",
	},
	MsgSecretValuesReturned: {
		"en": "secret {name}: SECRET VALUES (treat as sensitive)",
		"zh": "机密 {name}：以下是机密值（请按敏感信息处理）",
	},
	MsgSandboxCreated: {
		"en": "created sandbox {name}",
		"zh": "已创建沙箱 {name}",
	},
	MsgSandboxDeleted: {
		"en": "deleted sandbox {name}",
		"zh": "已删除沙箱 {name}",
	},
	MsgSandboxResolved: {
		"en": "sandbox {name} is reachable at {url} (token in data)",
		"zh": "沙箱 {name} 可通过 {url} 访问（token 在 data 中）",
	},
	MsgNoSandboxes: {
		"en": "no sandboxes",
		"zh": "没有沙箱",
	},
	MsgNoPVCsManaged: {
		"en": "no PVCs created by this deployment. This namespace may still have claims created outside it (postgres-data, garage-data, ...) - pass `include_all: true` to list every claim in the namespace.",
		"zh": "本部署没有创建过 PVC。该命名空间可能仍有外部创建的卷（postgres-data、garage-data 等）——传 `include_all: true` 可列出命名空间内的全部 PVC。",
	},
	MsgBuildStarted: {
		"en": "build {id} STARTED for {repo}:{tag} — it runs in the background; poll ops-get-build, or ops-cancel-build to stop it",
		"zh": "构建 {id} 已启动（{repo}:{tag}）——它在后台运行；用 ops-get-build 轮询，或 ops-cancel-build 取消",
	},
	MsgBuildCanceled: {
		"en": "build {id} is now {state}",
		"zh": "构建 {id} 当前状态：{state}",
	},
	MsgBuildNotFound: {
		"en": "build {id} not found (build records are process memory: a restart clears them, and each replica keeps its own set)",
		"zh": "未找到构建 {id}（构建记录存在进程内存中：重启会清空，且每个副本各自独立）",
	},
	MsgNoBuilds: {
		"en": "no builds (build records are process memory: a restart clears them, and each replica keeps its own set)",
		"zh": "没有构建记录（构建记录存在进程内存中：重启会清空，且每个副本各自独立）",
	},
}

// Params are the `{placeholder}` interpolation values.
type Params = easybusi18n.Params

var tr = easybusi18n.New(Catalog, "")

// T resolves a message key for a locale (” => the base locale).
func T(locale, key string, params Params) string {
	return tr.T(locale, key, params)
}
