---
name: koc
description: >
  Manage KubeOpenCode AI tasks and agents on Kubernetes. This skill handles
  anything related to running AI-powered tasks on a Kubernetes cluster via
  KubeOpenCode — creating tasks, listing agents, checking task status, viewing
  logs, stopping tasks, and monitoring task progress. Trigger this skill when the
  user wants to: create or run an AI task on the cluster, see what agents are
  available, check on a running task, view task output or logs, stop a task,
  use a specific agent for a job, or anything involving KubeOpenCode resources
  (Task, Agent, TaskTemplate). Also trigger when users say things like "run this
  on the cluster", "use agent X to do Y", "what's my task doing", "show me the
  logs", "is it done yet", or refer to kubeopencode, tk, ag, or tt resources.
---

# KubeOpenCode Skill

Interact with KubeOpenCode Kubernetes resources (Task, Agent, TaskTemplate) using kubectl.

## Prerequisites

Requires `kubectl` configured with access to a cluster running KubeOpenCode.

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `KUBEOPENCODE_KUBECONFIG` | **Yes** | Path to kubeconfig for the KubeOpenCode cluster |
| `KUBEOPENCODE_DEFAULT_TASK_NAMESPACE` | No | Default Task namespace (if unset, use `--all-namespaces` for list, ask on create) |
| `KUBEOPENCODE_DEFAULT_AGENT_NAMESPACE` | No | Default Agent namespace (if unset, same as task namespace) |

**CRITICAL**: Every `kubectl` command MUST include `--kubeconfig "$KUBEOPENCODE_KUBECONFIG"`. If this env var is not set, ask the user to set it before proceeding — do NOT fall back to the default kubeconfig. Check this env var in each Bash call since shell state does not persist between calls.

## API Quick Reference

- **API Group**: `kubeopencode.io/v1alpha1`
- **Resources & Short Names**: Task (`tk`), Agent (`ag`), TaskTemplate (`tt`), KubeOpenCodeConfig (`ktc`)
- **Task Phases**: `Pending` -> `Queued` -> `Running` -> `Completed` | `Failed`
- **Stop Annotation**: `kubeopencode.io/stop=true`
- **Pod Naming**: `<task-name>-pod` (same namespace), `<task-ns>-<task-name>-pod` (cross-namespace)
- **Pod Location**: Pod always runs in the Agent's namespace

For detailed field reference, read: `.claude/skills/koc/references/api-reference.md`

## Operations

### 1. List Agents

```bash
# All namespaces
kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" get ag --all-namespaces

# Specific namespace
kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" get ag -n <namespace>

# Detailed view (shows profile, executorImage, maxConcurrentTasks)
kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" get ag -n <namespace> -o wide
```

Show: name, namespace, serviceAccount, age. With `-o wide`: profile, executorImage, maxConcurrentTasks.

### 2. Get Agent Details

```bash
kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" get ag <name> -n <namespace> -o yaml
```

Summarize: profile, executorImage, workspaceDir, maxConcurrentTasks, quota, serverConfig presence, credentials (names only — never show secret values), allowedNamespaces.

### 3. List TaskTemplates

```bash
# All namespaces
kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" get tt --all-namespaces

# Specific namespace
kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" get tt -n <namespace>
```

Show: name, namespace, agentRef, age.

### 4. List Tasks

```bash
# All namespaces
kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" get tk --all-namespaces

# Specific namespace
kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" get tk -n <namespace>
```

Show: name, phase, agent, pod, pod namespace, age.

### 5. Create Task

**Agent Resolution Order:**
1. User-specified agent name
2. Auto-match by profile: run `kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" get ag --all-namespaces -o wide` to get all agents
   with their `profile` field. Compare each agent's profile against the user's task and:
   - **Single best match**: Propose it to the user for confirmation, e.g.:
     > Based on your task, I recommend **go-agent** ("Go development with GitHub access"). Use this agent? (y/n)
   - **Multiple matches**: Present a numbered list and let the user pick, e.g.:
     > Multiple agents match your task:
     > 1. **go-agent** — "Go development with GitHub access"
     > 2. **fullstack-agent** — "Full-stack dev with Node.js and Go"
     > Which agent would you like to use? (1/2)
   - **No match**: Show all available agents with profiles and ask the user to choose.

**Namespace Resolution Order:**
1. User-specified namespace
2. `KUBEOPENCODE_DEFAULT_TASK_NAMESPACE` env var
3. Ask user

**Task Name:** User-specified or auto-generate as `task-<short-slug>-<4-random-hex>`.

**Generate YAML and show to user for confirmation before applying.**

#### Basic Task (with agentRef)

