package terminal

import (
	"slices"
	"testing"

	"clara-code/internal/session"
)

func TestMatchInputHistoryOrder(t *testing.T) {
	entries := []session.InputHistoryEntry{
		{Text: "Fix tests", SessionID: "old-session"},
		{Text: "see FIX\n\t\u2003Tests below", Kind: "prompt", SessionID: "this-session"},
		{Text: "fix the failing tests", Kind: "prompt"},
		{Text: "first inspect tests", Kind: "prompt"},
		{Text: "tests fix", Kind: "prompt"},
		{Text: "fix more tests", Kind: "prompt"},
	}
	if got, want := matchInputHistory(entries, " \u00a0fIx  tEsTs \n"), []int{1, 0, 5, 2}; !slices.Equal(got, want) {
		t.Fatalf("matches = %v, want %v", got, want)
	}
}

func TestMatchInputHistoryUnicode(t *testing.T) {
	entries := []session.InputHistoryEntry{
		{Text: "修复旧记录\n和测试"},
		{Text: "解析 ς K K"},
		{Text: "解析 σ alone"},
		{Text: "解析 Α Σ K K"},
	}
	for _, test := range []struct {
		query string
		want  []int
	}{
		{query: "修旧测", want: []int{0}},
		{query: "测修"},
		{query: "Σ k K", want: []int{3, 1}},
		{query: "解析σKk", want: []int{3, 1}},
	} {
		if got := matchInputHistory(entries, test.query); !slices.Equal(got, test.want) {
			t.Errorf("query %q matched %v, want %v", test.query, got, test.want)
		}
	}
}

func TestMatchInputHistoryIncludesExistingKinds(t *testing.T) {
	entries := []session.InputHistoryEntry{
		{Text: "/status", Kind: "command"},
		{Text: "legacy entry"},
		{Attachments: []session.Attachment{{Filename: "fixture.png"}}},
	}
	for _, test := range []struct {
		query string
		want  []int
	}{
		{query: "", want: []int{2, 1, 0}},
		{query: "\t\u3000", want: []int{2, 1, 0}},
		{query: "sts", want: []int{0}},
		{query: "legacy", want: []int{1}},
		{query: "missing"},
	} {
		if got := matchInputHistory(entries, test.query); !slices.Equal(got, test.want) {
			t.Errorf("query %q matched %v, want %v", test.query, got, test.want)
		}
	}
}
