# Plugins

OpenCode plugins are TypeScript functions that extend Agent capabilities through 17+ hook points: custom tools, auth providers, message/tool interception, event handling, system prompt transformation, and more. KubeOpenCode supports declaring plugins directly on the Agent spec.

The controller creates a **plugin-init** init container that runs `npm install` to download plugins and their dependencies into a shared `/plugins` volume. OpenCode loads them via `file://` paths from `/plugins/node_modules/` — the executor container does not need npm installed.

Plugins are semantically different from Skills and Contexts:
- **Contexts** provide knowledge ("what the agent knows")
- **Skills** provide capabilities ("what the agent can do")
- **Plugins** provide deep customization ("how the agent behaves")

## Basic Usage

```yaml
apiVersion: kubeopencode.io/v1alpha1
kind: Agent
metadata:
  name: my-agent
spec:
  workspaceDir: /workspace
  serviceAccountName: kubeopencode-agent
  plugins:
    - name: "cc-safety-net"
    - name: "@nicholasgriffintn/opencode-plugin-otel"
      options:
        endpoint: "http://otel-collector:4318"
```

The `name` field is the npm package specifier. Version pinning is supported:

```yaml
plugins:
  - name: "cc-safety-net@0.8.2"          # specific version
  - name: "@aexol/opencode-tui@^1.0.0"   # version range
```

## Plugin with Options

Use the `options` field to pass configuration to the plugin. Options are arbitrary JSON objects — each plugin defines its own schema:

```yaml
plugins:
  - name: "@nicholasgriffintn/opencode-plugin-otel"
    options:
      endpoint: "http://otel-collector:4318"
      verbose: true
      serviceName: "my-agent"
```

This generates the following in the OpenCode config:

```yaml
plugin:
  - - "file:///plugins/node_modules/@nicholasgriffintn/opencode-plugin-otel"
    - endpoint: "http://otel-collector:4318"
      verbose: true
      serviceName: "my-agent"
```

## Plugin Credentials

Plugins that need credentials (API keys, tokens) should use `spec.credentials` to inject them as environment variables:

```yaml
apiVersion: kubeopencode.io/v1alpha1
kind: Agent
metadata:
  name: slack-agent
spec:
  workspaceDir: /workspace
  serviceAccountName: kubeopencode-agent
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
```

## How It Works

1. The controller creates a **plugin-init** init container that runs `npm install` with all declared plugin packages
2. Packages and their dependencies are installed into `/plugins/node_modules/` (an emptyDir volume)
3. The executor container mounts `/plugins` as read-only — no npm needed at runtime
4. The controller merges `spec.plugins` into the OpenCode configuration's `plugin` array using `file:///plugins/node_modules/<package>` paths
5. Plugins without options become string entries: `"file:///plugins/node_modules/<package>"`
6. Plugins with options become tuple entries: `["file:///plugins/node_modules/<package>", { ...options }]`
7. If `spec.config` already contains a `plugin` array, new plugins are appended (deduplicated by name)

## Plugins in Templates

Plugins can be defined in AgentTemplates and inherited by Agents. Agent-level plugins replace template-level plugins (same merge strategy as contexts and skills):

```yaml
apiVersion: kubeopencode.io/v1alpha1
kind: AgentTemplate
metadata:
  name: base-template
spec:
  workspaceDir: /workspace
  serviceAccountName: kubeopencode-agent
  plugins:
    - name: "@nicholasgriffintn/opencode-plugin-otel"
      options:
        endpoint: "http://otel-collector:4318"
    - name: "cc-safety-net"
```

## Plugin Targets: Server vs TUI

OpenCode has two plugin runtimes. The `target` field controls which runtime loads the plugin:

| Target | Runtime | Config File | Use Case |
|--------|---------|-------------|----------|
| `server` (default) | `opencode serve` | `opencode.json` | Backend hooks: tools, auth, events, message interception |
| `tui` | Interactive TUI | `tui.json` | Terminal UI: custom commands, themes, UI slots |

```yaml
plugins:
  # Server plugin (default) — runs in the background
  - name: "cc-safety-net"
  
  # TUI plugin — runs when users attach interactively
  - name: "plugin-gentleman"
    target: tui
```

TUI plugins are loaded when users interact with the Agent via `kubeoc agent attach` or the web terminal. The controller generates a separate `tui.json` and sets the `OPENCODE_TUI_CONFIG` environment variable.

## Field Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | (required) | npm package specifier (e.g., `cc-safety-net`, `@org/plugin@^1.0.0`) |
| `target` | string | `server` | Plugin runtime: `server` or `tui` |
| `options` | object | - | Arbitrary JSON object passed to the plugin |

## Community Plugins

OpenCode has 60+ community plugins. Some popular ones for KubeOpenCode:

| Plugin | npm Package | Description |
|--------|-------------|-------------|
| Safety Net | `cc-safety-net` | Intercepts dangerous git/filesystem commands |
| OpenTelemetry | `@nicholasgriffintn/opencode-plugin-otel` | OpenTelemetry telemetry (traces, metrics) |
| Gentleman | `plugin-gentleman` | Animated ASCII mascot during busy states (TUI) |
| Memory | `@anthropics/agent-memory` | Persistent memory across sessions |
| Context Pruning | `opencode-dcp` | Optimize token consumption |

See the [OpenCode Ecosystem Research](https://github.com/kubeopencode/kubeopencode/blob/main/docs/opencode-ecosystem-research.md) for a full catalog.
