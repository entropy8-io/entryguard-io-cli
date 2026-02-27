package agent

import (
	"log"
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
	var scriptPath string
	switch cmd.CommandType {
	case "APPLY":
		scriptPath = p.scripts.Apply
	case "REVOKE":
		scriptPath = p.scripts.Revoke
	default:
		log.Printf("[poller] unknown command type: %s (command=%s)", cmd.CommandType, cmd.ID)
		p.client.ReportResult(cmd.ID, CommandResultRequest{
			Success:       false,
			ResultMessage: "Unknown command type: " + cmd.CommandType,
		})
		return
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
