# ADR 0034: First-Class Plugin Support and Slack Integration

## Status

Accepted (Part 1 — `spec.plugins` implemented; Part 2 — Slack plugin pending)

## Date

2026-04-15

## Context

KubeOpenCode currently leverages only OpenCode's basic `run` and `serve` capabilities. OpenCode's **Plugin System** — one of its five major extension mechanisms — is entirely untapped (see `docs/opencode-ecosystem-research.md`).

OpenCode plugins are async TypeScript functions that receive a `PluginInput` (including a full SDK client and project context) and return a `Hooks` object with 17+ extension points: custom tools, auth providers, message/tool interception, event subscriptions, system prompt transformation, and more. Plugins are loaded from local files via `file://` paths, configured via the `plugin` array in `opencode.json`.

### Why Plugins Matter for KubeOpenCode

The plugin system is a core reason for choosing OpenCode as our Agent Engine. It enables:

1. **Team-level customization** — Each team can develop domain-specific plugins (Slack bot, Jira integration, custom guardrails) without modifying KubeOpenCode or OpenCode itself
2. **Enterprise integration** — Observability (OTel), safety nets, token quota, custom auth providers
3. **Ecosystem leverage** — 60+ community plugins available (see ecosystem research)

### Current Limitation

Users *can* pass plugins via `spec.config`:

```yaml
spec:
  config:
    plugin:
      - "@some-org/some-plugin"
```

This has significant UX problems:
- Manual config editing is error-prone and hard to validate
- Plugin configuration is mixed with model/provider settings
- Per-plugin options require nested JSON arrays (`["pkg", { opts }]`)
- Controller has no visibility into which plugins are loaded
- If the user already has `spec.config` for other settings, they must manually merge JSON

### Driving Use Case: Slack Integration

Our roadmap explicitly lists "Instant Messaging Integration" as Direction 1 (see `website/docs/roadmap.md`). The specific need is:

- Team members `@mention` a Slack bot in a channel or send DMs
- The bot creates/resumes an OpenCode session on a KubeOpenCode Agent
- Responses and tool execution status stream back to the Slack thread
- Each thread maintains its own session context

OpenCode already has an official Slack bot (`packages/slack/`) that demonstrates this pattern as a standalone service. Our approach is to **repackage this as an OpenCode server plugin** — running in-process within the Agent's OpenCode server, using the plugin `client` for session management and the `event` hook for real-time updates.

### Design Evolution

The design went through three iterations:

1. **npm-based (initial)** — Use npm registry directly. Rejected because executor containers shouldn't need npm at runtime.
2. **Git-based (v2)** — Clone plugin repos via git-init containers. Rejected because Git repos contain source code (often needs build), not installable packages, and don't include transitive dependencies.
3. **npm-based with init container (final)** — A dedicated `plugin-init` init container runs `npm install` to download compiled packages with all dependencies into a shared emptyDir volume. The executor container loads plugins from the pre-installed `node_modules/` — no npm needed at runtime.

The final approach is superior because:
- npm packages contain compiled/built artifacts (not source code)
- `npm install` resolves the full dependency tree in one step
- The kubeopencode image already includes Node.js/npm (needed for the init container)
- The executor container only needs read access to `/plugins/node_modules/`

### Related ADRs and Research

- **ADR 0011** — Originally proposed "Slack Bot with Repository Context" as the driving use case for Agent Server Mode
- **ADR 0026** — Skills as a top-level Agent field (precedent for promoting extension mechanisms)
- **ADR 0026-mcp** — MCP Server support (another extension mechanism elevated to first-class)
- **Ecosystem Research** — Section C proposed "Plugin Pre-installation" with `spec.plugins` field

## Decision

### Part 1: `spec.plugins` Field on Agent and AgentTemplate

Add a first-class `plugins` field to `AgentSpec` and `AgentTemplateSpec`:

