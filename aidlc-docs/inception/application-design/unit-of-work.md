# Unit of Work — xk6-otel-gen

本書は Construction フェーズの per-unit loop の対象となる **ユニット** を確定します。各ユニットは Functional Design / NFR Requirements / NFR Design / Code Generation を 1 ループとして経ます。

---

## ユニット一覧

| Unit ID | 名前 | パッケージ | レイヤ | 由来 (Application Design) |
|---|---|---|---|---|
| **U1** | Topology Schema & Parser | `topology/` | Domain | C1 |
| **U2** | Journey Engine | `journey/` | Application | C2 |
| **U3** | Signal Synthesizer | `synth/` | Application | C3 |
| **U4** | OTLP Exporter Pipeline (+ Pipeline registry) | `exporter/` | Infrastructure | C4 + 共有 Pipeline holder (内部 API) |
| **U5** | k6 JS Module | `k6otelgen/` | Boundary | C5 |
| **U6** | k6 Output Module | `k6output/` | Boundary | C6 |
| **U7** | PBT Test Utilities | `testutil/generators/` | — | 補助 (Q4=A、PBT-07 集約) |
| **U8** | Samples & Distribution | `examples/`, `cmd/`, build config, README, JSON Schema | — | 補助 (Q6=A、Construction 末尾) |

合計 8 ユニット。

---

## コード配置 (Greenfield)

Application Design で確定したトップレベル公開レイアウト:

```text
github.com/ymotongpoo/xk6-otel-gen/
├── topology/                  # U1
├── journey/                   # U2
├── synth/                     # U3 (sub-module は Functional Design で確定)
├── exporter/                  # U4 (registry/Pipeline holder を内部に含む)
├── k6otelgen/                 # U5 (k6 JS module 登録)
├── k6output/                  # U6 (k6 output module 登録)
├── testutil/
│   └── generators/            # U7 (公開 PBT ジェネレータ)
├── examples/                  # U8
│   ├── minimal/               # 3 サービス例
│   └── astroshop/             # 10+ サービス例 (OTel Demo 由来)
├── cmd/
│   └── xk6-otel-gen-schema/   # JSON Schema エクスポートヘルパー (任意)
├── test/
│   └── integration/           # Integration tests + Docker compose (Build and Test ステージで完成)
├── schemas/
│   └── topology.schema.json   # JSON Schema (NFR-6.2)
├── .github/workflows/         # CI (Build and Test ステージで完成)
├── AGENTS.md
├── README.md
├── go.mod (module github.com/ymotongpoo/xk6-otel-gen)
└── go.sum
```

レイヤ規律 (Boundary → Application → Domain / Boundary → Infrastructure) は `internal/` 機構ではなく **依存マトリクス** (`unit-of-work-dependency.md`) と **レビュー** で担保します。

---

## 各ユニットの責務サマリ

### U1 — Topology Schema & Parser (`topology/`)
- YAML スキーマ・パーサ・JSON Schema エクスポート
- Operation を第一級概念とする型 (`Schema`, `Service`, `Operation`, `CallNode`, `Edge`, `Journey`, `Step`, `FaultSpec`, `FaultTarget`)
- 2-pass parse (string ref → resolved `*Service` / `*Operation` / `*Edge` pointer)
- `ServiceID` newtype、`MarshalYAML` (ポインタ → 名前文字列)、`topology.Equal` (識別子ベース等価)
- バリデーション (DAG、参照解決、journey 到達可能性、fault target 実在)
- Fault Overlay の事前構築 (lookup 用辞書)

### U2 — Journey Engine (`journey/`)
- Topology + FaultOverlay から Plan (operation tree) を構築
- Plan の実行制御 (sequential `Calls` + `parallel:` の sync.WaitGroup 並列実行)
- エッジ呼び出し失敗時のリカバリーフロー (fallback chain → on_exhausted)
- 条件付きカスケード障害伝播 (リカバリー枯渇かつ `propagate` 指定時のみ)
- 実時間レイテンシシミュレーション (`time.Sleep`)

### U3 — Signal Synthesizer (`synth/`)
- OTel Semantic Conventions 主要部分準拠の span / metric / log 合成
- Resource 属性の per-service 生成 (`service.name`, `service.instance.id`, `telemetry.sdk.*`)
- HTTP / RPC / Error 属性付与
- Synthesizer interface (Journey Engine がこれを呼ぶ)
- TracerProvider / MeterProvider / LoggerProvider の **interface 注入** (U4 に直接依存しない)

