package agent

import (
	"log"
	"os"
	"runtime"
	"time"
)

type Heartbeater struct {
	client   *Client
	interval time.Duration
	version  string
	stopCh   chan struct{}
}

func NewHeartbeater(client *Client, interval time.Duration, version string) *Heartbeater {
	return &Heartbeater{
		client:   client,
		interval: interval,
		version:  version,
		stopCh:   make(chan struct{}),
	}
}

func (h *Heartbeater) Start() {
	go h.run()
}

func (h *Heartbeater) Stop() {
	close(h.stopCh)
}

func (h *Heartbeater) run() {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.sendHeartbeat()
		case <-h.stopCh:
			return
		}
	}
}

func (h *Heartbeater) sendHeartbeat() {
	hostname, _ := os.Hostname()
	req := HeartbeatRequest{
		AgentVersion: h.version,
		Hostname:     hostname,
		OsInfo:       runtime.GOOS + "/" + runtime.GOARCH,
	}
	if _, err := h.client.Heartbeat(req); err != nil {
		log.Printf("[heartbeat] failed: %v", err)
	}
}