```go
// PluginTarget specifies which OpenCode plugin runtime to load the plugin into.
// +kubebuilder:validation:Enum=server;tui
type PluginTarget string

const (
    PluginTargetServer PluginTarget = "server" // Backend hooks (tools, auth, events)
    PluginTargetTUI    PluginTarget = "tui"    // Terminal UI extensions (commands, themes)
)

// PluginSpec defines an OpenCode plugin to install from the npm registry.
type PluginSpec struct {
    // Name is the npm package specifier of the plugin.
    // Supports standard npm specifier formats:
    //   "cc-safety-net"                    — latest version
    //   "cc-safety-net@0.8.2"             — specific version
    //   "@aexol/opencode-tui"             — scoped package
    //   "@aexol/opencode-tui@^1.0.0"      — version range
    //
    // +required
    // +kubebuilder:validation:MinLength=1
    Name string `json:"name"`

    // Target specifies which runtime loads this plugin: "server" (default) or "tui".
    // +optional
    // +kubebuilder:default=server
    Target PluginTarget `json:"target,omitempty"`

    // Options is an arbitrary JSON object passed to the plugin function.
    // +optional
    // +kubebuilder:pruning:PreserveUnknownFields
    // +kubebuilder:validation:Schemaless
    Options *runtime.RawExtension `json:"options,omitempty"`
}
```

On `AgentSpec` and `AgentTemplateSpec`:

```go
// Plugins declares OpenCode plugins to install for this Agent.
// The controller creates a plugin-init container that runs `npm install`
// to download plugins and their dependencies into a shared /plugins volume.
// OpenCode loads plugins from file:///plugins/node_modules/{package} — the
// executor container does not need npm installed.
//
// Plugin name is the npm package specifier (e.g., "cc-safety-net",
// "@aexol/opencode-tui@^1.0.0").
//
// Plugin credentials should be provided via spec.credentials (as env vars).
//
// When templateRef is set, Agent plugins replace template plugins (same merge
// strategy as contexts, skills, and credentials).
//
// Example:
//   plugins:
//     - name: "cc-safety-net"
//     - name: "@nicholasgriffintn/opencode-plugin-otel"
//       options:
//         endpoint: "http://otel-collector:4318"
// +optional
Plugins []PluginSpec `json:"plugins,omitempty"`
```

#### Controller Config Merge Logic

The controller splits plugins by `target` field and builds two config files:

1. **Server plugins** (`target: server`, default) → merged into `/tools/opencode.json`
2. **TUI plugins** (`target: tui`) → written to `/tools/tui.json`, exposed via `OPENCODE_TUI_CONFIG` env var

For server plugins, the controller merges three sources into `opencode.json`:

1. **User's `spec.config`** — Base configuration (model, provider, permissions, etc.)
2. **`spec.plugins`** (server target) — Converted to OpenCode's plugin array format
3. **`spec.skills`** — Converted to `skills.paths` (existing logic)

Plugin conversion rules:
- Plugin without options → string entry: `"file:///plugins/node_modules/<package>"`
- Plugin with options → tuple entry: `["file:///plugins/node_modules/<package>", { ...options }]`

The package name is extracted from the npm specifier (e.g., `cc-safety-net@0.8.2` → `cc-safety-net`, `@org/plugin@^1.0` → `@org/plugin`).

Merge strategy: plugins from `spec.plugins` are **appended** to any plugins already present in `spec.config`. This allows users to declare some plugins in the structured field and others in raw config if needed. Duplicate detection is by name (first occurrence wins).

Example output (`/tools/opencode.json`):

```json
{
  "model": "anthropic/claude-sonnet-4-20250514",
  "plugin": [
    "file:///plugins/node_modules/@kubeopencode/opencode-slack-plugin",
    ["file:///plugins/node_modules/@nicholasgriffintn/opencode-plugin-otel", { "endpoint": "http://otel-collector:4318" }]
  ],
  "skills": {
    "paths": ["/workspace/.kubeopencode/skills/devops"]
  }
}
```

#### Template Merge Strategy

Consistent with all list fields: **Agent replaces template** if Agent specifies `plugins`. If Agent has no `plugins` field, template's `plugins` are inherited.

#### Plugin Installation

The controller creates a **plugin-init** init container (using the kubeopencode system image, which includes Node.js and npm) that:

1. Reads the `PLUGIN_PACKAGES` environment variable (JSON array of npm specifiers)
2. Runs `npm install --production --no-audit --no-fund` in the `/plugins` directory
3. All packages and their dependencies are installed to `/plugins/node_modules/`

The plugins volume is an emptyDir shared between:
- **plugin-init** container (read-write) — installs packages
- **executor** container (read-only) — OpenCode loads plugins via `file://` paths

