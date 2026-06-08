# U7 testutil/generators — NFR Requirements

本書は U7 に **適用される** NFR と **N/A の NFR** を、根拠と共に列挙する (Q10=A、明示一覧採用)。プロジェクト全体の NFR (`requirements.md` §4) との対応を traceability 表で示す。

---

## 1. U7 に適用される NFR

### NFR-U7-1: PBT フレームワークの確定 (PBT-09 compliance)

| 項目 | 内容 |
|---|---|
| **要件** | Property-Based Testing フレームワークとして `pgregory.net/rapid` を採用し、`go.mod` に依存として明示する |
| **根拠** | NFR-4.4 (プロジェクト全体), PBT-09 (Framework Selection) |
| **検証方法** | `go.mod` に `pgregory.net/rapid` エントリ存在 + `go list -m all` で確認 |
| **DoD への反映** | U7 Code Generation 完了時にこの依存が `go.mod` に登録されていること |

### NFR-U7-2: シード再現性 (PBT-08 compliance)

| 項目 | 内容 |
|---|---|
| **要件** | CI 実行時に `RAPID_SEED` または rapid が出力する seed 情報を **毎回ログに記録**。失敗時は同じ seed で確実に再現可能 |
| **方針** (Q3=A) | random seed + CI ログ出力。固定 seed は使わない (PBT の入力多様性を最大化) |
| **検証方法** | CI ログを抜き打ちで確認 (Build and Test ステージで CI 設計時に検証スクリプト追加) |
| **DoD への反映** | (本 NFR は U8 / Build and Test ステージで CI 側に組み込む。U7 単体では `rapid` のデフォルト動作を妨げない設計だけが責務) |

### NFR-U7-3: テスト実行時間予算 (per-test)

| 項目 | 内容 |
|---|---|
| **要件** | 各 PBT 関数 (`rapid.Check`) は **rapid 既定の ~100 iterations** で実行 (Q2=A) |
| **理由** | プロジェクト規模で 100 iterations が十分なカバレッジ、過剰最適化を避ける |
| **オーバーライド** | テスト個別に `rapid.Iterations(N)` を付ける場合は、根拠コメント必須 |
| **検証方法** | CI のテスト時間が U7 単体で 60 秒以内に収まること (NFR Design でランタイム budget を確定) |

### NFR-U7-4: テスト並列実行

| 項目 | 内容 |
|---|---|
| **要件** | U7 自身のテスト関数は `t.Parallel()` を付与し、Go test の並列実行に乗せる (Q4=A) |
| **前提** | U7 のジェネレータは thread-safe (NFR-U7-7 を満たす) であること |
| **検証方法** | `go test -race ./testutil/generators/...` でレースなく完了 |

### NFR-U7-5: コードカバレッジ

| 項目 | 内容 |
|---|---|
| **要件** | U7 自身のコードカバレッジ **80% 以上** (Q5=A) |
| **理由** | rapid generator は他全ユニットのテスト基盤。不具合が他テストの偽陽性/偽陰性を生む |
| **検証方法** | `go test -cover ./testutil/generators/...` で 80% 以上 |
| **CI 統合** | Build and Test ステージで coverage threshold をフックに |

### NFR-U7-6: ジェネレータの drawing コスト

| 項目 | 内容 |
|---|---|
| **要件** | `ValidSchema(MaxServices(10), MaxOpsPerService(5), MaxCallsPerOp(5)).Draw(t, ...)` の 1 回コール は **1 ms 以下** が目安 (Q6=A) |
| **理由** | 100 iterations × 1ms = 100ms /test、性能ボトルネックにならない |
| **検証方法** | U7 内に簡易ベンチマーク (`BenchmarkValidSchemaDraw`) を 1 件含める。Construction の Code Generation で実装 |
| **オーバーライド** | これを超えた場合は、Functional Design 段階で識別された invariant を残しつつ最適化検討 |

### NFR-U7-7: drawn schema のメモリサイズ

| 項目 | 内容 |
|---|---|
| **要件** | デフォルト範囲 (`MaxServices=10, MaxOpsPerService=5, MaxCallsPerOp=5`) の `ValidSchema()` 出力は **1 MB 以下** (Q7=A) |
| **理由** | 100 iterations 同時保持しても 100 MB 以下、Go test の標準ヒープに収まる |
| **検証方法** | NFR Design で `runtime.ReadMemStats` ベースのヘルパー検討、ベンチマークでサニティチェック |

### NFR-U7-8: 並行 safety

| 項目 | 内容 |
|---|---|
| **要件** | U7 のジェネレータは完全 thread-safe (グローバル状態を持たない) (Q8=A) |
| **理由** | `t.Parallel()` での並列実行 (NFR-U7-4) の前提、他ユニットの並列テストでも安全 |
| **検証方法** | `go test -race ./...` でレースなし |
| **設計制約** | パッケージレベル変数で mutable state を持たない (定数 / `sync.Once` 初期化の immutable オブジェクトは可) |

### NFR-U7-9: 後方互換性ポリシー

| 項目 | 内容 |
|---|---|
| **要件** | プロジェクト全体が v1.0.0 リリースする前は破壊変更 OK、v1.0.0 以降は SemVer 厳守 (Q9=A) |
| **適用範囲** | U7 の public 識別子 (`ValidSchema`, `MaxServices`, ...) |
| **検証方法** | `gorelease` (Go API diff ツール) を Build and Test ステージで使用 (将来) |
| **deprecation pattern** | 削除予定の API は `// Deprecated:` コメント + 1 minor version の猶予 |

