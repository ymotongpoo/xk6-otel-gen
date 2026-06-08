# U7 testutil/generators — Tech Stack Decisions

本書は U7 で採用する技術スタック・バージョン方針・CI 統合の詳細を確定する。**PBT-09 (Framework Selection) ルールの compliance 文書** を兼ねる。

---

## 1. 採用フレームワーク

### `pgregory.net/rapid` — Property-Based Testing

| 項目 | 内容 |
|---|---|
| **モジュールパス** | `pgregory.net/rapid` |
| **採用理由** | (1) Go 標準 testing と統合 (`*testing.T` ベース)、(2) 強力なシュリンカ (counterexample を自動で最小化)、(3) seed-based reproducibility、(4) ジェネリクスベースの type-safe ジェネレータ (`*rapid.Generator[T]`)、(5) Go コミュニティで広く採用、(6) AI-DLC PBT 拡張の推奨フレームワーク表に明記 |
| **ライセンス** | Mozilla Public License 2.0 (compatibility OK — 本プロジェクトは Apache-2.0、MPL-2.0 は依存として相互運用可能) |
| **代替候補との比較** | Go の PBT 選択肢には `pgregory.net/rapid` と `github.com/leanovate/gopter` がある。rapid は (a) シュリンカが優秀、(b) API が現代的 (Generics 対応)、(c) メンテが活発、で gopter より優位。AI-DLC PBT 拡張ルールの推奨もこちらに合致 |

### バージョン方針 (Q1=A)

| 項目 | 内容 |
|---|---|
| **戦略** | 最新 stable に追従 (active development tracking) |
| **`go.mod` 表現** | minimum version (例: `pgregory.net/rapid v1.0.0` 以上) を記載 |
| **アップデート手段** | (a) ローカル開発者は `go get -u pgregory.net/rapid` で更新、(b) CI で `dependabot.yml` (`.github/dependabot.yml`) による週次自動 PR (Build and Test ステージで設定確認) |
| **破壊変更時のポリシー** | rapid 自身の major version up (v2 等) は **手動で評価して採用**。マイナー/パッチは自動 PR を merge |
| **採用バージョンの確定タイミング** | U7 Code Generation 開始時に `go get pgregory.net/rapid@latest` の結果を `go.mod` に記録 |

---

## 2. テスト実行設定

### iteration 数

| 項目 | 内容 |
|---|---|
| **デフォルト** | rapid 既定 (~100 iterations per `rapid.Check`) |
| **オーバーライド方針** | 基本不要 (Q2=A)。テスト個別に `rapid.Iterations(N)` を付ける場合は **コードコメントで根拠必須** (重い check → 減らす、軽い + cover で増やす) |
| **環境変数** | rapid は `RAPID_CHECKS` 環境変数で全体制御可能。本プロジェクトでは設定しない (rapid デフォルトを尊重) |

### Seed 戦略 (Q3=A)

| 項目 | 内容 |
|---|---|
| **ローカル** | rapid デフォルト (時間ベース random seed)。失敗時、test 出力に `failure seed: <hex>` が表示される |
| **CI** | random seed を維持 + 全テスト run のシードを CI ログに出力 (`-rapid.log` フラグ or 標準 stdout)。失敗時にログから seed を抽出し、ローカルで `go test -run TestX -rapid.seed=<seed>` で再現可能 |
| **fixed seed**| 採用しない (Q3=A) |
| **CI 設定責任** | U8 (Build and Test) で `.github/workflows/ci.yml` に組み込み。U7 単体としては rapid の標準動作を妨げない設計を維持 |

### 並列実行 (Q4=A)

| 項目 | 内容 |
|---|---|
| **`t.Parallel()`** | U7 のすべてのテスト関数に付与 |
| **`go test -p N`** | デフォルト (= GOMAXPROCS) を維持 |
| **race detector** | `go test -race` を必須 (前提として NFR-U7-8 の thread-safety 保証) |

---

## 3. ファイル構成 (確定)

```text
testutil/
└── generators/
    ├── doc.go                  // パッケージ概要 + PBT-07/09 compliance ステートメント
    ├── options.go              // SchemaOption, ServiceOption (functional options 共通)
    ├── primitives.go           // ValidServiceID, ValidOperationName, ValidProbability, ValidReplicaCount, ValidLatencyPair, ValidTimeout, ValidServiceKind, ValidProtocol + Any 系
    ├── schema.go               // ValidSchema, AnySchema
    ├── service.go              // ValidService, AnyService
    ├── primitives_test.go      // example-based + meta-PBT for primitives
    ├── schema_test.go          // TP-U7-1, TP-U7-2, TP-U7-3, TP-U7-4 (PBT)
    ├── service_test.go         // ValidService の不変条件 (PBT)
    ├── options_test.go         // オプション尊重 PBT
    └── bench_test.go           // BenchmarkValidSchemaDraw (NFR-U7-6 検証用)
```

(後続ユニット FD の追加で `operation.go`, `edge.go`, `journey.go`, `fault.go` が増える)

---

## 4. 他の依存

