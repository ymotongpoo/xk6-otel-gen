# U1 topology — Tech Stack Decisions

本書は U1 で採用する技術スタック・バージョン方針・依存追加の判断を確定する。

---

## 1. YAML パーサ — `gopkg.in/yaml.v3`

| 項目 | 内容 |
|---|---|
| **モジュールパス** | `gopkg.in/yaml.v3` |
| **採用理由** | (1) Go コミュニティで広く採用、(2) YAML 1.2 仕様準拠、(3) `Marshaler` / `Unmarshaler` interface でカスタム encode/decode 可能 (本 unit の `MarshalYAML` で利用)、(4) `KnownFields(true)` で strict mode (Lint で使用) (5) U7 が既に types.go の struct tag で使用中 |
| **ライセンス** | Apache License 2.0 (本プロジェクトと互換) |
| **代替候補** | `sigs.k8s.io/yaml` (JSON 経由の YAML decode、サブセット制限あり) は不採用 — yaml.v3 の方が機能完全 |

### バージョン方針 (Q1=A)

| 項目 | 内容 |
|---|---|
| **戦略** | 最新 stable に追従 |
| **`go.mod` 表現** | minimum version (例: `gopkg.in/yaml.v3 v3.0.1` 以上) |
| **アップデート** | (a) ローカル `go get -u gopkg.in/yaml.v3`、(b) CI で dependabot 週次自動 PR (Build and Test ステージで設定) |
| **採用バージョンの確定タイミング** | U1 Code Generation 開始時に `go get gopkg.in/yaml.v3@latest` の結果を `go.mod` に記録 |

---

## 2. JSON Schema バリデータ — `github.com/santhosh-tekuri/jsonschema/v5` (test-only)

| 項目 | 内容 |
|---|---|
| **モジュールパス** | `github.com/santhosh-tekuri/jsonschema/v5` |
| **採用理由** | (1) JSON Schema Draft 2020-12 サポート (本 unit が export する仕様と一致)、(2) Go コミュニティで広く採用、(3) パフォーマンス良好 (compile once, validate many)、(4) Apache-2.0 ライセンス |
| **ライセンス** | Apache License 2.0 |
| **代替候補** | `github.com/xeipuuv/gojsonschema` (Draft 04/06/07 のみ、2020-12 非対応のため不採用) |

### スコープ (Q2=A)

| 項目 | 内容 |
|---|---|
| **依存タイプ** | **テスト依存のみ** — `_test.go` ファイルからのみ import (具体的には `topology/jsonschema_roundtrip_test.go`) |
| **本体 API への影響** | なし — `(*Schema).ValidateJSON([]byte) error` 等の API は公開しない (Q2=A) |
| **`go.mod` 表現** | `require` セクションに追加。本体コードからの import がないため、Go は indirect とは扱わない (test files も direct import 扱い) |
| **AGENTS.md §2 との関係** | AGENTS.md は「依存追加は `pgregory.net/rapid` と `gopkg.in/yaml.v3` のみ」と縛っていた。本 unit で **テスト依存に限り 1 件追加** する例外を作る。本 NFR-R で明文化済み — AGENTS.md 自体への更新は **行わない** (テスト依存はクリーンビルドに影響しないため、U1 Code Generation の中で `go.mod` に追加するに留める) |
| **バージョン方針** | 最新 stable 追従 (`v5` 系列内) |

### 代替案を採用しない理由

- **Option B (本体依存 + `ValidateJSON` API 公開)**: スコープ拡大、要件にない API を追加するのは AGENTS.md §2 (no scope creep) と矛盾
- **Option C (依存追加見送り、gold file 比較)**: gold file 化 はテスト品質を下げる (Schema 構造の変更検知が弱い)。PBT-02 round-trip の本質を失う

---

## 3. Go バージョン要件

