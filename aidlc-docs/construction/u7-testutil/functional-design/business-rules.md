# U7 testutil/generators — Business Rules

本書は U7 が提供するジェネレータが守るべき **不変条件・規則** を定義する。これは PBT-07 (Generator Quality) への適合と、ジェネレータ自身が「正しく生成する」ことを保証するための meta-rule。

---

## 1. Valid 系ジェネレータの不変条件 (Universal)

任意の Valid 系ジェネレータ `ValidX()` は、`*rapid.T` から `Draw` した値が以下を満たすこと:

| 規則 ID | 内容 | 適用範囲 |
|---|---|---|
| **R-V-1** | `ValidX()` の出力は、X 型に対するアプリケーション側の Validate 関数を通る (`Validate(x) == nil`) | 全 Valid 系 |
| **R-V-2** | `ValidX()` は副作用を持たない (鳥が決定論的、同一 `*rapid.T` シードに対し同一出力) | rapid 既定挙動 |
| **R-V-3** | `ValidX()` の出力は **構造的に完結** している (内部参照はすべて解決済み、nil ポインタなし — `topology.Equal` の前提を満たす) | U1 関連型のみ |
| **R-V-4** | `ValidX()` は **空集合や境界値を含む確率分布** を持つ (PBT-07 推奨。たとえば `ValidSchema()` は services 1 個のミニマル例から services 数 = maxServices まで出しうる) | 全 Valid 系 |
| **R-V-5** | `ValidX()` はリエントラント (複数 goroutine から呼ばれても安全)。グローバル状態を保持しない | 全 Valid 系 |

---

## 2. Any 系ジェネレータの不変条件 (Universal)

任意の Any 系ジェネレータ `AnyX()` は以下を満たすこと:

| 規則 ID | 内容 |
|---|---|
| **R-A-1** | `AnyX()` の出力は X 型の **構文的に合法な値**である (Go の型として組み立て可能。例: `*topology.Schema` の field がメモリ上正しい構造) |
| **R-A-2** | `AnyX()` は意味的に valid な値と invalid な値を **両方とも一定確率で含む**。Filter ベースでないこと (PBT-07 のジェネレータ品質、Filter は性能劣化) |
| **R-A-3** | `AnyX()` がカバーする invalid パターンは、想定されるバリデーション失敗ケースを **最低 1 つは含む** (例: `AnySchema()` は未解決参照を含む schema を生成しうる) |
| **R-A-4** | `AnyX()` の出力分布は **valid:invalid = 約 1:1 を目標** とする。Functional options で歪曲可能 (例: `BiasValid(0.8)` で valid 80%) |

---

## 3. ドメイン値範囲 (P-5、PBT-07 準拠)

実用的レンジを内蔵する。これは Valid 系・Any 系の **両方で共通の base range**:

| ドメイン値 | デフォルトレンジ | 上限の根拠 |
|---|---|---|
| Service 名 (`ServiceID`) | `^[a-z][a-z0-9-]{2,30}$` (kebab-case ASCII)、長さ 3〜31 | 既存マイクロサービス命名慣例 |
| Operation 名 | UTF-8 文字列 1〜120 文字 (一部に "/", "{", "}" を含む — HTTP path / gRPC method 両方を表現) | OTel `span.name` 推奨 ≤128 |
| Replicas | `[1, 100]` int | クラスタの現実的レプリカ数上限 |
| ErrorRate | `[0.0, 1.0]` float64 | 確率の定義域 |
| Latency p50 | `[1ms, 5s]` time.Duration | マイクロサービス呼び出しの実用域 |
| Latency p95 | `[p50, 30s]` time.Duration (p50 以上必須) | 同上 + 不変条件 R-DOM-1 |
| Timeout | `[100ms, 60s]` time.Duration | 実用的 RPC timeout 範囲 |
| Retries | `[0, 10]` int | 一般的な retry 上限 |
| Probability (severity 等) | `[0.0, 1.0]` float64 | 同上 |
| サービス数 (`maxServices`) | デフォルト 10、最小 1 | 「複数サービス」を作る最小 + 現実的上限 |
| Operations / service (`maxOpsPerService`) | デフォルト 5、最小 1 | 多すぎると test が遅い |
| Calls / operation (`maxCallsPerOp`) | デフォルト 5、最小 0 | leaf operation は 0 |
| Faults 数 (`maxFaults`) | デフォルト 3、最小 0 | 多すぎると意味不明な test |

**範囲外** 値はテストすべきではなく、それは Validate のテストで明示的に書く責務 (Any 系も基本は valid 範囲内、内部参照や DAG の不整合だけが invalid)。

---

## 4. 値レベルの不変条件

| 規則 ID | 内容 |
|---|---|
| **R-DOM-1** | `Latency.p95 >= Latency.p50` (Valid 系で必ず保証) |
| **R-DOM-2** | `RetryBackoff` enum 値は (`exponential` \| `linear` \| `constant`) のみ (Valid 系) |
| **R-DOM-3** | `ServiceKind` enum 値は (`application` \| `database` \| `external_api` \| `cache` \| `queue`) のみ |
| **R-DOM-4** | `Protocol` enum 値は (`http` \| `grpc` \| `messaging`) のみ |
| **R-DOM-5** | `ExhaustedAction` enum 値は (`propagate` \| `return_default` \| `succeed_silently`) のみ |
| **R-DOM-6** | `FaultKind` enum 値は (`latency_inflation` \| `error_rate_override` \| `disconnect` \| `crash`) のみ |

