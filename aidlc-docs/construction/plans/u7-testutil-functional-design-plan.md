# U7 (testutil/generators) — Functional Design Plan

## ユニットコンテキスト

- **Unit ID**: U7
- **パッケージ**: `testutil/generators/`
- **目的**: PBT (Full enforcement) のための **ドメイン特化ジェネレータ** を集約 (PBT-07 Generator Quality 準拠)
- **使用フレームワーク**: `pgregory.net/rapid` (NFR-4.4)
- **公開度**: public (下流テストツールや他リポジトリのテストでも再利用可)
- **進化モデル**: U7 は **骨格** を最初に作り、U1 → U4 → U3 → U2 → U5 → U6 の各 Functional Design で識別される Testable Properties に応じて拡張される (Q4=A)

## 今回の FD スコープ

**今 (U7 FD)**:
- ジェネレータパッケージの骨格・命名規約・組成パターンの確定
- U1 (topology) の主要型 (`Schema`, `Service`, `Operation`, `CallNode`, `Edge`, `Journey`, `Step`, `FaultSpec`) の初期ジェネレータ設計

**後で (U1〜U6 の各 FD)**:
- 各ユニットの "Testable Properties" 識別時に、必要な追加ジェネレータをこの U7 に追記する変更を含む

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u7-testutil/functional-design/business-logic-model.md` — ジェネレータの組成戦略、命名規約、Valid/Any 区分、依存ジェネレータの再利用
- [ ] `aidlc-docs/construction/u7-testutil/functional-design/business-rules.md` — 各ジェネレータが保証する不変条件 (Valid 系は DAG/参照解決/range/etc.)、PBT-07 への適合
- [ ] `aidlc-docs/construction/u7-testutil/functional-design/domain-entities.md` — 提供する初期ジェネレータの一覧 (型別)、シグネチャ、参考実装スケッチ
- [ ] (Frontend なし、UI なし)

---

## 設計確定のための質問

### Question 1: 初期スコープ — U7 骨格にどこまで詰めるか

U7 は他ユニットの FD と並走で拡張されますが、**最初のリリース** にどこまで含めますか?

A) **最小骨格のみ** — パッケージ作成と `Schema()` / `Service()` の 2 つだけ。U1 FD と同時並行で他を追加 (推奨、過剰先取り回避)

B) **U1 の主要型すべて** — `Schema()` / `Service()` / `Operation()` / `CallNode()` / `Edge()` / `Journey()` / `Step()` / `FaultSpec()` をまとめて初期実装

C) **U1〜U4 想定ジェネレータすべて** — Plan/Outcome/MetricInput/SpanInput など Application Design 段階で想定済みの型もここで先取り

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 2: Valid 系 vs Any 系の両立

PBT には 2 種類のジェネレータが必要です:
- **Valid 系**: 前提条件を満たす入力を生成 (例: `Schema` が常に Validate を pass する) — 下流ロジックのテスト用
- **Any 系**: 任意の入力を生成 (一部 invalid を含む) — バリデーションロジックのテスト用

どう提供しますか?

A) **両方を提供、命名で区別** — `ValidSchema()` / `AnySchema()` のように prefix で識別 (推奨、意図が明示的)

B) **Valid 系のみ** — 初期は valid だけ用意、invalid テストは個別ハードコード例で対応

C) **Any 系のみ** — 常に "anything" を返し、テスト側で `rapid.Filter` 等で valid 部分を抽出

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 3: ジェネレータの組成戦略

複雑な型 (`Schema`) は複数の小さな型 (`Service`, `Edge`) から構成されます。ジェネレータ間の依存をどう設計しますか?

A) **小さな atomic ジェネレータをエクスポートし、組成は呼び出し側で書く** — `Operation()`, `Service()`, `Schema()` がそれぞれ public、`Schema()` 内部で `Service()` を呼ぶ。テスト側は欲しい粒度で組み合わせ可能 (推奨、柔軟性最大)

B) **トップレベル `Schema()` のみ公開** — 内部の小さなジェネレータは unexported。インターフェースが minimal で覚えやすい

C) **両方** — atomic も公開しつつ、`Schema()` のような便利関数も提供

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 4: ジェネレータのパラメータ化

`Schema()` の生成時に「サービス数の上限」「edge の最大数」等を呼び出し側で制御したいケースがあります。

A) **オプション関数パターン** — `Schema(MaxServices(10), MaxEdgesPerService(5))` のような functional options で柔軟に (推奨、API が拡張に強い)

B) **明示的な引数** — `Schema(maxServices, maxEdges int)` で固定シグネチャ

C) **無し** — レンジは内部で hardcode、特殊ケースは別関数 (`SchemaSmall()`, `SchemaLarge()` 等)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 5: 命名規約

rapid ジェネレータの命名は規約をどう揃えますか?

A) **`<TypeName>()` 関数** — `Schema()`, `Service()`, `ValidSchema()` 等。Go の関数命名と自然 (推奨、`pgregory.net/rapid` 公式 example に近い)

B) **`Gen<TypeName>()` 関数** — `GenSchema()`, `GenService()`。"Gen" prefix で明示

C) **`<TypeName>Gen()` 関数** — `SchemaGen()` のように suffix

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 6: 範囲・分布の現実性

PBT-07 は「ドメイン特化」を要求します。例: `ErrorRate` は `[0.0, 1.0]` の範囲、`Latency` は実用的な分布 (lognormal 等)。

A) **PBT-07 準拠で realistic な範囲を内蔵** — `error_rate ∈ [0.0, 1.0]`, `latency p50 ∈ [1ms, 5s]`, `replicas ∈ [1, 100]` 等の現実的レンジをデフォルトに (推奨)

B) **理論上の範囲** — 数値はあえて広く取り (`replicas: int32 全範囲`等)、Validate のテスト負荷を増やす

C) **両方** — Valid 系は realistic、Any 系は理論上広範囲

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 7: 境界値の自動含有 (PBT-07 推奨)

PBT-07 は境界値 (空コレクション、最大値、Unicode 文字列など) を生成器が「**自然に含むよう設計せよ**」と推奨します。

A) **rapid のシュリンカと既定動作に任せる** — rapid は失敗時に境界値を自動探索するので、明示的な境界値テストは個別の example-based test に任せる (推奨、デフォルトで十分)

B) **境界値を明示的に高頻度生成** — `rapid.Custom` で確率分布を歪め、空コレクション・最大値・Unicode をより多く生成

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 8: 後続ユニット FD からの追加プロセス

U1 以降の各ユニット FD で新しいテスタブル特性が出るたびに、U7 に generator を追加する必要が出てきます。どう扱いますか?

A) **各ユニットの FD ドキュメントに「U7 に必要な追加ジェネレータ」セクションを設け、U7 の code-generation-plan.md に随時追記** — U7 の作業は単発で完結せず、各ユニットの CG とともに段階的に育つ (推奨、トレーサブル)

B) **U7 の FD/CG を 1 回だけ実施し、後の追加は他ユニットの CG 内で直接 testutil/generators/ に追記** — U7 の plan は完了状態を維持

C) **各ユニット完了後に U7 を再 FD する** — U7 を複数回のループにかける

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 3 つの設計アーティファクトを生成して承認ゲートへ進みます。
