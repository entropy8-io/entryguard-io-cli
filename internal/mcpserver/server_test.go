package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/entryguard-io/cli/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeBackend is just enough of the EntryGuard API for the tools to talk to.
type fakeBackend struct {
	mu       sync.Mutex
	started  []map[string]any
	stopped  []string
	extended []map[string]any
	polls    int
	apiKeys  []string
}

func (f *fakeBackend) handler(t *testing.T) http.Handler {
	const id = "0f0f0f0f-1111-2222-3333-444444444444"
	session := func(status string) api.Session {
		return api.Session{ID: id, Status: status, Label: "claude: test", Scoped: true, Ipv4Address: "203.0.113.9",
			ResourceIps: []api.SessionResourceIp{{ResourceName: "db-sg", Status: "APPLIED", IpVersion: 4, IpAddress: "203.0.113.9"},
				{ResourceName: "db-tunnel", Status: "APPLIED", ProviderRuleId: "tunnel:41234:203.0.113.9/32"}}}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.apiKeys = append(f.apiKeys, r.Header.Get("X-API-Key"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/auth/me":
			_ = json.NewEncoder(w).Encode(api.UserInfo{Email: "bot@example.com", Name: "Bot", OrganizationName: "Acme", SubscriptionTier: "PAID"})
		case r.Method == "GET" && r.URL.Path == "/sessions/resources":
			_ = json.NewEncoder(w).Encode(api.AvailableResources{MaxDurationMinutes: 480, Resources: []api.AvailableResource{
				{ID: "r1", Name: "db-sg", Provider: "AWS", ResourceType: "security_group"}}})
		case r.Method == "POST" && r.URL.Path == "/sessions":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if res, _ := body["resources"].([]any); len(res) > 0 && res[0] == "nope" {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"status":400,"message":"Not available to you (unknown, inactive or not assigned): nope"}`))
				return
			}
			f.started = append(f.started, body)
			_ = json.NewEncoder(w).Encode(session("PENDING"))
		case r.Method == "GET" && r.URL.Path == "/sessions/"+id:
			f.polls++
			status := "PENDING"
			if f.polls >= 2 {
				status = "ACTIVE"
			}
			_ = json.NewEncoder(w).Encode(session(status))
		case r.Method == "GET" && r.URL.Path == "/sessions":
			_ = json.NewEncoder(w).Encode([]api.Session{session("ACTIVE"), {ID: "aaaaaaaa-0000-0000-0000-000000000000", Status: "EXPIRED"}})
		case r.Method == "POST" && r.URL.Path == "/sessions/"+id+"/stop":
			f.stopped = append(f.stopped, id)
			_ = json.NewEncoder(w).Encode(session("CANCELLED"))
		case r.Method == "POST" && r.URL.Path == "/sessions/"+id+"/extend":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.extended = append(f.extended, body)
			_ = json.NewEncoder(w).Encode(session("ACTIVE"))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	})
}

func connect(t *testing.T, backend *fakeBackend) (*mcp.ClientSession, func()) {
	t.Helper()
	srv := httptest.NewServer(backend.handler(t))
	server := New(Options{
		Client:       api.NewClient(srv.URL, "eg_test"),
		Version:      "test",
		DetectIPs:    func() (string, string) { return "203.0.113.9", "" },
		PollInterval: time.Millisecond,
	})
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	return cs, func() {
		_ = cs.Close()
		_ = serverSession.Wait()
		srv.Close()
	}
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, map[string]any) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: protocol error %v", name, err)
	}
	var out map[string]any
	if res.StructuredContent != nil {
		raw, _ := json.Marshal(res.StructuredContent)
		_ = json.Unmarshal(raw, &out)
	}
	return res, out
}

func text(res *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func TestToolsAreListedWithAnnotations(t *testing.T) {
	cs, done := connect(t, &fakeBackend{})
	defer done()
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tool := range tools.Tools {
		byName[tool.Name] = tool
	}
	for _, name := range []string{"whoami", "detect_ip", "list_resources", "list_sessions", "get_session", "start_session", "stop_session", "extend_session"} {
		if byName[name] == nil {
			t.Errorf("tool %s missing", name)
		}
	}
	if !byName["list_resources"].Annotations.ReadOnlyHint || byName["start_session"].Annotations.ReadOnlyHint {
		t.Error("read-only hints wrong")
	}
	schema, _ := json.Marshal(byName["start_session"].InputSchema)
	for _, required := range []string{`"resources"`, `"duration"`, `"label"`} {
		if !strings.Contains(string(schema), required) {
			t.Errorf("start_session schema lacks %s: %s", required, schema)
		}
	}
}

func TestListResourcesAndWhoami(t *testing.T) {
	backend := &fakeBackend{}
	cs, done := connect(t, backend)
	defer done()

	_, out := call(t, cs, "list_resources", nil)
	if out["maxDurationMinutes"] != float64(480) {
		t.Errorf("maxDurationMinutes %v", out["maxDurationMinutes"])
	}
	if res, _ := out["resources"].([]any); len(res) != 1 || res[0].(map[string]any)["name"] != "db-sg" {
		t.Errorf("resources %v", out["resources"])
	}

	_, me := call(t, cs, "whoami", nil)
	if me["email"] != "bot@example.com" || me["organization"] != "Acme" {
		t.Errorf("whoami %v", me)
	}
	if backend.apiKeys[0] != "eg_test" {
		t.Errorf("API key not sent: %v", backend.apiKeys)
	}
}

func TestStartSessionWaitsForActiveAndMapsTunnels(t *testing.T) {
	backend := &fakeBackend{}
	cs, done := connect(t, backend)
	defer done()

	res, out := call(t, cs, "start_session", map[string]any{
		"resources": []string{"db-sg", "db-tunnel"}, "duration": "15m", "label": "claude: test"})
	if res.IsError {
		t.Fatalf("tool error: %s", text(res))
	}
	if out["status"] != "ACTIVE" {
		t.Errorf("expected ACTIVE after waiting, got %v", out["status"])
	}
	if backend.polls < 2 {
		t.Errorf("expected polling until ACTIVE, polls=%d", backend.polls)
	}
	body := backend.started[0]
	if body["durationMinutes"] != float64(15) || body["label"] != "claude: test" || body["ipv4Address"] != "203.0.113.9" {
		t.Errorf("request body %v", body)
	}
	resources := out["resources"].([]any)
	if len(resources) != 2 {
		t.Fatalf("resources %v", resources)
	}
	tunnel := resources[1].(map[string]any)
	if tunnel["connect"] != "edge.entryguard.io:41234" {
		t.Errorf("tunnel connect %v", tunnel)
	}
}

func TestStartSessionRefusesUnscopedOrUnlabelled(t *testing.T) {
	backend := &fakeBackend{}
	cs, done := connect(t, backend)
	defer done()

	res, _ := call(t, cs, "start_session", map[string]any{"duration": "15m", "label": "x"})
	if !res.IsError || !strings.Contains(text(res), "resources") {
		t.Errorf("expected schema error for a missing resources field, got %q", text(res))
	}
	res, _ = call(t, cs, "start_session", map[string]any{"resources": []string{}, "duration": "15m", "label": "x"})
	if !res.IsError || !strings.Contains(text(res), "resources is required") {
		t.Errorf("expected resources error, got %q", text(res))
	}
	res, _ = call(t, cs, "start_session", map[string]any{"resources": []string{"db-sg"}, "duration": "15m", "label": "  "})
	if !res.IsError || !strings.Contains(text(res), "label is required") {
		t.Errorf("expected label error, got %q", text(res))
	}
	res, _ = call(t, cs, "start_session", map[string]any{"resources": []string{"db-sg"}, "duration": "2m", "label": "x"})
	if !res.IsError || !strings.Contains(text(res), "at least 5 minutes") {
		t.Errorf("expected duration error, got %q", text(res))
	}
	if len(backend.started) != 0 {
		t.Errorf("nothing should have reached the API, got %v", backend.started)
	}
}

func TestServerErrorsBecomeToolErrors(t *testing.T) {
	cs, done := connect(t, &fakeBackend{})
	defer done()
	res, _ := call(t, cs, "start_session", map[string]any{"resources": []string{"nope"}, "duration": "15m", "label": "x", "wait": false})
	if !res.IsError || !strings.Contains(text(res), "Not available to you") {
		t.Errorf("expected API message, got %q", text(res))
	}
}

func TestStopExtendAndListWithPrefixIds(t *testing.T) {
	backend := &fakeBackend{}
	cs, done := connect(t, backend)
	defer done()

	_, out := call(t, cs, "list_sessions", map[string]any{"active_only": true})
	sessions := out["sessions"].([]any)
	if len(sessions) != 1 || sessions[0].(map[string]any)["status"] != "ACTIVE" {
		t.Errorf("active_only: %v", sessions)
	}

	res, out := call(t, cs, "stop_session", map[string]any{"session_id": "0f0f0f0f"})
	if res.IsError || out["status"] != "CANCELLED" || len(backend.stopped) != 1 {
		t.Errorf("stop by prefix: err=%v out=%v stopped=%v", res.IsError, out, backend.stopped)
	}

	res, _ = call(t, cs, "extend_session", map[string]any{"session_id": "0f0f0f0f-1111-2222-3333-444444444444", "hours": 2})
	if res.IsError || backend.extended[0]["additionalHours"] != float64(2) {
		t.Errorf("extend: err=%v body=%v", res.IsError, backend.extended)
	}

	res, _ = call(t, cs, "get_session", map[string]any{"session_id": "zzz"})
	if !res.IsError || !strings.Contains(text(res), "no session found") {
		t.Errorf("unknown prefix: %q", text(res))
	}
}
