# Requirements Document — xk6-otel-gen

## 1. Intent Analysis Summary

- **User Request**: "AI-DLCを使って、負荷テストツールである k6 向けの拡張を作りたい。実際にマイクロサービスを作らずに、何かしらの方法でコンポーネントの関係 (YAML、Mermaid 図など) を提示すると、それに応じた擬似的なテレメトリーシグナル (メトリクス、ログ、分散トレース) を OpenTelemetry 形式で生成して、OTLP のエンドポイントに対して送信する。"
- **Request Type**: New Project (Greenfield)
- **Scope Estimate**: Single Component (k6 拡張バイナリ + JS API + 内部シミュレーション/出力モジュール)
- **Complexity Estimate**: Moderate–Complex
  - OTel SDK との統合、Semantic Conventions 準拠、トポロジー解釈、信号合成、xk6 ビルド、JS↔Go ブリッジ
- **Requirements Depth**: Standard
- **Project Vision**: 実際のマイクロサービスを構築・運用せずに、宣言的なトポロジー定義 (YAML) から「リアルに見える」擬似 OpenTelemetry シグナル (Metrics / Logs / Traces) を合成し、k6 の負荷モデルで時間駆動して OTLP エンドポイント (Collector / Backend) に送信できるようにする。可観測性バックエンドや SRE ツールチェーンの検証 / デモ / 評価 / 教育用途を想定。

---

## 2. Extension Configuration (確定)

| Extension | Enabled | Decided At | Notes |
|---|---|---|---|
| Security Baseline | No | Requirements Analysis | PoC/実験用途のためスキップ (Q15: B) |
| Resiliency Baseline | No | Requirements Analysis | PoC/実験用途のためスキップ (Q16: B) |
| Property-Based Testing | **Yes (Full)** | Requirements Analysis | 全 PBT ルールをブロッキング制約として適用 (Q17: A)。Go 実装のため `pgregory.net/rapid` を採用候補とする。|

---

## 3. Functional Requirements

### FR-1 — 拡張バイナリと配布形態
- **FR-1.1**: 拡張は Go で実装し、`xk6 build` で k6 本体に組み込み、カスタム k6 バイナリ (`k6` with `k6/x/otel-gen`) を生成可能とする。
- **FR-1.2**: 本拡張は **JS モジュール** および **出力モジュール (output extension)** の両方を提供する (Q1: C)。
  - JS モジュール: `import otelgen from 'k6/x/otel-gen'` で k6 スクリプトから利用できる。
  - 出力モジュール: `k6 run --out otel-gen=...` のように k6 の出力パイプラインに統合できる。
- **FR-1.3**: Apache License 2.0 で OSS 公開し、`xk6 build` の手順を README に明記する (Q10: A)。

### FR-2 — トポロジー入力フォーマット
- **FR-2.1**: 最初のサポート入力は **独自スキーマの YAML** とする (Q2)。Mermaid / OTel Service Graph JSON / Kubernetes manifests は本フェーズではサポートしない (将来検討)。
- **FR-2.2**: YAML スキーマは少なくとも以下を表現できること:
  - **サービス定義**:
    - `name` (string, required)
    - `kind` (`application` | `database` | `external_api` | `cache` | `queue` の enum、required)
    - `replicas` (int, default: 1) — `service.instance.id` で複数インスタンスを表現する
    - `language` / `framework` (任意 — `telemetry.sdk.language` などに反映)
  - **依存関係 (edge)**:
    - 呼び出し元と呼び出し先のサービス
    - プロトコル (`http` | `grpc` | `messaging` のいずれか) と操作 (`operation` / `endpoint` / `topic`)
    - レイテンシ分布 (例: `latency: { distribution: lognormal, p50: 30ms, p95: 200ms }`)
    - エラー率 (`error_rate: 0.01`)、タイムアウト (`timeout: 1s`)、リトライ方針 (`retries: 2, retry_backoff: exponential`)
  - **ユーザージャーニー (critical user journey)**:
    - エントリーポイントから経由するサービスの順序付きパスを 1 つ以上定義可能
    - k6 シナリオで「どのジャーニーを駆動するか」を選択できる
