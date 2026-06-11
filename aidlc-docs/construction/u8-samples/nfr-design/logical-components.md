# U8 Samples & Distribution — Logical Components

本書は U8 deliverable を **論理コンポーネント** (LC) として整理する。各 LC について 責務 / 公開 API / 実装スケッチ / 依存関係 を定義。

参照:
- FD: `aidlc-docs/construction/u8-samples/functional-design/`
- NFR Design Patterns: `nfr-design-patterns.md` (本ディレクトリ内)

---

## コンポーネント一覧

| LC | 名前 | 配置 | 責務 |
|---|---|---|---|
| LC-0 | Project root docs | `README.md`, `LICENSE` | full project doc + Apache-2.0 fulltext |
| LC-1 | cmd Schema Exporter | `cmd/xk6-otel-gen-schema/main.go + main_test.go` | JSON Schema export CLI |
| LC-2 | Examples — Minimal | `examples/minimal/` | 3-tier 古典 topology + k6 script + LGTM-lite k8s stack |
| LC-3 | Examples — Astroshop | `examples/astroshop/` | 18 services OTel Demo 模倣 + k6 scenarios + LGTM-lite k8s stack |
| LC-4 | Examples Test | `test/examples/examples_test.go` | topology Validate-passing test |
| LC-5 | AI Maintenance Skill | `.claude/skills/sync-astroshop/SKILL.md` | OTel Demo upstream review skill |
| LC-6 | CI Config | `.github/dependabot.yml`, `.lychee.toml`, `.goheader.txt`, `.golangci.yml` (delta) | Dependabot monthly bumps + lychee link check + SPDX header enforce |

---

## LC-0: Project Root Docs

### 責務
- `README.md`: 12-section project documentation (TOC + Quick Start + Features + Building + Usage + Topology YAML Ref + Configuration + Examples + Security + Contributing + License + Compatibility)
- `LICENSE`: Apache-2.0 fulltext (SPDX official)

### 公開 API
- なし (markdown / license file)

### 実装スケッチ
NFR Design Patterns §10 参照 (single-file with TOC, GitHub auto-anchor).

### 依存
- なし

---

## LC-1: cmd Schema Exporter

### 責務
- `xk6-otel-gen-schema [-output <path>]` CLI
- topology.ExportJSONSchema 呼び出し
- exit code semantics (0/1/2)

### 公開 API
```go
package main
func main()
```

### 実装スケッチ
NFR Design Patterns §1 参照 (main thin + run testable, run(args, stdout, stderr) int).

### 依存
- `flag`, `fmt`, `io`, `os` (stdlib)
- `github.com/ymotongpoo/xk6-otel-gen/topology`

### Test 構造
`main_test.go`:
- `TestRun_StdoutDefault` — `-output` 未指定 → stdout 出力
- `TestRun_OutputToFile` — `-output schema.json` → file 出力
- `TestRun_FlagParseError` — 不正 flag → exit 2
- `TestRun_FileCreateFailure` — invalid path → exit 1
- `TestRun_TopologyError` — (if mocked) ExportJSONSchema 失敗 → exit 1

Coverage target: ≥ 70% (NFR-U8-12).

---

## LC-2: Examples — Minimal

### 責務
- 3-tier classic topology demo (frontend / backend / database)
- 1 journey (`checkout`) + 1 fault (error_rate=0.05 on edge)
- k6 JS script (3-phase pattern)
- LGTM-lite k8s manifests + kind setup README

### Deliverable files

```text
examples/minimal/
├── README.md                    ← per-example README (7 sections: Description / Prerequisites / Setup / Run / View / Cleanup / Customize)
├── topology.yaml                ← 3-tier topology (~80 lines, with comments)
├── script.js                    ← k6 setup → default → teardown
├── otel-collector-config.yaml   ← 3 pipelines (traces→Tempo / metrics→Prometheus / logs→Loki)
└── k8s/
    ├── README.md
    ├── kustomization.yaml
    ├── namespace.yaml
    ├── collector.yaml           ← Deployment + Service + Kustomize-gen ConfigMap
    ├── tempo.yaml               ← Deployment + Service + inline ConfigMap
    ├── loki.yaml                ← Deployment + Service + inline ConfigMap
    ├── prometheus.yaml          ← Deployment + Service + ConfigMap
    ├── grafana.yaml             ← Deployment + Service (datasources/dashboards via Kustomize-gen)
    ├── datasources.yaml         ← Grafana datasource provisioning (Tempo/Prometheus/Loki)
    └── dashboard-overview.json  ← 3 panels (Tempo/Prometheus/Loki)
```

### 公開 API
- なし (user-facing static content)

### 実装スケッチ
NFR Design Patterns §3, §4 参照.

### 依存
- LGTM-lite container images (NFR-U8-7 monthly bump)
- Kubernetes cluster (kind / minikube / k3d 等、user 環境)
- xk6-built k6 binary (user が build)

---

## LC-3: Examples — Astroshop

### 責務
- OTel Demo (astronomy shop) 18 services を topology YAML で表現
- 5 journeys (browse / search / add-to-cart / checkout / place-order)
- 確率的 fault demos (error_rate / latency_inflation / crash / disconnect)
- weighted scenarios で k6 script
- LGTM-lite k8s manifests (minimal と並行 layout)

### Deliverable files

```text
examples/astroshop/
├── README.md                    ← per-example README (snapshot version reference 含む)
├── topology.yaml                ← 18 services (~500 lines, section comments per Q10=A)
├── script.js                    ← k6 scenarios (browse / checkout exec funcs)
├── otel-collector-config.yaml   ← 同 minimal
└── k8s/                         ← 同 minimal layout (resource limits は astroshop scale 調整)
```

