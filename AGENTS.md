# AGENTS.md — Implementation Agent Contract (xk6-otel-gen)

このファイルは **OpenAI Codex CLI (ヘッドレス自律エージェント)** および **Cursor Composer (対話的編集)** が、本リポジトリで実装作業を行うときに従う共通契約です。Claude Code は本ファイルを編集して構わないが、実装エージェントは原則として本ファイルを編集しないこと。

---

## 1. Project Summary

本プロジェクト **xk6-otel-gen** は、k6 (負荷テストツール) の拡張です。実際のマイクロサービスを構築せずに、YAML で宣言したコンポーネントトポロジーから、擬似的な OpenTelemetry シグナル (Metrics / Logs / Traces) を OTel Semantic Conventions に準拠して合成し、OTLP/gRPC + OTLP/HTTP の両方で送信します。

**言語**: Go、**ビルド**: xk6、**ライセンス**: Apache-2.0、**配布**: GitHub OSS。

詳細仕様は `aidlc-docs/inception/requirements/requirements.md` を Single Source of Truth (SSOT) とします。

---

## 2. Multi-Agent Workflow — Role Boundaries

本リポジトリは AI-DLC ワークフローに従って開発されます。3 種のエージェントが**互いに重ならない役割**で協働します:

| Agent | 担当 | 出力先 |
|---|---|---|
| **Claude Code** | プラン・設計・要件・テスタブルプロパティ識別。**コードは書かない** (例外: 設定/スキャフォルドファイルのみ) | `aidlc-docs/**` の全 Markdown ドキュメント |
| **OpenAI Codex CLI** (gpt-5.5 xhigh) | **長時間・自律的なバッチ実装**。1 ユニット丸ごとの Go コード/テストコード生成、PBT 実装、grunt work | リポジトリのソースツリー (Go パッケージ、テスト、サンプル) |
| **Cursor Composer 2.5** | **対話的編集**。Codex が出力したコードの微調整、レビュー反映、リファクタリング、デバッグ | リポジトリのソースツリー (差分単位の編集) |

### 厳守事項 (実装エージェント向け)

1. **設計ドキュメントの改変禁止**: `aidlc-docs/**` の Markdown ファイルを編集してはならない。要件・設計の変更が必要と判断した場合、`aidlc-docs/audit.md` の末尾に **追記のみで** "Implementation-time Question" として記録し、Claude Code のセッションでの解決を待つ。
2. **スコープを広げない**: 担当ユニットの "Code Generation Plan" に明記されたタスクのみを実装する。`aidlc-docs/construction/<unit-name>/code/code-generation-plan.md` を読んでから着手すること。
3. **書く場所**: アプリケーションコードはワークスペースルート (例: `cmd/`, `internal/`, `pkg/`, `examples/`)。`aidlc-docs/` 配下にコードを置かないこと。
4. **PBT は必須**: Property-Based Testing 拡張が **Full enforcement** で有効化されているため、対象パッケージには PBT が必ず含まれる。`aidlc-docs/construction/<unit-name>/functional-design/` の "Testable Properties" セクションを参照し、`pgregory.net/rapid` を用いて実装する。
5. **失敗時の出力**: 実装が完了しない・テストが通らない場合、`completed` と宣言してはならない。残課題を `TODO(agent):` コメントとしてコード内に明記し、`aidlc-docs/audit.md` に追記する。

---

## 3. Source of Truth Map

