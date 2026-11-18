// Package mcpserver exposes EntryGuard sessions to AI assistants over the Model
// Context Protocol. It is deliberately narrower than the REST API: an assistant
// can see what it may open, open a scoped and labelled session, wait for it,
// and stop it. It cannot change resources, roles or keys.
package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/entryguard-io/cli/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options wires the server to an EntryGuard API client.
type Options struct {
	Client  *api.Client
	Version string
	// DetectIPs finds the caller's public IPs; defaults to api.DetectIPs (ipify).
	DetectIPs func() (ipv4, ipv6 string)
	// Poll interval while waiting for a session to settle; defaults to 2s.
	PollInterval time.Duration
}

const (
	defaultWaitTimeout = 60 * time.Second
	maxWaitTimeout     = 5 * time.Minute
)

// New builds the MCP server with all EntryGuard tools registered.
func New(opts Options) *mcp.Server {
	if opts.DetectIPs == nil {
		opts.DetectIPs = api.DetectIPs
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = 2 * time.Second
	}
	s := &server{opts: opts}
	srv := mcp.NewServer(&mcp.Implementation{Name: "entryguard", Title: "EntryGuard", Version: opts.Version}, &mcp.ServerOptions{
		Instructions: `EntryGuard whitelists an IP on cloud resources for a limited time. To reach a protected resource:
1. list_resources to see what you may open and the longest duration allowed.
2. start_session with the exact resources you need, a short duration and a label saying why (wait=true, the default, returns once the rules are applied).
3. Do the work.
4. stop_session when done, even if the work failed.
Never ask for more resources or time than the task needs.`,
	})

	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(srv, &mcp.Tool{Name: "whoami", Description: "The user, organization and plan behind the configured API key.", Annotations: readOnly}, s.whoami)
	mcp.AddTool(srv, &mcp.Tool{Name: "detect_ip", Description: "The public IPv4/IPv6 this machine will be whitelisted with.", Annotations: readOnly}, s.detectIP)
	mcp.AddTool(srv, &mcp.Tool{Name: "list_resources", Description: "Resources the API key's user can open a session on (pass their names to start_session) and the longest session allowed.", Annotations: readOnly}, s.listResources)
	mcp.AddTool(srv, &mcp.Tool{Name: "list_sessions", Description: "The user's sessions, newest first. Use active_only to see what is open right now.", Annotations: readOnly}, s.listSessions)
	mcp.AddTool(srv, &mcp.Tool{Name: "get_session", Description: "Status and per-resource rule results of one session.", Annotations: readOnly}, s.getSession)
	mcp.AddTool(srv, &mcp.Tool{Name: "start_session", Description: "Whitelist this machine's public IP on the given resources for a limited time. Returns once the rules are applied (status ACTIVE) unless wait=false.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: false, OpenWorldHint: ptr(true)}}, s.startSession)
	mcp.AddTool(srv, &mcp.Tool{Name: "stop_session", Description: "End a session now and remove its IP rules. Call it when the work is done.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, DestructiveHint: ptr(false)}}, s.stopSession)
	mcp.AddTool(srv, &mcp.Tool{Name: "extend_session", Description: "Add whole hours to an active session, within the plan's maximum.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: false, DestructiveHint: ptr(false)}}, s.extendSession)
	return srv
}

type server struct {
	opts Options
}

func ptr[T any](v T) *T { return &v }

// --- inputs and outputs -----------------------------------------------------

type empty struct{}

type whoamiOut struct {
	Email            string `json:"email"`
	Name             string `json:"name"`
	Organization     string `json:"organization"`
	OrganizationSlug string `json:"organizationSlug"`
	SubscriptionTier string `json:"subscriptionTier"`
	IsOrgAdmin       bool   `json:"isOrgAdmin"`
}

type detectIPOut struct {
	Ipv4 string `json:"ipv4,omitempty" jsonschema:"public IPv4, empty when none"`
	Ipv6 string `json:"ipv6,omitempty" jsonschema:"public IPv6, empty when none"`
}

type listSessionsIn struct {
	ActiveOnly bool `json:"active_only,omitempty" jsonschema:"only sessions whose rules are currently applied (ACTIVE, PARTIAL, PENDING, EXPIRING)"`
}

type sessionIDIn struct {
	SessionID string `json:"session_id" jsonschema:"the session id (full UUID, or a unique prefix of it)"`
}

type startSessionIn struct {
	Resources   []string `json:"resources" jsonschema:"names or ids of the resources to open, from list_resources; only what the task needs"`
	Duration    string   `json:"duration" jsonschema:"how long the access is needed: minutes (15m, 1h30m) or hours (2h); minimum 5m; shortened to the allowed maximum"`
	Label       string   `json:"label" jsonschema:"why the access is needed, shown to admins in Session activity and the audit log (e.g. 'claude-code: run V041 migration on staging')"`
	Ipv4Address string   `json:"ipv4_address,omitempty" jsonschema:"public IPv4 to whitelist instead of this machine's detected address"`
	Ipv6Address string   `json:"ipv6_address,omitempty" jsonschema:"public IPv6 to whitelist instead of this machine's detected address"`
	Wait        *bool    `json:"wait,omitempty" jsonschema:"wait until the rules are applied before returning (default true)"`
	WaitSeconds int      `json:"wait_seconds,omitempty" jsonschema:"how long to wait, default 60, max 300"`
}

type extendSessionIn struct {
	SessionID string `json:"session_id" jsonschema:"the session id (full UUID, or a unique prefix of it)"`
	Hours     int    `json:"hours" jsonschema:"whole hours to add"`
}

// sessionOut is the tool-facing view of a session: the fields an assistant acts on.
type sessionOut struct {
	ID        string               `json:"id"`
	Status    string               `json:"status" jsonschema:"PENDING (rules being applied), ACTIVE, PARTIAL (some resources failed), FAILED, EXPIRING, EXPIRED, CANCELLED"`
	Label     string               `json:"label,omitempty"`
	Scoped    bool                 `json:"scoped"`
	Ipv4      string               `json:"ipv4,omitempty"`
	Ipv6      string               `json:"ipv6,omitempty"`
	StartedAt string               `json:"startedAt,omitempty"`
	ExpiresAt string               `json:"expiresAt,omitempty"`
	EndedAt   string               `json:"endedAt,omitempty"`
	Resources []sessionResourceOut `json:"resources"`
}

type sessionResourceOut struct {
	Name    string `json:"name"`
	Status  string `json:"status" jsonschema:"PENDING, APPLIED, FAILED or REMOVED"`
	Error   string `json:"error,omitempty"`
	Connect string `json:"connect,omitempty" jsonschema:"for tunnel resources: host:port to connect to"`
}

type listSessionsOut struct {
	Sessions []sessionOut `json:"sessions"`
}

func toSessionOut(s *api.Session) sessionOut {
	out := sessionOut{
		ID: s.ID, Status: s.Status, Label: s.Label, Scoped: s.Scoped,
		Ipv4: s.Ipv4Address, Ipv6: s.Ipv6Address,
		StartedAt: s.StartedAt, ExpiresAt: s.ExpiresAt, EndedAt: s.EndedAt,
		Resources: []sessionResourceOut{},
	}
	seen := map[string]bool{}
	for _, r := range s.ResourceIps {
		if seen[r.ResourceName] {
			continue
		}
		seen[r.ResourceName] = true
		res := sessionResourceOut{Name: r.ResourceName, Status: r.Status, Error: r.ErrorMessage}
		if strings.HasPrefix(r.ProviderRuleId, "tunnel:") {
			if parts := strings.SplitN(r.ProviderRuleId, ":", 3); len(parts) >= 2 {
				res.Connect = "edge.entryguard.io:" + parts[1]
			}
		}
		out.Resources = append(out.Resources, res)
	}
	return out
}

// --- handlers ---------------------------------------------------------------

func (s *server) whoami(_ context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, whoamiOut, error) {
	me, err := s.opts.Client.GetMe()
	if err != nil {
		return nil, whoamiOut{}, err
	}
	return nil, whoamiOut{Email: me.Email, Name: me.Name, Organization: me.OrganizationName,
		OrganizationSlug: me.OrganizationSlug, SubscriptionTier: me.SubscriptionTier, IsOrgAdmin: me.IsOrgAdmin}, nil
}

func (s *server) detectIP(_ context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, detectIPOut, error) {
	ipv4, ipv6 := s.opts.DetectIPs()
	if ipv4 == "" && ipv6 == "" {
		if ip, err := s.opts.Client.DetectIP(); err == nil {
			if ip.Version == 6 {
				ipv6 = ip.IP
			} else {
				ipv4 = ip.IP
			}
		}
	}
	if ipv4 == "" && ipv6 == "" {
		return nil, detectIPOut{}, fmt.Errorf("could not detect a public IP address")
	}
	return nil, detectIPOut{Ipv4: ipv4, Ipv6: ipv6}, nil
}

func (s *server) listResources(_ context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, *api.AvailableResources, error) {
	out, err := s.opts.Client.ListAvailableResources()
	if err != nil {
		return nil, nil, err
	}
	if out.Resources == nil {
		out.Resources = []api.AvailableResource{}
	}
	return nil, out, nil
}

func (s *server) listSessions(_ context.Context, _ *mcp.CallToolRequest, in listSessionsIn) (*mcp.CallToolResult, listSessionsOut, error) {
	sessions, err := s.opts.Client.ListSessions()
	if err != nil {
		return nil, listSessionsOut{}, err
	}
	out := listSessionsOut{Sessions: []sessionOut{}}
	for i := range sessions {
		if in.ActiveOnly && !isOpen(sessions[i].Status) {
			continue
		}
		out.Sessions = append(out.Sessions, toSessionOut(&sessions[i]))
	}
	return nil, out, nil
}

func isOpen(status string) bool {
	switch status {
	case "ACTIVE", "PARTIAL", "PENDING", "EXPIRING":
		return true
	}
	return false
}

func (s *server) getSession(_ context.Context, _ *mcp.CallToolRequest, in sessionIDIn) (*mcp.CallToolResult, sessionOut, error) {
	id, err := s.resolveID(in.SessionID)
	if err != nil {
		return nil, sessionOut{}, err
	}
	session, err := s.opts.Client.GetSession(id)
	if err != nil {
		return nil, sessionOut{}, err
	}
	return nil, toSessionOut(session), nil
}

func (s *server) startSession(_ context.Context, _ *mcp.CallToolRequest, in startSessionIn) (*mcp.CallToolResult, sessionOut, error) {
	if len(in.Resources) == 0 {
		return nil, sessionOut{}, fmt.Errorf("resources is required: name only what the task needs (see list_resources)")
	}
	if strings.TrimSpace(in.Label) == "" {
		return nil, sessionOut{}, fmt.Errorf("label is required: say which tool is asking and why")
	}
	duration, err := api.ParseSessionDuration(in.Duration)
	if err != nil {
		return nil, sessionOut{}, err
	}
	if duration.Hours == nil && duration.Minutes == nil {
		return nil, sessionOut{}, fmt.Errorf("duration is required, e.g. 15m or 2h")
	}
	req := &api.StartSessionRequest{Resources: in.Resources, Label: strings.TrimSpace(in.Label),
		Ipv4Address: in.Ipv4Address, Ipv6Address: in.Ipv6Address}
	duration.Apply(req)
	if req.Ipv4Address == "" && req.Ipv6Address == "" {
		req.Ipv4Address, req.Ipv6Address = s.opts.DetectIPs()
	}
	session, err := s.opts.Client.StartSession(req)
	if err != nil {
		return nil, sessionOut{}, err
	}
	if in.Wait == nil || *in.Wait {
		timeout := defaultWaitTimeout
		if in.WaitSeconds > 0 {
			timeout = min(time.Duration(in.WaitSeconds)*time.Second, maxWaitTimeout)
		}
		settled, waitErr := s.opts.Client.WaitForSession(session.ID, timeout, s.opts.PollInterval)
		if settled != nil {
			session = settled
		}
		if waitErr != nil {
			return nil, toSessionOut(session), fmt.Errorf("%v; the session is still open — stop it with stop_session if you no longer need it", waitErr)
		}
	}
	return nil, toSessionOut(session), nil
}

func (s *server) stopSession(_ context.Context, _ *mcp.CallToolRequest, in sessionIDIn) (*mcp.CallToolResult, sessionOut, error) {
	id, err := s.resolveID(in.SessionID)
	if err != nil {
		return nil, sessionOut{}, err
	}
	session, err := s.opts.Client.StopSession(id)
	if err != nil {
		return nil, sessionOut{}, err
	}
	return nil, toSessionOut(session), nil
}

func (s *server) extendSession(_ context.Context, _ *mcp.CallToolRequest, in extendSessionIn) (*mcp.CallToolResult, sessionOut, error) {
	if in.Hours <= 0 {
		return nil, sessionOut{}, fmt.Errorf("hours must be positive")
	}
	id, err := s.resolveID(in.SessionID)
	if err != nil {
		return nil, sessionOut{}, err
	}
	session, err := s.opts.Client.ExtendSession(id, in.Hours)
	if err != nil {
		return nil, sessionOut{}, err
	}
	return nil, toSessionOut(session), nil
}

// resolveID accepts a full UUID or a unique prefix, like the CLI does.
func (s *server) resolveID(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("session_id is required")
	}
	if len(input) >= 36 {
		return input, nil
	}
	sessions, err := s.opts.Client.ListSessions()
	if err != nil {
		return "", err
	}
	var matches []string
	for _, sess := range sessions {
		if strings.HasPrefix(sess.ID, input) {
			matches = append(matches, sess.ID)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no session found matching %q", input)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous session id %q matches %d sessions", input, len(matches))
	}
}
