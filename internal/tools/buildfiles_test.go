package tools

import (
	"archive/tar"
	"bytes"
	"strings"
	"testing"
)

func tarOf(files map[string]string) []byte {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, body := range files {
		_ = tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg,
		})
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	return buf.Bytes()
}

// TestListTarSeparatesTextFromBinary: a text file is inlined so a caller can
// READ the artifact; a binary one is listed only (embedding its bytes would
// corrupt the response and a model cannot read it).
func TestListTarSeparatesTextFromBinary(t *testing.T) {
	bin := string([]byte{'a', 0, 'b', 0})
	raw := tarOf(map[string]string{
		"index.html": "<html>ok</html>",
		"app":        bin,
	})
	entries, err := listTar(raw)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]tarEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if byName["index.html"].Text != "<html>ok</html>" {
		t.Errorf("text file must be inlined, got %q", byName["index.html"].Text)
	}
	if byName["app"].Text != "" {
		t.Errorf("binary file must NOT be inlined")
	}
	if byName["app"].Size != int64(len(bin)) {
		t.Errorf("binary size = %d", byName["app"].Size)
	}
}

// TestListTarIsSortedAndSkipsDirs: output is deterministic and directories are
// not listed as files.
func TestListTarIsSortedAndSkipsDirs(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{Name: "dist/", Typeflag: tar.TypeDir, Mode: 0o755})
	for _, n := range []string{"dist/b.js", "dist/a.js"} {
		_ = tw.WriteHeader(&tar.Header{Name: n, Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte("x"))
	}
	_ = tw.Close()

	entries, err := listTar(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir {
			names = append(names, e.Name)
		}
	}
	got := strings.Join(names, ",")
	if got != "dist/a.js,dist/b.js" {
		t.Errorf("files = %q, want sorted a then b", got)
	}
}

// TestLooksTextual: a NUL byte or invalid UTF-8 marks a file as binary.
func TestLooksTextual(t *testing.T) {
	if !looksTextual([]byte("hello\n")) {
		t.Error("plain text must pass")
	}
	if looksTextual([]byte{'a', 0}) {
		t.Error("NUL must mark binary")
	}
	if looksTextual([]byte{0xff, 0xfe}) {
		t.Error("invalid UTF-8 must mark binary")
	}
}