Any 系は R-DOM-1 を **時々違反** する余地を持ってよい (Validate のテスト用)。R-DOM-2〜6 は Go の型システムで担保されるため、Any 系も違反しない (enum を Go の `iota` ベース型として実装する前提)。

---

## 5. 構造的不変条件 (U1 型関連、Valid 系のみ)

`ValidSchema()` が返す `*topology.Schema` について:

| 規則 ID | 内容 |
|---|---|
| **R-STR-1** | `schema.Services` 中の各 `*Service` の `Name` フィールドはマップキーと一致 (`schema.Services[svc.Name] == svc`) |
| **R-STR-2** | すべての `*Operation` は所属 `*Service` を `Service` フィールドに back-pointer として持つ |
| **R-STR-3** | すべての `*Edge.From` / `*Edge.To` は `schema.Services[...].Operations[...]` のいずれかを指す (未解決参照なし) |
| **R-STR-4** | Operations 間の `Edge` グラフは DAG (循環なし) |
| **R-STR-5** | すべての `*Journey.Steps[i].Op` は `schema` 内の実在 Operation を指す |
| **R-STR-6** | すべての `FaultSpec.Target.Service` / `.Operation` / `.Edge` は `schema` 内の実在エンティティを指す |
| **R-STR-7** | 各 `*Operation.Calls` の `CallNode` は **必ず Edge or Parallel のいずれかが non-nil、もう一方は nil** (variant 制約) |
| **R-STR-8** | `RecoveryPolicy.Fallback[i].From` は親 edge の `From` と同じ Operation を指す (fallback edge は親 edge と同じ caller operation 起点) |

Any 系は R-STR-1〜R-STR-8 のいずれかを違反する余地あり (主に R-STR-3, R-STR-4, R-STR-5 を狙う)。

---

## 6. PBT-07 Generator Quality 準拠の確認

| PBT-07 verification 項目 | U7 での充足方法 |
|---|---|
| ドメインオブジェクトに対し custom generator を用意 | `ValidSchema`, `ValidService`, ... 全 atomic 生成器を public でエクスポート |
| プリミティブ生成器を直接使わない | `ValidLatency` / `ValidProbability` 等の wrapper を経由 (`rapid.Int()` 単体をテストで直接使わない) |
| ドメイン制約を尊重 (正値、有効フォーマット等) | §3 のデフォルトレンジ + §4 §5 の不変条件で担保 |
| 境界値を含む | rapid のデフォルト動作 (P-6) + §3 のレンジに 0/1/最大値が含まれる |
| 共有ドメイン型ジェネレータを再利用 | atomic export (P-1) + functional options (P-3) で重複排除 |

---

## 7. PBT-08 Shrinking & Reproducibility 準拠

| 項目 | 充足方法 |
|---|---|
| シュリンクが有効 | rapid 既定挙動を維持、無効化しない (P-6) |
| 失敗時に minimal 入力が出力 | rapid 自動 |
| シード値をログ出力 | CI 設定で `RAPID_SEED` を毎回ログ出力 (NFR-4.3、Build and Test ステージで実装確定) |
| フレーキー対策 | rapid テストは決定論的 (シード固定で再実行可能)。フレーキーが疑われたら新規 issue 化 |

---

## 8. PBT-09 Framework Selection 準拠

| 項目 | 充足方法 |
|---|---|
| カスタムジェネレータをサポート | rapid の `rapid.Custom[T]` を全面採用 |
| シュリンク自動 | rapid 自動 |
| シードベース再現性 | rapid + `RAPID_SEED` |
| テストランナー統合 | `go test` でそのまま動作 |
| 依存追加 | `go.mod` に `pgregory.net/rapid` を明示 (NFR Design で minimum version 確定) |

---

## 9. PBT-10 Complementary Testing 準拠

U7 自体は他ユニットのテストを支える Support unit。PBT-10 は U7 のスコープでは「U7 のテスト自身が PBT のみで構成されないこと」を要求するに過ぎない:

- U7 のジェネレータ自身を検証するテストは **example-based test** + **meta-PBT** の両方:
  - example: `TestValidSchemaProducesValidSchemas` (5 個 draw して `Validate` がすべて nil)
  - meta-PBT: `rapid.Check(t, func(t *rapid.T) { s := ValidSchema().Draw(t, "s"); require.NoError(t, topology.Validate(s)) })` で 100 回検証

---

## 10. U7 自身の Testable Properties (PBT-01)

U7 のジェネレータをテストする際の不変条件 (meta-properties):

| プロパティ ID | 内容 | カテゴリ (PBT-01) |
|---|---|---|
| **TP-U7-1** | `ValidSchema().Draw(t)` は `topology.Validate(s) == nil` を満たす | Invariant |
| **TP-U7-2** | `ValidSchema()` は構造的に DAG (R-STR-4) | Invariant |
| **TP-U7-3** | `AnySchema()` を 100 回 draw すると invalid 例が少なくとも 1 つ含まれる | Statistical Invariant |
| **TP-U7-4** | `ValidSchema(MaxServices(5))` の `len(schema.Services) <= 5` | Invariant (オプション尊重) |
| **TP-U7-5** | `ValidSchema()` 2 回 draw は **異なる schema を出しうる** (退化しない) | Statistical Invariant |
| **TP-U7-6** | `ValidLatency().Draw(t)` は `p95 >= p50` (R-DOM-1) | Invariant |

これらは U7 の Functional Design 段階で識別 (PBT-01 適合)。実装は U7 Code Generation で。
