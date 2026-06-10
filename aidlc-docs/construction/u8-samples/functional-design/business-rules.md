# U8 Samples & Distribution — Business Rules

本書は U8 (`examples/` + `cmd/xk6-otel-gen-schema/` + project root) の **deliverable 規則** を確定する。

---

## 1. Topology YAML 規則

### 1.1 examples/minimal/topology.yaml

- 3 services: `frontend` (application), `backend` (application), `database` (database)
- 2 edges: `frontend_to_backend` (http), `backend_to_db` (rpc)
- 1 journey: `checkout`
- 1 fault: `error_rate_override` on `frontend_to_backend` edge (rate=0.05)
- すべて U1 `topology.Schema.Validate()` を passes する
- file は cmd/xk6-otel-gen-schema が出す JSON Schema に整合
- comment で各 field の意味を簡単に解説

### 1.2 examples/astroshop/topology.yaml

- 18 services (14 application + 4 dependency)
- 5 journeys (browse / search / add-to-cart / checkout / place-order)
- 複数 fault (error_rate / latency_inflation / crash / disconnect)
- 個別 fault は subtle 設定 (10% 以下) で over-stressed にしない
- すべて Validate を passes

### 1.3 共通規則

- service.name は kebab-case (e.g., `product-catalog`)
- operation.name は snake_case (e.g., `get_user`)
- LatencyDist は `distribution: lognormal` + p50/p95 を base とする
- Validate で fail する例 (negative timeout 等) は examples に含めない

---

## 2. k6 script 規則 (Q3=A)

### 2.1 共通 pattern

- `import otelgen from "k6/x/otel-gen";` を最上部
- `setup()` で `configure()` + `load()`
- `default function (data)` で `runJourney()` 呼び出し
- `teardown()` は空 (U6 が shutdown する)
- options: `vus`, `duration` のみで簡素

### 2.2 examples/minimal/script.js

- vus=10, duration='30s'
- 1 journey `"checkout"`

### 2.3 examples/astroshop/script.js

- `options.scenarios` で weighted scenarios
- 2 exec function (`browse`, `checkout`) per Q3=A 拡張

### 2.4 thresholds は含めない

- 学習用 example として overly opinionated にしない (Q3=A 通り)
- user が自分の SLO に応じて追加する

---

## 3. Kubernetes Manifests 規則 (Q4=X)

### 3.1 layout convention (LGTM-lite stack)

```
examples/<example>/k8s/
├── kustomization.yaml
├── namespace.yaml
├── collector.yaml
├── tempo.yaml          ← traces backend
├── loki.yaml           ← logs backend
├── prometheus.yaml     ← metrics backend
├── grafana.yaml        ← unified UI + provisioned datasources
└── README.md
```

### 3.2 namespace

- 全 manifest は `xk6-otel-gen-demo` namespace を target
- cleanup: `kubectl delete namespace xk6-otel-gen-demo`

### 3.3 image pin

- Collector: `otel/opentelemetry-collector-contrib:0.105.0` (NFR Design で specific version 確定)
- Tempo: `grafana/tempo:2.6.0`
- Loki: `grafana/loki:3.2.0`
- Prometheus: `prom/prometheus:v2.55.0`
- Grafana: `grafana/grafana:11.3.0`

NFR Design で最新 stable を確認、pin する。

### 3.4 Service exposure

- Collector: ClusterIP service on 4317 (gRPC) and 4318 (HTTP)
- Tempo: ClusterIP service on 3200 (HTTP API for Grafana datasource) + 4317 (OTLP receive)
- Loki: ClusterIP service on 3100 (HTTP API for Grafana datasource + OTLP receive at /otlp)
- Prometheus: ClusterIP service on 9090 (HTTP API + UI) + 8889 (scrape target on Collector)
- Grafana: ClusterIP service on 3000 (UI) — **user が port-forward する main UI**

User の primary entry point は **Grafana** (port 3000)。Tempo/Loki/Prometheus は cluster 内部通信のみ、port-forward 不要 (Grafana datasource として cluster 内 DNS で resolve)。

### 3.5 ConfigMap

- Collector config は ConfigMap として供給
- `examples/<example>/otel-collector-config.yaml` を Kustomize `configMapGenerator` で ConfigMap 化
- Grafana datasource (`datasources.yaml`) と sample dashboard (`dashboard-*.json`) も ConfigMap として provisioning される

### 3.6 resources / limits

- 各 Deployment に conservative `resources.requests` を設定 (256Mi/100m 程度)
- kind / minikube の小 cluster でも動作する

### 3.7 K8s manifest validation

- `kubectl apply --dry-run=server -k examples/<example>/k8s/` で CI validate (Build and Test stage で)
- U8 自身では actual cluster での test なし (Q9=A build only)

---

## 4. README 規則 (Q5=A)

### 4.1 project root README.md

- 12 sections per Q5=A
- 各 section header は `##` (project title は `#` 1 つのみ)
- code block は language 指定必須 (` ```bash`, ` ```yaml` 等)

