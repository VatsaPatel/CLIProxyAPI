package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"github.com/tidwall/gjson"
)

func TestOpenAICompatExecutorEmbeddings(t *testing.T) {
	var upstreamBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("path = %q, want /embeddings", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Copilot-Integration-Id"); got != "copilot-developer-cli" {
			t.Errorf("Copilot-Integration-Id = %q", got)
		}
		upstreamBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","embedding":[0.25],"index":0}],"usage":{"prompt_tokens":1,"total_tokens":1}}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{
		Provider: "openai-compatibility",
		Attributes: map[string]string{
			"base_url":                      server.URL,
			"api_key":                       "test-key",
			"header:Copilot-Integration-Id": "copilot-developer-cli",
		},
	}
	request := cliproxyexecutor.Request{
		Model:   "text-embedding-3-small",
		Payload: []byte(`{"model":"public-embedding","input":["hello"]}`),
	}
	response, errExecute := executor.Execute(context.Background(), auth, request, cliproxyexecutor.Options{Alt: "embeddings"})
	if errExecute != nil {
		t.Fatalf("Execute error: %v", errExecute)
	}
	if got := gjson.GetBytes(upstreamBody, "model").String(); got != "text-embedding-3-small" {
		t.Fatalf("upstream model = %q", got)
	}
	if got := gjson.GetBytes(response.Payload, "data.0.embedding.0").Float(); got != 0.25 {
		t.Fatalf("embedding = %v", got)
	}
}
