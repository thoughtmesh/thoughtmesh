# ThoughtMesh

> Your cluster, now thinking.

ThoughtMesh is a Kubernetes-native agent orchestration framework. Define autonomous AI agents as custom resources, let the operator handle the rest — lifecycle management, shared memory, inter-agent messaging, and Slack integration — all from within your cluster.

---

## Overview

ThoughtMesh brings autonomous agent orchestration to Kubernetes. Agents are first-class resources: define an objective, a set of tasks, and an ending condition, and the operator spins up a stateful agentic loop with access to tools, shared memory, and inter-agent messaging.

Everything is managed declaratively with `kubectl`. No separate control plane. No external orchestrator. Just Kubernetes.

---

## Features

- **Declarative agents** — define objectives, tasks, and stopping conditions as Kubernetes CRDs
- **Stateful agentic loop** — each agent runs as a StatefulSet with its own service and message queue
- **Shared memory** — attach a `Memory` resource to one or more agents for collaborative, persistent context
- **Inter-agent messaging** — agents have awareness of each other and can send messages directly via the operator
- **Slack integration** — message an agent from Slack; it picks it up at the next task boundary
- **Built-in tools** — `read`, `write`, `bash`, `todo`, `message`
- **Flexible ending conditions** — natural language, max turns, timeout, or any combination

---

## Concepts

### Agent

The core resource. An `Agent` defines what to do, how to do it, and when to stop.

```yaml
apiVersion: goas.io/v1alpha1
kind: Agent
metadata:
  name: research-agent
spec:
  objective: "Research the latest papers on distributed systems and write a summary"
  tasks:
    - description: "Search for papers published in the last 6 months"
      priority: 1
    - description: "Write a structured summary and save it to output"
      priority: 2
  endingCondition:
    natural: "when the summary has been written and saved"
    maxTurns: 50
    timeoutSeconds: 3600
  tools: [read, write, bash, todo, message]
  output:
    type: file
    path: "/data/summary.md"
```

### Memory

A shared, persistent memory volume that can be attached to one or more agents.

```yaml
apiVersion: goas.io/v1alpha1
kind: Memory
metadata:
  name: shared-research-memory
spec:
  storageSize: 10Gi
  accessMode: ReadWriteMany
  description: "Shared knowledge base for research agents"
  agents:
    - research-agent
    - synthesis-agent
```

---

## How It Works

1. You apply an `Agent` CRD to your cluster
2. The ThoughtMesh operator creates a `StatefulSet` running the `goas-agent` image, a `Service`, and a message queue
3. The agent starts its agentic loop — reasoning over its objective, executing tasks using its tools, and evaluating its ending condition each turn
4. Messages from other agents or from Slack are enqueued and consumed at task boundaries (non-preemptive by default)
5. When an ending condition is met, the agent updates its status and shuts down gracefully

---

## Messaging

Agents are aware of each other by Kubernetes name. The `message` tool lets an agent send a message directly to another agent — the operator routes it to the target's queue.

```
agent-a  →  message("agent-b", "here are the results")
         →  operator enqueues in agent-b's queue
         →  agent-b picks it up at next task boundary
```

Slack messages directed at an agent (via `@mention` or channel mapping) are treated the same way.

---

## Status

Every agent exposes a rich status you can inspect with `kubectl`:

```bash
kubectl get agents
```

```
NAME              PHASE     TURN   CURRENT TASK                        AGE
research-agent    Running   12     Search for papers                   4m
synthesis-agent   Pending   0      -                                   1m
```

---

## Roadmap

- [ ] Skills as CRDs — attach reusable capabilities to agents
- [ ] Agent-to-agent task delegation
- [ ] Web UI for observability
- [ ] Multi-namespace agent discovery

---

## License

MIT
