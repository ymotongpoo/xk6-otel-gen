# U8 Samples & Distribution — Business Logic Model

本書は U8 (`examples/` + `cmd/xk6-otel-gen-schema/` + project root docs) の **deliverable と user-facing コンテンツ** を確定する。

参照: Application Design (`unit-of-work.md` §U8)、`plans/u8-samples-fd-plan.md` の Q1..Q13 回答。

---

## 1. パッケージの責務

U8 は **他 unit と異なり Go public API を持たない** distribution / docs / sample unit:

1. **`examples/minimal/`** — 古典 3-tier (frontend → backend → database) の minimal topology
2. **`examples/astroshop/`** — OTel Demo (astronomy shop) 模倣の 14 services / 多 journey demo
3. **`cmd/xk6-otel-gen-schema/`** — JSON Schema export CLI helper
4. **project root README.md** — full project documentation
5. **LICENSE** — Apache-2.0 fulltext

### 1.1 User journey (どう使うか)

```text
1. User が GitHub で xk6-otel-gen project に到達
2. README で project overview を読む (Quick Start で 5 行コマンドが目に入る)
3. `xk6 build --with github.com/ymotongpoo/xk6-otel-gen` で k6 binary 作成
4. `examples/minimal/` で動作確認:
   - kind cluster を起動 (`kind create cluster`)
   - kubectl apply -k examples/minimal/k8s/  で Collector + Tempo + Prometheus + Loki + Grafana deploy
   - kubectl port-forward で Grafana UI を公開
   - k6 run examples/minimal/script.js --out otel-gen=endpoint=localhost:4317
5. Grafana (統一 UI) で 3 signals (traces via Tempo / metrics via Prometheus / logs via Loki) を確認
6. user は自分の topology.yaml を書き始める (cmd/xk6-otel-gen-schema で JSON Schema 取得、IDE 補完)
7. astroshop で大規模 example を確認
```

### 1.2 deliverable の構造

```text
xk6-otel-gen/  (project root)
├── README.md                      ← full project doc
├── LICENSE                        ← Apache-2.0
├── examples/
│   ├── minimal/
│   │   ├── README.md              ← step-by-step quick start
│   │   ├── topology.yaml          ← 3-tier 古典 topology
│   │   ├── script.js              ← k6 script
│   │   ├── k8s/                   ← Kubernetes manifests (Q4=X)
│   │   │   ├── kustomization.yaml
│   │   │   ├── collector.yaml     ← Collector Deployment + Service + ConfigMap
│   │   │   ├── tempo.yaml         ← Tempo (traces backend)
│   │   │   ├── loki.yaml          ← Loki (logs backend)
│   │   │   ├── prometheus.yaml    ← Prometheus (metrics backend)
│   │   │   ├── grafana.yaml       ← Grafana (統一 UI) + datasources/dashboards ConfigMap
│   │   │   └── README.md          ← kubectl apply 手順 + kind setup
│   │   └── otel-collector-config.yaml  ← Collector config (k8s ConfigMap 元データ)
│   └── astroshop/
│       ├── README.md
│       ├── topology.yaml          ← 14 services
│       ├── script.js              ← weighted journey scenarios
│       ├── k8s/                   ← same structure as minimal
│       │   └── ...
│       └── otel-collector-config.yaml
└── cmd/
    └── xk6-otel-gen-schema/
        ├── main.go                ← CLI: stdlib flag + topology.ExportJSONSchema 呼び出し
        └── main_test.go
```

---

## 2. examples/minimal/ の構成 (Q1=A)

### 2.1 Topology (3-tier 古典)

```yaml
# examples/minimal/topology.yaml
services:
  frontend:
    kind: application
    replicas: 2
    language: go
    framework: net/http
    version: 1.0.0
    operations:
      get_index:
        latency: { distribution: lognormal, p50: 50ms, p95: 150ms }
        calls:
          - edge: frontend_to_backend
            opName: get_user

  backend:
    kind: application
    replicas: 3
    language: java
    framework: spring-boot
    version: 2.5.0
    operations:
      get_user:
        latency: { distribution: lognormal, p50: 20ms, p95: 80ms }
        calls:
          - edge: backend_to_db
            opName: select

  database:
    kind: database
    replicas: 1
    language: c
    framework: postgresql
    version: 14.5
    operations:
      select:
        latency: { distribution: lognormal, p50: 5ms, p95: 30ms }

edges:
  frontend_to_backend:
    from: frontend
    to: backend
    protocol: http
    error_rate: 0.05
  backend_to_db:
    from: backend
    to: database
    protocol: rpc

journeys:
  checkout:
    weight: 1.0
    steps:
      - op: get_index   # frontend's operation
```

