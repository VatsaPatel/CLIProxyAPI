package config

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGitHubCopilotGPT6ClaudeExample(t *testing.T) {
	raw, errRead := os.ReadFile("../../examples/github-copilot-gpt6-claude.yaml")
	if errRead != nil {
		t.Fatalf("read example: %v", errRead)
	}

	var cfg Config
	if errUnmarshal := yaml.Unmarshal(raw, &cfg); errUnmarshal != nil {
		t.Fatalf("decode example: %v", errUnmarshal)
	}
	if len(cfg.CodexKey) != 1 {
		t.Fatalf("codex-api-key count = %d, want 1", len(cfg.CodexKey))
	}
	if len(cfg.ClaudeKey) != 2 {
		t.Fatalf("claude-api-key count = %d, want 2", len(cfg.ClaudeKey))
	}
	if got := cfg.CodexKey[0].Headers["Copilot-Integration-Id"]; got != "copilot-developer-cli" {
		t.Fatalf("Codex Copilot-Integration-Id = %q, want copilot-developer-cli", got)
	}
	if got := cfg.ClaudeKey[0].Headers["Copilot-Integration-Id"]; got != "copilot-developer-cli" {
		t.Fatalf("Claude Copilot-Integration-Id = %q, want copilot-developer-cli", got)
	}

	wantCodex := map[string]int{
		"gpt-6-astra": 1050000,
		"gpt-6-sol":   872000,
		"gpt-6-luna":  872000,
	}
	for _, model := range cfg.CodexKey[0].Models {
		wantContext, ok := wantCodex[model.Alias]
		if !ok {
			t.Fatalf("unexpected Codex model alias %q", model.Alias)
		}
		if model.Name != model.Alias || model.MaxContextLength != wantContext || model.MaxCompletionTokens != 128000 {
			t.Fatalf("Codex model = %+v, want direct GitHub mapping with context %d and output 128000", model, wantContext)
		}
		delete(wantCodex, model.Alias)
	}
	if len(wantCodex) != 0 {
		t.Fatalf("missing Codex models: %v", wantCodex)
	}

	foundOpus55 := false
	for _, model := range cfg.ClaudeKey[0].Models {
		if model.Alias != "claude-opus-5-5" {
			continue
		}
		foundOpus55 = true
		if model.Name != "claude-opus-5.5" || model.MaxContextLength != 872000 || model.MaxCompletionTokens != 128000 {
			t.Fatalf("Opus 5.5 mapping = %+v", model)
		}
	}
	if !foundOpus55 {
		t.Fatal("GitHub Claude models do not contain claude-opus-5-5")
	}

	if len(cfg.ClaudeKey[1].Models) != 1 {
		t.Fatalf("OpenRouter Claude model count = %d, want only Fable 5.1", len(cfg.ClaudeKey[1].Models))
	}
	fable := cfg.ClaudeKey[1].Models[0]
	if fable.Name != "anthropic/claude-fable-5.1" || fable.Alias != "claude-fable-5-1" {
		t.Fatalf("OpenRouter Claude mapping = %+v, want Fable 5.1", fable)
	}
}