### 公開 API
- なし

### 実装スケッチ
- topology.yaml は NFR Design Patterns §9 の section grouping (Frontend & API / Core commerce / Support / Infrastructure dependencies の 4 groups)
- 各 service に inline description comment
- README の冒頭で "Modeled after OpenTelemetry Demo v<X.Y.Z>" を明記

### 依存
- LC-2 と同じ (LGTM-lite images + Kubernetes + xk6-built k6)
- AI skill (LC-5) で年 1 回 sync review

---

## LC-4: Examples Test

### 責務
- `examples/*/topology.yaml` を CI で Parse + Validate
- examples が常に valid な topology であることを enforce

### 公開 API
- `go test ./test/examples/...` の test functions

### 実装スケッチ
NFR Design Patterns §2 参照 (sub-test per example directory, parallel).

### 依存
- `testing` (stdlib)
- `github.com/ymotongpoo/xk6-otel-gen/topology` (Parse + Validate)
- `os`, `path/filepath`, `io/fs`

---

## LC-5: AI Maintenance Skill

### 責務
- Claude Code 等 AI ツールが OTel Demo upstream review を driving する手順を encode
- `.claude/skills/<name>/SKILL.md` convention に従う

### 公開 API
- skill file 自体が "API"; AI agent が読んで実行

### 実装スケッチ
NFR Design Patterns §7 参照 — frontmatter (name, description) + markdown body (When to use / Out of scope / Steps / Anti-patterns / Output).

### 依存
- Claude Code skill 機構 (`.claude/skills/` 認識)
- 上流 repo (OTel Demo `open-telemetry/opentelemetry-demo`)
- 本 project の `examples/astroshop/topology.yaml`

---

## LC-6: CI Config

### 責務
- `.github/dependabot.yml` — monthly docker + weekly gomod
- `.lychee.toml` — README link check config
- `.goheader.txt` — SPDX header template
- `.golangci.yml` の delta — goheader linter 追加

### 公開 API
- なし (CI / lint configuration)

### 実装スケッチ
NFR Design Patterns §5, §6, §8 参照.

### 依存
- GitHub Dependabot service
- lychee CLI (CI で利用)
- golangci-lint (project 共通)

---

## コンポーネント間依存図

```text
              ┌──────────────────┐
              │ LC-0 README.md   │
              │ + LICENSE        │
              └──────────────────┘

              ┌──────────────────┐
              │ LC-1 cmd schema  │ ──► references topology.ExportJSONSchema
              │ - main()         │
              │ - run()          │
              │ - main_test.go   │
              └──────────────────┘

              ┌──────────────────┐
              │ LC-2 Minimal     │ ──► uses LGTM-lite images, xk6 k6 binary
              │ examples/minimal │
              │   /topology.yaml │
              │   /script.js     │
              │   /k8s/          │
              └──────────────────┘
                       ▲
                       │ similar layout
                       │
              ┌────────┴─────────┐
              │ LC-3 Astroshop   │
              │ examples/        │
              │ astroshop/...    │
              └──────────────────┘
                       ▲
                       │ informs / drives annual update
                       │
              ┌────────┴─────────┐
              │ LC-5 AI Skill    │
              │ .claude/skills/  │
              │   sync-astroshop │
              └──────────────────┘

              ┌──────────────────┐
              │ LC-4 Examples    │ ──► validates LC-2 + LC-3 topology.yaml
              │ Test             │
              │ test/examples/   │
              └──────────────────┘

              ┌──────────────────┐
              │ LC-6 CI Config   │ ──► drives LC-2/3 image bumps, LC-0/2/3 link
              │ .github/         │      checks, LC-1 SPDX header enforce
              │ + dotfiles       │
              └──────────────────┘
```

---

## ビルド時の依存外部パッケージ

| 用途 | パッケージ |
|---|---|
| cmd CLI | `flag`, `fmt`, `io`, `os` (stdlib) |
| cmd local dep | `github.com/ymotongpoo/xk6-otel-gen/topology` |
| examples test | `testing`, `os`, `path/filepath`, `io/fs` (stdlib) |
| examples test local dep | `topology` |

**Excluded**: cobra / urfave/cli (Q8-FD rejected)、Helm (Q7-FD rejected)、Node.js (lychee は Rust binary)、外部 Go module zero。

---

## テストコンポーネント

| テストファイル | LC 対象 | テスト形式 |
|---|---|---|
| `cmd/xk6-otel-gen-schema/main_test.go` | LC-1 | example-based (4-5 cases) |
| `test/examples/examples_test.go` | LC-2, LC-3 | sub-test per example dir |

CI workflow tasks (Build and Test stage で actual 配置):
- `go build ./cmd/...`
- `go test -race ./cmd/...`
- `go test ./test/examples/...`
- `xk6 build --with .`
- `golangci-lint run ./cmd/...`
- `lychee --config .lychee.toml README.md 'examples/**/README.md'`
- `kubectl apply --dry-run=server -k examples/*/k8s/`

---

## まとめ

- **7 logical components** に集約 (cmd 1 + examples 2 + test 1 + skill 1 + CI config 1 + project root docs 1)
- **公開 API は cmd の main() のみ** (extremely minimal Go surface)
- **deliverable は ~3,300 lines** (FD §3 推定)
- 依存関係は **stdlib + topology のみ** (Go side、container images は runtime dep)
- **AI maintenance skill** が成果物の一部 — project の "AI-driven workflow" 自体を deliverable に含める
- **これが U8 NFR Design — 最終 NFR Design アーティファクト** (Construction 全 unit の NFR-D が完了)
