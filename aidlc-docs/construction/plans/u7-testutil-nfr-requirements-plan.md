# U7 (testutil/generators) — NFR Requirements Plan

## ユニットコンテキスト

- **Unit ID**: U7
- **パッケージ**: `testutil/generators/`
- **Functional Design**: `aidlc-docs/construction/u7-testutil/functional-design/` を前提
- **位置づけ**: Test Support パッケージ (アプリケーションコードではない)
- **PBT-09 (Framework Selection) の責任を持つ**: 本 NFR Requirements でフレームワーク・バージョン・CI 統合方針を確定する

## NFR スコープ (U7 用)

U7 は Test Support のため、典型的な「性能」「可用性」「セキュリティ」の多くは N/A。代わりに以下が中心:
- **PBT-09 compliance** — フレームワーク選定の確定
- **テスト実行時間予算** — CI 全体での総実行時間に影響
- **シード再現性** — PBT-08 compliance
- **保守性・拡張性** — 各ユニット FD からの incremental 追加に耐える

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u7-testutil/nfr-requirements/nfr-requirements.md` — U7 に適用される NFR 一覧 (適用 / N/A 含む)
- [ ] `aidlc-docs/construction/u7-testutil/nfr-requirements/tech-stack-decisions.md` — `pgregory.net/rapid` 採用根拠、バージョン、CI 統合方針

---

## 設計確定のための質問

### Question 1: `pgregory.net/rapid` のバージョン方針

PBT-09 (NFR-4.4) で `pgregory.net/rapid` 採用は確定済み。バージョン管理方針は?

A) **`rapid` の最新 stable に追従** — `go get -u pgregory.net/rapid` で常時最新。`go.mod` には minimum version (例: `v1.0.0` 以上) を記載 (推奨、外部 OSS 標準)

B) **特定 minor version で固定** — 例えば `v1.x.0` に pin して破壊的変更を避ける、定期的に dependabot で見直し

C) **コミット pin (Go module 機能)** — 特定 commit を SHA で固定し、決定論性を最重視

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 2: PBT のテスト実行時間予算 (per-test)

rapid は既定で 1 テストあたり ~100 iterations 試行します (`rapid.Check`)。CI 全体への影響を考慮し、1 テストあたりの実行時間予算は?

A) **rapid 既定 (~100 iterations)** — チューニングなし、過剰最適化を避ける (推奨、ほとんどの PBT で十分)

B) **テスト関数の `rapid.Check` に `rapid.Iterations(N)` を明示** — テスト個別に iteration 数を制御 (重いテストは少なく、軽いテストは多く)

C) **環境変数で全体制御** — `RAPID_CHECKS=N` で CI/local で切り替え

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 3: CI でのシード戦略 (PBT-08)

PBT-08 は「CI でシードを毎回ログ出力 OR 固定シード」を要求します。本プロジェクトの方針は?

A) **毎回ランダム + シードを CI ログに出力** — 毎ラン異なる入力でカバレッジ最大化、失敗時はログのシード値で再現 (推奨、PBT の本来の効用を活かす)

B) **CI では固定シード (`RAPID_SEED=42` 等)** — 完全に決定論的、CI flakiness の懸念を最小化

C) **両方走らせる** — メイン CI は random、別 nightly job で fixed seed (合計コスト増)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 4: U7 自体のテスト並列実行 (`t.Parallel()`)

PBT は per-test goroutine だが、複数の PBT 関数を同時並列で走らせると CPU 競合する可能性あり。

A) **`t.Parallel()` を付ける** — Go test の並列実行に任せ、CPU を使い切る (推奨、ローカル/CI とも高速化)

B) **付けない** — sequential 実行で結果の決定論性を最優先

C) **PBT 関数だけ並列、example-based test は逐次** — 混合方針

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 5: U7 自身の Code Coverage 目標

U7 はテストコード支援パッケージ。U7 自身のコードカバレッジ目標は?

A) **80% 以上** — 通常の Go OSS プロジェクトの慣例 (推奨)

B) **90% 以上** — テスト品質を最重視 (rapid generator は他テストの基盤なので不具合の影響大)

C) **目標設定なし** — Code coverage は副次指標、機能的に動けば良い

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 6: ジェネレータの drawing コストの上限

`ValidSchema(MaxServices(10), MaxOpsPerService(5), MaxCallsPerOp(5))` を 1 回 draw するのに目安となる時間上限は?

A) **1 ms 以下 / draw** — rapid の通常の draw コストレベル、性能ボトルネックにならない (推奨)

B) **10 ms 以下 / draw** — 多少の余裕を持つ、複雑なジェネレータでもこの範囲

C) **目標値なし** — 動けば良い、明らかに遅い場合だけ後で最適化

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 7: メモリ消費 (drawn schema のサイズ)

1 つの `ValidSchema()` 出力は最大でどれくらいのメモリを消費するか目安を決めますか?

A) **デフォルト範囲では 1 MB 以下** — `MaxServices=10, MaxOpsPerService=5, MaxCallsPerOp=5` で 250 個程度の Operation + 200 個程度の Edge、十分小さい (推奨)

B) **明示的に制限する仕組みを入れる** — `MaxMemoryBytes(N)` のような option で抑制

C) **特に意識しない** — schema は小さいので問題にならない想定

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 8: 並行アクセス safety

U7 ジェネレータは複数 goroutine から同時に呼ばれる可能性があります (Go test の並列実行や、別ユニットの並列テスト)。

A) **完全に thread-safe (グローバル状態なし)** — 各 draw は独立、生成器はリエントラント (推奨、rapid の標準動作)

B) **`sync.Pool` でアロケーション再利用** — 性能優先

C) **テスト 1 個ずつ実行する前提でシングルスレッド可** — 並行を想定しない

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 9: 後方互換性ポリシー (U7 公開 API)

U7 は public パッケージ。将来 generator API が変わると下流テストが壊れる可能性があります。

A) **メジャーバージョン (v1) リリース前は break OK、v1.0.0 以降は SemVer 厳守** — 通常の Go OSS パターン (推奨)

B) **どんな変更も deprecation period (≥ 2 リリース) を経て廃止** — 厳格

C) **API は破壊変更を躊躇しない** — 内部 lib 扱い、外部利用は推奨しない

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 10: その他の NFR の適用範囲

セキュリティ・可用性・国際化など、典型的 NFR で U7 に **適用しないもの** を明示しますか?

A) **明示的に N/A 一覧を nfr-requirements.md に書く** — Audit 性向上 (推奨、PBT 拡張の compliance 文書化と整合)

B) **暗黙的に省く** — 適用するものだけ書けば十分

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 2 つの NFR Requirements アーティファクトを生成して承認ゲートへ進みます。
