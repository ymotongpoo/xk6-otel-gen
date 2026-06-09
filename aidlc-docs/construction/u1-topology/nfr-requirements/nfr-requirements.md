# U1 topology — NFR Requirements

本書は U1 (`topology/`) に適用される NFR と N/A の NFR を、根拠と共に列挙する。プロジェクト全体の NFR (`requirements.md` §4) との対応は §3 で示す。

---

## 1. U1 に適用される NFR

### NFR-U1-1: Parse 性能 (典型 YAML 規模)

| 項目 | 内容 |
|---|---|
| **要件** | 典型的 YAML (10 services, 30 operations, 50 edges, 5 journeys, 3 faults) を `Parse(io.Reader)` が **10 ms 以下** で完了 |
| **根拠** | FD Q11=A、k6 init time に影響しないため |
| **検証方法** | `BenchmarkParse` (`topology/bench_test.go`) で `b.ReportAllocs()` を有効化、目標 ≤ 10,000,000 ns/op |
| **超過時の対応** | プロファイル後、Phase 2b の参照解決ループ最適化 (`map[ServiceID]*Service` 直接 lookup、append 容量予約) |

### NFR-U1-2: 大規模 YAML はモニタ対象

| 項目 | 内容 |
|---|---|
| **要件** | 100 services / 500 operations / 1000 edges 規模の YAML は **目標値なし**、ただし `BenchmarkParse` の入力スケールを揃えて回帰検知用にベンチを追加 (任意) |
| **根拠** | FD Q11=A、過剰検証回避 |

### NFR-U1-3: メモリ消費 (Parse 出力)

| 項目 | 内容 |
|---|---|
| **要件** | 典型 YAML 1 件分の `*Schema` が **1 MB 以下** |
| **根拠** | Q4=A、U7 の NFR-U7-7 と整合 |
| **検証方法** | 暗黙 (CI 自動チェックなし)、`BenchmarkParse` の `b.ReportAllocs()` 出力を開発者が確認 |

### NFR-U1-4: ライブラリ内ログ出力なし

| 項目 | 内容 |
|---|---|
| **要件** | `topology` パッケージは **ログ出力を行わない** (`log` / `slog` パッケージへの依存なし)。エラーは戻り値のみで表現 (`*ParseError` / `*ValidationError` / `errors.Join`) |
| **根拠** | Q5=A、Go ライブラリ標準 (利用側にログ管理を委ねる)、`k6otelgen` 等の上流が k6 logger 経由で表示 |
| **検証方法** | `go list -deps ./topology/...` の出力に `log/*` が含まれないこと |

### NFR-U1-5: 不変性 (immutability) は規約のみ

| 項目 | 内容 |
|---|---|
| **要件** | Parse 後の `*Schema` は **read-only** として扱う。Schema の field を変更してはならない (Go の型システムでは強制不可、GoDoc に明記) |
| **根拠** | Q6=A、Go の慣習通り。getter 強制や Clone API は過剰 |
| **GoDoc 文言** | `Schema is immutable after Parse. Mutating any field after Parse returns leads to undefined behavior (race conditions in concurrent reads, inconsistent FaultOverlay, etc.).` |
| **検証方法** | コードレビュー時に Schema を mutate するパスがないか確認。Lint カスタムルール (将来) |

### NFR-U1-6: 並行アクセス safety (immutable 前提)

| 項目 | 内容 |
|---|---|
| **要件** | Parse 後の `*Schema` を複数 goroutine から **同時読み取り** することは安全 (immutable 規約に依存)。並行 write は規約違反のため未定義 |
| **根拠** | Q7=A |
| **検証方法** | `go test -race ./topology/...` で race なし。U2 (journey) が複数 VU から同時参照するため特に重要 |

### NFR-U1-7: Go バージョン要件

