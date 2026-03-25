// Copyright Contributors to the KubeOpenCode project

package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubeopenv1alpha1 "github.com/kubeopencode/kubeopencode/api/v1alpha1"
	"github.com/kubeopencode/kubeopencode/internal/server/types"
)

var hitlLog = ctrl.Log.WithName("hitl")

// HITLHandler handles Human-in-the-Loop HTTP requests
type HITLHandler struct {
	defaultClient client.Client
	httpClient    *http.Client
}

// NewHITLHandler creates a new HITLHandler
func NewHITLHandler(c client.Client) *HITLHandler {
	return &HITLHandler{
		defaultClient: c,
		httpClient: &http.Client{
			Timeout: 0, // No timeout for SSE streams
		},
	}
}

// getClient returns the client from context or falls back to default
func (h *HITLHandler) getClient(ctx context.Context) client.Client {
	if c, ok := ctx.Value(clientContextKey{}).(client.Client); ok && c != nil {
		return c
	}
	return h.defaultClient
}

// getServerURL resolves the OpenCode server URL for a Task's Agent.
// Returns the URL and an error if the server cannot be resolved.
func (h *HITLHandler) getServerURL(ctx context.Context, namespace, taskName string) (string, error) {
	k8sClient := h.getClient(ctx)

	// Get the Task
	var task kubeopenv1alpha1.Task
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: taskName}, &task); err != nil {
		return "", fmt.Errorf("task not found: %w", err)
	}

	// Get the Agent reference
	agentName := ""
	if task.Status.AgentRef != nil {
		agentName = task.Status.AgentRef.Name
	} else if task.Spec.AgentRef != nil {
		agentName = task.Spec.AgentRef.Name
	}
	if agentName == "" {
		return "", fmt.Errorf("task has no agent reference")
	}

	// Get the Agent
	var agent kubeopenv1alpha1.Agent
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: agentName}, &agent); err != nil {
		return "", fmt.Errorf("agent not found: %w", err)
	}

	// Check if Agent is in Server mode
	if agent.Spec.ServerConfig == nil {
		return "", fmt.Errorf("agent %q is not in Server mode (no serverConfig)", agentName)
	}

	// Get server URL from status
	if agent.Status.ServerStatus == nil || agent.Status.ServerStatus.URL == "" {
		return "", fmt.Errorf("agent %q server is not ready (no server URL in status)", agentName)
	}

	return agent.Status.ServerStatus.URL, nil
}

// StreamEvents proxies SSE events from the OpenCode server to the client.
// GET /api/v1/namespaces/{namespace}/tasks/{name}/events
func (h *HITLHandler) StreamEvents(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")
	ctx := r.Context()

	serverURL, err := h.getServerURL(ctx, namespace, name)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Cannot resolve server", err.Error())
		return
	}

	// Connect to OpenCode server's SSE endpoint
	eventURL := serverURL + "/event"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, eventURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create request", err.Error())
		return
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Failed to connect to OpenCode server", err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		writeError(w, http.StatusBadGateway, "OpenCode server returned error", fmt.Sprintf("status: %d", resp.StatusCode))
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming not supported", "")
		return
	}

	// Start heartbeat goroutine
	heartbeatDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// SSE comment for heartbeat (not a data event)
				_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
				flusher.Flush()
			case <-ctx.Done():
				return
			case <-heartbeatDone:
				return
			}
		}
	}()
	defer close(heartbeatDone)

	// Proxy SSE events from upstream to client
	reader := bufio.NewReader(resp.Body)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					_, _ = fmt.Fprintf(w, "data: {\"type\":\"stream.closed\"}\n\n")
					flusher.Flush()
					return
				}
				hitlLog.Error(err, "SSE read error", "task", name)
				return
			}
			// Forward the raw SSE line to client
			_, _ = w.Write(line)
			// Flush after each complete SSE message (empty line)
			if len(strings.TrimSpace(string(line))) == 0 {
				flusher.Flush()
			}
		}
	}
}

