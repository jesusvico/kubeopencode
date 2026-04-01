---
sidebar_position: 1
title: Getting Started
description: Set up KubeOpenCode locally in minutes with Kind
---

# Getting Started

:::caution Alpha Project
KubeOpenCode is in **early alpha** (v0.0.x). It is **not recommended for production use**. The API (`v1alpha1`) may introduce breaking changes between releases — backward compatibility is not guaranteed at this stage. We welcome contributions and feedback!
:::

This guide gets you running KubeOpenCode on a local Kind cluster in minutes. The default setup uses the free `opencode/big-pickle` model — **no API key required**.

## Quick Start (AI-Assisted)

The fastest way to get started — let your AI agent set up everything for you:

```bash
git clone https://github.com/kubeopencode/kubeopencode.git
cd kubeopencode
```

Then tell your AI agent (Claude Code, Cursor, Windsurf, etc.):

> Follow `deploy/local-dev/local-development.md` to set up a local development environment for me.

The agent will handle Kind cluster creation, image builds, Helm installation, and test resource deployment automatically.

## Manual Setup

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Kind](https://kind.sigs.k8s.io/) (`brew install kind` on macOS)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Helm](https://helm.sh/) 3.8+
- [Go](https://go.dev/) 1.25+

### Step 1: Clone and Create Cluster

```bash
# Clone the repository
git clone https://github.com/kubeopencode/kubeopencode.git
cd kubeopencode

# Create a Kind cluster
kind create cluster --name kubeopencode
```

### Step 2: Build and Load Images

```bash
# Build the controller image
make docker-build

# Build all agent images (opencode, devbox, attach)
make agent-build-all

# Load images into Kind
kind load docker-image quay.io/kubeopencode/kubeopencode:latest --name kubeopencode
for img in opencode devbox attach; do
  kind load docker-image quay.io/kubeopencode/kubeopencode-agent-${img}:latest --name kubeopencode
done
```

### Step 3: Deploy

```bash
# Install with Helm (UI enabled)
helm upgrade --install kubeopencode ./charts/kubeopencode \
  --namespace kubeopencode-system \
  --create-namespace \
  --set controller.image.pullPolicy=Never \
  --set agent.image.pullPolicy=Never \
  --set server.enabled=true

# Deploy local-dev resources (AgentTemplate + Agents, no API key needed)
kubectl apply -k deploy/local-dev/
```

Verify everything is running:

```bash
# Controller and UI server
kubectl get pods -n kubeopencode-system

# Agents in test namespace
kubectl get agent -n test
```

### Step 4: Access the Web UI

```bash
kubectl port-forward -n kubeopencode-system svc/kubeopencode-server 2746:2746
```

Open [http://localhost:2746](http://localhost:2746). The UI provides:
- **Task List** — View and filter Tasks across namespaces
- **Task Detail** — Monitor execution with real-time log streaming
- **Task Create** — Submit new Tasks to Agents
- **Agent Browser** — View Agents and AgentTemplates

### Step 5: Try It Out

### Submit a Task

The local-dev setup includes two pre-configured Agents. Submit a task to one:

```bash
kubectl apply -n test -f - <<EOF
apiVersion: kubeopencode.io/v1alpha1
kind: Task
metadata:
  name: hello-world
spec:
  agentRef:
    name: dev-agent
  description: "Say hello and tell me what tools you have available"
EOF

# Watch the task
kubectl get task -n test -w
```

### Explore the Pre-configured Resources

The `deploy/local-dev/` directory sets up:

| Resource | Name | Description |
|----------|------|-------------|
| AgentTemplate | `local-dev-base` | Shared base configuration (images, model, workspace) |
| Agent | `persistent-agent` | Persistent agent with session + workspace storage, standby, concurrency control |
| Agent | `dev-agent` | Lightweight agent with ephemeral storage |

These demonstrate key features: template inheritance, persistence, suspend/resume, and concurrency control. See [Features](features.md) for details.

### Using a Paid Model

The default setup uses the free `opencode/big-pickle` model. To switch to a paid model (Anthropic, Google, etc.), see `deploy/local-dev/secrets.yaml.example` for instructions.

## Next Steps

- [Features](features.md) — Learn about Live Agents (human-in-the-loop) and automated workflows
- [Agent Images](agent-images.md) — Build custom agent images
- [Security](security.md) — RBAC, credential management, and best practices
- [Architecture](architecture.md) — System design and API reference

## Production Installation

For production clusters, install from the OCI registry:

```bash
kubectl create namespace kubeopencode-system

helm install kubeopencode oci://quay.io/kubeopencode/helm-charts/kubeopencode \
  --namespace kubeopencode-system \
  --set server.enabled=true
```

See [Operations](operations/upgrading.md) for upgrade and maintenance guides.
