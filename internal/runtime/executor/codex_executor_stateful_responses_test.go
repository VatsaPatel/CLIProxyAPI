package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	internalcache "github.com/router-for-me/CLIProxyAPI/v7/internal/cache"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestCodexExecutorStatefulClaudeUsesPreviousResponseID(t *testing.T) {
	internalcache.ClearCodexResponseStateCache()
	internalcache.ClearCodexReasoningReplayCache()
	t.Cleanup(internalcache.ClearCodexResponseStateCache)
	t.Cleanup(internalcache.ClearCodexReasoningReplayCache)

	var (
		mu     sync.Mutex
		bodies [][]byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, errRead := io.ReadAll(r.Body)
		if errRead != nil {
			t.Errorf("read body: %v", errRead)
			return
		}
		mu.Lock()
		bodies = append(bodies, body)
		requestNumber := len(bodies)
		mu.Unlock()

		responseID := "resp-1"
		text := "answer one"
		if requestNumber == 2 {
			responseID = "resp-2"
			text = "answer two"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"type":"response.created","response":{"id":"` + responseID + `","model":"gpt-5.6-sol"}}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.completed","response":{"id":"` + responseID + `","model":"gpt-5.6-sol","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + text + `"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}` + "\n\n"))
	}))
	defer server.Close()

	executor := NewCodexExecutor(&config.Config{SDKConfig: config.SDKConfig{DisableImageGeneration: config.DisableImageGenerationAll}})
	auth := &cliproxyauth.Auth{
		ID: "azure-auth",
		Attributes: map[string]string{
			"base_url":           server.URL,
			"api_key":            "test",
			"stateful_responses": "true",
		},
	}
	opts := cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FormatClaude,
		Stream:       true,
	}
	first := cliproxyexecutor.Request{
		Model: "gpt-5.6-sol",
		Payload: []byte(`{"model":"gpt-5.6-sol","stream":true,"metadata":{"user_id":"test_session_11111111-1111-1111-1111-111111111111"},"messages":[` +
			`{"role":"user","content":"first"}]}`),
	}
	firstHeaders := drainCodexStatefulStream(t, executor, auth, first, opts)

	second := cliproxyexecutor.Request{
		Model: "gpt-5.6-sol",
		Payload: []byte(`{"model":"gpt-5.6-sol","stream":true,"metadata":{"user_id":"test_session_11111111-1111-1111-1111-111111111111"},"messages":[` +
			`{"role":"user","content":"first"},` +
			`{"role":"assistant","content":"answer one"},` +
			`{"role":"user","content":"second"}]}`),
	}
	secondHeaders := drainCodexStatefulStream(t, executor, auth, second, opts)

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("request count = %d, want 2", len(bodies))
	}
	if !gjson.GetBytes(bodies[0], "store").Bool() {
		t.Fatalf("first store = false: %s", bodies[0])
	}
	if gjson.GetBytes(bodies[0], "previous_response_id").Exists() {
		t.Fatalf("first request has previous_response_id: %s", bodies[0])
	}
	if got := gjson.GetBytes(bodies[1], "previous_response_id").String(); got != "resp-1" {
		t.Fatalf("second previous_response_id = %q, want resp-1: %s", got, bodies[1])
	}
	firstCacheKey := gjson.GetBytes(bodies[0], "prompt_cache_key").String()
	secondCacheKey := gjson.GetBytes(bodies[1], "prompt_cache_key").String()
	if firstCacheKey == "" || secondCacheKey != firstCacheKey {
		t.Fatalf("prompt cache keys are not stable: first=%q second=%q", firstCacheKey, secondCacheKey)
	}
	if got := firstHeaders.Get("X-CLIProxy-Response-State"); got != "full" {
		t.Fatalf("first response state header = %q, want full", got)
	}
	if got := secondHeaders.Get("X-CLIProxy-Response-State"); got != "previous" {
		t.Fatalf("second response state header = %q, want previous", got)
	}
	if got := secondHeaders.Get("X-CLIProxy-Previous-Response-ID"); got != "resp-1" {
		t.Fatalf("second previous response header = %q, want resp-1", got)
	}
	input := gjson.GetBytes(bodies[1], "input").Array()
	if len(input) != 1 || input[0].Get("role").String() != "user" || input[0].Get("content.0.text").String() != "second" {
		t.Fatalf("second input is not incremental: %s", bodies[1])
	}
}

