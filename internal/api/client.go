package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type APIError struct {
	Message          string            `json:"message"`
	Status           int               `json:"status"`
	Error            string            `json:"error"`
	ValidationErrors map[string]string `json:"validationErrors"`
}

func (e *APIError) String() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Error != "" {
		return e.Error
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

// Response types

type UserInfo struct {
	ID               string `json:"id"`
	Email            string `json:"email"`
	Name             string `json:"name"`
	IsOrgAdmin       bool   `json:"isOrgAdmin"`
	PlatformRole     string `json:"platformRole"`
	OrganizationID   string `json:"organizationId"`
	OrganizationName string `json:"organizationName"`
	OrganizationSlug string `json:"organizationSlug"`
	SubscriptionTier string `json:"subscriptionTier"`
	MfaEnabled       bool   `json:"mfaEnabled"`
}

type IpResponse struct {
	IP        string `json:"ip"`
	Version   int    `json:"version"`
	Timestamp string `json:"timestamp"`
}

type SessionResourceIp struct {
	ID           string `json:"id"`
	ResourceID   string `json:"resourceId"`
	ResourceName string `json:"resourceName"`
	IpVersion    int    `json:"ipVersion"`
	IpAddress    string `json:"ipAddress"`
	Status       string `json:"status"`
	AppliedAt    string `json:"appliedAt"`
	RemovedAt    string `json:"removedAt"`
	ErrorMessage string `json:"errorMessage"`
}

type Session struct {
	ID               string              `json:"id"`
	UserID           string              `json:"userId"`
	UserName         string              `json:"userName"`
	UserEmail        string              `json:"userEmail"`
	Ipv4Address      string              `json:"ipv4Address"`
	Ipv6Address      string              `json:"ipv6Address"`
	Status           string              `json:"status"`
	StartedAt        string              `json:"startedAt"`
	ExpiresAt        string              `json:"expiresAt"`
	EndedAt          string              `json:"endedAt"`
	EndedReason      string              `json:"endedReason"`
	ResourceIps      []SessionResourceIp `json:"resourceIps"`
	CreatedAt        string              `json:"createdAt"`
	OrganizationName string              `json:"organizationName"`
}

type StartSessionRequest struct {
	DurationHours *int   `json:"durationHours,omitempty"`
	Ipv4Address   string `json:"ipv4Address,omitempty"`
	Ipv6Address   string `json:"ipv6Address,omitempty"`
}

type ExtendSessionRequest struct {
	AdditionalHours int `json:"additionalHours"`
}

// API methods

func (c *Client) do(method, path string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.APIKey != "" {
		req.Header.Set("X-API-Key", c.APIKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr APIError
		if json.Unmarshal(respBody, &apiErr) == nil && (apiErr.Message != "" || apiErr.Error != "") {
			return nil, fmt.Errorf("%s", apiErr.String())
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (c *Client) GetMe() (*UserInfo, error) {
	data, err := c.do("GET", "/auth/me", nil)
	if err != nil {
		return nil, err
	}
	var user UserInfo
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &user, nil
}

func (c *Client) DetectIP() (*IpResponse, error) {
	data, err := c.do("GET", "/detect-ip", nil)
	if err != nil {
		return nil, err
	}
	var ip IpResponse
	if err := json.Unmarshal(data, &ip); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &ip, nil
}

func (c *Client) StartSession(req *StartSessionRequest) (*Session, error) {
	data, err := c.do("POST", "/sessions", req)
	if err != nil {
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &session, nil
}

func (c *Client) StopSession(id string) (*Session, error) {
	data, err := c.do("POST", fmt.Sprintf("/sessions/%s/stop", id), struct{}{})
	if err != nil {
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &session, nil
}

func (c *Client) ListSessions() ([]Session, error) {
	data, err := c.do("GET", "/sessions", nil)
	if err != nil {
		return nil, err
	}
	var sessions []Session
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return sessions, nil
}

func (c *Client) GetSession(id string) (*Session, error) {
	data, err := c.do("GET", fmt.Sprintf("/sessions/%s", id), nil)
	if err != nil {
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &session, nil
}

func (c *Client) ExtendSession(id string, hours int) (*Session, error) {
	data, err := c.do("POST", fmt.Sprintf("/sessions/%s/extend", id), &ExtendSessionRequest{
		AdditionalHours: hours,
	})
	if err != nil {
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &session, nil
}