| 依存 | 用途 | 採用理由 |
|---|---|---|
| `github.com/ymotongpoo/xk6-otel-gen/topology` | U1 の型 (Schema, Service, ServiceID, ...) を import | U7 はジェネレータを通じて U1 型を produce する。**ただし U1 はまだ実装されていない** — 詳細は §6 参照 |
| (標準ライブラリのみ) | `time`, `fmt`, `testing` | rapid 以外の外部依存は最小化 |

**追加依存の方針**: U7 の Code Generation では `pgregory.net/rapid` + 標準ライブラリ + 本プロジェクトの `topology` パッケージのみ。それ以外は追加しない (依存最小化、license 監査を簡単に保つ)。

---

## 5. CI / Build 統合 (U7 単体での要求)

| 項目 | 内容 |
|---|---|
| **`go.mod` 登録** | U7 Code Generation 中に `go get pgregory.net/rapid` 実行、`go.sum` も生成 |
| **test target** | `go test ./testutil/generators/...` |
| **coverage report** | `go test -cover ./testutil/generators/...` |
| **race detector** | `go test -race ./testutil/generators/...` |
| **lint** | `golangci-lint run ./testutil/generators/...` (NFR-U7-9 の支援) |
| **bench (任意)** | `go test -bench=. ./testutil/generators/...` |

これら個別コマンドの **CI ワークフローへの組み込み** は U8 (Build and Test) ステージで一括設計。U7 自体は各コマンドが動く状態を担保する。

---

## 6. U1 (topology) との循環依存問題と対処

**問題**: U7 は `topology.Schema` 等の型を import するため U1 に依存する。しかし Construction 順序は U7 が U1 より **前**。

**対処**: 段階的 build を採用。

```text
段階 1 (U7 FD 後):
  - U7 の FD ドキュメント + code-generation-plan.md だけ完成
  - 実コードはまだ無い

段階 2 (U7 Code Generation):
  - 最低限の topology 型 (Schema, Service, ServiceID, Operation, Edge 等の型シグネチャ) を
    U7 と並走で先に書く必要が出る。これを「pre-U1 型骨格」として U7 CG の冒頭で扱う
  - または、U7 CG で先に **mock/placeholder の topology 型** を書き、U1 CG で書き換える方針も可
  - どちらにするかは U7 Code Generation Planning ステージで詰める

段階 3 (U1 FD 開始):
  - U7 で書いた骨格を「公式仕様」に拡張する
  - U7 の generators は U1 の追加型 (CallNode, RecoveryPolicy, FaultSpec, ...) を扱うために
    `domain-entities.md §8` のリクエストセクションに追記され、incremental に育つ (Q8 of U7 FD)
```

**判断のための補足**: 段階 2 では「**pre-U1 型骨格を本物として書く**」のが現実的。理由:
- U1 FD で型自体は変更されない (Application Design の `component-methods.md` で固まっている)
- U7 が import 可能な型がないと、Code Generation で意味のあるコードが書けない
- U1 Code Generation 時点でこの骨格に新メソッド (`Parse`, `Validate`, `MarshalYAML`, `Equal`) を追加していく

この方針は U7 Code Generation Planning で正式に確定する。

---

## 7. メンテナンス・進化

### バージョン更新
- `pgregory.net/rapid` のリリースを subscriber 通知で追跡 (個人対応)
- `dependabot.yml` 設定後 (U8 / Build and Test) は自動 PR
- Breaking change が来た場合: changelog 確認 → 影響範囲評価 → 必要なら U7 generator API も major version up

### Deprecation
- 削除予定の generator は `// Deprecated: use XxxNew instead.` コメント
- 1 minor version の猶予期間
- v1.0.0 リリース前は猶予なしの削除も許容 (Q9=A)

### 拡張時のチェックリスト (各ユニット FD から U7 へのリクエスト処理時)
- 新規 generator が atomic / composed のどちらかに区分される
- Valid/Any 両方を提供する (Q2 of U7 FD)
- functional options を sensible なデフォルト + 拡張可能な形に設計
- realistic range をデフォルトに (Q6 of U7 FD)
- 既存テストを壊さない (NFR-U7-10)

---

## 8. 採用しなかった代替案

| 代替案 | 不採用理由 |
|---|---|
| `github.com/leanovate/gopter` | API が古い (Generics 対応していない)、シュリンカが弱い、メンテ頻度が下がっている |
| 自前 ad-hoc test helpers (rapid 不採用) | PBT 拡張ルール (Full enforcement) に違反、再利用性なし、PBT 本来の効用 (counterexample 自動探索) が得られない |
| `pgregory.net/rapid` を直接呼ぶ (U7 を作らない) | PBT-07 (Generator Quality) に違反、各ユニットが自前 generator を書くと重複・品質劣化、メンテ困難 |

---

## 9. PBT-09 Compliance Statement

本ステージにおいて、PBT-09 (Framework Selection) の verification 項目はすべて満たされる:

- [x] PBT framework が選定され、tech stack decisions に記録 (本書)
- [x] フレームワークが `go.mod` の依存に登録される (U7 Code Generation で実装)
- [x] フレームワークが custom generators / 自動 shrinking / seed-based reproducibility をサポート (`rapid` の標準機能)
- [x] 単一言語プロジェクト (Go) のため、複数言語対応の項は N/A
