# U8 Samples & Distribution — Domain Entities & Method Contracts

本書は U8 (`examples/` + `cmd/xk6-otel-gen-schema/` + project root) の **artifact 内容仕様 + cmd CLI contract** を確定する。

U8 は他 unit と異なり Go public API がほぼないため、本書は **artifact specification** が中心。

---

## 1. cmd/xk6-otel-gen-schema の Go API

### 1.1 main.go の public surface

```go
package main

// public surface: none beyond main()
func main()
```

- 通常の Go executable、library として import される想定なし
- internal helper も unexported

### 1.2 flag

```go
var output = flag.String("output", "", "output file path (default stdout)")
```

- `-output` のみ
- `flag.Parse()` でデフォルト error handling
- usage の自動生成は `flag` パッケージに任せる

### 1.3 exit code

| Code | 意味 |
|---|---|
| 0 | success (JSON Schema が書き出された) |
| 1 | error (topology.ExportJSONSchema 失敗 or file write 失敗) |
| 2 | flag parse error (`flag.Parse` が自動で出す) |

---

## 2. Artifact 仕様 (file-by-file)

### 2.1 `LICENSE`

```text
                                 Apache License
                           Version 2.0, January 2004
                        http://www.apache.org/licenses/

   [Apache 2.0 fulltext as published by SPDX]
   ...
```

- file 名: `LICENSE` (suffix なし、project root)
- content: Apache 2.0 standard text (SPDX 公式から取得、改変なし)
- copyright statement は project author info (NOTICE ファイルや AUTHORS は optional)

### 2.2 `README.md` (project root)

#### 2.2.1 必須 sections (12)

| # | Section | 内容 |
|---|---|---|
| 1 | Project description | k6 拡張で OTel pseudo-telemetry を topology YAML から合成する、の 1-2 段落 |
| 2 | Badges | Go version, License, CI status (placeholder OK) |
| 3 | Quick Start | xk6 build + kind setup + k6 run の 5-step (`bash` code block) |
| 4 | Features | 主要 feature の bullet list (Topology DSL / fault injection / OTLP egress / k6 native metric forwarding 等) |
| 5 | Building | `xk6 build --with github.com/ymotongpoo/xk6-otel-gen` |
| 6 | Usage | JS API summary (`otelgen.configure`, `otelgen.load`, `handle.runJourney`) + link to examples |
| 7 | Topology YAML Reference | brief overview + link to U1's full YAML schema doc + `xk6-otel-gen-schema` CLI usage |
| 8 | Configuration | JS API > `--out args` > env > default の priority table + 例 |
| 9 | Examples | examples/minimal/ と examples/astroshop/ への link |
| 10 | Security | self-build only stance + rationale |
| 11 | Contributing | brief contribution guideline (TODO: link to CONTRIBUTING.md if exists) |
| 12 | License | Apache-2.0 + LICENSE ファイルへのリンク |

#### 2.2.2 サイズ目安

- 全体で 300-400 行程度
- code blocks 多用 (実行可能 commands)

### 2.3 `examples/minimal/topology.yaml`

#### 2.3.1 構造

per `business-logic-model.md` §2.1:
- 3 services (frontend / backend / database)
- 2 edges (HTTP + RPC protocol)
- 1 journey (`checkout`)
- 1 fault (error_rate_override on edge)

#### 2.3.2 不変条件

- `topology.Parse + Validate` を passes
- LatencyDist は `lognormal` 統一
- service.name は kebab-case
- comments で各 field を解説 (新規 user の読みやすさ)

### 2.4 `examples/minimal/script.js`

per `business-logic-model.md` §2.3 の Q3=A pattern:
- 3-phase k6 script
- vus=10, duration='30s'
- 1 runJourney("checkout")

### 2.5 `examples/minimal/otel-collector-config.yaml`

Collector config (k8s ConfigMap に変換される元データ、LGTM-lite stack 対応で 3 pipelines):

