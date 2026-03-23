package tunnel

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Connector manages the WebSocket connection to the edge service
// and handles tunnel stream multiplexing.
type Connector struct {
	edgeURL  string
	apiKey   string
	ws       *websocket.Conn
	writeMu  sync.Mutex
	streams  sync.Map // streamID → net.Conn (local target connection)
	targets  sync.Map // "host:port" → true (allowed targets)
	stopCh   chan struct{}
}

func NewConnector(edgeURL, apiKey string) *Connector {
	return &Connector{
		edgeURL: edgeURL,
		apiKey:  apiKey,
		stopCh:  make(chan struct{}),
	}
}

// Run connects to the edge and handles messages. Reconnects with exponential backoff.
func (c *Connector) Run() {
	backoff := time.Second

	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		err := c.connect()
		if err != nil {
			log.Printf("[tunnel] connection lost: %v, reconnecting in %v", err, backoff)
			select {
			case <-time.After(backoff):
			case <-c.stopCh:
				return
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		backoff = time.Second
	}
}

func (c *Connector) Stop() {
	close(c.stopCh)
	if c.ws != nil {
		c.ws.Close()
	}
}

func (c *Connector) connect() error {
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.apiKey)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	ws, _, err := dialer.Dial(c.edgeURL+"/agent/connect", header)
	if err != nil {
		return fmt.Errorf("WebSocket dial failed: %w", err)
	}
	c.ws = ws
	log.Printf("[tunnel] connected to edge at %s", c.edgeURL)

	// Set up ping/pong keepalive
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(90 * time.Second))
	})
	go c.pingLoop()

	return c.readLoop()
}

func (c *Connector) pingLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.writeMu.Lock()
			err := c.ws.WriteMessage(websocket.PingMessage, nil)
			c.writeMu.Unlock()
			if err != nil {
				return
			}
		case <-c.stopCh:
			return
		}
	}
}

func (c *Connector) readLoop() error {
	defer c.closeAllStreams()

	for {
		msgType, data, err := c.ws.ReadMessage()
		if err != nil {
			return err
		}

		switch msgType {
		case websocket.TextMessage:
			c.handleControlMessage(data)

		case websocket.BinaryMessage:
			msg, err := decodeMessage(data)
			if err != nil {
				log.Printf("[tunnel] invalid binary message: %v", err)
				continue
			}
			c.handleStreamMessage(msg)
		}
	}
}

func (c *Connector) handleControlMessage(data []byte) {
	var ctrl struct {
		Type    string   `json:"type"`
		Targets []string `json:"targets"`
	}
	if err := json.Unmarshal(data, &ctrl); err != nil {
		log.Printf("[tunnel] invalid control message: %v", err)
		return
	}

	switch ctrl.Type {
	case "targets_update":
		// Clear and repopulate allowed targets
		c.targets.Range(func(key, _ interface{}) bool {
			c.targets.Delete(key)
			return true
		})
		for _, target := range ctrl.Targets {
			c.targets.Store(target, true)
		}
		log.Printf("[tunnel] targets updated: %v", ctrl.Targets)
	}
}

func (c *Connector) handleStreamMessage(msg *Message) {
	switch msg.Type {
	case MsgNewStream:
		go c.handleNewStream(msg)

	case MsgData:
		if conn, ok := c.streams.Load(msg.StreamID); ok {
			if _, err := conn.(net.Conn).Write(msg.Payload); err != nil {
				c.closeStream(msg.StreamID)
			}
		}

	case MsgCloseStream:
		c.closeStream(msg.StreamID)
	}
}

func (c *Connector) handleNewStream(msg *Message) {
	target := string(msg.Payload)

	// Security: only dial allowed targets
	if _, allowed := c.targets.Load(target); !allowed {
		log.Printf("[tunnel] rejected connection to disallowed target: %s", target)
		c.sendMessage(&Message{Type: MsgCloseStream, StreamID: msg.StreamID})
		return
	}

	conn, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		log.Printf("[tunnel] failed to dial %s: %v", target, err)
		c.sendMessage(&Message{Type: MsgCloseStream, StreamID: msg.StreamID})
		return
	}

	c.streams.Store(msg.StreamID, conn)

	// Read from local target and send to edge
	go func() {
		defer func() {
			c.closeStream(msg.StreamID)
		}()

		buf := make([]byte, 32*1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				if err != io.EOF {
					// Connection closed
				}
				return
			}
			payload := make([]byte, n)
			copy(payload, buf[:n])
			if sendErr := c.sendMessage(&Message{
				Type:     MsgData,
				StreamID: msg.StreamID,
				Payload:  payload,
			}); sendErr != nil {
				return
			}
		}
	}()
}

func (c *Connector) sendMessage(msg *Message) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.ws == nil {
		return fmt.Errorf("not connected")
	}
	return c.ws.WriteMessage(websocket.BinaryMessage, encodeMessage(msg))
}

func (c *Connector) closeStream(streamID uint32) {
	if conn, ok := c.streams.LoadAndDelete(streamID); ok {
		conn.(net.Conn).Close()
		c.sendMessage(&Message{Type: MsgCloseStream, StreamID: streamID})
	}
}

func (c *Connector) closeAllStreams() {
	c.streams.Range(func(key, value interface{}) bool {
		value.(net.Conn).Close()
		c.streams.Delete(key)
		return true
	})
}
