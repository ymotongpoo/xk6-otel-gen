# U1 (topology) — NFR Requirements Plan

## ユニットコンテキスト

- **Unit ID**: U1
- **パッケージ**: `topology/`
- **Functional Design**: `aidlc-docs/construction/u1-topology/functional-design/` を前提
- **位置づけ**: Domain layer。プロジェクト全体の型供給元 + Parse/Validate/Marshal/Equal/ApplyFaults/JSON Schema 提供

## NFR スコープ

U1 は **ライブラリパッケージ** で:
- ネットワーク I/O なし → セキュリティ NFR の多くは N/A
- 外部システム依存なし → 可用性/SLA は N/A
- 入力データ (YAML) の信頼性 → 入力検証は本質 (Validate で対応済み)

中心となる NFR:
- **性能**: Parse 10ms / 典型 YAML 規模 (FD Q11=A 確定)
- **メモリ**: Parse 後の Schema サイズ目安
- **エラー処理**: ParseError / ValidationError 型階層 (FD `domain-entities.md` §5)
- **保守性**: yaml.v3 / jsonschema/v5 バージョン方針、後方互換性
- **可観測性**: Parse / Validate のログ出力方針
- **不変性 (immutability)**: Parse 後の Schema を read-only 扱いとする規約

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u1-topology/nfr-requirements/nfr-requirements.md` — U1 に適用される NFR 一覧 (適用 / N/A)
- [ ] `aidlc-docs/construction/u1-topology/nfr-requirements/tech-stack-decisions.md` — yaml.v3 / jsonschema/v5 (test-only) のバージョン方針、Go バージョン要件

---

## 設計確定のための質問

### Question 1: `gopkg.in/yaml.v3` のバージョン方針

YAML パーサとして `yaml.v3` を採用 (U7 でも types.go の yaml タグ用に登録済み)。バージョン方針は?

A) **`yaml.v3` の最新 stable に追従** — `go get -u gopkg.in/yaml.v3`、minimum version を go.mod に記載 (推奨、外部 OSS 標準)

B) **特定 minor で固定** — dependabot で見直し

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 2: `github.com/santhosh-tekuri/jsonschema/v5` の扱い

TP-U1-8 (JSON Schema round-trip) で必要なテスト依存。

A) **テスト依存として `go.mod` の indirect か require セクションに追加** — `_test.go` ファイルからのみ import (推奨、本体ビルドに影響なし)

B) **本体依存として追加し、`(*Schema).ValidateJSON([]byte)` のような API を公開** — Schema が JSON 検証機能を提供 (機能拡張だがスコープ拡大)

C) **依存追加を見送り、TP-U1-8 を別の方法で実現** — 例えば static JSON Schema を gold file 化し、それを Parse で読めるか確認するだけにする (依存最小化)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 3: Parse 性能の追加目標

FD Q11=A で「典型 YAML (10 services / 30 operations / 50 edges) ≤ 10 ms」と確定。追加の目標は?

A) **典型 YAML のみ目標、それ以上はモニタ対象** — `BenchmarkParse` で 1 件、退化したら検知 (推奨、過剰検証回避)

B) A に加えて **`BenchmarkValidate`、`BenchmarkApplyFaults` も** ベンチを書く — それぞれ <1 ms 目標

C) Aの代わりに **大規模 YAML (100 svc / 500 op) も目標値設定** — < 100 ms

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 4: メモリ消費目標

Parse 後の `*Schema` 1 件のメモリ目安は?

A) **典型 YAML で 1 MB 以下** — 暗黙、CI 自動チェックなし (U7 NFR-U7-7 の方針と整合、推奨)

B) **明示的に runtime.ReadMemStats でベンチ** — bench_test.go に追加

C) **意識しない** — 小さい入力前提

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 5: Parse / Validate のログ出力方針

Parse / Validate の内部処理 (debug 用):

A) **ライブラリ内ではログ出力なし** — エラーは戻り値のみ。利用側 (k6 module) がログ管理 (推奨、`log` パッケージへの依存を避ける、Go ライブラリ標準)

B) **`log/slog` でログを残す** — `slog.Default()` 経由でデバッグ可能

C) **テスト時のみログ** — `-v` フラグで詳細出力

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 6: 不変性 (immutability) の強制方法

FD `business-rules.md` §11 で「Parse 後の Schema は read-only convention」と記載。これを Go の型レベルで強制するか?

A) **規約 (convention) のみ** — GoDoc に明記、Code Review で守る。実装は変更可能 (Go の immutability 機構なし、推奨)

B) **getter のみ公開、フィールド unexport** — `s.Services` でなく `s.GetServices()` 等を強制。利用側コードが煩雑になるが安全

C) **deep copy API を提供** — `(*Schema).Clone() *Schema` で利用側が安全な copy を取得可能、元 Schema は read-only convention

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 7: 並行アクセス safety

Parse 後の `*Schema` を複数 goroutine から同時参照するシナリオ (k6 が複数 VU で同時に Journey Engine を起動):

A) **完全 thread-safe (immutable 規約に依存)** — Schema は Parse 後 immutable、Read-only アクセスは並行安全。Write は規約違反 (推奨)

B) **`sync.RWMutex` でラップ** — Schema 構造体に内蔵、`s.RLock() / s.RUnlock()` 強制 (オーバーキル)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 8: Go バージョン要件

`errors.Join` を Phase 2/3 で使う計画。Go 最低バージョンは?

A) **Go 1.21+ (`errors.Join` は 1.20、`slog` は 1.21)** — slog 採用可否次第。U7 は 1.24 で動作。推奨: `go 1.21` 以上を go.mod に明記

B) **Go 1.22+** — `range over int` や `slices.Sorted` 等の最新機能を使えるように

C) **U7 が選んだ 1.24 のまま** — U7 の go.mod 設定を踏襲

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 9: コードカバレッジ目標

U7 と同じ 80% を目標とするか?

A) **80% 以上** — 通常の Go OSS 慣例、U7 と整合 (推奨)

B) **90% 以上** — U1 はドメインの中核、より厳しく

C) **目標設定なし** — 必須 TP 8 件 + example-based test で実質的に高カバレッジに到達するはず

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 10: `Lint` API の用途と性能

Lint は CLI ツール (`cmd/xk6-otel-gen-schema/`) から呼ばれる想定 (FD `business-logic-model.md` §2)。性能要件は?

A) **Parse と同じ性能特性** — Parse + 未知キー検出の差分のみ、典型 YAML で 15 ms 以下 (推奨)

B) **目標なし** — Lint は遅くて OK (人間が CLI で 1 回呼ぶだけ)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 11: 後方互換性ポリシー (U1 公開 API)

U7 と同じ SemVer ポリシーを採用?

A) **U7 と同じ — v1.0.0 リリース前は break OK、v1.0.0 以降は SemVer 厳守** (推奨、プロジェクト統一)

B) **U1 は厳格 — 全変更を deprecation period 経由で廃止** — Domain 層なので互換性最優先

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 12: その他の NFR の N/A 一覧

U7 NFR-R Q10=A で「N/A 一覧の明示」を採用済み。U1 でも同様?

A) **明示的に N/A 一覧を nfr-requirements.md に書く** — Audit 性向上、U7 と一貫 (推奨)

B) **暗黙的に省く**

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 2 つの NFR Requirements アーティファクトを生成して承認ゲートへ進みます。