```yaml
receivers:
  otlp:
    protocols:
      grpc: { endpoint: 0.0.0.0:4317 }
      http: { endpoint: 0.0.0.0:4318 }

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

### 2.6 `examples/minimal/k8s/*.yaml`

per `business-rules.md` §3:

#### 2.6.1 kustomization.yaml

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: xk6-otel-gen-demo

resources:
  - namespace.yaml
  - collector.yaml
  - tempo.yaml
  - loki.yaml
  - prometheus.yaml
  - grafana.yaml

configMapGenerator:
  - name: otel-collector-config
    files:
      - config.yaml=../otel-collector-config.yaml
  - name: grafana-datasources
    files:
      - datasources.yaml
  - name: grafana-dashboards
    files:
      - dashboard-overview.json
```

#### 2.6.2 namespace.yaml

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: xk6-otel-gen-demo
```

#### 2.6.3 collector.yaml (sketch)

- Deployment (1 replica, image `otel/opentelemetry-collector-contrib:<tag>`)
- Service (ClusterIP, ports 4317 / 4318)
- ConfigMap (Kustomize generated)
- 256Mi / 100m resources.requests

#### 2.6.4 tempo.yaml (sketch)

- Deployment (1 replica, image `grafana/tempo:<tag>`)
- Service (ClusterIP, ports 3200 HTTP API + 4317 OTLP gRPC receive)
- inline ConfigMap with minimal Tempo config (file-backed storage for demo)
- 512Mi / 100m resources.requests

#### 2.6.5 loki.yaml (sketch)

- Deployment (1 replica, image `grafana/loki:<tag>`)
- Service (ClusterIP, port 3100 HTTP API + `/otlp` for OTLP/HTTP receive)
- inline ConfigMap with minimal Loki config (boltdb-shipper for demo)
- 512Mi / 100m resources.requests

#### 2.6.6 prometheus.yaml (sketch)

- Deployment + Service (port 9090)
- ConfigMap with scrape target = `otel-collector:8889`

#### 2.6.7 grafana.yaml (sketch)

- Deployment (image `grafana/grafana:<tag>`)
- Service (ClusterIP, port 3000) — **user の primary entry point**
- env: `GF_AUTH_ANONYMOUS_ENABLED=true`, `GF_AUTH_ANONYMOUS_ORG_ROLE=Editor` (demo 用、admin login バイパス)
- volumeMounts:
  - `/etc/grafana/provisioning/datasources/datasources.yaml` ← grafana-datasources ConfigMap
  - `/etc/grafana/provisioning/dashboards/dashboard-overview.json` ← grafana-dashboards ConfigMap
- 256Mi / 100m resources.requests

### 2.7 `examples/minimal/README.md`

per `business-rules.md` §4.2 の 7 sections。

### 2.8 `examples/minimal/k8s/README.md`

per `business-rules.md` §4.3。

### 2.9 `examples/astroshop/*`

- topology.yaml: 18 services (14 application + 4 dependency) per `business-logic-model.md` §3
- script.js: weighted scenarios (browse + checkout) per §3.4
- k8s/ + otel-collector-config.yaml + README.md: minimal と同 layout
- 各 file は意図的に minimal と並行 structure (consistency per business-rules §9.1)

---

## 3. ファイル一覧と line count 目安

```text
examples/
├── minimal/                                 ~900 lines total
│   ├── README.md                            (~120 lines)
│   ├── topology.yaml                        (~80 lines)
│   ├── script.js                            (~30 lines)
│   ├── otel-collector-config.yaml           (~40 lines, 3 pipelines)
│   └── k8s/
│       ├── kustomization.yaml               (~25 lines)
│       ├── namespace.yaml                   (~5 lines)
│       ├── collector.yaml                   (~80 lines)
│       ├── tempo.yaml                       (~80 lines, incl inline config)
│       ├── loki.yaml                        (~90 lines, incl inline config)
│       ├── prometheus.yaml                  (~70 lines)
│       ├── grafana.yaml                     (~80 lines)
│       ├── datasources.yaml                 (~20 lines, Tempo/Prometheus/Loki provisioning)
│       ├── dashboard-overview.json          (~200 lines, JSON dashboard for 3 signals)
│       └── README.md                        (~80 lines)
│
└── astroshop/                               ~1800 lines total
    ├── README.md                            (~150 lines)
    ├── topology.yaml                        (~500 lines, 18 services)
    ├── script.js                            (~80 lines, scenarios)
    ├── otel-collector-config.yaml           (~40 lines)
    └── k8s/                                 ~similar to minimal/k8s + astroshop dashboard

cmd/
└── xk6-otel-gen-schema/                     ~150 lines total
    ├── main.go                              (~50 lines)
    └── main_test.go                         (~100 lines)

README.md                                    (~400 lines, project root)
LICENSE                                       (~200 lines, Apache-2.0)
```

合計概算: **~3,000 lines**。

---

## 4. 依存

### 4.1 cmd/xk6-otel-gen-schema/

| 依存 | 用途 |
|---|---|
| `flag` (stdlib) | CLI parsing |
| `fmt`, `os`, `io` (stdlib) | I/O |
| `github.com/ymotongpoo/xk6-otel-gen/topology` | `ExportJSONSchema()` |

外部 deps なし (project deps のみ)。

### 4.2 examples/

- production code 依存なし — 静的 file のみ
- `topology.yaml` は U1 `topology.Schema` schema に合致
- `script.js` は U5 `k6/x/otel-gen` の JS API に合致
- k8s manifests は Kubernetes API objects (公式 schema)

### 4.3 README

- markdown のみ、依存なし
- link references を `topology` / `synth` / `exporter` の godoc に向けることがある (将来 godoc.io URL)

---

## 5. CI / Lint 統合 (Out of Scope for U8)

CI workflow は Build and Test stage で確定。U8 では deliverable のみ。

### 5.1 deliverable validation (CI で行うべき)

- `kubectl apply --dry-run=server -k examples/*/k8s/`
- topology.yaml を U1 `topology.Parse + Validate` で apply (Go test in CI)
- README link check (broken link detection)

これらは Build and Test stage の CI workflow に含める想定。

---

## 6. 公開 API シグネチャ一覧

```go
// cmd/xk6-otel-gen-schema/main.go — single binary, no library surface
package main
func main()
```

- 全 deliverable のうち Go API として exposed なのは `main()` 1 つのみ
- `examples/` は static file
- `LICENSE`, `README.md` も file

---

## 7. U7 への generator 追加リクエスト (Q12=A)

**なし**。U8 では PBT generator を新規追加しない:
- cmd/xk6-otel-gen-schema は example-based test で十分
- examples は static file、generator 不要

---

## 8. Application Design `unit-of-work.md` U8 説明からの修正点

| 修正 | 理由 |
|---|---|
| Docker Compose → **Kubernetes manifests** | ユーザー指摘 (Q4=X)、production-like + cluster-aware deployment 学習 |
| Jaeger → **Tempo + Grafana** (LGTM-lite stack) | ユーザー指摘で更新。3 signals (traces / metrics / logs) を **Grafana 1 UI で統一可視化**、Tempo は OTLP native、Loki も追加して synth の logs も展示 |
| **pre-built binary 配布なし** で確定 | Q6/Q10=A、supply chain risk 回避 |
| `cmd/xk6-otel-gen-schema/` は **stdlib flag のみ、export サブコマンドのみ** | Q7/Q8=A、minimal CLI |
| examples LICENSE は project Apache-2.0 と統一 | Q11=A |
| `examples/astroshop/` を **OTel Demo 14 services 完全模倣** | Q2=A |

---

## 9. Out of Scope (再掲)

`business-logic-model.md` §10 と同じ。