| 項目 | 内容 |
|---|---|
| **要件** | **Go 1.25 以上** |
| **根拠** | Go 公式サポートポリシー: 最新 stable + 1 つ前の minor のみがサポート対象。プロジェクト全体 NFR-3.2 で 1.25 以上を最低要件と確定。U1 NFR-R Q8=A (Go 1.21+) は古い判断であり、本要件で **上書き** する |
| **`go.mod`** | `go 1.25` を `go.mod` の go directive に明記。U7 が `go 1.24` で初期化したため、U1 Code Generation 開始時に `go mod edit -go=1.25` で更新する |
| **Go 1.25+ で使える機能 (任意採用)** | `errors.Join` (1.20+)、`log/slog` (1.21+、ただし NFR-U1-4 によりライブラリ内では不採用)、`range over int` (1.22+)、`slices.Sorted` (1.23+) など、1.25 以下で導入されたあらゆる安定機能を躊躇なく使ってよい |

### NFR-U1-8: コードカバレッジ

| 項目 | 内容 |
|---|---|
| **要件** | U1 自身のコードカバレッジ **80% 以上** |
| **根拠** | Q9=A、U7 の NFR-U7-5 と統一 |
| **検証方法** | `go test -cover ./topology/...` で 80% 以上 |
| **CI 統合** | Build and Test ステージで threshold gate を設置 |

### NFR-U1-9: Lint API 性能

| 項目 | 内容 |
|---|---|
| **要件** | `Lint(io.Reader)` は典型 YAML で **15 ms 以下** (Parse 10 ms + 未知キー検出のオーバーヘッド分) |
| **根拠** | Q10=A |
| **検証方法** | `BenchmarkLint` (任意、`bench_test.go` に追加) |

### NFR-U1-10: 後方互換性ポリシー

| 項目 | 内容 |
|---|---|
| **要件** | プロジェクト全体が v1.0.0 リリース前は破壊変更 OK、v1.0.0 以降は SemVer 厳守 |
| **根拠** | Q11=A、U7 の NFR-U7-9 と統一 |
| **適用範囲** | `topology` パッケージの public 識別子全て (`Schema`, `Parse`, `Validate`, `Equal`, `Lint`, `ParseError`, `ValidationError`, `FaultOverlay`, ...) |
| **Deprecation pattern** | `// Deprecated:` GoDoc コメント + 1 minor version の猶予 |

---

## 2. N/A 一覧 (Q12=A — 明示)

### N/A: ネットワーク性能 / スループット
- **理由**: U1 はネットワーク I/O を行わない。`Parse` は `io.Reader` から、`MarshalYAML` は構造体から。RPS / 同時接続数の概念なし

### N/A: 可用性・SLA・RTO/RPO
- **理由**: U1 はライブラリパッケージ。production ランタイムサービスではない

### N/A: セキュリティ (認証/認可)
- **理由**: 外部 I/O なし、認証境界を持たない
- **補足**: プロジェクト全体の Security Baseline 拡張はオプトアウト済み (Requirements Q15=B)

### N/A: コンプライアンス (GDPR / SOC2 等)
- **理由**: エンドユーザーデータを扱わない。トポロジー YAML は **合成シナリオ定義** であり、実顧客データは含まない

### N/A: 国際化 (i18n) / アクセシビリティ (a11y)
- **理由**: Go プログラマ向けライブラリ、UI を持たない。エラーメッセージは英語 (Go OSS 標準)

### N/A: モニタリング/アラート/可観測性 (production の意味で)
- **理由**: U1 はライブラリ。本拡張全体の self-observability は U4 / U6 (NFR-5)
- **補足**: U1 自身は `slog` 等の構造化ログ出力もしない (NFR-U1-4)

### N/A: バックアップ・ディザスタリカバリ
- **理由**: 永続化対象なし、`*Schema` はメモリ上のみ

### N/A: 入力サイズの DoS リスク
- **理由**: 利用者が自分の YAML を Parse するシナリオのみ。外部入力ではない。よって `io.LimitReader` などのサイズ制限は導入しない (`io.ReadAll` で全読み込み、Q10=A 確定)

### N/A: 暗号化 (at-rest / in-transit)
- **理由**: 永続化なし、ネットワーク I/O なし

---

## 3. プロジェクト全体 NFR との traceability

`unit-of-work-traceability.md` の NFR 表に対する U1 の役割を再掲:

