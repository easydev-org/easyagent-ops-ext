package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	easybus "github.com/easydev-org/easybus-sdk"
	"github.com/easydev-org/easybus-sdk/extension"

	opsv1 "github.com/easydev-org/easyagent-ops-ext/gen/easyops/v1"
	"github.com/easydev-org/easyagent-ops-ext/internal/i18n"
)

// buildLogLimit bounds the log the tool returns (the stream can be megabytes).
const buildLogLimit = 12000

// buildArgs maps the tool arguments onto a BuildImageRequest.
//
// `archive` is the repo tarball as TEXT. Tarballs are binary, so in practice a
// caller base64-encodes it; `archive_base64` says so explicitly and we decode
// it here (a raw tar can contain NULs and would not survive a JSON string).
func (t *toolset) buildRequest(args map[string]any, tenant, session string) (*opsv1.BuildImageRequest, error) {
	repo := argStr(args, "repo")
	if repo == "" {
		return nil, errArg("repo")
	}
	archive, err := buildArchive(args)
	if err != nil {
		return nil, err
	}
	req := &opsv1.BuildImageRequest{
		Owner: tenant, Archive: archive,
		Dockerfile: orDefault(argStr(args, "dockerfile"), "Dockerfile"),
		Context:    argStr(args, "context"),
		Repo:       repo,
		Tag:        orDefault(argStr(args, "tag"), "latest"),
		BuildArgs:  argMap(args, "build_args"),
	}
	if argBool(args, "cache", true) {
		req.ImageCache = &opsv1.CacheOptions{
			Enabled:    true,
			Repo:       argStr(args, "cache_repo"),
			Tag:        argStr(args, "cache_tag"),
			Mode:       orDefault(argStr(args, "cache_mode"), "max"),
			ImportOnly: argBool(args, "cache_import_only", false),
		}
	}
	return req, nil
}

func buildArchive(args map[string]any) ([]byte, error) {
	if b64 := argStr(args, "archive_base64"); b64 != "" {
		// Accept both padded and unpadded, with or without embedded newlines
		// (a base64 tool may have wrapped the tarball at 76 columns).
		clean := strings.NewReplacer("\n", "", "\r", "", " ", "").Replace(b64)
		raw, err := base64.StdEncoding.DecodeString(clean)
		if err != nil {
			raw, err = base64.RawStdEncoding.DecodeString(clean)
		}
		if err != nil {
			return nil, &extension.TypedError{
				Code:    easybus.ToolResultErrorCodeInvalidArgument,
				Message: "archive_base64 is not valid base64: " + err.Error(),
			}
		}
		return raw, nil
	}
	if s := argStr(args, "archive"); s != "" {
		return []byte(s), nil
	}
	return nil, &extension.TypedError{
		Code:    easybus.ToolResultErrorCodeInvalidArgument,
		Message: "one of `archive` (text) or `archive_base64` (base64 of a tar.gz) is required",
	}
}

// buildImage runs a build and returns the whole log once it finishes.
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
	req, err := t.buildRequest(args, tenant, session)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	res, err := cl.BuildImage(ctx, req)
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgImageBuilt, i18n.Params{"ref": res.GetImageRef()}) +
			"\n" + tailText(res.GetLog(), buildLogLimit),
		Data: map[string]any{"imageRef": res.GetImageRef(), "cacheEnabled": req.GetImageCache().GetEnabled()},
	}, nil
}

// buildImageStream starts a build and streams its log. Because a tool call must
// eventually return, this waits for the build to FINISH and returns the
// collected log plus the outcome — which is exactly what "wait for the build"
// means for a caller. The live chunk callback is used to keep a bounded log.
func (t *toolset) buildImageStream(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	if err := t.guard(cfg, "", true, locale); err != nil {
		return extension.ToolResultData{}, err
	}
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	req, err := t.buildRequest(args, tenant, session)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	started := time.Now()
	var mu sync.Mutex
	var log strings.Builder
	// Forward each chunk to the console as it arrives.
	//
	// The stream was already here — the chunks were simply collected into the final
	// result and nothing else, so a five-minute build showed "running" and then
	// everything at once. The tool's RESULT is unchanged; this only makes the wait
	// observable.
	prog := NewProgress()
	defer prog.Flush(ctx)
	stream, err := cl.BuildImageStream(ctx, req)
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	defer stream.Close()

	var final *opsv1.BuildImageStreamEvent
	for {
		ev, rerr := stream.Receive()
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return extension.ToolResultData{}, wrapErr(rerr)
		}
		if chunk := ev.GetChunk(); chunk != "" {
			mu.Lock()
			if log.Len() < buildLogLimit*2 {
				log.WriteString(chunk)
			}
			mu.Unlock()
			prog.Add(ctx, chunk)
		}
		if st := ev.GetState(); st != "" && st != "done" && st != "failed" {
			// A build state change ("exporting", "pushing") is what the user is
			// waiting on, and it carries no output of its own.
			prog.Phase(ctx, "running", "build: "+st)
		}
		if st := ev.GetState(); st == "done" || st == "failed" {
			final = ev
			break
		}
	}
	elapsed := time.Since(started)
	if final == nil {
		return extension.ToolResultData{}, &extension.TypedError{
			Code:    easybus.ToolResultErrorCodeBusiness,
			Message: "the build stream closed without a terminal state",
		}
	}

	mu.Lock()
	body := log.String()
	mu.Unlock()

	if final.GetState() == "failed" {
		return extension.ToolResultData{
			Content: i18n.T(locale, i18n.MsgBuildFailed, i18n.Params{"err": final.GetError()}) +
				fmt.Sprintf("\n(elapsed %s)\n", elapsed.Round(time.Second)) + tailText(body, buildLogLimit),
			Data: map[string]any{"state": "failed", "error": final.GetError(),
				"elapsedMs": elapsed.Milliseconds(), "log": tailText(body, buildLogLimit)},
		}, nil
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgBuildDone, i18n.Params{
			"ref": final.GetImageRef(), "elapsed": elapsed.Round(time.Second).String(),
		}) + "\n" + tailText(body, buildLogLimit),
		Data: map[string]any{
			"state": "done", "imageRef": final.GetImageRef(),
			"elapsedMs": elapsed.Milliseconds(), "log": tailText(body, buildLogLimit),
			"cacheEnabled": req.GetImageCache().GetEnabled(),
		},
	}, nil
}

// importImage mirrors an upstream image into the deployment registry.
func (t *toolset) importImage(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	if err := t.guard(cfg, "", true, locale); err != nil {
		return extension.ToolResultData{}, err
	}
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	src, repo := argStr(args, "source"), argStr(args, "repo")
	if src == "" || repo == "" {
		return extension.ToolResultData{}, &extension.TypedError{
			Code: easybus.ToolResultErrorCodeInvalidArgument, Message: "source and repo are required",
		}
	}
	res, err := cl.ImportImage(ctx, &opsv1.ImportImageRequest{
		Owner: tenant, Source: src, Repo: repo,
		Tag:       orDefault(argStr(args, "tag"), "latest"),
		AuthUser:  argStr(args, "auth_user"),
		AuthToken: argStr(args, "auth_token"),
	})
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	return extension.ToolResultData{
		Content: i18n.T(locale, i18n.MsgImageImported, i18n.Params{"ref": res.GetImageRef()}) +
			"\n" + tailText(res.GetLog(), buildLogLimit),
		Data: map[string]any{"imageRef": res.GetImageRef()},
	}, nil
}

// tailText keeps the LAST n bytes (a failed build explains itself at the end).
func tailText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "...(truncated)...\n" + s[len(s)-n:]
}
