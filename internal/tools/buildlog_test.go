package tools

import (
	"strings"
	"testing"

	opsv1 "github.com/easydev-org/easyagent-ops-ext/gen/easyops/v1"
)

func pageResp(lines []string, first, total int, more, truncated, dropped bool) *opsv1.GetBuildResponse {
	return &opsv1.GetBuildResponse{
		Build:      &opsv1.BuildRecord{LogTruncated: truncated, LogDropped: dropped},
		LogLines:   lines,
		FirstLine:  int32(first),
		TotalLines: int32(total),
		More:       more,
	}
}

// TestLogLinesAreNumbered: every line carries its 1-based number, so a reader
// can act on a specific line (like sandbox-file-read).
func TestLogLinesAreNumbered(t *testing.T) {
	out := renderLogPage(pageResp([]string{"step 1", "step 2"}, 1, 2, false, false, false))
	if !strings.Contains(out, "1: step 1") || !strings.Contains(out, "2: step 2") {
		t.Errorf("lines must be numbered:\n%s", out)
	}
}

// TestPageStartingMidLog: a page from line 201 must be numbered 201, not 1 —
// otherwise the number is useless for referring back.
func TestPageStartingMidLog(t *testing.T) {
	out := renderLogPage(pageResp([]string{"a", "b"}, 201, 300, false, false, false))
	if !strings.Contains(out, "201: a") || !strings.Contains(out, "202: b") {
		t.Errorf("mid-log page must keep absolute numbering:\n%s", out)
	}
	if !strings.Contains(out, "lines 201-202 of 300") {
		t.Errorf("footer must state the window:\n%s", out)
	}
}

// TestMoreHintNamesTheNextOffset: the footer must tell the caller exactly what
// to pass next, and never suggest a limit above the server cap.
func TestMoreHintNamesTheNextOffset(t *testing.T) {
	out := renderLogPage(pageResp([]string{"a", "b"}, 1, 500, true, false, false))
	if !strings.Contains(out, "offset=2") {
		t.Errorf("must name the next offset:\n%s", out)
	}
	if !strings.Contains(out, "1000") {
		t.Errorf("must state the limit cap:\n%s", out)
	}
	if strings.Contains(out, "99999") {
		t.Errorf("must not suggest an invalid limit:\n%s", out)
	}
}

// TestTruncationIsLoud: a truncated log must say that later lines DO NOT EXIST,
// so nobody concludes "the build stopped there".
func TestTruncationIsLoud(t *testing.T) {
	out := renderLogPage(pageResp([]string{"a"}, 1, 1, false, true, false))
	if !strings.Contains(out, "NOT stored") {
		t.Errorf("truncation must be explicit:\n%s", out)
	}
}

// TestDroppedLogExplainsItself: a released log says why and what still works.
func TestDroppedLogExplainsItself(t *testing.T) {
	out := renderLogPage(pageResp(nil, 0, 0, false, false, true))
	if !strings.Contains(out, "released") || !strings.Contains(out, "status and timing") {
		t.Errorf("dropped log must explain itself:\n%s", out)
	}
}

// TestEmptyLog: no output is a fact worth stating, not a blank response.
func TestEmptyLog(t *testing.T) {
	out := renderLogPage(pageResp(nil, 0, 0, false, false, false))
	if !strings.Contains(out, "empty") {
		t.Errorf("empty log must say so:\n%s", out)
	}
}