| 項目 | 内容 |
|---|---|
| **最低バージョン** | **Go 1.25** |
| **理由** | Go 公式サポートポリシー: 最新 stable + 1 つ前の minor のみが Go チームのセキュリティ修正対象。それ未満のバージョンを公式に support しないため、開発者にも 1.25 以上の使用を要求する。U1 NFR-R Q8=A (Go 1.21+) はプロジェクトオーナー判断で 1.25+ に上書きされた (2026-06-09) |
| **`go.mod` の `go` directive** | `go 1.25`。U7 Code Generation で `go 1.24` を設定したため、U1 Code Generation 開始時に `go mod edit -go=1.25` で更新する。`go.mod` 全体の整合性 (toolchain directive 等) も同時に確認 |
| **CI matrix (Build and Test で確定)** | `1.25` および将来 `1.26` などの後続版でテスト。`1.24` 以下はサポート対象外、CI で testing しない |
| **コード上の影響** | 1.25 で安定した全機能を利用可能 (`errors.Join` 1.20+、`log/slog` 1.21+、`range over int` 1.22+、`slices.Sorted` 1.23+、`maps.Collect` 1.23+、その他 1.24/1.25 の追加機能)。本 unit では `errors.Join` を主に使用 (Parse / Validate のエラー集約)、それ以外は実装者裁量 |

---

## 4. Module-level configuration

### `go.mod` (U1 完了時点で想定される内容)

```text
module github.com/ymotongpoo/xk6-otel-gen

go 1.25

require (
    gopkg.in/yaml.v3 v3.x.y                    // U1 本体 (Parse, MarshalYAML, rawSchema struct tags)
    pgregory.net/rapid v1.3.0                  // U7 から継承 (PBT framework)
    github.com/santhosh-tekuri/jsonschema/v5 v5.x.y  // U1 test-only (TP-U1-8)
)
```

注: 実バージョン (`v3.x.y`, `v5.x.y`) は U1 Code Generation 実行時に `go get @latest` で確定。

### Build tags

本 unit では build tags を使わない。すべてのファイルは標準ビルドに含まれる。

---

## 5. ファイル構成 (FD `domain-entities.md` §3 を再掲)

```text
topology/
├── doc.go                       # U7 scaffold (U1 で内容更新)
├── enums.go                     # U7 scaffold (変更なし)
├── types.go                     # U7 scaffold (変更なし)
├── stubs.go                     # U7 scaffold (U1 で削除)
├── raw.go                       # NEW: rawSchema / rawService / rawOperation / rawCallNode (Parse 内部型)
├── parse.go                     # NEW: Parse / ParseFile / decodeRaw / buildSchema / resolveReferences
├── validate.go                  # NEW: Validate + validateXxx ヘルパー群 (R-STR + D-* 検証)
├── marshal.go                   # NEW: (*Schema).MarshalYAML
├── equal.go                     # NEW: Equal + equalXxx ヘルパー
├── faults.go                    # NEW: (*Schema).ApplyFaults + FaultOverlay の lookup メソッド
├── jsonschema.go                # NEW: (*Schema).ExportJSONSchema (//go:embed)
├── jsonschema/
│   └── topology.schema.json     # NEW: JSON Schema Draft 2020-12 テンプレート
├── lint.go                      # NEW: Lint / LintIssue / LintSeverity
├── errors.go                    # NEW: *ParseError / *ValidationError
├── doc_test.go                  # NEW: Example functions
├── parse_test.go                # NEW: example-based for Parse
├── parse_roundtrip_test.go      # NEW: TP-U1-1
├── parse_pointers_test.go       # NEW: TP-U1-2
├── parse_consistency_test.go    # NEW: TP-U1-3
├── validate_dag_test.go         # NEW: TP-U1-4
├── validate_idempotent_test.go  # NEW: TP-U1-6
├── validate_test.go             # NEW: example-based for Validate (R-STR + D-*)
├── applyfaults_test.go          # NEW: TP-U1-5 + TP-U1-7
├── jsonschema_roundtrip_test.go # NEW: TP-U1-8 (jsonschema/v5 利用)
├── marshal_test.go              # NEW: example-based for MarshalYAML
├── equal_test.go                # NEW: example-based for Equal
└── bench_test.go                # NEW: BenchmarkParse + BenchmarkLint (optional)
```

