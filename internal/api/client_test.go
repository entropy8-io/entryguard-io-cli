package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseSessionDuration(t *testing.T) {
	hours := func(n int) SessionDuration { return SessionDuration{Hours: &n} }
	minutes := func(n int) SessionDuration { return SessionDuration{Minutes: &n} }
	cases := []struct {
		in   string
		want SessionDuration
		err  bool
	}{
		{"", SessionDuration{}, false},
		{"4", hours(4), false},
		{"4h", hours(4), false},
		{"120m", hours(2), false},
		{"15m", minutes(15), false},
		{"1h30m", minutes(90), false},
		{"0", SessionDuration{}, true},
		{"-1", SessionDuration{}, true},
		{"4m", SessionDuration{}, true},
		{"30s", SessionDuration{}, true},
		{"abc", SessionDuration{}, true},
	}
	for _, c := range cases {
		got, err := ParseSessionDuration(c.in)
		if c.err {
			if err == nil {
				t.Errorf("%q: expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error %v", c.in, err)
			continue
		}
		if !sameInt(got.Hours, c.want.Hours) || !sameInt(got.Minutes, c.want.Minutes) {
			t.Errorf("%q: got hours=%v minutes=%v", c.in, deref(got.Hours), deref(got.Minutes))
		}
	}
}

func sameInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestStartSessionSendsScopedFieldsAndApiKey(t *testing.T) {
	var got map[string]any
	var apiKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/sessions" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		apiKey = r.Header.Get("X-API-Key")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(Session{ID: "s1", Status: "PENDING", Label: "ci run", Scoped: true})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "eg_test_key")
	req := &StartSessionRequest{Resources: []string{"db-sg"}, Label: "ci run", Ipv4Address: "203.0.113.5"}
	d, _ := ParseSessionDuration("15m")
	d.Apply(req)
	session, err := client.StartSession(req)
	if err != nil {
		t.Fatal(err)
	}
	if apiKey != "eg_test_key" {
		t.Errorf("X-API-Key = %q", apiKey)
	}
	if got["durationMinutes"] != float64(15) || got["durationHours"] != nil {
		t.Errorf("duration fields: %v", got)
	}
	if res, _ := got["resources"].([]any); len(res) != 1 || res[0] != "db-sg" {
		t.Errorf("resources: %v", got["resources"])
	}
	if got["label"] != "ci run" || got["ipv4Address"] != "203.0.113.5" {
		t.Errorf("body: %v", got)
	}
	if !session.Scoped || session.Label != "ci run" {
		t.Errorf("response not parsed: %+v", session)
	}
}

func TestStartSessionWithoutResourcesOmitsThem(t *testing.T) {
	var raw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		raw = string(buf[:n])
		_ = json.NewEncoder(w).Encode(Session{ID: "s1"})
	}))
	defer srv.Close()
	if _, err := NewClient(srv.URL, "k").StartSession(&StartSessionRequest{}); err != nil {
		t.Fatal(err)
	}
	if raw != "{}" {
		t.Errorf("empty request should serialize to {}, got %s", raw)
	}
}

func TestListAvailableResources(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions/resources" {
			t.Errorf("path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(AvailableResources{MaxDurationMinutes: 480, Resources: []AvailableResource{
			{ID: "r1", Name: "db-sg", Provider: "AWS", ResourceType: "security_group", Region: "eu-central-1"},
		}})
	}))
	defer srv.Close()
	out, err := NewClient(srv.URL, "k").ListAvailableResources()
	if err != nil {
		t.Fatal(err)
	}
	if out.MaxDurationMinutes != 480 || len(out.Resources) != 1 || out.Resources[0].Name != "db-sg" {
		t.Errorf("parsed %+v", out)
	}
}

func TestWaitForSessionPollsUntilSettled(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		status := "PENDING"
		if n >= 3 {
			status = "ACTIVE"
		}
		_ = json.NewEncoder(w).Encode(Session{ID: "s1", Status: status})
	}))
	defer srv.Close()
	session, err := NewClient(srv.URL, "k").WaitForSession("s1", 5*time.Second, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != "ACTIVE" || atomic.LoadInt32(&calls) != 3 {
		t.Errorf("status %s after %d calls", session.Status, calls)
	}
}

func TestWaitForSessionTimesOutButReturnsTheSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Session{ID: "s1", Status: "PENDING"})
	}))
	defer srv.Close()
	session, err := NewClient(srv.URL, "k").WaitForSession("s1", 5*time.Millisecond, time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if session == nil || session.ID != "s1" {
		t.Errorf("expected last session on timeout, got %+v", session)
	}
}

func TestApiErrorsSurfaceTheServerMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"status":400,"message":"Not available to you (unknown, inactive or not assigned): nope"}`))
	}))
	defer srv.Close()
	_, err := NewClient(srv.URL, "k").StartSession(&StartSessionRequest{Resources: []string{"nope"}})
	if err == nil || err.Error() != "Not available to you (unknown, inactive or not assigned): nope" {
		t.Errorf("got %v", err)
	}
}
