package cache

import (
	"testing"
	"time"
)

func TestCodexResponseStateCacheScopesAndClones(t *testing.T) {
	ClearCodexResponseStateCache()
	t.Cleanup(ClearCodexResponseStateCache)

	state := CodexResponseState{
		ResponseID:           "resp-1",
		AssistantFingerprint: "assistant-1",
		CallIDs:              []string{"call-1", "call-1", ""},
	}
	if !SetCodexResponseState("auth-a", "model-a", "session-a", state) {
		t.Fatal("SetCodexResponseState() = false")
	}

	got, ok := GetCodexResponseState("auth-a", "model-a", "session-a")
	if !ok {
		t.Fatal("GetCodexResponseState() = not found")
	}
	if got.ResponseID != "resp-1" || got.AssistantFingerprint != "assistant-1" {
		t.Fatalf("unexpected state: %+v", got)
	}
	if len(got.CallIDs) != 1 || got.CallIDs[0] != "call-1" {
		t.Fatalf("call IDs = %v, want [call-1]", got.CallIDs)
	}
	got.CallIDs[0] = "mutated"
	again, _ := GetCodexResponseState("auth-a", "model-a", "session-a")
	if again.CallIDs[0] != "call-1" {
		t.Fatalf("cached call IDs mutated: %v", again.CallIDs)
	}
	if _, ok = GetCodexResponseState("auth-b", "model-a", "session-a"); ok {
		t.Fatal("state leaked across auth IDs")
	}
}

func TestCodexResponseStateCacheExpires(t *testing.T) {
	ClearCodexResponseStateCache()
	t.Cleanup(ClearCodexResponseStateCache)

	if !SetCodexResponseState("auth", "model", "session", CodexResponseState{ResponseID: "resp-1"}) {
		t.Fatal("SetCodexResponseState() = false")
	}
	key := codexResponseStateKey("auth", "model", "session")
	codexResponseStateMu.Lock()
	entry := codexResponseStateEntries[key]
	entry.Timestamp = time.Now().Add(-CodexResponseStateTTL - time.Second)
	codexResponseStateEntries[key] = entry
	codexResponseStateMu.Unlock()

	if _, ok := GetCodexResponseState("auth", "model", "session"); ok {
		t.Fatal("expired state remained available")
	}
}
