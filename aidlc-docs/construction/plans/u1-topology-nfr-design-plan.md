# U1 (topology) — NFR Design Plan

## ユニットコンテキスト

- **Unit ID**: U1
- **パッケージ**: `topology/`
- **FD**: `aidlc-docs/construction/u1-topology/functional-design/`
- **NFR-R**: `aidlc-docs/construction/u1-topology/nfr-requirements/` (NFR-U1-1〜10、Go 1.25+)

## NFR Design の焦点

FD で「何をする」「業務ロジック」を決定済み、NFR-R で「何を達成する」「NFR」を決定済み。本 NFR Design では **「どう実装するか」のパターン** を確定する。

中心となる事項:

- **Performance patterns**: Parse 10ms 目標を達成する具体手段 (アロケーション最小化、Map 初期容量、ループ構造)
- **Error aggregation patterns**: `errors.Join` の使い方、`*ParseError` / `*ValidationError` の作り方、エラーパス情報の組み立て
- **MarshalYAML strategy**: rawSchema 経由 vs 各型に Marshaler を実装、どちらが順序保証と round-trip に強いか
- **Validate algorithm choices**: DAG 検証は Kahn's vs DFS、`validateBackPointers` 等のヘルパー設計
- **Immutability enforcement**: convention のみで、GoDoc にどう書くか、defensive copy をどう避けるか
- **Test organization**: テストファイル粒度、fixture 形式、bench fixture

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u1-topology/nfr-design/nfr-design-patterns.md` — Performance / Error / Immutability / Concurrency / API 拡張 / Documentation の各パターン群
- [ ] `aidlc-docs/construction/u1-topology/nfr-design/logical-components.md` — `topology/` 内の論理コンポーネント (LC-0..LC-N) とそれぞれの責務・公開 API・実装スケッチ

---

## 設計確定のための質問

### Question 1: YAML decode の strict / lax 切り替え方法

`Parse` (lax) と `Lint` (strict) で `yaml.Decoder` の挙動を切り替える。実装は?

A) **共通の `decodeRaw(r io.Reader, strict bool) (*rawSchema, error)` を内部関数として 1 本化** — Parse は `strict=false`、Lint は `strict=true`。共通コード経路で重複を避ける (推奨)

B) **完全分離** — `parseDecode` と `lintDecode` の 2 関数。挙動差が明示的だが、重複コード発生

C) **オプション関数** — `decodeRaw(r, decodeOption...)` で柔軟、将来の strict 以外の切り替えにも対応

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 2: `errors.Join` 内の各エラーの型

集約エラー (Phase 2b の参照解決 + Phase 3 の Validate) の各要素の Go 型:

A) **`*ParseError` (Phase 2b) と `*ValidationError` (Phase 3) を別型に保ち、`errors.Join` で混在** — `errors.As(err, &validationErr)` で個別フィルタ可能 (推奨)

B) **すべて同一型 (`*TopologyError`) に統合** — 内部 `Stage` フィールドで識別

C) **plain `error` (= `fmt.Errorf`)** — 構造化は諦め、メッセージ文字列のみ

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 3: MarshalYAML の実装戦略

`(*Schema).MarshalYAML` の実装方針:

A) **Schema 全体で 1 つの `MarshalYAML`** — `*rawSchema` を組み立てて返す。各型 (Service, Operation, etc.) は MarshalYAML を持たない。シンプルだが、再帰的型 (CallNode の variant) の処理が `MarshalYAML` 内で複雑化 (推奨、yaml.v3 標準パターン)

B) **各型に個別の MarshalYAML を実装** — `*Service`, `*Operation`, `*Edge`, `*CallNode`, `*Journey`, ... それぞれが自前で `MarshalYAML`。yaml.v3 が再帰的に呼ぶ

C) **ハイブリッド** — Schema は raw 経由、CallNode の variant 部分のみ個別実装

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 4: DAG 検証アルゴリズム

`validateDAG(s *Schema)` の実装:

A) **Kahn's algorithm (topological sort)** — 入次数 0 ノードから BFS、全 node を訪問できれば DAG。シンプル、循環があるとどのノードが含まれているか報告しやすい (推奨)

B) **DFS-based cycle detection** — 各 node から DFS、訪問中の node に再び到達したら循環。Kahn より定数倍高速 (差は実質ない、O(V+E) 同等)

C) **Tarjan's SCC** — 強連結成分を計算、SCC サイズ > 1 なら循環。一般的すぎる

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 5: Validate の検証順序

R-STR-1..8 + D-1..D-14 を `validate.go` でチェックするときの順序:

A) **構造的 (R-STR) → ドメイン (D) の順序固定** — 構造エラーが多いと domain check は意味薄、早期に R-STR を出す (推奨)

B) **すべてのエラーを全部集めるため順序は不問** — Validate 内部で 22 種類のチェックを **並列実行** (goroutine 不要、単純な順次関数呼び出しでもよい)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 6: デフォルト値の適用箇所

Q3=A (Parse 直後にデフォルトを適用) は決定済み。具体的にどこ?

A) **`buildSchema` 内で各型の構築時に適用** — `intDefault(raw.Replicas, 1)` のような小さなヘルパー (推奨、シンプル)

B) **各型のコンストラクタ関数 (`newService`, `newEdge` 等) を `topology` 内に作り、その内部で適用** — テストで再利用しやすいが、API 拡大

C) **`(*Schema).applyDefaults()` というメソッドを内部で呼ぶ** — Parse の終わりに 1 回。読みやすいが、複数パスになる

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 7: 性能最適化の優先度

NFR-U1-1 (Parse ≤ 10ms) を達成する最適化:

A) **必要最小限のみ** — `make(map, expectedSize)` で初期容量予約、append には `cap` ヒント、文字列連結は `strings.Builder`、それ以外は標準的な書き方 (推奨、過剰最適化回避)

B) A に加えて **sync.Pool で rawSchema を再利用** — 連続 Parse でアロケーション削減

C) **measure first** — まずは素直に書き、ベンチ結果次第で対応

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 8: 不変性の GoDoc 表記

NFR-U1-5 (immutability convention) を GoDoc にどう書くか:

A) **パッケージ doc.go + Schema 型 GoDoc の冒頭に明示** — 利用者が型を見たときに即座に分かる (推奨)

B) **README.md にのみ書く** — GoDoc は最小限

C) **両方** — GoDoc に短い注記、README に詳細

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 9: テストフィクスチャ形式

example-based test (`parse_test.go`, `validate_test.go` 等) の YAML フィクスチャ:

A) **inline string literal in test files** — `const yaml1 = "..."` で同ファイル内、テストとフィクスチャが近接 (推奨、シンプル)

B) **`testdata/*.yaml` ファイル** — Go 標準慣習 (`testdata/` ディレクトリは `go test` が認識)。フィクスチャが多数になるなら有利

C) **両方** — minimal な例は inline、複雑な fixture は `testdata/`

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 10: BenchmarkParse の入力規模

NFR-U1-1 (Parse 10ms) を検証する `BenchmarkParse` の入力:

A) **典型 YAML (10 svc / 30 op / 50 edges) 1 種類** — 標準ベンチ、退化検知 (推奨)

B) **3 種類** — minimal (3 svc), 典型 (10 svc), large (100 svc)。`b.Run(subname, ...)` でサブベンチ

C) **動的生成** — `generators.ValidSchema` で draw した schema を seed 固定で bench (生成 cost も含まれて bias 出やすい、非推奨)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 11: JSON Schema テンプレートの生成方法

`topology/jsonschema/topology.schema.json` の生成:

A) **hand-written + manual maintain** — JSON Schema を手書き、型変更時に手動更新 (推奨、ファイル数小、シンプル)

B) **`go run` ツールで Go の型から自動生成** — リフレクション or AST ベース、`cmd/jsonschema-gen/` のような generator tool 追加 (オーバースペック)

C) **テスト時に自動検証** — JSON Schema 自体は手書き、ただし「Go 型と Schema の整合性」を TP として追加

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 12: ファイル分割の粒度確認

FD `domain-entities.md` §3 で 16 production files + 11 test files を提案 (各メソッド毎に 1 ファイル)。これを維持?

A) **そのまま採用** — 各メソッドが 1 ファイル、`parse.go` / `validate.go` / `marshal.go` / `equal.go` / `faults.go` / `jsonschema.go` / `lint.go` / `errors.go` / `raw.go` (推奨、保守性高い)

B) **`parse.go` に Parse + Lint + Validate を統合** — Parse 関連を 1 ファイルにまとめる、ファイル数削減

C) **完全に細分化** — `validate_dag.go`, `validate_domain.go` 等もさらに分ける

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 2 つの NFR Design アーティファクトを生成して承認ゲートへ進みます。