// ReplyPermission forwards a permission reply to the OpenCode server.
// POST /api/v1/namespaces/{namespace}/tasks/{name}/permission/{id}
func (h *HITLHandler) ReplyPermission(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")
	permissionID := chi.URLParam(r, "id")
	ctx := r.Context()

	var req types.PermissionReplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Validate reply value
	switch req.Reply {
	case "once", "always", "reject":
		// valid
	default:
		writeError(w, http.StatusBadRequest, "Invalid reply", "reply must be 'once', 'always', or 'reject'")
		return
	}

	serverURL, err := h.getServerURL(ctx, namespace, name)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Cannot resolve server", err.Error())
		return
	}

	// Forward to OpenCode server
	targetURL := fmt.Sprintf("%s/permission/%s/reply", serverURL, permissionID)
	body, _ := json.Marshal(req)
	proxyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, strings.NewReader(string(body)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create request", err.Error())
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(proxyReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Failed to forward to OpenCode server", err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		writeError(w, resp.StatusCode, "OpenCode server error", string(respBody))
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ReplyQuestion forwards a question reply to the OpenCode server.
// POST /api/v1/namespaces/{namespace}/tasks/{name}/question/{id}
func (h *HITLHandler) ReplyQuestion(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")
	questionID := chi.URLParam(r, "id")
	ctx := r.Context()

	var req types.QuestionReplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	serverURL, err := h.getServerURL(ctx, namespace, name)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Cannot resolve server", err.Error())
		return
	}

	// Forward to OpenCode server
	targetURL := fmt.Sprintf("%s/question/%s/reply", serverURL, questionID)
	body, _ := json.Marshal(req)
	proxyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, strings.NewReader(string(body)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create request", err.Error())
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(proxyReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Failed to forward to OpenCode server", err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		writeError(w, resp.StatusCode, "OpenCode server error", string(respBody))
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RejectQuestion forwards a question rejection to the OpenCode server.
// POST /api/v1/namespaces/{namespace}/tasks/{name}/question/{id}/reject
func (h *HITLHandler) RejectQuestion(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")
	questionID := chi.URLParam(r, "id")
	ctx := r.Context()

	serverURL, err := h.getServerURL(ctx, namespace, name)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Cannot resolve server", err.Error())
		return
	}

	// Forward to OpenCode server
	targetURL := fmt.Sprintf("%s/question/%s/reject", serverURL, questionID)
	proxyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create request", err.Error())
		return
	}

	resp, err := h.httpClient.Do(proxyReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Failed to forward to OpenCode server", err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		writeError(w, resp.StatusCode, "OpenCode server error", string(respBody))
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// SendMessage forwards a message to the OpenCode server session.
// POST /api/v1/namespaces/{namespace}/tasks/{name}/message
func (h *HITLHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")
	ctx := r.Context()

	var req types.SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.SessionID == "" {
		writeError(w, http.StatusBadRequest, "sessionId is required", "")
		return
	}
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required", "")
		return
	}

	serverURL, err := h.getServerURL(ctx, namespace, name)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Cannot resolve server", err.Error())
		return
	}

	// Build the message payload matching OpenCode's expected format
	messagePayload := map[string]interface{}{
		"parts": []map[string]string{
			{"type": "text", "text": req.Message},
		},
	}

	// Forward to OpenCode server using prompt_async (non-blocking)
	targetURL := fmt.Sprintf("%s/session/%s/prompt_async", serverURL, req.SessionID)
	body, _ := json.Marshal(messagePayload)
	proxyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, strings.NewReader(string(body)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create request", err.Error())
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(proxyReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Failed to forward to OpenCode server", err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		writeError(w, resp.StatusCode, "OpenCode server error", string(respBody))
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Interrupt sends an abort signal to the OpenCode server session.
// POST /api/v1/namespaces/{namespace}/tasks/{name}/interrupt
func (h *HITLHandler) Interrupt(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")
	ctx := r.Context()

	// Parse optional sessionID from body
	var body struct {
		SessionID string `json:"sessionId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	serverURL, err := h.getServerURL(ctx, namespace, name)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Cannot resolve server", err.Error())
		return
	}

	if body.SessionID == "" {
		writeError(w, http.StatusBadRequest, "sessionId is required", "")
		return
	}

	// Forward abort to OpenCode server
	targetURL := fmt.Sprintf("%s/session/%s/abort", serverURL, body.SessionID)
	proxyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create request", err.Error())
		return
	}

	resp, err := h.httpClient.Do(proxyReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Failed to forward to OpenCode server", err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