- **FR-2.3**: YAML スキーマは厳格にバリデーションされ、エラー時には参照箇所と理由を明示する。

### FR-3 — 生成するテレメトリーシグナル
- **FR-3.1**: 以下の **3 信号をすべてサポート** する (Q3: D):
  - **Traces**: ユーザージャーニー単位で 1 本の trace を生成し、各サービス呼び出しを span として連鎖させる (parent/child を正しく構築)。
  - **Metrics**: `http.server.request.duration` 等のヒストグラム、`http.server.active_requests` ゲージ、リクエスト回数のカウンターを各サービスインスタンスから定期的に出力する。
  - **Logs**: 各 span ライフサイクル (start / end / error) に対応する構造化ログレコードを生成し、`trace_id` / `span_id` を含める。

### FR-4 — OTLP 送信
- **FR-4.1**: 送信プロトコルは **OTLP/gRPC** と **OTLP/HTTP (protobuf)** の両方をサポートする (Q4: C)。
- **FR-4.2**: エンドポイント・ヘッダー・TLS 設定は環境変数 (`OTEL_EXPORTER_OTLP_*` 標準) と JS API オプションの双方から指定可能。両方が指定された場合は JS API オプションが優先される。
- **FR-4.3**: ペイロードは OpenTelemetry の公式 Protobuf スキーマ (`opentelemetry-proto`) に準拠する。

### FR-5 — シミュレーションモデル
- **FR-5.1**: シミュレーションは **エンドツーエンド視点 (E2E)** で行う (Q5: C)。
  - k6 VU が 1 iteration につき 1 つの「ユーザージャーニー」をトリガーし、そのジャーニーが通過する全サービスから signals を合成する。
  - ジャーニーが「クリティカルユーザージャーニー」として複数定義されている場合、k6 スクリプトはどのジャーニーを駆動するかを iteration ごとに選択できる (重み付き選択をサポート)。
  - すべてのサービスを毎回通過する必要はない (例: 一部の依存は条件分岐で発火する)。
- **FR-5.2**: サーバー側視点の補完: 各仮想サービスからは「自分宛のリクエストを受けて応答した」という観点での metrics / logs もあわせて生成する。
- **FR-5.3**: 負荷プロファイル (RPS, バースト, ランプ) は **k6 のシナリオ機能** (`executor`, `vus`, `rate`, `stages`) によって駆動する (Q8: A)。拡張側独自の時間制御は持たない。

### FR-6 — Semantic Conventions
- **FR-6.1**: 初期実装では **OpenTelemetry Semantic Conventions の主要部分のみ** に準拠する (Q7: B)。
  - **Resource 属性 (必須)**: `service.name`, `service.namespace`, `service.instance.id`, `service.version`, `telemetry.sdk.name`, `telemetry.sdk.language`, `telemetry.sdk.version`, `host.name` (合成値で可)
  - **HTTP 属性 (必須)**: `http.request.method`, `http.response.status_code`, `url.path`, `url.scheme`, `server.address`, `server.port`, `network.peer.address`
  - **RPC 属性 (必須)**: `rpc.system`, `rpc.service`, `rpc.method`, `rpc.grpc.status_code`
  - **共通**: `error.type`, `exception.type`, `exception.message` (エラー時)
- **FR-6.2**: 将来的に messaging / db / FaaS 等の Semantic Conventions が安定化次第、属性セットを段階的に拡張できるよう、属性マッピング層を抽象化する。

### FR-7 — 失敗・異常シナリオ
- **FR-7.1**: 以下のすべての異常を、トポロジー YAML の設定により確率的にシミュレートできる (Q9: E):
  - HTTP 4xx / 5xx の確率発生 (`error_rate` ベース)
  - 高レイテンシ・タイムアウト (`timeout` 超過時に span は error として記録される)
  - 部分障害 / サービス停止 (`disabled: true` または `error_rate: 1.0` で常時失敗)
  - リトライストーム (`retries` と `retry_backoff` を尊重し、上流の遅延が下流に伝播する)
  - カスケード障害 (依存先のエラーが上流に伝播し、上流が遅延・失敗する)