### 2.2 1 journey + 1 fault

- journey: `checkout` (frontend → backend → database via Calls)
- fault: `backend` service に `error_rate_override = 0.1` (10% 失敗で cascade demonstration)

### 2.3 k6 script (Q3=A 3-phases pattern)

```javascript
// examples/minimal/script.js
import otelgen from "k6/x/otel-gen";

export const options = {
    vus: 10,
    duration: '30s',
};

export function setup() {
    otelgen.configure({
        endpoint: "localhost:4317",
        protocol: "grpc",
        insecure: true,
    });
    const topology = otelgen.load("./topology.yaml");
    return { topology };
}

export default function (data) {
    data.topology.runJourney("checkout");
}

export function teardown() {
    // Pipeline shutdown is handled by U6 Output.Stop()
}
```

---

## 3. examples/astroshop/ の構成 (Q2=A)

### 3.1 OTel Demo 完全模倣 — 14 services

```text
services:
  frontend:           application (Next.js)
  ad:                 application (Java)
  cart:               application (.NET)
  checkout:           application (Go)
  currency:           application (C++)
  email:              application (Ruby)
  fraud-detection:    application (Kotlin)
  payment:            application (Node.js)
  product-catalog:    application (Go)
  quote:              application (PHP)
  recommendation:     application (Python)
  shipping:           application (Rust)
  image-provider:     application (Nginx)
  accounting:         application (.NET)

  # external dependencies (OTel Demo はこれらも含む):
  redis-cache:        cache (Redis)
  postgres:           database (PostgreSQL)
  kafka:              queue (Kafka)
  flagd:              external_api (feature flag)
```

合計 14 application + 4 dependency service = 18 services (OTel Demo 構造に整合)。Application Design 言及の "10+" を満たす。

### 3.2 Journeys (5 種)

- `browse` — frontend → product-catalog → image-provider (read-heavy)
- `search` — frontend → product-catalog → recommendation
- `add-to-cart` — frontend → cart → redis-cache
- `checkout` — frontend → checkout → (cart, payment, fraud-detection, shipping, email)
- `place-order` — checkout cascade with kafka publish

### 3.3 Fault demonstrations

- payment に periodic error_rate spike (10%)
- shipping に latency_inflation
- 1 service (e.g., recommendation) に occasional crash
- email service edge に disconnect

### 3.4 k6 weighted scenarios

```javascript
export const options = {
    scenarios: {
        browse: { executor: 'constant-vus', vus: 20, duration: '60s', exec: 'browse' },
        checkout: { executor: 'constant-vus', vus: 5, duration: '60s', exec: 'checkout' },
    },
};

export function browse(data) {
    data.topology.runJourney("browse");
}

export function checkout(data) {
    data.topology.runJourney("checkout");
}
```

---

## 4. Kubernetes Manifests + LGTM-lite Stack (Q4=X with Tempo+Grafana)

### 4.1 採用理由

User 指摘により Docker Compose ではなく Kubernetes manifests を採用。さらに **可視化 backend を Jaeger → Tempo+Grafana に変更**:
- production-like deployment pattern を提供
- cluster-aware Collector deployment を学習可能
- **3 signals (traces / metrics / logs) すべてを 1 UI (Grafana) で可視化** — 本ツールが synth する logs も活用できる
- Tempo は OTLP native (Jaeger v1 vs Tempo の比較で、OTel native 化が advanced)
- LGTM-lite stack (Loki + Grafana + Tempo + Prometheus、Mimir は省略して Prometheus 据置)

### 4.2 manifest layout