### NFR-U7-10: 保守性 (incremental 拡張)

| 項目 | 内容 |
|---|---|
| **要件** | U7 は U1〜U6 各 FD で incremental に拡張される (Q8 of U7 FD)。新規ジェネレータの追加が既存ジェネレータを壊さない設計 |
| **方針** | atomic + composed パターン (FD §3) で疎結合、追加は新規関数の追加が基本 (既存関数の signature 変更は避ける) |
| **検証方法** | 各ユニット FD 後、U7 の既存テストが pass し続けること |

---

## 2. N/A 一覧 (Q10=A — 明示的に N/A の根拠を残す)

### N/A: スケーラビリティ・スループット (一般的な NFR カテゴリ)
- **理由**: U7 はテストコード支援パッケージ。実行時の RPS / 同時接続数等の概念がない。runtime テスト性能の目標は NFR-U7-3 / NFR-U7-6 で個別に扱う。

### N/A: 可用性・SLA・RTO/RPO
- **理由**: U7 はテスト時のみロードされ、production ランタイムには含まれない。可用性目標が定義されない。

### N/A: セキュリティ (認証/認可/データ保護)
- **理由**: U7 は外部ネットワーク通信せず、機密データを扱わない。テスト時のみメモリ内で動作。
- **補足**: プロジェクト全体の Security Baseline 拡張は **オプトアウト** されている (Q15 of Requirements Analysis)。

### N/A: コンプライアンス (GDPR / SOC2 / HIPAA 等)
- **理由**: テストコードのため、エンドユーザーデータを扱わない。

### N/A: 国際化 (i18n) / アクセシビリティ (a11y)
- **理由**: U7 は Go プログラマ向けライブラリ、UI を持たない。

### N/A: モニタリング/アラート/可観測性 (production の意味で)
- **理由**: U7 はテスト時のみ動作。production 観測対象ではない。
- **補足**: テスト失敗時の seed/shrunk-input ログ出力は PBT-08 で扱う (NFR-U7-2)。

### N/A: バックアップ・ディザスタリカバリ
- **理由**: U7 は無状態のテストコード。永続化対象なし。

### N/A: ライセンス遵守 (依存)
- **明示**: `pgregory.net/rapid` のライセンス確認は U8 (Distribution) / Build and Test ステージで一括点検。U7 単体としては transitive license check 不要だが、依存追加時のスナップショット記録のみ義務 (Code Generation で実装)。

---

## 3. プロジェクト全体 NFR との traceability

`unit-of-work-traceability.md` の NFR 表に対する U7 の役割を再掲:

| プロジェクト NFR | U7 の役割 | 対応する U7 NFR |
|---|---|---|
| NFR-4.1 (Unit + Integration テスト) | Supporting — テストインフラ提供 | NFR-U7-5 (coverage), NFR-U7-3 (時間予算) |
| NFR-4.2 (PBT Full 必須適用) | **Primary** — ドメインジェネレータ集約 | NFR-U7-1, NFR-U7-6, NFR-U7-7, NFR-U7-8, NFR-U7-10 |
| NFR-4.3 (CI でシードログ + 再現性) | Supporting — rapid デフォルトを尊重 | NFR-U7-2 |
| NFR-4.4 (フレームワーク = `pgregory.net/rapid`) | **Primary** — go.mod 依存登録 | NFR-U7-1 |

NFR-1 〜 NFR-3, NFR-5, NFR-6 は U7 に Primary 担当なし。U7 は NFR-4 (テスト戦略) に集中したユニット。

---

## 4. PBT 拡張ルール compliance summary

PBT 拡張 (Full enforcement) のうち、本 NFR Requirements ステージで verification 可能なルール:

| ルール | 状態 | 根拠 |
|---|---|---|
| PBT-01 (Property Identification) | Compliant (FD で 6 properties 識別) | U7 FD `business-rules.md` §10 TP-U7-1〜6 |
| PBT-07 (Generator Quality) | Compliant (FD で realistic range 採用) | U7 FD `business-rules.md` §3, §6 |
| PBT-08 (Shrinking & Reproducibility) | Compliant (rapid デフォルト + CI seed log) | NFR-U7-2 |
| PBT-09 (Framework Selection) | **Compliant (本 NFR-R で確定)** | NFR-U7-1; `tech-stack-decisions.md` |
| PBT-02〜06, PBT-10 | N/A (U7 自体は対象ロジックなし、各ユニットで適用) | — |

(PBT-02〜06, PBT-10 の compliance は U1〜U6 の Functional Design / Code Generation で個別検証)

---

## 5. NFR 検証のチェックリスト (Construction 完了時)

U7 Code Generation 完了時に以下を確認:

- [ ] `go.mod` に `pgregory.net/rapid` エントリあり (NFR-U7-1)
- [ ] `go test -race ./testutil/generators/...` で race なし (NFR-U7-4, NFR-U7-8)
- [ ] `go test -cover ./testutil/generators/...` で coverage ≥ 80% (NFR-U7-5)
- [ ] `BenchmarkValidSchemaDraw` (簡易ベンチ) で 1ms/draw を下回る (NFR-U7-6)
- [ ] U7 のテスト関数すべてに `t.Parallel()` (NFR-U7-4)
- [ ] パッケージレベル mutable state なし (NFR-U7-8)
- [ ] public 識別子に GoDoc あり (NFR-U7-9 の前提)