- **FR-7.2**: span は失敗時に `status = ERROR` とし、`error.type` / `exception.*` 属性を付与する。

### FR-8 — k6 スクリプトからの利用 (DX)
- **FR-8.1**: JS API は **宣言的・最小** とする (Q11: C):

  ```javascript
  import otelgen from 'k6/x/otel-gen';
  import { check } from 'k6';

  const topology = otelgen.load('./topology.yaml');

  export const options = {
    scenarios: {
      checkout: {
        executor: 'constant-arrival-rate',
        rate: 100, timeUnit: '1s', duration: '5m',
        preAllocatedVUs: 50,
      },
    },
  };

  export default function () {
    // 単一の呼び出しで「ユーザージャーニー」を駆動し、信号合成と OTLP 送信は拡張内部で完結
    topology.runJourney('checkout');
  }
  ```

- **FR-8.2**: `runJourney(name)` は trace_id を新たに発行し、ジャーニーパス上のすべての span/metric/log を生成して非同期送信する。スクリプト側で個別の span/metric API を呼ぶ必要はない。
- **FR-8.3**: 設定オプション (OTLP endpoint, headers, resource overrides, sampler 等) は `topology.configure({...})` または環境変数で受け取る。

### FR-9 — サンプル / 同梱トポロジー
- **FR-9.1**: リポジトリには **minimal な例** (3 サービス: `frontend → backend → postgres`) を同梱する (Q12: C)。
- **FR-9.2**: あわせて **realistic な例** として OpenTelemetry Demo (Astronomy Shop) を題材にした 10+ サービスのトポロジーを同梱する。
- **FR-9.3**: 各サンプルには対応する k6 スクリプトと、想定される実行コマンド (Collector 起動含む) の README を同梱する。

---

## 4. Non-Functional Requirements

### NFR-1 — Performance & Scale
- **NFR-1.1**: 1 k6 ランナーあたり **持続 1,000 RPS** のジャーニー駆動 (= 〜1,000 trace/s) を、4 vCPU / 8 GB RAM の標準的なホストで安定実行できる (Q13: B)。
- **NFR-1.2**: 信号生成のオーバーヘッドにより k6 本来の負荷生成性能を著しく阻害しないこと (現実的目安: k6 単体比でスループット低下 30% 以内)。
- **NFR-1.3**: 送信は非同期バッチで行い、k6 iteration の同期パスをブロックしない。
- **NFR-1.4**: バックプレッシャー時には sender 内部キューをドロップ (loss-counted) または背圧でき、メモリリークを防ぐ。

### NFR-2 — Reliability / Operational
- **NFR-2.1**: OTLP エンドポイント未到達時にも k6 ランは継続でき、メトリクスに「送信失敗カウント」を出力する。
- **NFR-2.2**: 設定エラー (YAML 不正、未定義サービスの参照等) は k6 ラン起動時に検出し、停止する (`fail fast`)。

### NFR-3 — Compatibility
- **NFR-3.1**: 対応 k6: **最新 stable と直近 2 マイナーバージョン**。
- **NFR-3.2**: 対応 Go: Go の現行 stable と 1 つ前の minor。
- **NFR-3.3**: 対応 OS: Linux (amd64 / arm64) と macOS (arm64) — Windows は best-effort。

### NFR-4 — Testability
- **NFR-4.1**: 自動テストレベルは **Unit + Integration** (Q14: B)。
  - Unit: Go パッケージ単位のテーブル駆動 + Property-Based Tests。
  - Integration: 実 OTel Collector (Docker) に対する送信を起動し、受信した OTLP ペイロードを検証する。