This approach ensures the executor container does not need npm installed. The kubeopencode system image includes Node.js + npm specifically for the plugin-init container.

Init container execution order: `opencode-init` → `context-init` → `plugin-init`

### Part 2: Slack Plugin (`@kubeopencode/opencode-slack-plugin`)

An OpenCode server plugin that bridges Slack and the Agent's OpenCode sessions.

#### Architecture

```
Slack Cloud (api.slack.com)
    ↕ WebSocket (Socket Mode)
┌───────────────────────────────────────────────┐
│  Agent Pod                                    │
│  ┌─────────────────────────────────────────┐  │
│  │  opencode serve (port 4096)             │  │
│  │  ┌───────────────────────────────────┐  │  │
│  │  │  slack-plugin (in-process)        │  │  │
│  │  │  - @slack/bolt Socket Mode        │  │  │
│  │  │  - Thread ↔ Session mapping       │  │  │
│  │  │  - Event stream → Slack thread    │  │  │
│  │  └───────────────────────────────────┘  │  │
│  └─────────────────────────────────────────┘  │
└───────────────────────────────────────────────┘
```

#### Why Plugin, Not Sidecar

| Aspect | Plugin (chosen) | Sidecar |
|--------|----------------|---------|
| Process overhead | None (in-process) | +1 container |
| Client access | Direct (`input.client`) | HTTP via localhost |
| Configuration | `opencode.json` plugin array | Container spec changes |
| Lifecycle | Tied to OpenCode server | Independent |
| KubeOpenCode changes | Config merge only | pod_builder changes |
| Event access | `event` hook (bus subscription) | SSE subscription |

The plugin approach minimizes KubeOpenCode controller changes — all Slack logic lives in the TypeScript plugin.

#### Plugin Behavior

**Initialization** (plugin function body):

1. Read Slack credentials from environment variables (`SLACK_BOT_TOKEN`, `SLACK_SIGNING_SECRET`, `SLACK_APP_TOKEN`)
2. Create `@slack/bolt` App in Socket Mode
3. Register event handlers:
   - `app_mention` — Channel @mention triggers session creation/prompt
   - `message` (DM) — Direct message handling
4. Start the Slack connection (`app.start()`)
5. Return `Hooks` object with `event` handler

**Message Flow**:

1. Slack message arrives (via Socket Mode WebSocket)
2. Plugin computes session key: `${channel}-${threadTs}`
3. If no session exists for this thread:
   - `input.client.session.create()` — Create new OpenCode session
   - Post session share link to thread (optional)
4. `input.client.session.prompt()` — Send message text to session
5. Wait for response → post to Slack thread

**Event Streaming** (via `event` hook):

- `message.part.updated` (tool type, status=completed) → Post tool execution summary to Slack thread (e.g., `*file_read* - Read package.json`)
- `session.idle` → Optional: post "done" indicator
- `server.instance.disposed` → `app.stop()` for graceful cleanup

**Thread Context**:

Same Slack thread reuses the same OpenCode session. The mapping is in-memory (`Map<string, string>`). On pod restart, sessions are lost but new messages create new sessions. Future enhancement: persist mapping via session metadata/title for recovery.

#### Required Slack App Configuration

OAuth Scopes:
- `app_mentions:read` — Receive @mention events
- `chat:write` — Post messages to channels/threads
- `channels:history` — Read channel message history (for thread context)
- `groups:history` — Read private channel history
- `im:history` — Read DM history
- `im:write` — Send DM messages

Socket Mode: Enabled (requires App-Level Token with `connections:write` scope)

#### Agent YAML Example

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: slack-credentials
  namespace: team-alpha
type: Opaque
stringData:
  bot-token: "xoxb-..."
  signing-secret: "..."
  app-token: "xapp-..."
---
apiVersion: kubeopencode.io/v1alpha1
kind: Agent
metadata:
  name: slack-agent
  namespace: team-alpha