### 4.2 examples/<example>/README.md

- 各 example ごとに per-example README
- Sections:
  1. Description
  2. Prerequisites (kind / kubectl / xk6 install)
  3. Setup steps (kind cluster + kubectl apply)
  4. Run k6
  5. View results (port-forward + browse)
  6. Cleanup
  7. Customize (topology.yaml を編集して遊ぶ guidance)

### 4.3 examples/<example>/k8s/README.md

- per-example の k8s manifests 専用 README
- kind cluster 起動 + kubectl apply + port-forward + cleanup
- 親 README からリンク

### 4.4 Code block convention

- shell コマンドは `bash` highlight
- YAML は `yaml` highlight
- Go is `go`
- JS は `javascript`

---

## 5. cmd/xk6-otel-gen-schema/ 規則 (Q7=A + Q8=A)

### 5.1 CLI 仕様

```text
xk6-otel-gen-schema [-output <path>]

-output string
    output file path. If unset, write to stdout.
```

- `-version` flag は省略 (将来 ldflags injection で追加検討)
- 不正 argument → `flag.Parse()` の default error 処理 (k6/xk6 慣例に整合)

### 5.2 exit code

- 0: success
- 1: error (topology.ExportJSONSchema が error 返却 or file write 失敗)

### 5.3 output format

- JSON Schema Draft 2020-12 (U1 で確定済)
- `topology.ExportJSONSchema()` の戻り値をそのまま write
- pretty-print (indent 2) は **U1 の ExportJSONSchema が既に行う前提**、本 CLI では追加処理なし

---

## 6. LICENSE 規則 (Q11=A)

### 6.1 fulltext

- Apache License 2.0 fulltext を `LICENSE` ファイルに配置
- standard text を SPDX 公式 ([https://spdx.org/licenses/Apache-2.0.html](https://spdx.org/licenses/Apache-2.0.html)) から取得

### 6.2 個別 file の license header

- 各 `.go` ファイルに SPDX header (U1-U6 と同 convention):
  ```go
  // SPDX-License-Identifier: Apache-2.0
  ```
- examples 内 file は **header 不要** (project LICENSE が covers、Q11=A)
- README / YAML / shell スクリプトも header 不要

### 6.3 copyright

- copyright holder: project author / contributors (specific 名前は project root の AUTHORS or NOTICE で管理、未作成なら不要)

---

## 7. CI build 規則 (Q9=A)

### 7.1 U8 build check

- `go build ./cmd/...` — cmd/xk6-otel-gen-schema build 成功
- `go test -race ./cmd/...` — cmd の test passes
- `xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.` (CI で integration test 内で実行) — k6 binary build 成功

### 7.2 examples の static validation

- `kubectl apply --dry-run=server -k examples/*/k8s/` (CI に Kubernetes cluster があれば、Build and Test stage で)
- topology.yaml の Validate check: U1 の `topology.Parse + Validate` を CI test 内で example yaml に適用

### 7.3 actual `k6 run` test

- U5/U6 integration test ですでに sample script を test 済
- U8 自身では追加の k6 run test なし (Q9=A)

---

## 8. PBT / Testable Properties (Q12=A)

### 8.1 PBT なし

U8 では rapid PBT は不採用 — code が極小、deliverable は static file が主。

### 8.2 cmd/xk6-otel-gen-schema/ の test

`cmd/xk6-otel-gen-schema/main_test.go`:
- table-driven test for flag parsing + output destination
- Example based:
  - `TestRun_StdoutDefault` — `-output` 未指定 → stdout に出力
  - `TestRun_FileOutput` — `-output schema.json` → temp file に出力
  - `TestRun_OutputContainsTopologyHandle` — 出力 JSON に `Topology` schema 定義が含まれる

---

## 9. Pattern consistency 規則

### 9.1 examples 間 consistency

- both `minimal` and `astroshop` で同じ file layout (topology.yaml / script.js / k8s/ / otel-collector-config.yaml)
- README structure 一貫
- Collector config の receiver / exporter 設定 一貫

### 9.2 image tag consistency

- examples 全体で同じ Collector / Jaeger / Prometheus image tag
- 更新時は 1 箇所 (NFR Design に list 化) を確認

### 9.3 namespace consistency

- 全 example の k8s manifest が `xk6-otel-gen-demo` namespace を target
- user が `minimal` と `astroshop` を同時 deploy する場合は namespace を変えて conflict 回避 (各 example の k8s/README.md で言及)

---

## 10. パフォーマンスとリソース (FD 時点目安)

| 項目 | 期待値 |
|---|---|
| `cmd/xk6-otel-gen-schema` 実行時間 | < 100 ms (JSON Schema 出力のみ) |
| examples/minimal の k8s cluster minimum memory | < 1 GB (kind cluster + Collector + Jaeger + Prometheus) |
| examples/astroshop の k8s cluster minimum memory | < 2 GB (より複雑な topology) |

NFR-R で詳細閾値を確定。

---

## 11. Out of Scope (再掲)

`business-logic-model.md` §10 と同じ。
