package coding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"clara-code/internal/session"
	"clara-code/internal/setting"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
)

func imageFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
func TestImageAttachmentRecoveryForkAndTextModelProjection(t *testing.T) {
	data := imageFixture(t)
	var calls atomic.Int32
	c, h := testHost(t, t.TempDir(), fixtureModel{reply: func(call model.Call) (*model.Response, error) {
		calls.Add(1)
		return demoText("Screenshot shows a red pixel."), nil
	}})
	a, err := AttachBytes(t.Context(), h.Config.Home, "shot.png", data, setting.Images{})
	if err != nil {
		t.Fatal(err)
	}
	files, err := AttachmentFiles(h.Config.Home, []session.Attachment{a}, setting.Images{})
	if err != nil {
		t.Fatal(err)
	}
	run, err := c.StartWithFiles(t.Context(), prompt.Execute, "Inspect this image.", false, files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	turn, err := c.Store.ReadTurn(0)
	if err != nil || len(turn.Attachments) != 1 || turn.Attachments[0].Width != 8 || turn.Attachments[0].Height != 6 {
		t.Fatal("image transcript missing", turn.Attachments, err)
	}
	manifest := filepath.Join(h.Config.Home, "sessions", c.Session.ID()+".json")
	raw, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("clara-blob:")) || bytes.Contains(raw, []byte("iVBOR")) {
		t.Fatal("image data was inlined into the manifest")
	}
	history := c.Session.Messages()
	projected := withoutImages(history)
	if !strings.Contains(session.Text(projected[0]), omittedImage) {
		t.Fatal("text model did not receive omission explanation")
	}
	found := false
	for _, part := range history[0].Content {
		if file, ok := model.AsMessagePart[model.FilePart](part); ok {
			found = bytes.Equal(file.Data, files[0].Data)
		}
	}
	if !found {
		t.Fatal("request projection destroyed image history")
	}
	branchStore, err := c.Store.Fork(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer branchStore.Close()
	if branchStore.TurnCount() != 0 {
		t.Fatal("rewind retained selected turn")
	}
	id := c.Session.ID()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := session.Open(h.Config.Home, id, c.Workspace.Root)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := h.Open(t.Context(), c.Workspace, store, NewQuestions(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if !bytes.Equal(restored.Session.Messages()[0].Content[1].(model.FilePart).Data, files[0].Data) {
		t.Fatal("image did not survive recovery")
	}
	if err := restored.Compact(t.Context()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(session.Text(restored.Session.Messages()[0]), "iVBOR") {
		t.Fatal("base64 entered compaction text")
	}
	if _, err := a.File(h.Config.Home); err != nil {
		t.Fatal("compaction lost original image", err)
	}
	if err := os.WriteFile(filepath.Join(h.Config.Home, "attachments", a.Hash), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.File(h.Config.Home); err == nil {
		t.Fatal("corrupt attachment accepted")
	}
}
func TestImagePathsLimitsAndUnsupportedNewInput(t *testing.T) {
	root := t.TempDir()
	imagePath := filepath.Join(root, "one shot.png")
	if err := os.WriteFile(imagePath, imageFixture(t), 0600); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{imagePath, fmt.Sprintf("%q", imagePath), strings.ReplaceAll(imagePath, " ", `\ `), "file://" + strings.ReplaceAll(imagePath, " ", "%20")} {
		paths, ok, err := ImagePaths(raw)
		if err != nil || !ok || len(paths) != 1 {
			t.Fatal("path not recognized", raw, paths, err)
		}
	}
	if _, ok, _ := ImagePaths("please read " + imagePath); ok {
		t.Fatal("ordinary text became an attachment")
	}
	c, h := testHost(t, root, DemoModel{})
	cfg := c.Host.Config
	cfg.Models = map[string]setting.Model{"fixture": {ID: "fixture", InputModalities: []string{"text"}}}
	c.Host.Config = cfg
	if _, err := c.StartWithFiles(t.Context(), prompt.Execute, "image", false, []model.FilePart{{MediaType: "image/png", Data: imageFixture(t)}}); err == nil {
		t.Fatal("new unsupported image accepted")
	}
	a, err := LoadAttachment(t.Context(), h.Config.Home, root, imagePath, setting.Images{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AttachmentFiles(h.Config.Home, []session.Attachment{a, a}, setting.Images{MaxAttachments: 1}); err == nil {
		t.Fatal("attachment count ignored")
	}
	if _, err := AttachBytes(t.Context(), h.Config.Home, "bad.png", []byte("not image"), setting.Images{}); err == nil {
		t.Fatal("bad image accepted")
	}
}
func TestImagesReachBothProtocols(t *testing.T) {
	for _, protocol := range []string{"completions", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			var requests atomic.Int32
			c, _ := remoteFixture(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				var body any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				text := fmt.Sprint(body)
				if !strings.Contains(text, "data:image/png;base64,") {
					t.Error("image did not reach wire", protocol)
				}
				if protocol == "completions" {
					sendTextSSE(w, "Image received.", true)
				} else {
					sendResponseText(w)
				}
			})
			if protocol == "responses" {
				c.Host.Config.Protocol = protocol
				factory, err := c.Host.Config.Factory()
				if err != nil {
					t.Fatal(err)
				}
				c.Host.Models = factory
				if err := c.ChangeModel(t.Context(), "fixture", ""); err != nil {
					t.Fatal(err)
				}
			}
			run, err := c.StartWithFiles(t.Context(), prompt.Execute, "", false, []model.FilePart{{Filename: "one.png", MediaType: "image/png", Data: imageFixture(t)}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := run.Wait(t.Context()); err != nil {
				t.Fatal(err)
			}
			if requests.Load() != 1 {
				t.Fatal(requests.Load())
			}
		})
	}
}
func TestProviderRoutingAndModeSpecificEffort(t *testing.T) {
	var requests []string
	c, _ := remoteFixture(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model, ReasoningEffort string
			Reasoning              struct{ Effort string }
		}
		var raw map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Error(err)
		}
		json.Unmarshal(raw["model"], &body.Model)
		json.Unmarshal(raw["reasoning_effort"], &body.ReasoningEffort)
		requests = append(requests, body.Model+" "+body.ReasoningEffort)
		sendTextSSE(w, "done", true)
	})
	c.Host.Config.PlanEffort = "high"
	if err := c.Store.SetSelection("", "fixture", "low", "high"); err != nil {
		t.Fatal(err)
	}
	finish(t, c, prompt.Execute, "agent mode")
	finish(t, c, prompt.Plan, "plan mode")
	if len(requests) != 2 || requests[0] != "fixture low" || requests[1] != "fixture high" {
		t.Fatal("mode-specific efforts not applied", requests)
	}
}
func TestConfiguredToolTimeoutAndExecDefaults(t *testing.T) {
	c, _ := testHost(t, t.TempDir(), DemoModel{})
	slow := model.NewAgentTool("slow", "wait", func(ctx context.Context, _ struct{}, _ model.ToolCall) (model.ToolResponse, error) {
		<-ctx.Done()
		return model.ToolResponse{}, ctx.Err()
	})
	wrapped := configuredTool{AgentTool: slow, timeout: 10 * time.Millisecond, parallel: true}
	if _, err := wrapped.Run(t.Context(), model.ToolCall{Input: `{}`}); err == nil {
		t.Fatal("tool timeout did not execute")
	}
	var input string
	capture := model.NewAgentTool("exec_command", "capture", func(_ context.Context, args map[string]any, _ model.ToolCall) (model.ToolResponse, error) {
		bytes, _ := json.Marshal(args)
		input = string(bytes)
		return model.NewTextResponse("ok"), nil
	})
	c.Host.Config.Tools.Exec = setting.Exec{YieldSeconds: 1, TimeoutSeconds: new(10)}
	tools := c.configureTools([]model.AgentTool{capture}, setting.Model{})
	if _, err := tools[0].Run(t.Context(), model.ToolCall{Input: `{"command":"test","yield_seconds":2}`}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(input, `"yield_seconds":2`) || !strings.Contains(input, `"timeout_seconds":10`) {
		t.Fatal(input)
	}
}

func TestSameModelAcrossProvidersAndRequestCapabilities(t *testing.T) {
	var seen [2]atomic.Int32
	providers := map[string]setting.Provider{}
	for i, name := range []string{"first", "second"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var raw map[string]any
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				t.Error(err)
			}
			if r.Header.Get("Authorization") != "Bearer "+name || raw["model"] != "same-model" {
				t.Error("provider routing/auth mismatch", i)
			}
			if raw["parallel_tool_calls"] != false || raw["max_tokens"] != float64(64) {
				t.Error("capability options absent", raw["parallel_tool_calls"], raw["max_completion_tokens"])
			}
			seen[i].Add(1)
			sendTextSSE(w, "done", true)
		}))
		t.Cleanup(server.Close)
		providers[name] = setting.Provider{BaseURL: server.URL + "/v1", WireAPI: "completions", APIKey: name, Models: map[string]setting.Model{"same": {ID: "same-model", MaxOutputTokens: 64, SupportsParallelToolCalls: new(false), SupportedReasoningLevels: []model.ReasoningEffort{"low", "high"}, DefaultReasoningLevel: "low"}}}
	}
	root := t.TempDir()
	workspace, err := ResolveWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := (setting.Config{Home: t.TempDir(), DefaultModel: "same", ModelProvider: "first", Providers: providers}).SelectProvider("first")
	if err != nil {
		t.Fatal(err)
	}
	h := Host{Config: cfg}
	store := session.New(cfg.Home, session.Meta{Project: workspace.Root, Cwd: workspace.Cwd, Model: "same"})
	c, err := h.Open(t.Context(), workspace, store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	finish(t, c, prompt.Execute, "first provider")
	if err := c.ChangeSelection(t.Context(), "second", "same", "high", prompt.Execute); err != nil {
		t.Fatal(err)
	}
	finish(t, c, prompt.Execute, "second provider")
	if seen[0].Load() != 1 || seen[1].Load() != 1 || c.Store.Meta().Provider != "second" {
		t.Fatal("same-name selection confused providers", seen[0].Load(), seen[1].Load(), c.Store.Meta())
	}
	if err := c.ChangeSelection(t.Context(), "first", "same", "ultra", prompt.Plan); err == nil {
		t.Fatal("unsupported explicit effort accepted")
	}
}

func TestExecDefaultsAreVisibleInActualSchema(t *testing.T) {
	c, _ := testHost(t, t.TempDir(), DemoModel{})
	c.Host.Config.Tools.Exec = setting.Exec{YieldSeconds: 2, TimeoutSeconds: new(15)}
	tools := c.configureTools(c.items, setting.Model{})
	for _, item := range tools {
		info := item.Info()
		if info.Name != "exec_command" {
			continue
		}
		schema := info.InputSchema["properties"].(map[string]any)
		for key, want := range map[string]string{"yield_seconds": "default 2", "timeout_seconds": "default 15"} {
			if description, _ := schema[key].(map[string]any)["description"].(string); !strings.Contains(description, want) {
				t.Fatal("model schema disagrees with execution", key, description)
			}
		}
		return
	}
	t.Fatal("exec_command missing")
}
