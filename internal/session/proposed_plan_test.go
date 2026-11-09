package session

import (
	"reflect"
	"testing"
)

func TestPlanCellsPreserveReplyAndRecognizeFencedTags(t *testing.T) {
	const example = "```xml\n<proposed_plan>\n# Example\n</proposed_plan>\n```"
	const body = "# Plan\n\n- Run verification."
	const text = "Before.\n\n" + example + "\n\n<proposed_plan>\r\n" + body + "\r\n</proposed_plan>\n\nAfter."
	want := []Cell{
		{Kind: "assistant", Text: "Before.\n\n```xml"},
		{Kind: ProposedPlanKind, Text: "# Example"},
		{Kind: "assistant", Text: "```"},
		{Kind: ProposedPlanKind, Text: body},
		{Kind: "assistant", Text: "After."},
	}
	if got := PlanCells(text, false); !reflect.DeepEqual(got, want) {
		t.Fatalf("cells: %+v", got)
	}
	if got := CompletePlan(text); got != "" {
		t.Fatalf("multiple plans approved: %q", got)
	}
}

func TestPlanCellsIgnoreNestedAndExtraCodeFences(t *testing.T) {
	const body = "# Plan\n\n```markdown\n# Target\n\n```sh\ngo test ./...\n```\n\n```sh\ngo vet ./...\n```\n```\n\n## Verify\n\n- Run `make check`."
	const text = "Planning preface.\n\n<proposed_plan>\n" + body + "\n</proposed_plan>"
	want := []Cell{
		{Kind: "assistant", Text: "Planning preface."},
		{Kind: ProposedPlanKind, Text: body},
	}
	if got := PlanCells(text, false); !reflect.DeepEqual(got, want) {
		t.Fatalf("cells: %+v", got)
	}
	if got := CompletePlan(text); got != body {
		t.Fatalf("plan source: %q", got)
	}
}

func TestPlanPreviewDoesNotApproveMalformedBlocks(t *testing.T) {
	const open = "<proposed_plan>\n"
	const complete = open + "# Preview\n</proposed_plan>"
	preview := PlanCells(open+"# Preview\n</proposed_pl", true)
	if len(preview) != 1 || preview[0].Kind != ProposedPlanKind || preview[0].Text != "# Preview" {
		t.Fatalf("preview: %+v", preview)
	}
	for _, text := range []string{
		open + "# Preview",
		open + "\n</proposed_plan>",
		complete + "\n" + complete,
		complete + "\n" + open,
		open + complete,
		"Before <proposed_plan>\n# Preview\n</proposed_plan>",
	} {
		if got := CompletePlan(text); got != "" {
			t.Errorf("accepted %q as a plan: %q", text, got)
		}
	}
}
