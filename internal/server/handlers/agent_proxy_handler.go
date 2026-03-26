// Copyright Contributors to the KubeOpenCode project

package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/go-chi/chi/v5"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubeopenv1alpha1 "github.com/kubeopencode/kubeopencode/api/v1alpha1"
)

var proxyLog = ctrl.Log.WithName("agent-proxy")

// AgentProxyHandler handles reverse-proxying requests to OpenCode agent servers.
// It resolves an Agent's in-cluster server URL and uses httputil.ReverseProxy
// to forward all requests, supporting both HTTP REST and SSE streaming.
type AgentProxyHandler struct {
	defaultClient client.Client
}

// NewAgentProxyHandler creates a new AgentProxyHandler
func NewAgentProxyHandler(c client.Client) *AgentProxyHandler {
	return &AgentProxyHandler{defaultClient: c}
}

// getClient returns the impersonated client from context or falls back to default
func (h *AgentProxyHandler) getClient(ctx context.Context) client.Client {
	if c, ok := ctx.Value(clientContextKey{}).(client.Client); ok && c != nil {
		return c
	}
	return h.defaultClient
}

// resolveAgentServerURL looks up the Agent CR and returns its in-cluster server URL.
// This uses the impersonated client, so RBAC is enforced automatically.
func (h *AgentProxyHandler) resolveAgentServerURL(ctx context.Context, namespace, agentName string) (string, error) {
	k8sClient := h.getClient(ctx)

	var agent kubeopenv1alpha1.Agent
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: agentName}, &agent); err != nil {
		return "", fmt.Errorf("agent not found: %w", err)
	}

	if agent.Spec.ServerConfig == nil {
		return "", fmt.Errorf("agent %q is not in Server mode (no serverConfig)", agentName)
	}

	if agent.Status.ServerStatus == nil || agent.Status.ServerStatus.URL == "" {
		return "", fmt.Errorf("agent %q server is not ready (no server URL in status)", agentName)
	}

	return agent.Status.ServerStatus.URL, nil
}

// ServeProxy is the catch-all handler for /api/v1/namespaces/{namespace}/agents/{name}/proxy/*
// It resolves the Agent's server URL, rewrites the request path, and proxies via httputil.ReverseProxy.
func (h *AgentProxyHandler) ServeProxy(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")
	agentName := chi.URLParam(r, "name")

	// Detach from chi's timeout context (60s) to support long-lived SSE streams.
	// context.WithoutCancel preserves values but does not inherit cancellation.
	// The proxy will still terminate when the client disconnects (write errors).
	ctx := context.WithoutCancel(r.Context())

	serverURL, err := h.resolveAgentServerURL(ctx, namespace, agentName)
	if err != nil {
		proxyLog.Error(err, "Failed to resolve agent server URL", "namespace", namespace, "agent", agentName)
		writeError(w, http.StatusBadGateway, "Cannot resolve agent server", err.Error())
		return
	}

	target, err := url.Parse(serverURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Invalid server URL", err.Error())
		return
	}

	// Extract the wildcard path suffix from chi route
	proxyPath := chi.URLParam(r, "*")
	if proxyPath == "" {
		proxyPath = "/"
	} else if proxyPath[0] != '/' {
		proxyPath = "/" + proxyPath
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = proxyPath
			req.Host = target.Host
			// Remove Authorization header — internal traffic does not need external auth
			req.Header.Del("Authorization")
		},
		// FlushInterval -1 means flush immediately after each write,
		// which is critical for SSE streaming.
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			proxyLog.Error(err, "Proxy error", "namespace", namespace, "agent", agentName, "path", proxyPath)
			writeError(w, http.StatusBadGateway, "Proxy error", err.Error())
		},
	}

	proxyLog.V(1).Info("Proxying request", "namespace", namespace, "agent", agentName, "path", proxyPath, "method", r.Method)
	proxy.ServeHTTP(w, r.WithContext(ctx))
}
