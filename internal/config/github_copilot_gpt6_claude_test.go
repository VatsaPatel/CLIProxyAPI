package config

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGitHubCopilotCatalogExample(t *testing.T) {
	raw, errRead := os.ReadFile("../../examples/github-copilot-gpt6-claude.yaml")
	if errRead != nil {
		t.Fatalf("read example: %v", errRead)
	}

	var cfg Config
	if errUnmarshal := yaml.Unmarshal(raw, &cfg); errUnmarshal != nil {
		t.Fatalf("decode example: %v", errUnmarshal)
	}
	if len(cfg.CodexKey) != 1 || len(cfg.CodexKey[0].Models) != 19 {
		t.Fatalf("Codex groups/models = %d/%d, want 1/19", len(cfg.CodexKey), len(cfg.CodexKey[0].Models))
	}
	if len(cfg.ClaudeKey) != 2 || len(cfg.ClaudeKey[0].Models) != 8 || len(cfg.ClaudeKey[1].Models) != 1 {
		t.Fatalf("Claude groups/models = %d/%d/%d, want 2/8/1", len(cfg.ClaudeKey), len(cfg.ClaudeKey[0].Models), len(cfg.ClaudeKey[1].Models))
	}
	if len(cfg.OpenAICompatibility) != 1 || len(cfg.OpenAICompatibility[0].Models) != 18 {
		t.Fatalf("compatibility groups/models = %d/%d, want 1/18", len(cfg.OpenAICompatibility), len(cfg.OpenAICompatibility[0].Models))
	}

	if got := cfg.CodexKey[0].Headers["Copilot-Integration-Id"]; got != "copilot-developer-cli" {
		t.Fatalf("Codex Copilot-Integration-Id = %q", got)
	}
	if got := cfg.ClaudeKey[0].Headers["Copilot-Integration-Id"]; got != "copilot-developer-cli" {
		t.Fatalf("Claude Copilot-Integration-Id = %q", got)
	}
	if got := cfg.OpenAICompatibility[0].Headers["Copilot-Integration-Id"]; got != "copilot-developer-cli" {
		t.Fatalf("compatibility Copilot-Integration-Id = %q", got)
	}

	codexModels := make(map[string]CodexModel, len(cfg.CodexKey[0].Models))
	for _, model := range cfg.CodexKey[0].Models {
		codexModels[model.Alias] = model
	}
	gpt61, okGPT61 := codexModels["gpt-6.1-sol"]
	if !okGPT61 || gpt61.Name != "gpt-6.1-sol" || gpt61.MaxContextLength != 922000 || gpt61.MaxCompletionTokens != 128000 {
		t.Fatalf("GPT-6.1 Sol mapping = %+v", gpt61)
	}
	for _, id := range []string{"gpt-6-astra", "gpt-6-sol", "gpt-6-luna", "grok-4.7", "mai-code-1.1-flash"} {
		if _, ok := codexModels[id]; !ok {
			t.Errorf("missing Responses model %q", id)
		}
	}

	claudeModels := make(map[string]ClaudeModel, len(cfg.ClaudeKey[0].Models))
	for _, model := range cfg.ClaudeKey[0].Models {
		claudeModels[model.Alias] = model
	}
	opus55 := claudeModels["claude-opus-5-5"]
	if opus55.Name != "claude-opus-5.5" || opus55.MaxContextLength != 1000000 || opus55.MaxCompletionTokens != 128000 {
		t.Fatalf("Opus 5.5 mapping = %+v", opus55)
	}
	sonnet55 := claudeModels["claude-sonnet-5-5"]
	if sonnet55.Name != "claude-sonnet-5.5" || sonnet55.MaxContextLength != 936000 || sonnet55.MaxCompletionTokens != 128000 {
		t.Fatalf("Sonnet 5.5 mapping = %+v", sonnet55)
	}

	fable := cfg.ClaudeKey[1].Models[0]
	if fable.Name != "anthropic/claude-fable-5.1" || fable.Alias != "claude-fable-5-1" {
		t.Fatalf("OpenRouter Fable mapping = %+v", fable)
	}

	compatModels := make(map[string]OpenAICompatibilityModel, len(cfg.OpenAICompatibility[0].Models))
	for _, model := range cfg.OpenAICompatibility[0].Models {
		compatModels[model.Alias] = model
	}
	if model := compatModels["gemini-3.8-flash"]; model.Name != "gemini-3.8-flash" || model.MaxContextLength != 983040 {
		t.Fatalf("Gemini 3.8 mapping = %+v", model)
	}
	if model := compatModels["gpt-4o"]; model.MaxCompletionTokens != 4096 {
		t.Fatalf("GPT-4o output limit = %d, want 4096", model.MaxCompletionTokens)
	}
	if model := compatModels["text-embedding-3-small"]; model.Name != "text-embedding-3-small" {
		t.Fatalf("embedding mapping = %+v", model)
	}
}
