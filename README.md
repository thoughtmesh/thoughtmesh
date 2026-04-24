# ThoughtMesh

> Kubernetes-native orchestration for autonomous AI agents.

ThoughtMesh is a Kubernetes operator that manages AI agents as native cluster resources. Each agent runs as a `StatefulSet` with a stable DNS name, a declarative spec, and an agentic loop powered by [pi-coding-agent](https://github.com/badlogic/pi-mono/tree/main/packages/coding-agent).

> **Status:** Phase 0 — Early Development (`core.thoughtmesh.dev/v1alpha1`)

---

## How it works

When you apply an `Agent` manifest, the operator reconciles it into a `Deployment` and a headless `Service`. The agent container runs the agentic loop autonomously based on its `objective` and `system`. When the `termination` is satisfied the Agent is terminated.

```yaml
apiVersion: core.thoughtmesh.dev/v1alpha1
kind: Agent
metadata:
  name: my-agent
  namespace: default
spec:
  objective: "Summarise all open GitHub issues daily"
  system: "You are a concise technical writer."
  termination: "All issues have been summarised."
```

---

## Prerequisites

- Kubernetes cluster (v1.25+)
- `kubectl` configured
- Docker (for building images)
- Go 1.26.2+
- Node.js 22+ (for pi-coding-agent)

---

## Getting started

```bash
# Clone the repo
git clone https://github.com/your-org/thoughtmesh
cd thoughtmesh

# Install CRDs
make install

# Build and push the agent image
make docker-build-agent

# Deploy the operator
make deploy

# Apply an Agent resource
kubectl apply -f config/samples/core_v1alpha1_agent.yaml
```

---

## Agent status

```bash
kubectl get agents
kubectl describe agent my-agent
```

The `status` subresource exposes:

| Field          | Description                        |
|----------------|------------------------------------|
| `phase`        | `Pending`, `Running`, `Succeeded`, `Failed` |

---

## Development

```bash
# Run the operator locally (outside the cluster)
make run

# Run tests
make test

# Regenerate manifests after CRD changes
make generate && make manifests
```

---

## Roadmap

| Phase | Name | Status |
|-------|------|--------|
| 0 | Core Agent Operator | 🟡 In progress |
| 1 | Messaging & Discovery | Planned |
| 1.1 | Slack Integration | Planned |
| 2 | Skills as Kubernetes Resources | Planned |
| 3 | Observability | Planned |
| 4 | Agent-managed Agents | Exploratory |

---

## License

MIT
