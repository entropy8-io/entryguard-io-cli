package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"
)

type Poller struct {
	client   *Client
	executor *Executor
	scripts  ScriptsConfig
	interval time.Duration
	stopCh   chan struct{}
}

func NewPoller(client *Client, executor *Executor, scripts ScriptsConfig, interval time.Duration) *Poller {
	return &Poller{
		client:   client,
		executor: executor,
		scripts:  scripts,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

func (p *Poller) Run() {
	log.Printf("[poller] starting (interval=%s)", p.interval)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// Poll immediately on start
	p.poll()

	for {
		select {
		case <-ticker.C:
			p.poll()
		case <-p.stopCh:
			log.Println("[poller] stopped")
			return
		}
	}
}

func (p *Poller) Stop() {
	close(p.stopCh)
}

func (p *Poller) poll() {
	commands, err := p.client.PollCommands()
	if err != nil {
		log.Printf("[poller] failed to poll commands: %v", err)
		return
	}

	// Commands are returned in FIFO order (by createdAt ASC) and executed
	// sequentially — no parallelism, so earlier sessions are applied first.
	for _, cmd := range commands {
		p.processCommand(cmd)
	}
}

func (p *Poller) processCommand(cmd Command) {
	if cmd.CommandType != "APPLY" && cmd.CommandType != "REVOKE" {
		log.Printf("[poller] unknown command type: %s (command=%s)", cmd.CommandType, cmd.ID)
		p.client.ReportResult(cmd.ID, CommandResultRequest{
			Success:       false,
			ResultMessage: "Unknown command type: " + cmd.CommandType,
		})
		return
	}

	// Multi-script mode: scriptDir is set on the resource config
	if cmd.ScriptDir != "" {
		p.processMultiScript(cmd)
		return
	}

	// Single-script mode: use agent-level script config
	p.processSingleScript(cmd)
}

func (p *Poller) processSingleScript(cmd Command) {
	var scriptPath string
	switch cmd.CommandType {
	case "APPLY":
		scriptPath = p.scripts.Apply
	case "REVOKE":
		scriptPath = p.scripts.Revoke
	}

	if scriptPath == "" {
		log.Printf("[poller] no script configured for %s (command=%s)", cmd.CommandType, cmd.ID)
		p.client.ReportResult(cmd.ID, CommandResultRequest{
			Success:       false,
			ResultMessage: "No script configured for " + cmd.CommandType,
		})
		return
	}

	log.Printf("[poller] executing %s: cidr=%s resource=%s (command=%s)", cmd.CommandType, cmd.CIDR, cmd.ResourceIdentifier, cmd.ID)

	result := p.executor.Execute(scriptPath, cmd.CIDR, cmd.Description)

	log.Printf("[poller] %s result: success=%t duration=%s (command=%s)", cmd.CommandType, result.Success, result.Duration, cmd.ID)
	if result.Output != "" {
		log.Printf("[poller] output: %s", result.Output)
	}

	reportReq := CommandResultRequest{
		Success:       result.Success,
		ResultMessage: result.Output,
	}
	if result.Success {
		reportReq.ProviderRuleID = "agent-" + cmd.ID
	}

	if err := p.client.ReportResult(cmd.ID, reportReq); err != nil {
		log.Printf("[poller] failed to report result for command %s: %v", cmd.ID, err)
	}
}

func (p *Poller) processMultiScript(cmd Command) {
	subdir := strings.ToLower(cmd.CommandType) // "apply" or "revoke"
	dir := filepath.Join(cmd.ScriptDir, subdir)

	var timeout time.Duration
	if cmd.ScriptTimeout > 0 {
		timeout = time.Duration(cmd.ScriptTimeout) * time.Second
	}

	log.Printf("[poller] executing %s (multi-script): dir=%s cidr=%s resource=%s (command=%s)",
		cmd.CommandType, dir, cmd.CIDR, cmd.ResourceIdentifier, cmd.ID)

	scriptResults, allSuccess := p.executor.ExecuteDir(dir, cmd.CIDR, cmd.Description, timeout)

	// Log per-script results
	for _, sr := range scriptResults {
		log.Printf("[poller] script %s: success=%t duration=%dms (command=%s)", sr.ScriptName, sr.Success, sr.DurationMs, cmd.ID)
		if sr.Output != "" {
			log.Printf("[poller] output: %s", sr.Output)
		}
	}

	// Serialize script results as JSON for the resultMessage
	// The frontend detects JSON arrays and renders per-script details
	resultJSON, err := json.Marshal(scriptResults)
	if err != nil {
		// Fallback to plain text summary
		var summary strings.Builder
		succeeded := 0
		for _, sr := range scriptResults {
			if sr.Success {
				succeeded++
			}
		}
		fmt.Fprintf(&summary, "%d/%d scripts succeeded", succeeded, len(scriptResults))
		resultJSON = []byte(summary.String())
	}

	reportReq := CommandResultRequest{
		Success:       allSuccess,
		ResultMessage: string(resultJSON),
		ScriptResults: scriptResults,
	}
	if allSuccess {
		reportReq.ProviderRuleID = "agent-" + cmd.ID
	}

	if err := p.client.ReportResult(cmd.ID, reportReq); err != nil {
		log.Printf("[poller] failed to report result for command %s: %v", cmd.ID, err)
	}
}
