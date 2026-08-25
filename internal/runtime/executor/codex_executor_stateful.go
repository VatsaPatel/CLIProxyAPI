package executor

import (
	"context"
	"net/http"
	"strings"

	internalcache "github.com/router-for-me/CLIProxyAPI/v7/internal/cache"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type codexResponseStateScope struct {
	authID             string
	modelName          string
	sessionKey         string
	previousResponseID string
}

func (s codexResponseStateScope) valid() bool {
	return strings.TrimSpace(s.authID) != "" &&
		strings.TrimSpace(s.modelName) != "" &&
		strings.TrimSpace(s.sessionKey) != ""
}

func codexStatefulResponsesEnabled(auth *cliproxyauth.Auth) bool {
	if auth == nil || len(auth.Attributes) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(auth.Attributes["stateful_responses"]), "true")
}

func codexResponseStateScopeFromRequest(ctx context.Context, auth *cliproxyauth.Auth, from sdktranslator.Format, req cliproxyexecutor.Request, opts cliproxyexecutor.Options, body []byte) codexResponseStateScope {
	if !codexStatefulResponsesEnabled(auth) || !sourceFormatEqual(from, sdktranslator.FormatClaude) {
		return codexResponseStateScope{}
	}
	modelName := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if modelName == "" {
		modelName = thinking.ParseSuffix(req.Model).ModelName
	}
	return codexResponseStateScope{
		authID:     strings.TrimSpace(auth.ID),
		modelName:  modelName,
		sessionKey: codexReasoningReplaySessionKey(ctx, from, req, opts, body),
	}
}

func prepareCodexStatefulRequest(ctx context.Context, auth *cliproxyauth.Auth, from sdktranslator.Format, req cliproxyexecutor.Request, opts cliproxyexecutor.Options, body []byte) ([]byte, codexResponseStateScope, bool) {
	scope := codexResponseStateScopeFromRequest(ctx, auth, from, req, opts, body)
	if !scope.valid() {
		return body, scope, false
	}

	body, _ = sjson.SetBytes(body, "store", true)
	body, _ = sjson.DeleteBytes(body, "previous_response_id")

	state, ok := internalcache.GetCodexResponseState(scope.authID, scope.modelName, scope.sessionKey)
	if !ok {
		return body, scope, false
	}

	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		internalcache.DeleteCodexResponseState(scope.authID, scope.modelName, scope.sessionKey)
		return body, scope, false
	}
	inputItems := input.Array()
	anchor := codexResponseStateAnchorIndex(inputItems, state)
	if anchor < 0 || anchor+1 >= len(inputItems) {
		internalcache.DeleteCodexResponseState(scope.authID, scope.modelName, scope.sessionKey)
		return body, scope, false
	}

	incremental := make([]string, 0, len(inputItems)-anchor-1)
	for _, item := range inputItems[anchor+1:] {
		incremental = append(incremental, item.Raw)
	}
	updated, errSet := sjson.SetRawBytes(body, "input", []byte("["+strings.Join(incremental, ",")+"]"))
	if errSet != nil {
		internalcache.DeleteCodexResponseState(scope.authID, scope.modelName, scope.sessionKey)
		return body, scope, false
	}
	updated, _ = sjson.SetBytes(updated, "previous_response_id", state.ResponseID)
	scope.previousResponseID = state.ResponseID
	return updated, scope, true
}

func codexResponseStateAnchorIndex(inputItems []gjson.Result, state internalcache.CodexResponseState) int {
	anchor := -1
	if fingerprint := strings.TrimSpace(state.AssistantFingerprint); fingerprint != "" {
		for index := len(inputItems) - 1; index >= 0; index-- {
			if codexReplayAssistantMessageFingerprint(inputItems[index]) == fingerprint {
				anchor = index
				break
			}
		}
	}

	if len(state.CallIDs) == 0 {
		return anchor
	}
	callIDs := make(map[string]struct{}, len(state.CallIDs))
	for _, callID := range state.CallIDs {
		for _, candidate := range codexReplayComparableCallIDs(callID) {
			callIDs[candidate] = struct{}{}
		}
	}
	for index := len(inputItems) - 1; index >= 0; index-- {
		itemType := strings.TrimSpace(inputItems[index].Get("type").String())
		if itemType != "function_call" && itemType != "custom_tool_call" {
			continue
		}
		for _, candidate := range codexReplayComparableCallIDs(inputItems[index].Get("call_id").String()) {
			if _, ok := callIDs[candidate]; ok {
				if index > anchor {
					anchor = index
				}
				return anchor
			}
		}
	}
	return -1
}

func cacheCodexResponseStateFromCompleted(scope codexResponseStateScope, completedData []byte) {
	if !scope.valid() {
		return
	}
	response := gjson.GetBytes(completedData, "response")
	responseID := strings.TrimSpace(response.Get("id").String())
	output := response.Get("output")
	if responseID == "" || !output.IsArray() {
		return
	}

	state := internalcache.CodexResponseState{ResponseID: responseID}
	for _, item := range output.Array() {
		switch strings.TrimSpace(item.Get("type").String()) {
		case "message":
			if fingerprint := codexReplayAssistantMessageFingerprint(item); fingerprint != "" {
				state.AssistantFingerprint = fingerprint
			}
		case "function_call", "custom_tool_call":
			if callID := strings.TrimSpace(item.Get("call_id").String()); callID != "" {
				state.CallIDs = append(state.CallIDs, callID)
			}
		}
	}
	if state.AssistantFingerprint == "" && len(state.CallIDs) == 0 {
		return
	}
	internalcache.SetCodexResponseState(scope.authID, scope.modelName, scope.sessionKey, state)
}

func clearCodexResponseState(scope codexResponseStateScope) {
	if scope.valid() {
		internalcache.DeleteCodexResponseState(scope.authID, scope.modelName, scope.sessionKey)
	}
}

func isCodexPreviousResponseNotFound(statusCode int, body []byte) bool {
	code, _, ok := codexStatusErrorClassification(statusCode, body)
	return ok && code == "previous_response_not_found"
}

func codexResponseStateHeaders(headers http.Header, scope codexResponseStateScope, usedPreviousResponse bool) http.Header {
	out := headers.Clone()
	if !scope.valid() {
		return out
	}
	out.Set("X-CLIProxy-Response-State", "full")
	if usedPreviousResponse && scope.previousResponseID != "" {
		out.Set("X-CLIProxy-Response-State", "previous")
		out.Set("X-CLIProxy-Previous-Response-ID", scope.previousResponseID)
	}
	return out
}