| プロジェクト NFR | U1 の役割 | 対応する U1 NFR |
|---|---|---|
| NFR-1.x (Performance/Scale) | Supporting (Parse は 1 回 / VU init) | NFR-U1-1, NFR-U1-2, NFR-U1-9 |
| NFR-2.2 (fail fast on config error) | **Primary** | Parse + Validate のエラー報告 (FD `business-rules.md` §3, §7) |
| NFR-3.x (Compatibility) | Supporting | NFR-U1-7 (Go 1.25+) |
| NFR-4.1 (Unit + Integration tests) | Supporting | NFR-U1-8 (coverage) |
| NFR-4.2 (PBT Full) | **Primary** | 8 testable properties TP-U1-1..8 |
| NFR-4.3 (CI seed log) | Supporting | rapid デフォルト挙動を尊重 (U7 の NFR-U7-2 と整合) |
| NFR-6.1 (README + reference) | Supporting | GoDoc 完備 (NFR-U1-10 が前提とする) |
| NFR-6.2 (JSON Schema 公開) | **Primary** | `ExportJSONSchema` 提供 |

---

## 4. PBT 拡張ルール compliance summary

| ルール | 状態 | 根拠 |
|---|---|---|
| PBT-01 (Property Identification) | Compliant | FD `business-rules.md` §10 で 8 properties (TP-U1-1..8) 識別済み |
| PBT-02 (Round-trip) | Compliant (本 unit で実装) | TP-U1-1 (`Equal(Parse(Marshal(s)), s)`)、TP-U1-8 (JSON Schema round-trip) |
| PBT-03 (Invariants) | Compliant (本 unit で実装) | TP-U1-2, TP-U1-3, TP-U1-4, TP-U1-5 |
| PBT-04 (Idempotency) | Compliant (本 unit で実装) | TP-U1-6 (Validate), TP-U1-7 (ApplyFaults) |
| PBT-05 (Oracle) | N/A | 本 unit に reference 実装は存在しない |
| PBT-06 (Stateful) | N/A | Parse / Validate / ApplyFaults は純粋関数、Schema は immutable |
| PBT-07 (Generator Quality) | Inherits from U7 | ValidSchema / AnySchema (既存) + 本 FD で 18 新規 generator を U7 へ依頼 (FD `domain-entities.md` §6) |
| PBT-08 (Shrinking & Reproducibility) | Inherits from U7 | rapid デフォルト + CI seed log |
| PBT-09 (Framework Selection) | Already satisfied | U7 NFR-R で確定済み (`pgregory.net/rapid`) |
| PBT-10 (Complementary) | Compliant | `_test.go` (example-based) と `_property_test.go` / `*_test.go` 内の `rapid.Check` を分離 |

---

## 5. NFR 検証のチェックリスト (Construction 完了時)

U1 Code Generation 完了時に以下を確認:

- [ ] `go build ./...` が成功 (NFR-U1-7)
- [ ] `go test -race -count=1 ./topology/...` で race なし (NFR-U1-6)
- [ ] `go test -cover ./topology/...` で coverage ≥ 80% (NFR-U1-8)
- [ ] `BenchmarkParse` で典型 YAML が ≤ 10 ms (NFR-U1-1)
- [ ] `Lint` の性能を Bench で確認 (NFR-U1-9) — オプション
- [ ] `go list -deps ./topology/...` に `log` / `log/slog` が含まれない (NFR-U1-4)
- [ ] `topology` パッケージの GoDoc に immutability 規約が明記 (NFR-U1-5)
- [ ] すべての public 識別子に GoDoc あり (NFR-U1-10)
- [ ] `go.mod` の go directive は `1.25` 以上 (NFR-U1-7)
- [ ] `gopkg.in/yaml.v3` と `github.com/santhosh-tekuri/jsonschema/v5` (test-only) が `go.mod` に登録 (Tech Stack §1, §2)
- [ ] U7 への 18 generator 追加リクエスト (FD `domain-entities.md` §6) が `code-generation-plan.md` の Phase 計画に組み込まれている
