package prompt_test

import (
	"reflect"
	"strings"
	"testing"

	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
)

func TestEnvironmentFragmentsAndStableContinuation(t *testing.T) {
	env := prompt.Environment{Cwd: "/repo<&", Shell: "bash", CurrentDate: "2026-11-09", Timezone: "Asia/Shanghai", WorkspaceRoots: []string{"/repo<&", "/other"}, FileSystemAccess: "unrestricted"}
	fragments := env.Fragments()
	if len(fragments) != 5 {
		t.Fatal("unexpected fields", fragments)
	}
	history := []model.Message{submission("first task")}
	first, window, err := (prompt.Window{}).Prepare(history, fragments)
	if err != nil {
		t.Fatal(err)
	}
	body := windowText(first)
	for _, expected := range []string{"<cwd>/repo&lt;&amp;</cwd>", "<shell>bash</shell>", "<current_date>2026-11-09</current_date>", "<timezone>Asia/Shanghai</timezone>", "<filesystem><workspace_roots><root>/repo&lt;&amp;</root><root>/other</root></workspace_roots><file_system><access>unrestricted</access></file_system></filesystem>"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("missing %s in %s", expected, body)
		}
	}
	history = append(history, model.Message{Role: model.MessageRoleAssistant}, submission("continue"))
	second, next, err := window.Prepare(history, env.Fragments())
	if err != nil || !reflect.DeepEqual(first, second[:len(first)]) || len(next.Entries) != 1 {
		t.Fatal("unchanged environment altered the prefix", err)
	}
	env.CurrentDate, env.Timezone = "2026-11-10", "UTC-05:00"
	env.FileSystemAccess = "read_only"
	third, _, err := next.Prepare(history, env.Fragments())
	if err != nil {
		t.Fatal(err)
	}
	body = windowText(third)
	if strings.Count(body, "<cwd>") != 1 || strings.Count(body, "<shell>") != 1 || !strings.Contains(body, "<current_date>2026-11-10</current_date>") || !strings.Contains(body, "<access>read_only</access>") {
		t.Fatal("environment change duplicated unchanged fields", body)
	}
	if len((prompt.Environment{}).Fragments()) != 0 {
		t.Fatal("empty environment produced context")
	}
}

func TestEnvironmentEscapesUntrustedFields(t *testing.T) {
	fragments := (prompt.Environment{WorkspaceRoots: []string{"</root><injected>bad</injected>"}, FileSystemAccess: "</access>"}).Fragments()
	body := windowText([]model.Message{fragments[0].Message()})
	if strings.Contains(body, "<injected>") || !strings.Contains(body, "&lt;/access&gt;") {
		t.Fatal("filesystem values escaped their field", body)
	}
	malformed := prompt.Fragment{Kind: "environment", Source: "filesystem", Role: model.MessageRoleUser, Text: "<not_filesystem>bad</not_filesystem>"}
	if strings.Contains(windowText([]model.Message{malformed.Message()}), "<not_filesystem>") {
		t.Fatal("invalid filesystem XML rendered as markup")
	}
}
