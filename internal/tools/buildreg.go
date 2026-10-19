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

// Asynchronous builds: CreateBuild returns an id immediately, so a caller can
// list, inspect and cancel. This is what `ops-build-image` cannot do — that one
// blocks until the build finishes and leaves nothing behind.

// describeBuild renders one build for a human (or a model).
func describeBuild(b *opsv1.BuildRecord) string {
	if b == nil {
		return ""
	}
	var parts []string
	parts = append(parts, fmt.Sprintf("%s %s:%s", b.GetId(), b.GetRepo(), b.GetTag()))
	parts = append(parts, b.GetState())
	if d := buildDuration(b); d != "" {
		parts = append(parts, d)
	}
	line := strings.Join(parts, "  ")
	if b.GetImage() != "" {
		line += "\n    image: " + b.GetImage()
	}
	if e := b.GetError(); e != "" {
		line += "\n    error: " + firstLine(e)
	}
	return line
}

// buildDuration renders "3s" for a finished build, or the elapsed time for a
// running one — a running build that has been going for 20 minutes is a very
// different situation from one that started 2 seconds ago.
func buildDuration(b *opsv1.BuildRecord) string {
	if b.GetStartedAt() == 0 {
		return ""
	}
	end := b.GetFinishedAt()
	if end == 0 {
		end = nowMillis()
		return "elapsed " + shortDuration((end-b.GetStartedAt())/1000)
	}
	return "took " + shortDuration((end-b.GetStartedAt())/1000)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// createBuild starts a build in the background and returns its id.
func (t *toolset) createBuild(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
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
	archive, err := buildArchive(args)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	req := &opsv1.CreateBuildRequest{
		Owner: tenant, Session: session, Archive: archive,
		Dockerfile: orDefault(argStr(args, "dockerfile"), "Dockerfile"),
		Context:    argStr(args, "context"),
		Repo:       repo,
		Tag:        orDefault(argStr(args, "tag"), "latest"),
		BuildArgs:  argMap(args, "build_args"),
	}
	if argBool(args, "cache", true) {
		req.ImageCache = &opsv1.CacheOptions{
			Enabled: true, Repo: argStr(args, "cache_repo"),
			Tag:        argStr(args, "cache_tag"),
			Mode:       orDefault(argStr(args, "cache_mode"), "max"),
			ImportOnly: argBool(args, "cache_import_only", false),
		}
	}
	res, err := cl.CreateBuild(ctx, req)
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	b := res.GetBuild()
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgBuildStarted, i18n.Params{"id": b.GetId(), "repo": repo, "tag": req.GetTag()}) +
			"\n" + describeBuild(b),
		Data: buildMap(b),
	}, nil
}

// listBuilds lists builds, newest first.
func (t *toolset) listBuilds(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	res, err := cl.ListBuilds(ctx, &opsv1.ListBuildsRequest{
		Owner: tenant, Limit: int32(argInt(args, "limit", 50)),
	})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	if len(res.GetBuilds()) == 0 {
		return extension.ToolResultData{Content: i18n.T(locale, i18n.MsgNoBuilds, nil)}, nil
	}
	lines := make([]string, 0, len(res.GetBuilds()))
	rows := make([]map[string]any, 0, len(res.GetBuilds()))
	for _, b := range res.GetBuilds() {
		lines = append(lines, describeBuild(b))
		rows = append(rows, buildMap(b))
	}
	return extension.ToolResultData{
		Content: strings.Join(lines, "\n"),
		Data: map[string]any{
			"builds": rows,
			// Say it once, plainly: these records do not survive a restart.
			"durability": res.GetBuilds()[0].GetDurability(),
		},
	}, nil
}

// getBuild shows one build, including its log tail.
func (t *toolset) getBuild(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	id := argStr(args, "id")
	if id == "" {
		return extension.ToolResultData{}, errArg("id")
	}
	res, err := cl.GetBuild(ctx, &opsv1.GetBuildRequest{Owner: tenant, Id: id})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	if !res.GetFound() {
		return extension.ToolResultData{}, &extension.TypedError{
			Code:    easybus.ToolResultErrorCodeNotFound,
			Message: i18n.T(locale, i18n.MsgBuildNotFound, i18n.Params{"id": id}),
		}
	}
	b := res.GetBuild()
	body := describeBuild(b)
	if log := strings.TrimSpace(b.GetLog()); log != "" {
		body += "\n  log (tail):\n" + indent(log, "    ")
	}
	return extension.ToolResultData{Content: body, Data: buildMap(b)}, nil
}

// cancelBuild stops a running build.
func (t *toolset) cancelBuild(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	if err := t.guard(cfg, "", true, locale); err != nil {
		return extension.ToolResultData{}, err
	}
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	id := argStr(args, "id")
	if id == "" {
		return extension.ToolResultData{}, errArg("id")
	}
	res, err := cl.CancelBuild(ctx, &opsv1.CancelBuildRequest{Owner: tenant, Id: id})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	if !res.GetFound() {
		return extension.ToolResultData{}, &extension.TypedError{
			Code:    easybus.ToolResultErrorCodeNotFound,
			Message: i18n.T(locale, i18n.MsgBuildNotFound, i18n.Params{"id": id}),
		}
	}
	b := res.GetBuild()
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgBuildCanceled, i18n.Params{"id": id, "state": b.GetState()}) +
			"\n" + describeBuild(b),
		Data: buildMap(b),
	}, nil
}

func buildMap(b *opsv1.BuildRecord) map[string]any {
	if b == nil {
		return nil
	}
	return map[string]any{
		"id": b.GetId(), "repo": b.GetRepo(), "tag": b.GetTag(),
		"state": b.GetState(), "image": b.GetImage(), "error": b.GetError(),
		"startedAt": b.GetStartedAt(), "finishedAt": b.GetFinishedAt(),
		"cacheUsed": b.GetCacheUsed(), "durability": b.GetDurability(),
	}
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}