---

## 6. CI / Build 統合 (U1 単体での要求)

| 項目 | 内容 |
|---|---|
| **test target** | `go test ./topology/...` |
| **coverage report** | `go test -cover ./topology/...` |
| **race detector** | `go test -race ./topology/...` |
| **lint** | `golangci-lint run ./topology/...` |
| **bench (任意)** | `go test -bench=. ./topology/...` |
| **dep listing** | `go list -deps ./topology/...` に `log` が含まれないこと (NFR-U1-4) |

これらの CI ワークフローへの組み込みは U8 (Build and Test) で一括設計。

---

## 7. U7-U1 関係の最終確定

U7 が scaffold した topology 型骨格 (`doc.go`, `enums.go`, `types.go`, `stubs.go`) に対し、U1 は:

1. **`doc.go`** — 更新 (immutability 規約を明記、AUTOGEN-MARKER-U1 削除)
2. **`enums.go`** — 変更なし (型定義のみ、メソッドなし)
3. **`types.go`** — 変更なし (struct + field 定義は最終仕様、Application Design 通り)
4. **`stubs.go`** — **削除**。各メソッド (`Parse`, `Validate`, `MarshalYAML`, ...) の実装は新規ファイル (`parse.go` 等) に分散
5. **新規ファイル** — §5 のリスト通り作成

U7 がテストで `t.Skip("U1: topology.Validate not implemented yet")` していた `TestValidSchema_ValidatePlaceholder` (`testutil/generators/schema_test.go`) は U1 Code Generation 完了後に `t.Skip` を外して有効化する。これは U1 の DoD に含まれる。

---

## 8. メンテナンス・進化

### バージョン更新
- `gopkg.in/yaml.v3` / `pgregory.net/rapid` / `jsonschema/v5` のリリースを dependabot で追跡 (U8 で `.github/dependabot.yml` を設定)
- Breaking change が来た場合: changelog 確認 → 影響範囲評価 → 必要なら U1 を含む全 unit の依存更新を一括 PR で

### Deprecation (U1 公開 API)
- 削除予定の関数 / 型は `// Deprecated: use X instead.` GoDoc コメント
- 1 minor version (本プロジェクトのリリース基準で) の猶予期間
- v1.0.0 リリース前は猶予なしの削除も許容 (NFR-U1-10)

### 拡張時のチェックリスト
- 新規 public 関数追加 → patch リリース OK
- 既存 public 関数のシグネチャ変更 → major version up が必要 (v1.0.0 以降)
- yaml タグの追加 → patch OK (既存 YAML との後方互換性が保たれる限り)
- yaml タグの **変更 / 削除** → major version up (既存 YAML を壊す)

---

## 9. PBT-09 Compliance Statement (再確認)

PBT-09 (Framework Selection) は U7 NFR-R で既に compliant 状態。U1 はそれを継承し、追加の framework は採用しない。`jsonschema/v5` は **JSON Schema バリデータ** であり、PBT framework ではない (test-only 依存だが PBT カテゴリには該当しない)。

---

## 10. 採用しなかった代替案

| 代替案 | 不採用理由 |
|---|---|
| `sigs.k8s.io/yaml` | JSON 経由の YAML 処理、`Marshaler` interface の柔軟性で yaml.v3 に劣る |
| `github.com/xeipuuv/gojsonschema` | Draft 04/06/07 のみ、本 unit が export する 2020-12 非対応 |
| 自前 YAML パーサ | 巨大な実装コスト、yaml.v3 で十分 |
| JSON Schema export 機能を **省略** | NFR-6.2 違反、エディタ補完サポートを失う |
| `*Schema` を Clone API で deep copy 強制 | Q6=A により convention のみで十分、Clone は過剰投資 |
