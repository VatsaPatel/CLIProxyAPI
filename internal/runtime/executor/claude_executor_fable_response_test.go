package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestClaudeOpenRouterRedactedThinkingRepairSupportsFable51(t *testing.T) {
	if !claudeOpenRouterRedactedThinkingRepairEnabled("https://openrouter.ai/api", "anthropic/claude-fable-5.1") {
		t.Fatal("OpenRouter Fable 5.1 response repair is disabled")
	}
}

func TestRepairClaudeOpenRouterJSONResponseDropsRedactedThinking(t *testing.T) {
	input := []byte(`{"content":[{"type":"redacted_thinking","data":"hidden"},{"type":"text","text":"ok"}]}`)
	got := repairClaudeOpenRouterJSONResponse(input)
	if gjson.GetBytes(got, `content.#(type=="redacted_thinking")`).Exists() {
		t.Fatalf("redacted_thinking was not removed: %s", got)
	}
	if text := gjson.GetBytes(got, "content.0.text").String(); text != "ok" {
		t.Fatalf("content.0.text = %q, want ok; body=%s", text, got)
	}
}

func TestRepairClaudeOpenRouterSSEStreamDropsAndRenumbersRedactedThinking(t *testing.T) {
	input := []byte(strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"m","model":"claude-fable-5"}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"redacted_thinking","data":"hidden"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hidden"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"ok"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":1}`,
		``,
	}, "\n"))

	got := repairClaudeOpenRouterSSEStream(input)
	if bytes.Contains(got, []byte("redacted_thinking")) || bytes.Contains(got, []byte("hidden")) {
		t.Fatalf("redacted block leaked: %s", got)
	}
	if !bytes.Contains(got, []byte(`"index":0,"content_block":{"type":"text"`)) {
		t.Fatalf("text start was not renumbered to index 0: %s", got)
	}
	if !bytes.Contains(got, []byte(`"index":0,"delta":{"type":"text_delta","text":"ok"}`)) {
		t.Fatalf("text delta was not renumbered to index 0: %s", got)
	}
	if bytes.Contains(got, []byte(`"index":1`)) {
		t.Fatalf("old index leaked: %s", got)
	}
}
