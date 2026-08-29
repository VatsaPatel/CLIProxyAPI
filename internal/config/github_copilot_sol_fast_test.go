package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGitHubCopilotSolFastTransparentAliasConfig(t *testing.T) {
	const yamlConfig = `codex-api-key:
  - api-key: test-token
    base-url: https://api.githubcopilot.com
    headers:
      Copilot-Integration-Id: copilot-developer-cli
    models:
      - name: gpt-5.6-sol-fast
        alias: gpt-5.6-sol
        force-mapping: true
        max-context-length: 922000
        max-completion-tokens: 128000
payload:
  filter:
    - models:
        - name: gpt-5.6-sol
          protocol: codex
        - name: gpt-5.6-sol-fast
          protocol: codex
      params: [service_tier]
`

	var cfg Config
	if errUnmarshal := yaml.Unmarshal([]byte(yamlConfig), &cfg); errUnmarshal != nil {
		t.Fatalf("decode config: %v", errUnmarshal)
	}
	if len(cfg.CodexKey) != 1 {
		t.Fatalf("codex-api-key count = %d, want 1", len(cfg.CodexKey))
	}
	key := cfg.CodexKey[0]
	if got := key.Headers["Copilot-Integration-Id"]; got != "copilot-developer-cli" {
		t.Fatalf("Copilot-Integration-Id = %q, want copilot-developer-cli", got)
	}
	if len(key.Models) != 1 {
		t.Fatalf("model count = %d, want 1", len(key.Models))
	}
	model := key.Models[0]
	if model.Name != "gpt-5.6-sol-fast" || model.Alias != "gpt-5.6-sol" || !model.ForceMapping {
		t.Fatalf("model mapping = %+v, want transparent Sol Fast mapping", model)
	}
	if model.MaxContextLength != 922000 || model.MaxCompletionTokens != 128000 {
		t.Fatalf("model limits = (%d, %d), want (922000, 128000)", model.MaxContextLength, model.MaxCompletionTokens)
	}
	if len(cfg.Payload.Filter) != 1 || len(cfg.Payload.Filter[0].Params) != 1 || cfg.Payload.Filter[0].Params[0] != "service_tier" {
		t.Fatalf("payload filters = %+v, want service_tier filter", cfg.Payload.Filter)
	}
}