| 知りたいこと | 参照先 |
|---|---|
| **セッション開始時に必ず読む共有メモリ** | `.agent-memory/MEMORY.md` (全エージェント共通の索引) |
| プロジェクト全体の要件 | `aidlc-docs/inception/requirements/requirements.md` |
| 実行プラン (どのユニットが何の対象か) | `aidlc-docs/inception/plans/execution-plan.md` |
| アプリケーション設計 (コンポーネント境界、I/F) | `aidlc-docs/inception/application-design/**` |
| ユニット一覧と分担 | `aidlc-docs/inception/units-generation/units.md` |
| 各ユニットの機能設計 + テスタブル特性 | `aidlc-docs/construction/<unit-name>/functional-design/**` |
| 各ユニットの NFR (性能・テスト戦略・依存) | `aidlc-docs/construction/<unit-name>/nfr-requirements/**`, `nfr-design/**` |
| **各ユニットの実装計画 (これに従って実装)** | `aidlc-docs/construction/<unit-name>/code/code-generation-plan.md` |
| プロセスルール (AI-DLC ワークフロー定義) | リポジトリルートの `CLAUDE.md` および `.aidlc-rule-details/**` (下記 §3.1 参照) |
| 監査ログ (全エージェントの作業記録) | `aidlc-docs/audit.md` |

### 3.1 AI-DLC Rule Details — 実装エージェント向け抜粋

リポジトリ内の **`.aidlc-rule-details/`** ディレクトリには AI-DLC ワークフロー全体のルール定義が格納されており、すべてのエージェントが読み取れます。実装エージェント (Codex CLI / Cursor) は、自分の担当範囲に関連する以下のファイルを **必要に応じて参照** してください (全部を読む必要はなく、関連トピックのみ):

| ファイル | 用途 |
|---|---|
| `.aidlc-rule-details/construction/code-generation.md` | コード生成ステージの規約。`code-generation-plan.md` の構造と完了基準を理解するため必読 |
| `.aidlc-rule-details/construction/build-and-test.md` | ビルド・テストの規約。CI/Docker compose/テストハーネス生成時の参照 |
| `.aidlc-rule-details/extensions/testing/property-based/property-based-testing.md` | **PBT 規約 (Full enforcement で本プロジェクトに必須適用)**。テスト実装時に必読 |
| `.aidlc-rule-details/common/error-handling.md` | エラーハンドリングの一般原則 |
| `.aidlc-rule-details/common/content-validation.md` | Markdown 等のコンテンツ検証ルール |

実装エージェントは AI-DLC の **Inception 系ルール** (`workspace-detection.md`, `requirements-analysis.md` 等) を実行する必要はありません — それらは Claude Code の責任範囲です。

### 3.2 Shared Agent Memory

`.agent-memory/` ディレクトリには、全エージェント共通の永続メモリ (ユーザー設定、フィードバック、決定事項) が記録されています。**セッション開始時には必ず `.agent-memory/MEMORY.md` を読み**、関連するエントリを参照してから作業に入ってください。新しい知見を発見した場合は、Claude Code セッションでの追記を依頼するか、`aidlc-docs/audit.md` に "Implementation-time Insight" として追記してください (実装エージェントが直接 `.agent-memory/` を書き換えるのは避ける)。

---

## 4. Build & Test Commands

```bash
# xk6 ビルド (k6 本体と本拡張を静的リンクしてカスタムバイナリ生成)
xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.

# Go モジュール
go mod tidy

# Unit + Property-Based Tests
go test ./...

# Race detector 付き (CI 推奨)
go test -race -count=1 ./...

# 特定ユニットのみ
go test ./topology/...

# PBT のシード固定実行 (再現性確認)
RAPID_SEED=1 go test -run TestProperty ./...

# Integration test (Docker OTel Collector 起動が前提)
docker compose -f test/integration/docker-compose.yaml up -d
go test -tags=integration ./test/integration/...

# Lint (要 golangci-lint)
golangci-lint run
```

---

## 5. Code Conventions

