# ThoughtMesh

A Kubernetes-native orchestrator for short-running, single-objective AI agents.

---

## How It Works

1. Create an **`Agent`** — the operator spawns a Kubernetes Job and runs the agent.
2. The `Agent` object records the result, status, and runtime metrics when done.

```
Agent   →   Job   →   Pod
(spec)     (k8s)   (runtime)
```

---

## Agent

An AI agent that executes a single objective.

```yaml
apiVersion: core.thoughtmesh.dev/v1alpha1
kind: Agent
metadata:
  name: summarize-incidents
  namespace: production
spec:
  objective: |
    Summarize all PagerDuty incidents from the last 24 hours
    and post a digest to the #ops-digest Slack channel.
  
  keyResults:
    - "Generate digest with incident summaries"
    - "Post to #ops-digest Slack channel"

  model:
    worker:
      provider: anthropic
      apiName: claude-sonnet-4-20250514
      endpoint: ""              # omit for provider default
      temperature: "0.2"
      systemPrompt: ""          # omit for ThoughtMesh default
      params: {}                # optional provider-specific parameters

  image: ghcr.io/ufukbombar/thoughtmesh-runtime:latest  # omit to use ThoughtMesh default

  tools:
    - name: pagerduty-mcp
      type: mcp
      url: https://mcp.pagerduty.com/sse
    - name: slack-mcp
      type: mcp
      url: https://slack.mcp.example.com/sse

  context:
    configMapRefs:
      - ops-context          # applied first
      - pagerduty-config     # overwrites conflicting keys from above
    secretRefs:
      - anthropic-creds
      - pagerduty-creds
      - slack-creds

  limits:
    timeout: 5m

  lifecycle:
    retryPolicy:
      maxRetries: 2
      backoffSeconds: 30
    completion:
      onSuccess: delete               # delete | retain | archive
      onFailure: retain
```

### Fields

| Field | Required | Description |
|---|---|---|
| `objective` | yes | Plain-language goal for the agent. One task, one goal. |
| `keyResults` | no | List of measurable outcomes that indicate objective completion. |
| `model` | yes | Role-based model configuration. See [Model](#model). |
| `image` | no | Container image for the agent Pod. Omit to use the ThoughtMesh default runtime image. |
| `tools` | no | MCP servers available to the agent. |
| `context.configMapRefs` | no | Ordered list of ConfigMaps mounted as env vars. Later entries overwrite conflicting keys. |
| `context.secretRefs` | no | Ordered list of Secrets mounted as env vars. Later entries overwrite conflicting keys. |
| `limits.timeout` | no | Hard time limit in Go duration format (e.g. `5m`, `30s`, `1h`). If not specified, agent runs without timeout. |
| `lifecycle.retryPolicy` | no | Retry on failure with backoff. |
| `lifecycle.completion.onSuccess` | no | Pod disposition after success. Default: `retain`. |
| `lifecycle.completion.onFailure` | no | Pod disposition after failure. Default: `retain`. |

---

## Model

Models are configured per role under `spec.model`. Currently the only supported role is `worker`. Additional roles may be introduced in future versions.

```yaml
model:
  worker:
    provider: anthropic
    apiName: claude-sonnet-4-20250514
    endpoint: ""
    temperature: 0.2
    systemPrompt: ""
    params:
      top_p: "0.9"
      top_k: "40"
```

### Role Fields

| Field | Required | Description |
|---|---|---|
| `provider` | yes | LLM provider name. See [Supported Providers](#supported-providers). |
| `apiName` | yes | Model API identifier. See [Supported Providers](#supported-providers). |
| `endpoint` | no | Custom endpoint URL. Omit to use the provider default. Required for Ollama and Azure OpenAI. |
| `temperature` | no | Sampling temperature (0.0–1.0). Lower values are more deterministic. Default: `0.2`. |
| `systemPrompt` | no | Override the base system prompt injected by the ThoughtMesh runtime. Omit to use the default. |
| `params` | no | Free-form `map[string]string` for provider-specific parameters (e.g. `top_p`, `top_k`). Values are passed through to the provider client as-is. |

### Supported Providers

| Provider | `provider` | Example `apiName` | `endpoint` |
|---|---|---|---|
| Anthropic | `anthropic` | `claude-sonnet-4-20250514`, `claude-opus-4-20250514` | omit |
| OpenAI | `openai` | `gpt-4o`, `gpt-4o-mini`, `o3-mini` | omit |
| Ollama | `ollama` | `llama3.3`, `mistral`, `gemma3`, `qwen2.5-coder` | required (e.g. `http://ollama:11434`) |
| Google Vertex AI | `vertex` | `gemini-2.0-flash`, `gemini-2.5-pro` | omit |
| Azure OpenAI | `azure-openai` | your deployment name (e.g. `gpt-4o-prod`) | required |
| Mistral | `mistral` | `mistral-large-latest`, `mistral-small-latest` | omit |

---

## Context

`context` declares the Kubernetes-native config and credential sources the operator mounts into the agent Pod at spawn time. Values never appear in the `Agent` spec itself.

Both fields accept an ordered list. Keys are applied in order — later entries overwrite conflicting keys from earlier ones. Place general configs first, specific ones last.

```yaml
# ConfigMap — non-sensitive config
apiVersion: v1
kind: ConfigMap
metadata:
  name: ops-context
  namespace: production
data:
  ENVIRONMENT: production
  SLACK_CHANNEL: "#ops-digest"
```

```yaml
# Secret — sensitive credentials
apiVersion: v1
kind: Secret
metadata:
  name: anthropic-creds
  namespace: production
stringData:
  ANTHROPIC_API_KEY: sk-ant-...
```

The agent runtime reads these as standard environment variables (`$ENVIRONMENT`, `$ANTHROPIC_API_KEY`, etc.).

---

## Status

The operator updates `Agent.status` throughout the run.

```yaml
status:
  phase: Succeeded              # Pending | Running | Succeeded | Failed | Retrying
  startTime: "2026-04-29T10:00:00Z"
  completionTime: "2026-04-29T10:03:47Z"
  llmCallsUsed: 23
  jobRef:
    name: summarize-incidents-2026-04-29-job
  result: |
    Posted incident digest to #ops-digest. 3 incidents summarized.
```

---

## Design Principles

- **Short-running.** Agents complete and terminate. They are not services.
- **Single objective.** One goal per agent. Split complexity across multiple `Agent` runs.
- **Simple and direct.** Single CRD with all configuration inline. No templates or overrides.
- **Kubernetes-native.** Works with existing RBAC, namespacing, and resource quotas.
