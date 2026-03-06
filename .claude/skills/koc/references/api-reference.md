# KubeOpenCode API Reference

Detailed field reference for KubeOpenCode CRDs. API Group: `kubeopencode.io/v1alpha1`.

## Task (`tk`)

### TaskSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `agentRef` | AgentReference | No* | Agent to use. Required unless taskTemplateRef provides one. |
| `taskTemplateRef` | TaskTemplateReference | No | Reference to a TaskTemplate for base configuration. |
| `description` | string | No | Task instruction/prompt. Written to `${WORKSPACE_DIR}/task.md`. |
| `contexts` | []ContextItem | No | Additional context for the task. |

*Required unless `taskTemplateRef` with `agentRef` is used.

### AgentReference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Agent name. |
| `namespace` | string | No | Agent namespace. If empty, defaults to task's namespace. Pod runs here. |

### TaskTemplateReference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | TaskTemplate name. |
| `namespace` | string | No | TaskTemplate namespace. If empty, defaults to task's namespace. |

### TaskExecutionStatus

| Field | Type | Description |
|-------|------|-------------|
| `phase` | TaskPhase | Current phase: Pending, Queued, Running, Completed, Failed. |
| `agentRef` | AgentReference | Resolved agent reference. |
| `podName` | string | Pod name running the task. |
| `podNamespace` | string | Namespace where the pod runs (may differ from task namespace). |
| `startTime` | Time | When the task started running. |
| `completionTime` | Time | When the task finished. |
| `conditions` | []Condition | Standard Kubernetes conditions (Ready, Queued, Stopped). |

## Agent (`ag`)

### AgentSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `agentImage` | string | No | OpenCode init container image. Default: `quay.io/kubeopencode/kubeopencode-agent-opencode:latest`. |
| `executorImage` | string | No | Main worker container image. Default: `quay.io/kubeopencode/kubeopencode-agent-devbox:latest`. |
| `attachImage` | string | No | Lightweight image for server-mode attach pods. |
| `workspaceDir` | string | Yes | Working directory (must start with `/`). |
| `command` | []string | No | Entrypoint command override. Default: `["sh", "-c", "/tools/opencode run \"$(cat ${WORKSPACE_DIR}/task.md)\""]`. |
| `contexts` | []ContextItem | No | Default contexts for all tasks using this agent. |
| `config` | string | No | OpenCode config as JSON string. Written to `/tools/opencode.json`. |
| `credentials` | []Credential | No | Secrets mounted as env vars or files. |
| `podSpec` | AgentPodSpec | No | Advanced pod configuration (labels, scheduling, resources, runtimeClass). |
| `serviceAccountName` | string | Yes | Kubernetes ServiceAccount for agent pods. |
| `allowedNamespaces` | []string | No | Glob patterns restricting which namespaces can use this agent. Empty = all allowed. |
| `maxConcurrentTasks` | int32 | No | Max concurrent running tasks. nil/0 = unlimited. |
| `quota` | QuotaConfig | No | Rate limiting: `maxTaskStarts` per `windowSeconds`. |
| `serverConfig` | ServerConfig | No | Enables persistent server mode (Deployment + Service). |

### Credential

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Descriptive name. |
| `secretRef.name` | string | Yes | Secret name. |
| `secretRef.key` | string | No | Specific key. If omitted, entire secret is used. |
| `env` | string | No | Env var name (only with secretRef.key). |
| `mountPath` | string | No | File mount path. |
| `fileMode` | int32 | No | File permission (default: 0600). |

### QuotaConfig

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `maxTaskStarts` | int32 | Yes | Max task starts within window (min: 1). |
| `windowSeconds` | int32 | Yes | Sliding window in seconds (60-86400). |

### ServerConfig

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `port` | int32 | No | Server port (default: 4096, range: 1-65535). |

## TaskTemplate (`tt`)

### TaskTemplateSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `description` | string | No | Default task instruction. Overridden by Task's description. |
| `agentRef` | AgentReference | No | Default agent. Overridden by Task's agentRef. |
| `contexts` | []ContextItem | No | Default contexts. Merged with Task contexts (template first, then task). |

**Merge Strategy:** Task takes precedence for `agentRef` and `description`. Contexts are concatenated (template first).

## ContextItem

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | No | Identifier for logging and XML tag generation. |
| `description` | string | No | Human-readable documentation. |
| `type` | ContextType | Yes | `Text`, `ConfigMap`, `Git`, `Runtime`, or `URL`. |
| `mountPath` | string | No* | Mount location. Relative paths prefixed with workspaceDir. |
| `fileMode` | int32 | No | File permission for mounted file (default: 0644). |
| `text` | string | If Text | Inline text content. |
| `configMap` | ConfigMapContext | If ConfigMap | ConfigMap reference (`name`, optional `key`). |
| `git` | GitContext | If Git | Git repo (`repository`, `ref`, optional `path`, `depth`, `secretRef`). |
| `runtime` | RuntimeContext | If Runtime | No fields. Injects platform awareness. |
| `url` | URLContext | If URL | URL fetch (`source`, optional `headers`, `secretRef`, `timeout`). |

*`mountPath` is required for Git context type. If empty for other types, content goes to `task.md`.

## KubeOpenCodeConfig (`ktc`)

Cluster-scoped singleton. Must be named `cluster`.

### KubeOpenCodeConfigSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `systemImage` | SystemImageConfig | No | System image for init containers (`image`, `imagePullPolicy`). |
| `cleanup` | CleanupConfig | No | Task cleanup policies. |

### CleanupConfig

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ttlSecondsAfterFinished` | int32 | No | Delete tasks after N seconds from completion. |
| `maxRetainedTasks` | int32 | No | Max completed tasks per namespace (oldest deleted first). |