spec:
  workspaceDir: /workspace
  
  plugins:
    - name: "@kubeopencode/opencode-slack-plugin"
  
  credentials:
    - name: slack-bot-token
      secretRef:
        name: slack-credentials
        key: bot-token
      env: "SLACK_BOT_TOKEN"
    - name: slack-signing-secret
      secretRef:
        name: slack-credentials
        key: signing-secret
      env: "SLACK_SIGNING_SECRET"
    - name: slack-app-token
      secretRef:
        name: slack-credentials
        key: app-token
      env: "SLACK_APP_TOKEN"
  
  config:
    model: anthropic/claude-sonnet-4-20250514
  
  contexts:
    - name: project-repo
      type: Git
      git:
        url: https://github.com/myorg/myrepo.git
        ref: main
  
  persistence:
    sessions:
      size: 1Gi
  
  standby:
    idleTimeout: "2h"
```

## Consequences

### Positive

1. **Clean API** — Structured plugin declaration with per-plugin options, no JSON crafting
2. **Consistent pattern** — Follows the same design as `spec.skills` (ADR 0026) and `spec.contexts` for promoting extension mechanisms to first-class fields
3. **Minimal controller changes** — Plugin installation via npm in init container; controller only handles config merge
4. **Ecosystem access** — 60+ community plugins immediately usable via npm package name
5. **Slack integration** — Teams can interact with Agents directly from Slack without leaving their workflow
6. **Foundation for IM integrations** — The plugin pattern can be replicated for Lark/Feishu, Teams, Discord
7. **No npm at runtime** — Executor container loads pre-installed packages from `/plugins/node_modules/` — no npm needed

### Negative

1. **npm registry dependency** — Plugin-init container requires network access to the npm registry at pod startup. Mitigated by enterprise npm mirrors/proxies and the fact that this only runs during init (not at runtime)
2. **In-memory session mapping** — Slack thread ↔ OpenCode session mapping is lost on pod restart. Acceptable for MVP; can be improved via session metadata persistence
3. **No shutdown hook** — OpenCode plugin API lacks an explicit cleanup lifecycle hook. We rely on the `server.instance.disposed` event, which is less deterministic
4. **Network dependency** — Socket Mode requires outbound WebSocket to Slack. Enterprise firewalls may block this (requires allowlisting `wss://wss-primary.slack.com`)
5. **Image size** — Adding Node.js + npm to the kubeopencode alpine image increases image size (~60MB compressed). Acceptable tradeoff for plugin support.

### Risks

1. **Plugin API stability** — OpenCode's plugin API is still evolving. Breaking changes could require plugin version updates. Mitigated by pinning compatible versions in the npm specifier
2. **npm availability** — Air-gapped environments need an npm mirror. Can be configured via npm registry settings or HTTP proxy

## Implementation Plan

### Step 1: `spec.plugins` Support (KubeOpenCode, Go) — DONE

1. Add `PluginSpec` type and `Plugins` field to `agent_types.go` and `agenttemplate_types.go`
2. Run `make update` (deepcopy + CRD generation)
3. Add `kubeopencode plugin-init` subcommand (reads `PLUGIN_PACKAGES`, runs `npm install`)
4. Add Node.js + npm to Dockerfile
5. Update config merge logic in `skill_processor.go` (`resolvePluginPaths`, `injectPluginsIntoConfig`, `splitPluginsByTarget`)
6. Update `pod_builder.go` (`buildPluginInitContainer`, plugins volume)
7. Update `server_builder.go` (plugin-init integration for Agent Deployments)
8. Update template merge in `template_merge.go`
9. Unit tests (30+ test cases for plugin path resolution, config injection, target splitting)
10. Documentation updates

### Step 2: Slack Plugin (TypeScript) — PENDING

1. Create npm package `@kubeopencode/opencode-slack-plugin`
2. Implement core logic (Slack connection, session management, event streaming)
3. Test locally with `opencode serve` + Slack test workspace
4. Publish to npm registry
5. Add example YAML to `deploy/local-dev/`
6. User documentation and Slack App setup guide

### Future Enhancements (Out of Scope)

- **Session recovery** — Persist thread↔session mapping in session metadata for pod restart recovery
- **Slash commands** — `/kubeopencode` for creating tasks, checking status
- **Multi-Agent routing** — Route different channels to different Agents (1:N)
- **Lark/Feishu plugin** — Same pattern, different messaging SDK
- **Plugin status in AgentStatus** — Report loaded plugins and their health
- **Plugin caching** — Cache installed plugins across pod restarts via PVC
- **Offline/air-gapped support** — Pre-built plugin images or npm tarball injection