```text
examples/minimal/k8s/
├── kustomization.yaml       ← kustomize で apply 簡素化
├── namespace.yaml           ← xk6-otel-gen-demo namespace
├── collector.yaml           ← Collector Deployment + Service + ConfigMap
├── tempo.yaml               ← Tempo (traces) Deployment + Service
├── loki.yaml                ← Loki (logs) Deployment + Service
├── prometheus.yaml          ← Prometheus (metrics) + ConfigMap + Service
├── grafana.yaml             ← Grafana (UI) + datasources/dashboards ConfigMap
└── README.md                ← kubectl apply 手順 + kind setup
```

### 4.3 kustomize による deploy

```bash
# Local cluster setup
kind create cluster --name xk6-otel-gen-demo

# Deploy
kubectl apply -k examples/minimal/k8s/

# Port-forward Grafana (統一 UI for all 3 signals)
kubectl port-forward svc/grafana 3000:3000 -n xk6-otel-gen-demo &

# Port-forward Collector for k6 to send signals
kubectl port-forward svc/otel-collector 4317:4317 -n xk6-otel-gen-demo &

# Run k6 against forwarded Collector
k6 run examples/minimal/script.js --out otel-gen=endpoint=localhost:4317,insecure=true

# Browse Grafana: http://localhost:3000 (admin / admin)
# - Explore → Tempo datasource → see traces
# - Explore → Prometheus datasource → see metrics (k6.* + simulated service metrics)
# - Explore → Loki datasource → see logs (synth が emit した synthetic logs)
```

### 4.4 Collector ConfigMap

```yaml
# Snippet of examples/minimal/k8s/collector.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: otel-collector-config
data:
  config.yaml: |
    receivers:
      otlp:
        protocols:
          grpc:
            endpoint: 0.0.0.0:4317
          http:
            endpoint: 0.0.0.0:4318
    processors:
      batch: {}
    exporters:
      otlp/tempo:
        endpoint: tempo:4317
        tls: { insecure: true }
      prometheus:
        endpoint: 0.0.0.0:8889
      otlphttp/loki:
        endpoint: http://loki:3100/otlp
        tls: { insecure: true }
    service:
      pipelines:
        traces:
          receivers: [otlp]
          processors: [batch]
          exporters: [otlp/tempo]
        metrics:
          receivers: [otlp]
          processors: [batch]
          exporters: [prometheus]
        logs:
          receivers: [otlp]
          processors: [batch]
          exporters: [otlphttp/loki]
```

3 pipelines (traces / metrics / logs) を OTLP receiver で受け、それぞれ Tempo / Prometheus / Loki に egress。

### 4.5 Grafana datasource ConfigMap

```yaml
# Snippet of examples/minimal/k8s/grafana.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-datasources
data:
  datasources.yaml: |
    apiVersion: 1
    datasources:
      - name: Tempo
        type: tempo
        access: proxy
        url: http://tempo:3200
        isDefault: false
      - name: Prometheus
        type: prometheus
        access: proxy
        url: http://prometheus:9090
        isDefault: true
      - name: Loki
        type: loki
        access: proxy
        url: http://loki:3100
        isDefault: false
```

Grafana 起動時に provisioning 経由で datasources が自動構成、user は immediately Explore できる。

### 4.6 image pin

- Collector: `otel/opentelemetry-collector-contrib:<pinned-tag>` (U3/U4 integration test と同 tag)
- Tempo: `grafana/tempo:<pinned-tag>` (e.g., `2.6.0`)
- Loki: `grafana/loki:<pinned-tag>` (e.g., `3.2.0`)
- Prometheus: `prom/prometheus:<pinned-tag>` (e.g., `v2.55.0`)
- Grafana: `grafana/grafana:<pinned-tag>` (e.g., `11.3.0`)

具体 tag は NFR Design / Code Generation 時に latest stable を確認して確定。

### 4.6 README per example

`examples/minimal/k8s/README.md` には:
- prerequisites: `kind` / `kubectl` install instructions
- step-by-step deploy 手順
- port-forward 手順
- cleanup (`kind delete cluster`)

---

## 5. README.md (project root, Q5=A)

### 5.1 Section 構成

```text
1. Project description
2. Status / License badges
3. Quick Start
4. Features
5. Building
6. Usage (JS API)
7. Topology YAML Reference (link to detailed doc)
8. Configuration (priority order)
9. Examples (link to examples/)
10. Security
11. Contributing
12. License
```

