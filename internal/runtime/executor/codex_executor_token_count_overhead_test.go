package executor

import (
	"encoding/base64"
	"testing"

	"github.com/tiktoken-go/tokenizer"
)

func TestCountCodexInputTokensIncludesProtocolAndToolOverhead(t *testing.T) {
	enc, err := tokenizer.ForModel(tokenizer.GPT5)
	if err != nil {
		t.Fatalf("tokenizer: %v", err)
	}
	plain, err := countCodexInputTokens(enc, []byte(`{"instructions":"","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`))
	if err != nil {
		t.Fatalf("plain count: %v", err)
	}
	withTool, err := countCodexInputTokens(enc, []byte(`{"instructions":"be precise","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}],"tools":[{"type":"function","name":"lookup","description":"lookup","parameters":{"type":"object"}}]}`))
	if err != nil {
		t.Fatalf("tool count: %v", err)
	}
	if plain < 8 {
		t.Fatalf("plain count = %d, want protocol overhead", plain)
	}
	if withTool <= plain+6 {
		t.Fatalf("tool count = %d, plain = %d; want instructions and tool overhead", withTool, plain)
	}
}

func TestCountCodexInputTokensIncludesImageEstimate(t *testing.T) {
	enc, err := tokenizer.ForModel(tokenizer.GPT5)
	if err != nil {
		t.Fatalf("tokenizer: %v", err)
	}
	const png1x1 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	if _, errDecode := base64.StdEncoding.DecodeString(png1x1); errDecode != nil {
		t.Fatalf("fixture base64: %v", errDecode)
	}
	body := []byte(`{"input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,` + png1x1 + `"},{"type":"input_text","text":"describe"}]}]}`)
	count, err := countCodexInputTokens(enc, body)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count < 88 {
		t.Fatalf("count = %d, want image estimate", count)
	}
}