- **NFR-4.2**: **Property-Based Testing (Full enforcement, PBT 拡張)** を以下の対象に必須適用:
  - YAML パーサのラウンドトリップ (`parse(serialize(x)) == x`)
  - トポロジーグラフの不変条件 (DAG 性質、到達可能性、依存閉包の整合)
  - 信号合成の不変条件 (1 ジャーニー = 1 trace_id、各 span に正しい parent_span_id、metric サムの保存則)
  - レイテンシ分布生成器の範囲制約 (p50/p95 の単調性、正値性)
  - OTLP protobuf シリアライズ/デシリアライズの round-trip
- **NFR-4.3**: CI で PBT のシード値をログ出力し、失敗時の再現性を担保する (PBT-08)。
- **NFR-4.4**: 採用フレームワーク (NFR-4 暫定): Go の `pgregory.net/rapid` (PBT-09 推奨)。確定は NFR Requirements ステージで再評価する。

### NFR-5 — Observability of the Tool Itself
- **NFR-5.1**: 拡張自身の動作ログ (info/warn/error) は k6 のログ機構を通じて出力する。
- **NFR-5.2**: 拡張内部メトリクス (送信成功/失敗カウント、内部キュー長) を k6 メトリクスとして公開し、`k6 summary` に表示する。

### NFR-6 — Documentation
- **NFR-6.1**: README に xk6 ビルド手順、JS API リファレンス、YAML スキーマリファレンス、サンプル実行手順を含める。
- **NFR-6.2**: YAML スキーマは JSON Schema として公開し、エディタ補完を可能にする。

---

## 5. Glossary
- **xk6**: k6 拡張ビルドツール。Go モジュールを k6 本体に静的リンクしてカスタムバイナリを生成する。
- **OTLP**: OpenTelemetry Protocol。telemetry signals の送受信プロトコル。gRPC / HTTP(protobuf) / HTTP(JSON) の3 形態がある (本拡張は前2者を実装)。
- **Critical User Journey (CUJ)**: トポロジー上で定義された、エントリーポイントから複数サービスを経由する一連の業務フロー (例: checkout, search-and-view, sign-up)。
- **Service Graph (OTel Collector)**: 受信トレースから自動派生する、サービス間呼び出しの集計グラフ (Prometheus メトリクス)。本拡張の入力ではない。

---

## 6. Assumptions & Resolved Ambiguities

回答内容の解釈にあたり、以下の前提を採用した。問題があれば指摘されたい。

- **A-1 (Q2)**: トポロジー入力は **独自 YAML** とし、Mermaid / OTel Service Graph / Kubernetes manifests は本フェーズのスコープ外。サービスの種類 (application / database / external_api / cache / queue) とレプリカ数を表現可能にする。
- **A-2 (Q6)**: Q6 の選択は "D" のみだったが、ロジカルに D (失敗シナリオ) を成立させるためには A (サービスと依存) と B (プロトコル/エンドポイント) が必須であり、C (レイテンシ分布) もタイムアウトと不可分なため、**A + B + C + D をすべて MVP の最低限要件として採用** する (実質 E)。
- **A-3 (Q5)**: 「全サービスを通過するかどうかはクリティカルユーザージャーニーに応じて欲しい」を、ジャーニー定義によってトレースが経由するサービス集合が可変であると解釈し、複数のジャーニーを宣言できる仕様 (FR-2.2 ユーザージャーニー) に反映した。

---

## 7. Summary of Key Requirements

- **Topology-driven, declarative**: YAML でサービス・依存・ジャーニー・障害シナリオを宣言し、k6 シナリオはそれらを「再生」するだけ。
- **All three OTel signals**: Metrics / Logs / Traces を OTLP/gRPC と OTLP/HTTP の両方で送信。
- **k6-driven load**: 時間制御は k6 シナリオに任せる。拡張側は信号合成と OTLP 送信に専念。
- **Realistic enough to validate observability stacks**: Semantic Conventions の主要部分に準拠し、リアリスティックな失敗シナリオ (HTTP error / timeout / cascade) を表現できる。
- **OSS Apache-2.0**: GitHub 公開と xk6 ビルド手順整備。
- **Quality bar**: Unit + Integration テスト、PBT を主要ロジックに必須適用、1k RPS の持続スケール。