### 5.2 Quick Start example

```bash
# 1. Build a k6 binary with the extension
xk6 build --with github.com/ymotongpoo/xk6-otel-gen

# 2. Set up a local Kubernetes cluster (kind)
kind create cluster --name xk6-otel-gen-demo
kubectl apply -k examples/minimal/k8s/

# 3. Run the example
kubectl port-forward svc/otel-collector 4317:4317 &
./k6 run examples/minimal/script.js --out otel-gen=endpoint=localhost:4317,insecure=true

# 4. View traces
kubectl port-forward svc/jaeger 16686:16686 &
open http://localhost:16686
```

### 5.3 Security section content (Q6=A + Q10=A)

```markdown
## Security

This project does **not distribute pre-built k6 binaries**. Users must
build their own k6 binary with `xk6 build` to verify the supply chain.

Rationale:
- A pre-built k6 binary linked with xk6-otel-gen would be a custom distribution
  outside the official k6 release pipeline. Distributing it would require
  signing infrastructure (sigstore, GPG, etc.) and increase user trust risk.
- Building locally with `xk6` lets users inspect the dependency graph and
  pin a known-good commit of this repository.

Vulnerability disclosure: see SECURITY.md (TODO if/when a SECURITY.md exists).
License: Apache-2.0.
```

---

## 6. cmd/xk6-otel-gen-schema/ (Q7=A + Q8=A)

### 6.1 CLI usage

```bash
xk6-otel-gen-schema [--output <path>]
```

- `--output <path>`: schema output file path. Default stdout.

### 6.2 Behavior

```text
1. Parse flags via stdlib `flag`
2. Call topology.ExportJSONSchema() (U1 で実装済)
3. Write to stdout or `--output` file
```

### 6.3 main.go スケッチ

```go
package main

import (
    "flag"
    "fmt"
    "io"
    "os"

    "github.com/ymotongpoo/xk6-otel-gen/topology"
)

func main() {
    var output string
    flag.StringVar(&output, "output", "", "output file path (default stdout)")
    flag.Parse()

    schema, err := topology.ExportJSONSchema()
    if err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }

    var w io.Writer = os.Stdout
    if output != "" {
        f, err := os.Create(output)
        if err != nil { fmt.Fprintf(os.Stderr, "error: %v\n", err); os.Exit(1) }
        defer f.Close()
        w = f
    }
    if _, err := w.Write(schema); err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err); os.Exit(1)
    }
}
```

### 6.4 Usage example in README

```bash
# Generate JSON Schema for editor support
xk6-otel-gen-schema --output schema.json

# Reference in your topology.yaml
# yaml-language-server: $schema=./schema.json
```

---

## 7. PBT / test (Q12=A)

### 7.1 examples/

- **Static files**: topology.yaml / script.js / k8s manifests — Go test なし
- **CI build only test** (Q9=A): `go build ./...` で xk6 build が破綻していないことを確認

### 7.2 cmd/xk6-otel-gen-schema/

- `main_test.go`: example-based test (flag parse + ExportJSONSchema return → file write check)
- PBT なし (Q12=A、argument parsing は table-driven で十分)

---

## 8. License (Q11=A)

- project root `LICENSE`: Apache-2.0 fulltext
- examples/* も project LICENSE 配下、個別 license header 不要
- 各 .go file の SPDX header (`// SPDX-License-Identifier: Apache-2.0`) は project convention に従う (U1-U6 と一致)

---

## 9. CI / Release (Out of Scope for U8 FD)

CI workflow / GitHub release automation は **Build and Test stage** の責務 (AIDLC workflow より). U8 は deliverable のみを定義し、actual release pipeline は別途。

---

## 10. Out of Scope (U8 では扱わない)

- **k6 binary 配布** (Q10=A 自前ビルド方針)
- **Public web docs site** (project README で完結)
- **API auto-generation** — godoc が canonical
- **Multi-architecture binary** — user が自分の環境で xk6 build
- **CI / release automation** — Build and Test stage の責務
- **Helm chart / Operator** — k8s manifests のみ提供、より advanced setup は user に委ねる
