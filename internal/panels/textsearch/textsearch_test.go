package textsearch

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/panels"
)

func TestParseGrepOutput(t *testing.T) {
	t.Parallel()

	longSnippet := strings.Repeat("x", maxSnippetRunes+10)
	tests := []struct {
		name string
		raw  string
		want []Result
	}{
		{
			name: "normal rows",
			raw:  "internal/app.go:42:func main() {}\nREADME.md:7:grut",
			want: []Result{
				{Path: "internal/app.go", Line: 42, Snippet: "func main() {}"},
				{Path: "README.md", Line: 7, Snippet: "grut"},
			},
		},
		{
			name: "colons in content",
			raw:  "config.toml:12:url = \"https://example.com:443\" and value:34:kept",
			want: []Result{{Path: "config.toml", Line: 12, Snippet: "url = \"https://example.com:443\" and value:34:kept"}},
		},
		{
			name: "blank and garbage lines",
			raw:  "\nnot a grep line\nfile.go:0:bad\nfile.go:3: ok \r\n",
			want: []Result{{Path: "file.go", Line: 3, Snippet: "ok"}},
		},
		{
			name: "windows path",
			raw:  `C:\repo\file.go:9:matched text`,
			want: []Result{{Path: `C:\repo\file.go`, Line: 9, Snippet: "matched text"}},
		},
		{
			name: "snippet capping",
			raw:  "file.go:5:" + longSnippet,
			want: []Result{{Path: "file.go", Line: 5, Snippet: strings.Repeat("x", maxSnippetRunes-3) + "..."}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseGrepOutput(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("len(parseGrepOutput()) = %d, want %d (%#v)", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("result %d = %#v, want %#v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestTextSearchSelectCurrentEmitsFileSelectedMsg(t *testing.T) {
	t.Parallel()

	ts := New(".", nil)
	ts.results = []Result{
		{Path: "first.go", Line: 10, Snippet: "first"},
		{Path: "second.go", Line: 20, Snippet: "second"},
	}
	ts.query = "needle"
	ts.lastRun = "needle"
	ts.cursor = 1

	_, cmd := ts.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected selection command")
	}
	msg := cmd()
	got, ok := msg.(panels.FileSelectedMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want panels.FileSelectedMsg", msg)
	}
	if got.Path != "second.go" || got.Line != 20 {
		t.Fatalf("FileSelectedMsg = %#v, want path second.go line 20", got)
	}
}

func TestTextSearchSearchCapsResultsAndMarksOmitted(t *testing.T) {
	t.Parallel()

	ts := New(".", nil)
	ts.max = 2
	ts.query = "needle"
	ts.qCursor = len(ts.query)
	ts.searchFn = func(_ context.Context, query string, maxResults int) ([]Result, bool, error) {
		if query != "needle" {
			t.Fatalf("query = %q, want needle", query)
		}
		results := []Result{
			{Path: "a.go", Line: 1, Snippet: "a"},
			{Path: "b.go", Line: 2, Snippet: "b"},
			{Path: "c.go", Line: 3, Snippet: "c"},
		}
		return capResults(results, maxResults)
	}

	_, cmd := ts.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected search command")
	}
	_, _ = ts.Update(cmd())
	if len(ts.results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(ts.results))
	}
	if !ts.omitted {
		t.Fatal("omitted = false, want true")
	}
	if !strings.Contains(ts.status, "more results omitted") {
		t.Fatalf("status = %q, want more results omitted", ts.status)
	}
}
