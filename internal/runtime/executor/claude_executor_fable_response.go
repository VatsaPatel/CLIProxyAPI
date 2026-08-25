package executor

import (
	"bytes"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func claudeOpenRouterRedactedThinkingRepairEnabled(baseURL, model string) bool {
	baseURL = strings.ToLower(strings.TrimSpace(baseURL))
	model = strings.ToLower(strings.TrimSpace(thinking.ParseSuffix(model).ModelName))
	return strings.Contains(baseURL, "openrouter.ai") && strings.Contains(model, "claude-fable")
}

func repairClaudeOpenRouterJSONResponse(data []byte) []byte {
	if len(data) == 0 || !gjson.ValidBytes(data) {
		return data
	}
	content := gjson.GetBytes(data, "content")
	if !content.IsArray() {
		return data
	}
	out := data
	out, _ = sjson.SetRawBytes(out, "content", []byte("[]"))
	for _, block := range content.Array() {
		if strings.EqualFold(strings.TrimSpace(block.Get("type").String()), "redacted_thinking") {
			continue
		}
		out, _ = sjson.SetRawBytes(out, "content.-1", []byte(block.Raw))
	}
	return out
}

func repairClaudeOpenRouterSSEStream(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	frames := bytes.Split(data, []byte("\n\n"))
	out := make([]byte, 0, len(data))
	dropped := make(map[int64]struct{})
	indexMap := make(map[int64]int64)
	nextIndex := int64(0)
	for _, frame := range frames {
		if len(bytes.TrimSpace(frame)) == 0 {
			continue
		}
		repaired, ok := repairClaudeOpenRouterSSEFrame(frame, dropped, indexMap, &nextIndex)
		if !ok {
			continue
		}
		out = append(out, repaired...)
		out = append(out, "\n\n"...)
	}
	return out
}

func repairClaudeOpenRouterSSEFrame(frame []byte, dropped map[int64]struct{}, indexMap map[int64]int64, nextIndex *int64) ([]byte, bool) {
	lines := bytes.Split(frame, []byte("\n"))
	dataLineIndex := -1
	for i, line := range lines {
		if bytes.HasPrefix(bytes.TrimSpace(line), []byte("data:")) {
			dataLineIndex = i
			break
		}
	}
	if dataLineIndex < 0 {
		return frame, true
	}
	trimmed := bytes.TrimSpace(lines[dataLineIndex])
	payload := bytes.TrimSpace(trimmed[len("data:"):])
	if bytes.Equal(payload, []byte("[DONE]")) || !gjson.ValidBytes(payload) {
		return frame, true
	}
	eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	switch eventType {
	case "content_block_start":
		index := gjson.GetBytes(payload, "index").Int()
		if strings.EqualFold(strings.TrimSpace(gjson.GetBytes(payload, "content_block.type").String()), "redacted_thinking") {
			dropped[index] = struct{}{}
			return nil, false
		}
		mapped := *nextIndex
		*nextIndex = *nextIndex + 1
		indexMap[index] = mapped
		payload, _ = sjson.SetBytes(payload, "index", mapped)
	case "content_block_delta", "content_block_stop":
		index := gjson.GetBytes(payload, "index").Int()
		if _, ok := dropped[index]; ok {
			return nil, false
		}
		if mapped, ok := indexMap[index]; ok {
			payload, _ = sjson.SetBytes(payload, "index", mapped)
		}
	default:
		return frame, true
	}
	lines[dataLineIndex] = append([]byte("data: "), payload...)
	return bytes.Join(lines, []byte("\n")), true
}
