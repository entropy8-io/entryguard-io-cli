package agent

import "time"

type AgentResponse struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Status          string  `json:"status"`
	AgentVersion    *string `json:"agentVersion"`
	Hostname        *string `json:"hostname"`
	OsInfo          *string `json:"osInfo"`
	LastHeartbeatAt *string `json:"lastHeartbeatAt"`
	CreatedAt       string  `json:"createdAt"`
}

type Command struct {
	ID                 string `json:"id"`
	CommandType        string `json:"commandType"` // APPLY or REVOKE
	CIDR               string `json:"cidr"`
	Description        string `json:"description"`
	ResourceIdentifier string `json:"resourceIdentifier"`
	ResourceType       string `json:"resourceType"`
}

type RegisterRequest struct {
	Name         string `json:"name"`
	AgentVersion string `json:"agentVersion"`
	Hostname     string `json:"hostname"`
	OsInfo       string `json:"osInfo"`
}

type HeartbeatRequest struct {
	AgentVersion string `json:"agentVersion,omitempty"`
	Hostname     string `json:"hostname,omitempty"`
	OsInfo       string `json:"osInfo,omitempty"`
}

type CommandResultRequest struct {
	Success        bool   `json:"success"`
	ResultMessage  string `json:"resultMessage,omitempty"`
	ProviderRuleID string `json:"providerRuleId,omitempty"`
}

type ExecutionResult struct {
	Success  bool
	Output   string
	Duration time.Duration
}
