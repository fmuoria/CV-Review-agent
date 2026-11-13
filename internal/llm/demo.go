package llm

import (
	"context"
	"fmt"
	"sync"
)

// DemoHandler answers one kind of task without calling any AI service.
type DemoHandler func(req Request) (string, error)

// Demo is a deterministic stand-in for Gemini, used for local development
// and automated tests. Its scores are rough keyword matches — never use it
// to make real hiring decisions (the dashboard shows a banner in demo mode).
type Demo struct {
	mu       sync.RWMutex
	handlers map[string]DemoHandler
}

// NewDemo returns an empty demo client; register handlers with Handle.
func NewDemo() *Demo { return &Demo{handlers: map[string]DemoHandler{}} }

// Handle registers the handler for a task name.
func (d *Demo) Handle(task string, h DemoHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[task] = h
}

// Model implements Client.
func (d *Demo) Model() string { return "demo" }

// Generate implements Client.
func (d *Demo) Generate(_ context.Context, req Request) (Response, error) {
	d.mu.RLock()
	h, ok := d.handlers[req.Task]
	d.mu.RUnlock()
	if !ok {
		return Response{}, fmt.Errorf("demo client has no handler for task %q", req.Task)
	}
	text, err := h(req)
	if err != nil {
		return Response{}, err
	}
	return Response{Text: text}, nil
}