- **Go バージョン**: 直近 stable + 1 つ前の minor (例: 1.23 / 1.22)。go.mod に明記。
- **フォーマット**: `gofmt` / `goimports` 準拠。CI で強制。
- **モジュールパス**: `github.com/ymotongpoo/xk6-otel-gen`
- **パッケージ構成 (Application Design で確定 — 全パッケージをトップレベル公開):**
  - `cmd/xk6-otel-gen-build/` — ヘルパー (任意)
  - `topology/` — YAML スキーマ・パーサ・グラフモデル (公開)
  - `journey/` — Critical User Journey エンジン (公開)
  - `synth/` — Signal Synthesizer (spans, metrics, logs) (公開)
  - `exporter/` — OTLP/gRPC + OTLP/HTTP 送信パイプライン (公開)
  - `k6otelgen/` — k6 JS モジュール (k6 register)
  - `k6output/` — k6 Output モジュール (k6 register)
  - `testutil/generators/` — PBT ドメインジェネレータ (公開)
  - `examples/` — minimal / realistic サンプル
  - `test/integration/` — Integration tests + Docker compose
  - `registry/` (候補) — `k6otelgen` と `k6output` が共有する Pipeline holder。NFR Design で確定
- **命名**: パッケージ名は単数形・小文字・ハイフン無し。エクスポート識別子は GoDoc コメント付き。
- **エラー処理**: `errors.Wrap` 系ではなく `fmt.Errorf("...: %w", err)`。
- **ロギング**: k6 の `modules.VU.InitEnv().Logger` (k6 統合パスから取得可能なロガー) を使う。
- **依存ライブラリ**:
  - OTel SDK: `go.opentelemetry.io/otel` および `go.opentelemetry.io/otel/exporters/otlp/*`
  - OTLP Protobuf: `go.opentelemetry.io/proto/otlp`
  - PBT: `pgregory.net/rapid`
  - YAML: `gopkg.in/yaml.v3`
  - k6 拡張 SDK: `go.k6.io/k6`

---

## 6. PBT (Property-Based Testing) Compliance — MANDATORY

このプロジェクトでは PBT 拡張が **Full enforcement** で有効化されている。実装時に以下を必ず満たすこと:

- **PBT-02 (Round-trip)**: YAML パーサ、OTLP protobuf シリアライザ等のラウンドトリップテストを必須化
- **PBT-03 (Invariants)**: トポロジーグラフの不変条件 (DAG, reachability) と信号合成の不変条件 (1 ジャーニー = 1 trace_id, parent_span_id 連鎖, metric サムの保存則) をテスト
- **PBT-07 (Generator Quality)**: ドメイン特化ジェネレータを `testutil/generators/` に集約し再利用
- **PBT-08 (Shrinking & Repro)**: シード値を CI ログに出力する。`pgregory.net/rapid` のデフォルトを尊重し、shrink を無効化しない
- **PBT-09 (Framework)**: `pgregory.net/rapid` を `go.mod` に明示
- **PBT-10 (Complementary)**: 例ベーステストと PBT を別ファイル/別関数で明示 (例: `topology_test.go` vs `topology_property_test.go`)

詳細は `aidlc-docs/construction/<unit-name>/functional-design/` の "Testable Properties" セクションを参照。

---

## 7. Acceptance Criteria for "Done"

実装エージェントが 1 ユニットの作業を `done` とするためには:

- [ ] `go build ./...` が成功
- [ ] `go test -race ./...` が成功
- [ ] `golangci-lint run` が警告なし (現実的に達成可能な範囲)
- [ ] PBT がそのユニットに含まれている (該当する場合)
- [ ] ユニットに対応する `code-generation-plan.md` のチェックボックスがすべて `[x]`
- [ ] 残課題があれば `TODO(agent):` コメント + `audit.md` への追記
- [ ] 変更ファイルが `aidlc-docs/**` を含まない

---

## 8. Out of Scope (実装エージェントは扱わない)

- 要件・設計ドキュメントの新規作成や書き換え (Claude Code の領域)
- ライセンス変更、メジャーな依存追加 (Apache-2.0 / `go.mod` の主要依存変更には事前合意が必要)
- リリースタグ作成、`v*` タグの push
- Force push、main ブランチへの直接 push (PR ベース推奨)
- 既存の AI-DLC 設定ディレクトリ (`.aidlc-rule-details/`, `.kiro/`, `.claude/`) の改変
