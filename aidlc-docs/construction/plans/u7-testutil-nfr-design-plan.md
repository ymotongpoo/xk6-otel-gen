# U7 (testutil/generators) — NFR Design Plan

## ユニットコンテキスト

- **Unit ID**: U7
- **パッケージ**: `testutil/generators/`
- **FD**: `aidlc-docs/construction/u7-testutil/functional-design/`
- **NFR-R**: `aidlc-docs/construction/u7-testutil/nfr-requirements/` (NFR-U7-1〜10、PBT-09 compliance)

## NFR Design の焦点

U7 は test-support パッケージのため、典型的な resilience/scalability/security パターンは N/A です。本ステージは以下に集中:

- **Performance patterns** — NFR-U7-6 (1 ms/draw)、NFR-U7-7 (≤ 1 MB) を実現する設計
- **Logical components** — generator 実装パターン (atomic / composed)、options 構造、DAG 構築アルゴリズム、Any 系の degradation 注入手法
- **U7-U1 循環依存の実装パターン** — pre-U1 型骨格をどう書くか、各ジェネレータがどう型を参照するか

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u7-testutil/nfr-design/nfr-design-patterns.md` — パフォーマンス、保守性、API 拡張、test 並列の設計パターン
- [ ] `aidlc-docs/construction/u7-testutil/nfr-design/logical-components.md` — 内部論理コンポーネント (options resolver, DAG builder, degradation injector など) とその責務

---

## 設計確定のための質問

### Question 1: rapid generator の実装スタイル

`pgregory.net/rapid` には複数の generator 構築 API があります:
- `rapid.Custom[T](func(t *rapid.T) T)` — フル手動制御
- `rapid.Map[A,B](g, transform)` / `rapid.FlatMap` — combinator スタイル
- `rapid.SliceOfN`, `rapid.IntRange`, `rapid.SampledFrom` — プリミティブ

U7 のスタイルは?

A) **Custom 中心 + プリミティブ補助** — 複雑な型は `rapid.Custom` で明示的に組み立て、プリミティブ (Int/String/Bool) は標準 helper を利用。デバッグしやすく、shrinker も自然に動く (推奨)

B) **Combinator (Map/FlatMap) 中心** — 関数型スタイルで composable、コード量少。ただし shrink 動作のメンタルモデルが難しい

C) **混合** — シンプルなものは Map、複雑なものは Custom。判断ガイドラインを設けない (一貫性が低下するリスク)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 2: functional options の実装パターン

Q4 of FD で functional options 採用は決定。具体的なシグネチャパターンは?

A) **`type Option func(*options)` (unexported struct)** — Go コミュニティ標準パターン、kubernetes/grpc-go 等多数の OSS で採用 (推奨)

B) **`type Option interface { apply(*options) }`** — interface ベース、より型安全だがボイラープレート多

C) **可変引数の中身を `any` の slice で受ける** — flexibility 最大、type-safety 喪失。非推奨

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 3: DAG 順序確保アルゴリズム (`ValidSchema` 内部)

`ValidSchema()` は DAG を保証する必要があります (R-STR-4)。実装戦略は?

A) **生成時に topological order を作り、上位ノードから下位ノードへのみエッジを張る** — 構築時に DAG を強制、後検証不要 (推奨、性能と決定論性が高い)

B) **任意にエッジを張り、最後に cycle detection → invalid 例を破棄 (rapid.Filter)** — シンプルだが Filter のコストが高い (NFR-U7-6 達成困難)

C) **任意にエッジを張り、cycle 検出時にエッジを取り除く修復ロジック** — A と B の中間

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 4: `AnySchema` の degradation 注入パターン

`AnySchema` は invalid な schema を意図的に作る必要があります (R-STR-3〜6 違反パターン)。

A) **`ValidSchema` 出力に "壊し" を確率的に注入** — 50% は valid、50% は valid + 1 箇所だけ破壊。`mutate` という内部関数で実装 (推奨、Filter 不要、性能良)

B) **完全に独立な構築ロジック** — `ValidSchema` と並列の `AnySchema` を別実装

C) **rapid.OneOf で valid と invalid のサブジェネレータを選択** — どちらかを丸ごとサンプル

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 5: pre-U1 型骨格の場所と命名

U7-U1 循環依存の解決として「pre-U1 型骨格を U7 CG で書く」方針が確定 (NFR-R `tech-stack-decisions.md` §6)。これらの型ファイルはどこに置きますか?

A) **正式に `topology/` パッケージとして書く** — Application Design の `component-methods.md` に沿った型定義 (struct, enum, interface のみ、メソッド未実装)。U1 CG で同じファイルにメソッド追加 (推奨、構造的に最終形と一致)

B) **`testutil/generators/internal_types.go` などに置く** — 一時的、U1 CG 時に migrate する手間あり

C) **`topology/skeleton.go` のような stub ファイル** — 完成後はリネーム or 削除

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 6: ベンチマークの粒度

NFR-U7-6 (1ms/draw) を検証するベンチマークは何個書きますか?

A) **`BenchmarkValidSchemaDraw` のみ** — トップレベル代表だけ、シンプル (推奨、初期は十分)

B) **`Benchmark{ValidSchema,ValidService,ValidEdge,...}` の generator 毎** — 詳細プロファイル

C) **シナリオ別 (MaxServices=1/10/100)** — オプション空間も探索

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 7: メモリ予算の検証 (NFR-U7-7)

「`ValidSchema()` 1 出力 ≤ 1 MB」を CI で自動チェックしますか?

A) **暗黙 (NFR としては明記、CI 自動チェックなし)** — 開発者が必要時に `pprof` で確認 (推奨、初期は過剰検証を避ける)

B) **`runtime.ReadMemStats` を使った benchmark helper を 1 件用意し閾値判定** — 明示的保証

C) **runtime/memory tracking allocator を導入する** — 過剰投資

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 8: コンテキスト伝搬と timeout

rapid.Check は内部で goroutine を回しますが、テスト個別に context 経由のタイムアウトを設けますか?

A) **設けない** — rapid 自身に timeout 機構なし、Go test の `-timeout` フラグで全体制御 (推奨、シンプル)

B) **個別テストで `t.Deadline()` ベースの check 内タイムアウト** — テスト個別に意識的に書く

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 9: ドキュメントの実例 (GoDoc + Example function)

GoDoc に Example function (`func ExampleValidSchema()`) を含めますか?

A) **トップレベル generator (`ValidSchema`, `ValidService`, `AnySchema`) に Example を 1 件ずつ** — `go doc -src` で確認可能、`pkg.go.dev` で表示 (推奨、ユーザビリティ向上)

B) **GoDoc コメントだけ、Example はなし** — 軽量、最小限

C) **全 public 関数に Example** — 過剰、メンテ負担大

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 10: API 拡張時の互換性チェック (NFR-U7-9 補足)

NFR-U7-9 (SemVer 厳守 post-v1) のための運用パターン:

A) **GoDoc に `// Deprecated:` コメント運用のみ。tooling は導入しない** — シンプル、人間レビュー依存 (推奨、初期スコープ)

B) **`gorelease` を CI に組み込み API diff を自動検出** — Build and Test ステージで実装

C) **`apidiff` (golang.org/x/exp/cmd/apidiff) を pre-commit hook に** — 最厳格

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 2 つの NFR Design アーティファクトを生成して承認ゲートへ進みます。
