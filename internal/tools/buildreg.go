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

// getBuild shows one build plus a paged, line-numbered window of its log.
//
// The log is paged by LINE (offset/limit), mirroring sandbox-file-read: a log is
// read top-down, and a line number is what a reader can act on ("look at line
// 812"). Every line is prefixed with its number so a long log stays navigable.
//
// Default page: the first 200 lines. That is the OPPOSITE of a tail — and it is
// deliberate: the record keeps the HEAD of the log (see easyops buildreg), so
// offset 0 is line 1 and paging through the whole log stays coherent. Use
// offset to walk to the end (where a failure's exit status usually is).
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
	offset := argInt(args, "offset", 0)
	limit := argInt(args, "limit", 0)
	if offset < 0 {
		offset = 0
	}
	res, err := cl.GetBuild(ctx, &opsv1.GetBuildRequest{
		Owner: tenant, Id: id,
		LogOffset: int32(offset), LogLimit: int32(limit),
	})
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
	var sb strings.Builder
	sb.WriteString(describeBuild(b))
	sb.WriteString("\n")
	sb.WriteString(renderLogPage(res))
	data := buildMap(b)
	data["logLines"] = res.GetLogLines()
	data["firstLine"] = res.GetFirstLine()
	data["totalLines"] = res.GetTotalLines()
	data["more"] = res.GetMore()
	return extension.ToolResultData{Content: sb.String(), Data: data}, nil
}

// logPageData is the paging metadata a caller needs to ask for the next window.
type logPageData struct {
	FirstLine  int
	TotalLines int
	Shown      int
	More       bool
	Truncated  bool
	Dropped    bool
}

// It is returned separately so the tool can both render and hand back Data.
func logPage(res *opsv1.GetBuildResponse) logPageData {
	return logPageData{
		FirstLine:  int(res.GetFirstLine()),
		TotalLines: int(res.GetTotalLines()),
		Shown:      len(res.GetLogLines()),
		More:       res.GetMore(),
		Truncated:  res.GetBuild().GetLogTruncated(),
		Dropped:    res.GetBuild().GetLogDropped(),
	}
}

// renderLogPage prints a line-numbered window plus the paging footer.
//
// Line numbers are right-aligned to the width of the LAST line in the page, so
// the numbers form a column whether the page holds lines 8-12 or 9,981-9,995.
func renderLogPage(res *opsv1.GetBuildResponse) string {
	p := logPage(res)
	var b strings.Builder

	switch {
	case p.Dropped:
		b.WriteString("log: (released to stay within the memory budget — this build finished long ago;\n")
		b.WriteString("      its status and timing are still available, the log body is not)")
		return b.String()
	case p.TotalLines == 0 && p.Shown == 0:
		b.WriteString("log: (empty — the build produced no output)")
		return b.String()
	}

	last := p.FirstLine + p.Shown - 1
	width := len(fmt.Sprintf("%d", last))
	for i, line := range res.GetLogLines() {
		fmt.Fprintf(&b, "%*d: %s\n", width, p.FirstLine+i, line)
	}

	// Footer: always state the window, and how to get the rest.
	if p.Shown > 0 {
		fmt.Fprintf(&b, "--- lines %d-%d of %d ---\n", p.FirstLine, last, p.TotalLines)
	}
	if p.More {
		fmt.Fprintf(&b, "more lines available: re-run with offset=%d (limit up to %d)\n",
			last, buildLogMaxLines)
	}
	if p.Truncated {
		fmt.Fprintf(&b, "NOTE: the build output exceeded the 1 MiB record limit, so lines after the cap\n")
		fmt.Fprintf(&b, "      are NOT stored at all — do not read this as the complete log.\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// buildLogMaxLines mirrors easyops's per-page cap, so the "more lines" hint never
// suggests a limit the server will silently reduce.
const buildLogMaxLines = 1000

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
