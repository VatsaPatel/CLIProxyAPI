package cache

import (
	"strings"
	"sync"
	"time"
)

const (
	CodexResponseStateTTL        = time.Hour
	codexResponseStateMaxEntries = 10240
)

// CodexResponseState tracks one stored Responses API turn for a selected credential.
type CodexResponseState struct {
	ResponseID           string
	AssistantFingerprint string
	CallIDs              []string
}

type codexResponseStateEntry struct {
	State     CodexResponseState
	Timestamp time.Time
}

var (
	codexResponseStateMu      sync.Mutex
	codexResponseStateEntries = make(map[string]codexResponseStateEntry)
)

func SetCodexResponseState(authID, modelName, sessionKey string, state CodexResponseState) bool {
	key := codexResponseStateKey(authID, modelName, sessionKey)
	state.ResponseID = strings.TrimSpace(state.ResponseID)
	if key == "" || state.ResponseID == "" {
		return false
	}
	state.AssistantFingerprint = strings.TrimSpace(state.AssistantFingerprint)
	state.CallIDs = normalizedCodexResponseStateCallIDs(state.CallIDs)

	now := time.Now()
	codexResponseStateMu.Lock()
	codexResponseStateEntries[key] = codexResponseStateEntry{
		State:     cloneCodexResponseState(state),
		Timestamp: now,
	}
	if len(codexResponseStateEntries) > codexResponseStateMaxEntries {
		evictOldestCodexResponseStates(len(codexResponseStateEntries) - codexResponseStateMaxEntries + 128)
	}
	codexResponseStateMu.Unlock()
	return true
}

func GetCodexResponseState(authID, modelName, sessionKey string) (CodexResponseState, bool) {
	key := codexResponseStateKey(authID, modelName, sessionKey)
	if key == "" {
		return CodexResponseState{}, false
	}
	now := time.Now()
	codexResponseStateMu.Lock()
	entry, ok := codexResponseStateEntries[key]
	if !ok {
		codexResponseStateMu.Unlock()
		return CodexResponseState{}, false
	}
	if now.Sub(entry.Timestamp) > CodexResponseStateTTL {
		delete(codexResponseStateEntries, key)
		codexResponseStateMu.Unlock()
		return CodexResponseState{}, false
	}
	entry.Timestamp = now
	codexResponseStateEntries[key] = entry
	codexResponseStateMu.Unlock()
	return cloneCodexResponseState(entry.State), true
}

func DeleteCodexResponseState(authID, modelName, sessionKey string) {
	key := codexResponseStateKey(authID, modelName, sessionKey)
	if key == "" {
		return
	}
	codexResponseStateMu.Lock()
	delete(codexResponseStateEntries, key)
	codexResponseStateMu.Unlock()
}

func ClearCodexResponseStateCache() {
	codexResponseStateMu.Lock()
	codexResponseStateEntries = make(map[string]codexResponseStateEntry)
	codexResponseStateMu.Unlock()
}

func codexResponseStateKey(authID, modelName, sessionKey string) string {
	authID = strings.TrimSpace(authID)
	modelName = strings.TrimSpace(modelName)
	sessionKey = strings.TrimSpace(sessionKey)
	if authID == "" || modelName == "" || sessionKey == "" {
		return ""
	}
	return strings.Join([]string{"codex-response-state", authID, modelName, sessionKey}, "\x00")
}

func normalizedCodexResponseStateCallIDs(callIDs []string) []string {
	seen := make(map[string]struct{}, len(callIDs))
	out := make([]string, 0, len(callIDs))
	for _, callID := range callIDs {
		callID = strings.TrimSpace(callID)
		if callID == "" {
			continue
		}
		if _, ok := seen[callID]; ok {
			continue
		}
		seen[callID] = struct{}{}
		out = append(out, callID)
	}
	return out
}

func cloneCodexResponseState(state CodexResponseState) CodexResponseState {
	state.CallIDs = append([]string(nil), state.CallIDs...)
	return state
}

func evictOldestCodexResponseStates(count int) {
	for count > 0 && len(codexResponseStateEntries) > 0 {
		oldestKey := ""
		oldestTime := time.Time{}
		for key, entry := range codexResponseStateEntries {
			if oldestKey == "" || entry.Timestamp.Before(oldestTime) {
				oldestKey = key
				oldestTime = entry.Timestamp
			}
		}
		delete(codexResponseStateEntries, oldestKey)
		count--
	}
}
