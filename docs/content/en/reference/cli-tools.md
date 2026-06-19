---
title: CLI Tools
weight: 0
---

Companion CLI tools that work with topology YAML files. Both are built with
`go build` and require no external dependencies at runtime.

## xk6-otel-gen-viz

Generates a self-contained, interactive HTML visualization of a topology DAG.
The output file works offline — all JavaScript libraries (Cytoscape.js, dagre)
are embedded inline.

### Usage

```bash
# Build the tool
go build ./cmd/xk6-otel-gen-viz/...

# Generate HTML to a file (recommended — output is ~700 KB)
go run ./cmd/xk6-otel-gen-viz -input topology.yaml -output topology.html

# Or write to stdout
go run ./cmd/xk6-otel-gen-viz -input topology.yaml > topology.html
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-input` | *(required)* | Path to a topology YAML file |
| `-output` | stdout | Output HTML file path |

### What the visualization shows

The generated HTML page displays:

- **Service DAG** — nodes laid out hierarchically (dagre layout, top-to-bottom).
  Node shape and color indicate the service kind:

  | Kind | Color | Shape |
  |------|-------|-------|
  | `application` | Blue | Rounded rectangle |
  | `database` | Amber | Barrel |
  | `cache` | Green | Diamond |
  | `queue` | Purple | Pentagon |
  | `external_api` | Red | Octagon |

- **Edge styling** — line style indicates the protocol:

  | Protocol | Line |
  |----------|------|
  | `http` | Solid |
  | `grpc` | Dashed |
  | `messaging` | Dotted |

### Interactive features

**Journey toggle** (left sidebar) — click a journey name to highlight the
services and edges that the journey reaches. Non-reachable elements are dimmed.
The traffic weight is shown as a percentage. Click "All" to restore the full
view.

**Fault overlay** (right sidebar) — check a fault to mark its target node or
edge in red. A sparkline shows the fault's intensity schedule over time.
Multiple faults can be active simultaneously.

**Tooltips** — hover over a node to see the service kind, language, framework,
version, replica count, and operations. Hover over an edge to see the
source → target operation, protocol, latency (p50 / p95), error rate, and retry
count.

**Search** — type in the search box to highlight nodes whose name matches.

**Zoom / Pan** — scroll to zoom, drag to pan (built into Cytoscape.js).

### Example

```bash
# Visualize the astroshop example (23 services, 5 journeys, 4 faults)
go run ./cmd/xk6-otel-gen-viz \
  -input examples/astroshop/topology.yaml \
  -output astroshop.html
```

Open `astroshop.html` in a browser. Select the "place-order" journey to see the
full checkout path (frontend → checkout → payment / shipping / email / …), then
enable the "error\_rate\_override" fault to see which node is affected.

---

## xk6-otel-gen-schema

Exports the topology JSON Schema for editor integration and CI validation.

### Usage

```bash
# Write to stdout
go run ./cmd/xk6-otel-gen-schema > topology.schema.json

# Write to a file
go run ./cmd/xk6-otel-gen-schema -output topology.schema.json
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-output` | stdout | Output file path |

Configure your editor to use the generated schema for YAML auto-completion and
inline validation of topology files.
