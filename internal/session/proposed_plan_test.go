package session

import (
	"reflect"
	"testing"
)

func TestPlanCellsPreserveReplyAndFencedTags(t *testing.T) {
	const example = "```xml\n<proposed_plan>\n</proposed_plan>\n```"
	const body = "# Example\n\n- Run verification.\n\n" + example
	const text = "Before.\n\n" + example + "\n\n<proposed_plan>\r\n" + body + "\r\n</proposed_plan>\n\nAfter."
	want := []Cell{
		{Kind: "assistant", Text: "Before.\n\n" + example},
		{Kind: ProposedPlanKind, Text: body},
		{Kind: "assistant", Text: "After."},
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
		"```xml\n" + complete + "\n```",
	} {
		if got := CompletePlan(text); got != "" {
			t.Errorf("accepted %q as a plan: %q", text, got)
		}
	}
}