func TestCodexExecutorStatefulClaudeRetriesStaleResponseIDWithFullHistory(t *testing.T) {
	internalcache.ClearCodexResponseStateCache()
	internalcache.ClearCodexReasoningReplayCache()
	t.Cleanup(internalcache.ClearCodexResponseStateCache)
	t.Cleanup(internalcache.ClearCodexReasoningReplayCache)

	var (
		mu     sync.Mutex
		bodies [][]byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, body)
		requestNumber := len(bodies)
		mu.Unlock()

		if requestNumber == 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":"previous_response_not_found","message":"expired"}}`))
			return
		}
		responseID := "resp-1"
		text := "answer one"
		if requestNumber == 3 {
			responseID = "resp-3"
			text = "answer two"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"type":"response.created","response":{"id":"` + responseID + `","model":"gpt-5.6-sol"}}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.completed","response":{"id":"` + responseID + `","model":"gpt-5.6-sol","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + text + `"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}` + "\n\n"))
	}))
	defer server.Close()

	executor := NewCodexExecutor(&config.Config{SDKConfig: config.SDKConfig{DisableImageGeneration: config.DisableImageGenerationAll}})
	auth := &cliproxyauth.Auth{
		ID: "azure-auth",
		Attributes: map[string]string{
			"base_url":           server.URL,
			"api_key":            "test",
			"stateful_responses": "true",
		},
	}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatClaude, Stream: true}
	session := `"metadata":{"user_id":"test_session_22222222-2222-2222-2222-222222222222"},`
	drainCodexStatefulStream(t, executor, auth, cliproxyexecutor.Request{
		Model:   "gpt-5.6-sol",
		Payload: []byte(`{"model":"gpt-5.6-sol","stream":true,` + session + `"messages":[{"role":"user","content":"first"}]}`),
	}, opts)
	drainCodexStatefulStream(t, executor, auth, cliproxyexecutor.Request{
		Model: "gpt-5.6-sol",
		Payload: []byte(`{"model":"gpt-5.6-sol","stream":true,` + session + `"messages":[` +
			`{"role":"user","content":"first"},{"role":"assistant","content":"answer one"},{"role":"user","content":"second"}]}`),
	}, opts)

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 3 {
		t.Fatalf("request count = %d, want 3", len(bodies))
	}
	if gjson.GetBytes(bodies[2], "previous_response_id").Exists() {
		t.Fatalf("retry retained previous_response_id: %s", bodies[2])
	}
	if got := len(gjson.GetBytes(bodies[2], "input").Array()); got != 3 {
		t.Fatalf("retry input count = %d, want full history of 3: %s", got, bodies[2])
	}
}

func TestCodexExecutorStatelessClaudeReplaysOpenRouterContract(t *testing.T) {
	internalcache.ClearCodexReasoningReplayCache()
	t.Cleanup(internalcache.ClearCodexReasoningReplayCache)

	signature := validCodexReasoningEncryptedContentForTest()
	var (
		mu     sync.Mutex
		bodies [][]byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, body)
		requestNumber := len(bodies)
		mu.Unlock()

		responseID := "resp-1"
		text := "answer one"
		if requestNumber == 2 {
			responseID = "resp-2"
			text = "answer two"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"type":"response.created","response":{"id":"` + responseID + `","model":"openai/gpt-5.6-sol"}}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.completed","response":{"id":"` + responseID + `","model":"openai/gpt-5.6-sol","status":"completed","output":[{"type":"reasoning","encrypted_content":"` + signature + `","summary":[]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + text + `"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}` + "\n\n"))
	}))
	defer server.Close()

	executor := NewCodexExecutor(&config.Config{SDKConfig: config.SDKConfig{DisableImageGeneration: config.DisableImageGenerationAll}})
	auth := &cliproxyauth.Auth{
		ID: "openrouter-auth",
		Attributes: map[string]string{
			"base_url": server.URL,
			"api_key":  "test",
		},
	}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatClaude, Stream: true}
	session := `"metadata":{"user_id":"test_session_33333333-3333-3333-3333-333333333333"},`
	drainCodexStatefulStream(t, executor, auth, cliproxyexecutor.Request{
		Model:   "openai/gpt-5.6-sol",
		Payload: []byte(`{"model":"openai/gpt-5.6-sol","stream":true,` + session + `"thinking":{"type":"enabled","budget_tokens":24576},"messages":[{"role":"user","content":"first"}]}`),
	}, opts)
	drainCodexStatefulStream(t, executor, auth, cliproxyexecutor.Request{
		Model: "openai/gpt-5.6-sol",
		Payload: []byte(`{"model":"openai/gpt-5.6-sol","stream":true,` + session + `"thinking":{"type":"enabled","budget_tokens":24576},"messages":[` +
			`{"role":"user","content":"first"},` +
			`{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"` + signature + `"},{"type":"text","text":"answer one"}]},` +
			`{"role":"user","content":"second"}]}`),
	}, opts)

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("request count = %d, want 2", len(bodies))
	}
	for index, body := range bodies {
		if gjson.GetBytes(body, "store").Bool() {
			t.Fatalf("request %d store = true: %s", index+1, body)
		}
		if gjson.GetBytes(body, "previous_response_id").Exists() {
			t.Fatalf("request %d has previous_response_id: %s", index+1, body)
		}
		if got := gjson.GetBytes(body, "include.0").String(); got != "reasoning.encrypted_content" {
			t.Fatalf("request %d include = %q", index+1, got)
		}
	}
	firstCacheKey := gjson.GetBytes(bodies[0], "prompt_cache_key").String()
	secondCacheKey := gjson.GetBytes(bodies[1], "prompt_cache_key").String()
	if firstCacheKey == "" || secondCacheKey != firstCacheKey {
		t.Fatalf("prompt cache keys are not stable: first=%q second=%q", firstCacheKey, secondCacheKey)
	}
	if got := gjson.GetBytes(bodies[1], `input.#(type=="reasoning").encrypted_content`).String(); got != signature {
		t.Fatalf("second request did not replay encrypted reasoning")
	}
	if got := len(gjson.GetBytes(bodies[1], "input").Array()); got < 4 {
		t.Fatalf("second request input count = %d, want full transcript replay: %s", got, bodies[1])
	}
}

func drainCodexStatefulStream(t *testing.T, executor *CodexExecutor, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) http.Header {
	t.Helper()
	result, err := executor.ExecuteStream(context.Background(), auth, req, opts)
	if err != nil {
		t.Fatalf("ExecuteStream() error = %v", err)
	}
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream error = %v", chunk.Err)
		}
	}
	return result.Headers
}