### U4 — OTLP Exporter Pipeline (`exporter/`)
- OTel Go SDK の OTLP/gRPC + OTLP/HTTP exporter を構築・統合
- TracerProvider / MeterProvider / LoggerProvider の生成
- 設定マージ (JS API > env > YAML defaults > built-in)
- **共有 Pipeline holder の内部 API** (Q5=A) — `pkg/k6otelgen` と `pkg/k6output` から同一 Pipeline を取得可能に
- Stats (送信成功/失敗カウント、内部キュー長)
- Graceful shutdown

### U5 — k6 JS Module (`k6otelgen/`)
- k6 JS Module SDK 経由で `k6/x/otel-gen` を登録
- JS 側 API: `load(path)`, `configure(opts)`, `topology.runJourney(name)`
- Process singleton の Topology / FaultOverlay / Pipeline と per-VU Engine / Synthesizer の組み立て

### U6 — k6 Output Module (`k6output/`)
- k6 Output SDK 経由で `--out otel-gen=...` を登録
- **デュアル機能** (Q3=C):
  - (a) 合成シグナル egress — U5 と同一の Pipeline を共有
  - (b) k6 ネイティブメトリクス (http_req_*, vus, iterations 等) を OTLP/Metrics に変換 (`service.name="xk6-otel-gen-runner"`)
- End-of-run summary は責務外 (k6 標準機構)
- Pipeline shutdown のトリガ

### U7 — PBT Test Utilities (`testutil/generators/`)
- `pgregory.net/rapid` を使ったドメインジェネレータ集約 (PBT-07 Generator Quality)
- 各ユニットで再利用される `Service` / `Operation` / `Edge` / `Journey` / `Schema` / `FaultSpec` の構造的に valid な生成器
- 境界値 (空コレクション、最大値、Unicode 文字列) を含む配備
- Construction 開始時に **U1 より前に骨格** を作る — 各ユニットの FD 進行と並走で拡張 (Q4=A)

### U8 — Samples & Distribution
- `examples/minimal/` — 3 サービス minimal 例 + k6 スクリプト + Docker compose (Collector 起動)
- `examples/astroshop/` — OTel Demo 由来 10+ サービス例
- README — xk6 ビルド手順、JS API リファレンス、YAML スキーマリファレンス、セキュリティ告知 (Q10: プリビルドバイナリ提供だが自前ビルド推奨)
- `cmd/xk6-otel-gen-schema/` — JSON Schema エクスポートヘルパー
- ライセンスファイル (Apache-2.0)
- Code Generation でこれら成果物を作成し、Build and Test ステージで CI/release を完成

---

## Definition of Done (per unit) — Q7=A (AGENTS.md §7 を採用)

各ユニットが Construction の per-unit loop を完了したとみなす基準:

- [ ] `go build ./...` が成功
- [ ] `go test -race ./...` が成功
- [ ] `golangci-lint run` が警告なし (現実的に達成可能な範囲)
- [ ] PBT がそのユニットに含まれている (該当する場合、PBT-01〜10 の applicable rule を満たす)
- [ ] そのユニットの `code-generation-plan.md` のチェックボックスがすべて `[x]`
- [ ] 残課題があれば `TODO(agent):` コメント + `audit.md` への追記
- [ ] 変更ファイルが `aidlc-docs/**` を含まない

(AGENTS.md §7 と同一。Integration test pass / 性能テストは Build and Test ステージで一括検証する方針)

---

## Sub-Module 確定方針 (Q9=A)

Units Generation では **トップレベルパッケージ境界のみ** を確定します。例えば `synth/` 内の `attributes/`, `resources/`, `trace/`, `metric/`, `log/` のような sub-package 分割は **各ユニットの Functional Design で決定** します (過剰設計の回避)。

ただし sub-module の存在自体は Application Design で示唆済みで、その存在感を残すために以下のヒントを明示しておきます (確定は Functional Design):

- **U1 topology**: `rawSchema` (非公開) → `Schema` (公開) の 2-pass 解決ロジックは内部関数で。サブパッケージ化は不要の可能性が高い
- **U2 journey**: 実行制御 (`Engine`) と Plan 構築 (`builder`) は内部で分離する余地あり
- **U3 synth**: `attributes` (Semantic Conventions マッピング)、`resources` (Resource 構築)、`trace` / `metric` / `log` (信号別合成) の分離が有力
- **U4 exporter**: shared singleton holder と Pipeline 本体を内部分離する余地あり
- **U5/U6**: k6 SDK との接合層であり、外向き API は最小化

---

## Construction 進行順

Q2=A (依存ボトムアップ) + Q4=A (U7 を先行) + Q6=A (U8 を末尾) + Q3=A (完全逐次)。

```text
U7 (testutil 骨格) → U1 → U4 → U3 → U2 → U5 → U6 → U8
```

詳細な依存マトリクスと推奨ビルド順序の Mermaid 図は [`unit-of-work-dependency.md`](./unit-of-work-dependency.md) を参照。