```yaml
apiVersion: kubeopencode.io/v1alpha1
kind: Task
metadata:
  name: <task-name>
  namespace: <namespace>
spec:
  agentRef:
    name: <agent-name>
    namespace: <agent-namespace>  # omit if same as task namespace
  instruction: |
    <user's task description in natural language>
```

#### Task with TaskTemplate

```yaml
apiVersion: kubeopencode.io/v1alpha1
kind: Task
metadata:
  name: <task-name>
  namespace: <namespace>
spec:
  taskTemplateRef:
    name: <template-name>
    namespace: <template-namespace>  # omit if same as task namespace
  instruction: |
    <override description, or omit to use template's>
```

#### Task with Inline Contexts

```yaml
apiVersion: kubeopencode.io/v1alpha1
kind: Task
metadata:
  name: <task-name>
  namespace: <namespace>
spec:
  agentRef:
    name: <agent-name>
  instruction: |
    <task description>
  contexts:
    - name: extra-context
      type: Text
      text: |
        Additional instructions here
    - name: source-code
      type: Git
      git:
        repository: https://github.com/org/repo.git
        ref: main
      mountPath: source
```

**Apply the Task:**
```bash
kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" apply -f /tmp/kubeopencode-task-<name>.yaml
```

After successful creation, **automatically start monitoring** (see Operation 9).

### 6. Get Task Status

```bash
kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" get tk <name> -n <namespace> -o yaml
```

Extract and present:
- **Phase**: `.status.phase`
- **Pod**: `.status.podName` in namespace `.status.podNamespace`
- **Agent**: `.status.agentRef.name` / `.status.agentRef.namespace`
- **Start Time**: `.status.startTime`
- **Completion Time**: `.status.completionTime`
- **Conditions**: `.status.conditions` (type, status, reason, message)

Present a concise summary, e.g.:
> Task `my-task` is **Running** (started 5m ago). Pod: `my-task-pod` in namespace `default`.

### 7. Get Task Logs

First resolve pod info from the task status:
```bash
# Get pod name and namespace, then fetch logs — all in one call
POD_NAME=$(kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" get tk <task-name> -n <task-namespace> -o jsonpath='{.status.podName}')
POD_NS=$(kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" get tk <task-name> -n <task-namespace> -o jsonpath='{.status.podNamespace}')
kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" logs "$POD_NAME" -n "$POD_NS" -c executor
```

If the task is still running, add `--tail=100` to show recent logs.
If the pod is not found, the task may have been cleaned up — inform the user.

### 8. Stop Task

```bash
kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" annotate task <name> -n <namespace> kubeopencode.io/stop=true
```

**Always warn the user first**: Stopping a task deletes the pod, and **logs will be lost** unless an external log aggregation system is in place. Ask for confirmation before executing.

### 9. Delete Task

```bash
kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" delete task <name> -n <namespace>
```

Deleting a Task cascades to its Pod and ConfigMap via finalizer. Ask for confirmation — this is irreversible.

Only use delete when the user explicitly wants to remove the Task resource. For stopping a running task, prefer the stop annotation (Operation 8).

### 10. Monitor Task

After creating a task, poll periodically until completion. Since shell state doesn't persist between calls, run each check as a standalone command.

**Polling strategy:**

| Complexity | Poll Interval | Timeout |
|------------|--------------|---------|
| Simple (typo fix, small change) | 30s | 10min |
| Medium (feature, refactor) | 60s | 30min |
| Complex (large feature, multi-file) | 120s | 60min |

Default to **Medium** unless the user's description clearly indicates otherwise.

**Each check:**
```bash
kubectl --kubeconfig "$KUBEOPENCODE_KUBECONFIG" get tk <name> -n <namespace> -o jsonpath='{.status.phase}'
```

- **Pending/Queued**: Report status, wait, check again. If Queued, mention the agent may be at capacity.
- **Running**: Report elapsed time, wait, check again.
- **Completed**: Fetch logs (Operation 7), summarize the output.
- **Failed**: Fetch logs + conditions, report the error details.
- **Timeout reached**: Report current status and ask user whether to continue monitoring or stop the task.

**On completion, present a summary:**
> Task `my-task` completed in 3m 42s.
> [Summary of the agent's output from logs]

## Important Notes

- **Never expose secret values** — only show credential names when describing agents.
- **Cross-namespace**: When `agentRef.namespace` differs from the task namespace, the pod runs in the agent's namespace. This is by design for credential isolation.
- **Server mode**: If an agent has `serverConfig`, tasks use `--attach` to connect to a persistent server. No special handling needed from the user's perspective.
- **Cleanup**: Tasks may be automatically cleaned up by `KubeOpenCodeConfig` TTL/retention policies. If a task or its pod is not found, it may have been cleaned up.
