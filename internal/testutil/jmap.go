// Package testutil supplies isolated protocol servers, never real account access.
package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JangoCG/fastmail-cli/internal/jmap"
)

type Method struct {
	Name  string
	Args  map[string]any
	Using []string
}
type Handler func(Method) any

func Server(t *testing.T, handler Handler) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing bearer authentication")
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/session" {
			caps := map[string]any{}
			primary := map[string]string{}
			for _, cap := range []string{jmap.Core, jmap.Mail, jmap.Submission, jmap.Contacts, jmap.Masked, jmap.Vacation} {
				caps[cap] = map[string]any{}
				primary[cap] = "account"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": caps, "accounts": map[string]any{"account": map[string]any{"name": "Alice", "isPersonal": true, "accountCapabilities": caps}}, "primaryAccounts": primary, "username": "alice@example.com", "apiUrl": server.URL + "/api", "uploadUrl": server.URL + "/upload/{accountId}", "downloadUrl": server.URL + "/download/{accountId}/{blobId}/{name}?type={type}"})
			return
		}
		if r.URL.Path != "/api" {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		var req struct {
			Using       []string            `json:"using"`
			MethodCalls [][]json.RawMessage `json:"methodCalls"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("invalid JMAP request: %v", err)
			w.WriteHeader(400)
			return
		}
		if len(req.MethodCalls) != 1 || len(req.MethodCalls[0]) != 3 {
			t.Errorf("expected one JMAP method call")
			w.WriteHeader(400)
			return
		}
		var name, tag string
		var args map[string]any
		_ = json.Unmarshal(req.MethodCalls[0][0], &name)
		_ = json.Unmarshal(req.MethodCalls[0][1], &args)
		_ = json.Unmarshal(req.MethodCalls[0][2], &tag)
		if args["accountId"] != "account" {
			t.Errorf("account=%v", args["accountId"])
		}
		value := handler(Method{Name: name, Args: args, Using: req.Using})
		if err := json.NewEncoder(w).Encode(map[string]any{"methodResponses": []any{[]any{name, value, tag}}}); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	server.Start()
	t.Cleanup(server.Close)
	return server
}
func Client(t *testing.T, handler Handler) *jmap.Client {
	t.Helper()
	s := Server(t, handler)
	return jmap.New("test-token", s.URL+"/session", "")
}
