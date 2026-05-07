package ai

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/schema"
)

// WSHub manages WebSocket connections for streaming AI analysis steps.
type WSHub struct {
	mu    sync.RWMutex
	conns map[string][]*websocket.Conn // incidentID -> connections
}

func NewWSHub() *WSHub {
	return &WSHub{
		conns: make(map[string][]*websocket.Conn),
	}
}

// Register adds a WebSocket connection for the given incident.
func (h *WSHub) Register(incidentID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[incidentID] = append(h.conns[incidentID], conn)
}

// Unregister removes a WebSocket connection.
func (h *WSHub) Unregister(incidentID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	conns := h.conns[incidentID]
	for i, c := range conns {
		if c == conn {
			h.conns[incidentID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}
}

// Broadcast sends a message to all WebSocket connections for the given incident.
func (h *WSHub) Broadcast(incidentID string, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conn := range h.conns[incidentID] {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			log.Warn().Err(err).Str("incident_id", incidentID).Msg("ws broadcast error")
		}
	}
}

// WSEvent is the JSON structure sent over WebSocket.
type WSEvent struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// StreamCallback implements langchaingo callbacks.Handler and streams events to WebSocket.
type StreamCallback struct {
	hub        *WSHub
	incidentID string
}

func NewStreamCallback(hub *WSHub, incidentID string) *StreamCallback {
	return &StreamCallback{hub: hub, incidentID: incidentID}
}

func (c *StreamCallback) broadcast(eventType, content string) {
	evt := WSEvent{Type: eventType, Content: content}
	data, _ := json.Marshal(evt)
	c.hub.Broadcast(c.incidentID, data)
}

// ─── callbacks.Handler implementation ────────────────────────────────────────

func (c *StreamCallback) HandleText(_ context.Context, text string) {
	c.broadcast("text", text)
}

func (c *StreamCallback) HandleLLMStart(_ context.Context, prompts []string) {
	c.broadcast("llm_start", "Thinking...")
}

func (c *StreamCallback) HandleLLMGenerateContentStart(_ context.Context, _ []llms.MessageContent) {
}

func (c *StreamCallback) HandleLLMGenerateContentEnd(_ context.Context, res *llms.ContentResponse) {
	if res != nil && len(res.Choices) > 0 {
		c.broadcast("llm_response", res.Choices[0].Content)
	}
}

func (c *StreamCallback) HandleLLMError(_ context.Context, err error) {
	c.broadcast("error", err.Error())
}

func (c *StreamCallback) HandleChainStart(_ context.Context, inputs map[string]any) {}

func (c *StreamCallback) HandleChainEnd(_ context.Context, outputs map[string]any) {
	if output, ok := outputs["output"].(string); ok {
		c.broadcast("chain_end", output)
	}
}

func (c *StreamCallback) HandleChainError(_ context.Context, err error) {
	c.broadcast("error", err.Error())
}

func (c *StreamCallback) HandleToolStart(_ context.Context, input string) {
	c.broadcast("tool_start", input)
}

func (c *StreamCallback) HandleToolEnd(_ context.Context, output string) {
	c.broadcast("tool_end", output)
}

func (c *StreamCallback) HandleToolError(_ context.Context, err error) {
	c.broadcast("tool_error", err.Error())
}

func (c *StreamCallback) HandleAgentAction(_ context.Context, action schema.AgentAction) {
	msg, _ := json.Marshal(map[string]string{
		"tool":  action.Tool,
		"input": action.ToolInput,
	})
	c.broadcast("agent_action", string(msg))
}

func (c *StreamCallback) HandleAgentFinish(_ context.Context, finish schema.AgentFinish) {
	if output, ok := finish.ReturnValues["output"].(string); ok {
		c.broadcast("agent_finish", output)
	}
}

func (c *StreamCallback) HandleRetrieverStart(_ context.Context, _ string)                    {}
func (c *StreamCallback) HandleRetrieverEnd(_ context.Context, _ string, _ []schema.Document) {}

func (c *StreamCallback) HandleStreamingFunc(_ context.Context, chunk []byte) {
	c.broadcast("stream", string(chunk))
}
