package tool

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"clara-code/pkg/model"
	"clara-code/pkg/util/errs"
)

type interruptedTaskModel struct {
	model.LanguageModel
	requests int
}

func (m *interruptedTaskModel) Stream(context.Context, model.Call) (model.StreamResponse, error) {
	m.requests++
	parts := []model.StreamPart{{Type: model.StreamPartTypeTextStart, ID: "reply"}}
	if m.requests == 1 {
		parts = append(parts, model.StreamPart{Type: model.StreamPartTypeTextDelta, ID: "reply", Delta: "Partial reply"})
	} else {
		parts = append(parts,
			model.StreamPart{Type: model.StreamPartTypeTextDelta, ID: "reply", Delta: "Complete reply"},
			model.StreamPart{Type: model.StreamPartTypeTextEnd, ID: "reply"},
			model.StreamPart{Type: model.StreamPartTypeFinish, FinishReason: model.FinishReasonStop})
	}
	return slices.Values(parts), nil
}

func TestLocalTaskRetriesInterruptedStream(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		language := &interruptedTaskModel{}
		manager := NewTaskManager(func(context.Context, TaskSpec, *TaskScope) (TaskSetup, error) {
			return TaskSetup{Model: language, MaxRetries: new(1)}, nil
		}, 1, 1)
		scope := manager.Scope(t.Context(), "parent", ScopeOptions{})
		defer scope.Close(t.Context())
		info, err := scope.Spawn(t.Context(), TaskSpec{Name: "research", Role: "researcher", Message: "Summarize the results."})
		if err != nil {
			t.Fatal(err)
		}
		info, err = scope.Wait(t.Context(), info.ID, time.Minute)
		if err != nil || language.requests != 2 || info.Status != "completed" || info.Result != "Complete reply" {
			t.Fatalf("child made %d requests, result %+v, wait error %v", language.requests, info, err)
		}
	})
}

type taskProbe struct {
	TaskBackend
	calls int
	waits int
	spec  TaskSpec
}

func (p *taskProbe) Spawn(_ context.Context, spec TaskSpec) (TaskInfo, error) {
	p.calls++
	p.spec = spec
	return TaskInfo{ID: "task_fixture", Name: spec.Name, Status: "running"}, nil
}

func (p *taskProbe) Wait(_ context.Context, target string, _ time.Duration) (TaskInfo, error) {
	p.waits++
	return TaskInfo{ID: target, Name: p.spec.Name, Status: "completed", Result: "Verified."}, nil
}

func TestTaskToolsRejectInvalidFieldsWithoutStarting(t *testing.T) {
	for _, build := range []func(TaskBackend) Tool{SpawnAgent, RunTask} {
		for _, test := range []struct {
			field string
			value string
		}{
			{"task_name", "repo-survey"}, {"task_name", "Repo_survey"}, {"task_name", "repo/survey"},
			{"task_name", "repo survey"}, {"task_name", "调查"}, {"task_name", ""}, {"task_name", strings.Repeat("a", 101)},
			{"role", ""}, {"role", " \n\t"}, {"message", ""}, {"message", " \n\t"},
			{"message", strings.Repeat("中", (256<<10)/3+1)},
		} {
			t.Run(build(&taskProbe{}).Info().Name+"/"+test.field+"/"+test.value[:min(len(test.value), 12)], func(t *testing.T) {
				probe := &taskProbe{}
				item := build(probe)
				input := map[string]string{"task_name": "repo_survey", "role": "explorer", "message": "Inspect go.mod; report the module name. Do not edit."}
				input[test.field] = test.value
				data, _ := json.Marshal(input)
				result, err := item.Run(t.Context(), model.ToolCall{Name: item.Info().Name, Input: string(data)})
				text := result.Content
				if err != nil {
					text += err.Error()
				}
				if probe.calls != 0 || probe.waits != 0 || !strings.Contains(text, test.field) {
					t.Fatal("invalid input reached backend or error omitted field", probe.calls, text)
				}
			})
		}
	}
}

func TestTaskToolsExposeAndExecuteDocumentedArguments(t *testing.T) {
	for _, build := range []func(TaskBackend) Tool{SpawnAgent, RunTask} {
		probe := &taskProbe{}
		item := build(probe)
		property := item.Info().Parameters["task_name"].(map[string]any)
		if property["pattern"] != "^[a-z0-9_]{1,100}$" || !strings.Contains(property["description"].(string), "repo_survey") {
			t.Fatal("tool schema hides name restrictions", property)
		}
		if len(item.Info().Required) != 3 {
			t.Fatal("required delegation fields changed", item.Info().Required)
		}
		result, err := item.Run(t.Context(), model.ToolCall{Input: `{"task_name":"repo_survey","role":"explorer","message":"Inspect go.mod and the top-level directories using file tools. Do not modify files. Report the module name and directory responsibilities with supporting paths."}`})
		if err != nil || probe.calls != 1 || probe.spec.Name != "repo_survey" || probe.spec.Fork || probe.spec.Model != "" || probe.spec.Reasoning != "" || !strings.Contains(result.Content, "task_fixture") {
			t.Fatal("documented input rejected or changed", probe, result, err)
		}
		if item.Info().Name == "subagent_task" && probe.waits != 1 {
			t.Fatal("synchronous tool did not wait")
		}
	}
}

func TestDirectTaskScopeErrorsIdentifyInvalidFields(t *testing.T) {
	scope := NewTaskManager(nil, 1, 1).Scope(t.Context(), "parent", ScopeOptions{})
	for _, test := range []struct {
		spec  TaskSpec
		field string
	}{
		{TaskSpec{Name: "repo-survey", Role: "explorer", Message: "Inspect."}, "task_name"},
		{TaskSpec{Name: "repo_survey", Role: " ", Message: "Inspect."}, "role"},
		{TaskSpec{Name: "repo_survey", Role: "explorer", Message: " \t"}, "message"},
		{TaskSpec{Name: "repo_survey", Role: "explorer", Message: strings.Repeat("a", (256<<10)+1)}, "message"},
	} {
		_, err := scope.Spawn(t.Context(), test.spec)
		if !errs.IsCode(err, errs.CodeInvalidArgument) || !strings.Contains(err.Error(), test.field) || !strings.Contains(err.Error(), "No task was started") {
			t.Fatal("direct API returned unclear validation error", err)
		}
	}
}
