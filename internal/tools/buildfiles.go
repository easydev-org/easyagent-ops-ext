package tools

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	easybus "github.com/easydev-org/easybus-sdk"
	"github.com/easydev-org/easybus-sdk/extension"

	opsv1 "github.com/easydev-org/easyagent-ops-ext/gen/easyops/v1"
	"github.com/easydev-org/easyagent-ops-ext/internal/i18n"
)

// maxInlineFileBytes bounds a single file returned INLINE. A dist/ or a binary
// can be large; the tar is always available as base64, but a caller usually
// wants to LOOK at a small file (index.html, a build report) without decoding
// anything.
const maxInlineFileBytes = 64 << 10

// buildFiles runs a build whose PRODUCT IS A FILE TREE — a compiled binary, a
// frontend dist/, a test report — instead of a container image.
//
// The result is returned BOTH ways:
//   - `files` (base64 tar): the complete tree, for a caller that will untar it;
//   - `contents` (text): every small file inline, so a model can actually read
//     what was built (and check e.g. index.html references the right assets).
func (t *toolset) buildFiles(ctx context.Context, args map[string]any, session, tenant string) (extension.ToolResultData, error) {
	locale := t.locale(session, tenant)
	cfg := t.cfg(session, tenant)
	if err := t.guard(cfg, "", true, locale); err != nil {
		return extension.ToolResultData{}, err
	}
	cl, err := t.client(cfg)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	archive, err := buildArchive(args)
	if err != nil {
		return extension.ToolResultData{}, err
	}
	req := &opsv1.BuildImageRequest{
		Owner: tenant, Archive: archive,
		Dockerfile: orDefault(argStr(args, "dockerfile"), "Dockerfile"),
		Context:    argStr(args, "context"),
		// repo/tag are OPTIONAL for a file build: they only name the cache.
		Repo:           argStr(args, "repo"),
		Tag:            orDefault(argStr(args, "tag"), "latest"),
		BuildArgs:      argMap(args, "build_args"),
		Output:         "files",
		OutputStage:    orDefault(argStr(args, "stage"), "export"),
		MaxOutputBytes: int64(argInt(args, "max_bytes", 0)),
	}
	if argBool(args, "cache", true) {
		req.ImageCache = &opsv1.CacheOptions{
			Enabled: true, Repo: argStr(args, "cache_repo"),
			Tag:        argStr(args, "cache_tag"),
			Mode:       orDefault(argStr(args, "cache_mode"), "max"),
			ImportOnly: argBool(args, "cache_import_only", false),
		}
	}
	res, err := cl.BuildFiles(ctx, req)
	if err != nil {
		return extension.ToolResultData{}, wrapErr(err)
	}
	if res.GetTruncated() {
		return extension.ToolResultData{
			Content: i18n.T(locale, i18n.MsgFilesTooBig, i18n.Params{
				"bytes": fmt.Sprintf("%d", res.GetFileBytes())}),
			Data: map[string]any{
				"truncated": true, "fileBytes": res.GetFileBytes(),
				"log": tailText(res.GetLog(), buildLogLimit),
			},
		}, nil
	}

	entries, lerr := listTar(res.GetFiles())
	if lerr != nil {
		return extension.ToolResultData{}, &extension.TypedError{
			Code:    easybus.ToolResultErrorCodeBusiness,
			Message: "the exported tree could not be read: " + lerr.Error(),
		}
	}

	var b strings.Builder
	b.WriteString(i18n.T(locale, i18n.MsgFilesBuilt, i18n.Params{
		"n":     fmt.Sprintf("%d", res.GetFileCount()),
		"bytes": fmt.Sprintf("%d", res.GetFileBytes()),
	}))
	b.WriteString("\n")
	for _, e := range entries {
		if e.IsDir {
			continue
		}
		fmt.Fprintf(&b, "  %-48s %8d B", e.Name, e.Size)
		if e.Text != "" || e.Size == 0 {
			b.WriteString("  (inline below)")
		} else {
			b.WriteString("  (binary or large — use `files` to get the tar)")
		}
		b.WriteString("\n")
	}
	for _, e := range entries {
		if e.Text == "" {
			continue
		}
		fmt.Fprintf(&b, "\n--- %s ---\n%s\n", e.Name, e.Text)
	}

	data := map[string]any{
		"fileCount": res.GetFileCount(),
		"fileBytes": res.GetFileBytes(),
		"files":     filesData(entries),
		// The tar, base64: the caller can untar it to get exact bytes.
		"filesBase64": encodeBase64(res.GetFiles()),
		"log":         tailText(res.GetLog(), buildLogLimit),
	}
	return extension.ToolResultData{Content: strings.TrimRight(b.String(), "\n"), Data: data}, nil
}

// tarEntry is one file from the exported tree.
type tarEntry struct {
	Name  string
	Size  int64
	IsDir bool
	// Text is the file's content when it is small and non-binary.
	Text string
}

// listTar walks the exported tar.
//
// A file is inlined only when it is BOTH small and looks like text: putting
// binary bytes into a tool response corrupts it (and a model cannot read it).
func listTar(raw []byte) ([]tarEntry, error) {
	tr := tar.NewReader(bytes.NewReader(raw))
	var out []tarEntry
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		e := tarEntry{Name: hdr.Name, Size: hdr.Size}
		if hdr.Typeflag == tar.TypeDir {
			e.IsDir = true
			out = append(out, e)
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		data, rerr := io.ReadAll(tr)
		if rerr != nil {
			return nil, rerr
		}
		if len(data) <= maxInlineFileBytes && looksTextual(data) {
			e.Text = string(data)
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// looksTextual reports whether data is safe to embed in a text response: no NUL
// byte, and it decodes as valid UTF-8.
func looksTextual(data []byte) bool {
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	return utf8.Valid(data)
}

// encodeBase64 renders the exported tar for transport.
func encodeBase64(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func filesData(entries []tarEntry) []map[string]any {
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		if e.IsDir {
			continue
		}
		out = append(out, map[string]any{
			"name": e.Name, "size": e.Size, "text": e.Text,
		})
	}
	return out
}
