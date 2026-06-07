# 要件確認のための質問 (xk6-otel-gen)

以下の質問にお答えください。各質問の `[Answer]:` タグの後に、選択肢のアルファベット（A/B/C/...）を記入してください。複数選択が想定される項目は、その旨を質問文に明記しています。どの選択肢にも当てはまらない場合は `X) Other` を選び、`[Answer]:` の後に自由記述してください。

すべて記入し終わったら、「完了しました」「done」などとお伝えください。

---

## Question 1: 拡張の実装言語とビルド方式

k6拡張は通常 [xk6](https://github.com/grafana/xk6) を用いてGoで実装し、k6本体に組み込んでカスタムバイナリを生成します。本拡張の実装方針はどうしますか？

A) Go + xk6 (k6/JavaScript からインポートして使う標準的なk6拡張)

B) Go + xk6 (出力モジュールとして実装し、k6 のメトリクスパイプラインに統合)

C) Go + xk6 (両方をサポート — JS API と出力モジュールの両方)

X) Other (please describe after [Answer]: tag below)

[Answer]: Cになると思う。テストシナリオとしてトポロジーを認識する部分はJavaScriptフロントエンドで読み込める必要があると思うし、出力はOTLP形式で送る必要があるので出力モジュールも構成する必要があると思う。

---

## Question 2: トポロジー入力形式

「コンポーネントの関係」を記述する入力形式として何をサポートしますか？（最重要要件のため、複数選択可。複数選んだ場合はカンマ区切りで記入してください。例: `A,B`）

A) YAML (独自スキーマ — サービス・依存・呼び出しパターンを記述)

B) Mermaid (graph / flowchart / sequenceDiagram から抽出)

C) OpenTelemetry Service Graph 風の JSON

D) 既存標準 (例: OpenTelemetry Demo の `service.yaml` 形式、Kubernetes manifests など)

X) Other (please describe after [Answer]: tag below)

[Answer]: CのOpenTelemetry Service Graphというものが何かよくわからない。またDのOpenTelemetry Demoの `service.yaml` も初めて聞く。しかしサービスごとの依存関係と、各サービスの設定はYAMLで定義するのが良さそうかなと思っています。例えばサービスのレプリカ数、サービスの種類（アプリケーションなのか、データベースなのか、外部APIなのか）などを定義できると良さそうです。

---

## Question 3: 生成するテレメトリーシグナル

どのシグナルを生成・送信しますか？（複数選択可、カンマ区切り）

A) Metrics (カウンター, ヒストグラム, ゲージなど)

B) Logs (構造化ログ)

C) Traces (分散トレース — サービス間のspan)

D) All of the above

X) Other (please describe after [Answer]: tag below)

[Answer]: D

---

## Question 4: OTLP送信プロトコル

OTLP エンドポイントへの送信プロトコルとして何をサポートしますか？（複数選択可）

A) OTLP/gRPC のみ

B) OTLP/HTTP (protobuf) のみ

C) 両方 (gRPC + HTTP)

X) Other (please describe after [Answer]: tag below)

[Answer]: C

---

## Question 5: シミュレーションの観点 (どの視点から生成するか)

擬似テレメトリーをどのような視点でシミュレートしますか？

A) サービス側視点 — 各仮想サービスがリクエストを受け、ダウンストリームを呼び、span/metric/logを発行する (フルなサーバー側テレメトリ)

B) クライアント側視点 — k6 VU がトポロジー先頭のエントリーポイントに対して仮想的に負荷を生成し、トレースが伝播する

C) 両方 — エンドツーエンド (k6 VU 起点 → トポロジー内の全サービスを通過するトレースを生成)

X) Other (please describe after [Answer]: tag below)

[Answer]: C - 全サービスを通過するかどうかはそのシステムのクリティカルユーザージャーニーに応じて欲しい 

---

## Question 6: トポロジー記述の豊かさ

トポロジー入力で表現できる要素として、最低限何を含めますか？（複数選択可）

A) サービス名と依存関係 (有向グラフ)

B) 呼び出しプロトコル (HTTP/gRPC/messaging)、エンドポイント名、操作名

C) 各サービスのレイテンシ分布 (例: p50/p95、または分布パラメーター)

D) エラー率 / リトライ / タイムアウトなどの失敗シナリオ

E) 上記すべて (A+B+C+D)

X) Other (please describe after [Answer]: tag below)

[Answer]: D 

---

## Question 7: セマンティック規約 (Semantic Conventions)

生成するシグナルの属性は OpenTelemetry Semantic Conventions に準拠しますか？

A) Yes — 安定版の最新セマンティック規約 (HTTP, RPC, messaging, db, etc.) に厳密準拠

B) Yes — ただし主要なリソース属性 (service.name, service.namespace 等) と HTTP/RPC の基本属性のみ

C) No — 独自属性命名で良い (まずは動くことを優先)

X) Other (please describe after [Answer]: tag below)

[Answer]: B - ただし将来的には確定次第より多くのセマンティック規約に準じたい

---

## Question 8: 負荷プロファイルと時間制御

擬似負荷の量と時間プロファイル (RPS, バースト, ランプ) はどう制御しますか？

A) k6 のシナリオ機能 (executor, VUs, rate) をそのまま利用し、各 iteration が 1 リクエストを生成する

B) 拡張側で独自に load profile を YAML / Mermaid から読み取り、k6 シナリオとは独立に時間制御する

C) 両方サポート — k6 シナリオで全体駆動しつつ、トポロジー内の二次的呼び出しは拡張側で制御

X) Other (please describe after [Answer]: tag below)

[Answer]: A 

---

## Question 9: 失敗・異常シナリオ

エラーや異常状態の擬似生成は必要ですか？（複数選択可）

A) HTTPステータスコードのエラー (4xx/5xx) を確率的に発生

B) 高レイテンシ・タイムアウトのシミュレーション

C) サービス停止 / 部分障害 (一部依存先がerrorを返し続ける)

D) リトライストーム / カスケード障害

E) 上記すべて

F) シナリオは必要なし — 正常系のみ

X) Other (please describe after [Answer]: tag below)

[Answer]: E

---

## Question 10: 配布形態とライセンス

このプロジェクトをどのような形で配布する予定ですか？

A) OSS (Apache-2.0) として GitHub 公開、xk6 ビルド手順を README に明記

B) OSS (MIT)

C) 当面は社内利用 / 個人用 — ライセンスは未定

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

## Question 11: 設定と実行例 (DX)

k6 スクリプトから本拡張をどう使いますか? (JS API のイメージ)

A) `import otelgen from 'k6/x/otel-gen'` のように import し、`otelgen.load('topology.yaml')` → `otelgen.run()` の最小APIにする

B) k6 のシナリオ内で各 VU iteration が `otelgen.emit('checkout-service')` のように呼び、特定サービスのspan/metric/logだけ生成する

C) より宣言的 — k6 スクリプトはエンドポイントとトポロジーパスだけ指定し、信号の合成はすべて拡張内部で完結

X) Other (please describe after [Answer]: tag below)

[Answer]: C

---

## Question 12: サンプル/同梱トポロジー

開発と動作確認のためにどんなサンプルトポロジーを同梱しますか？

A) シンプルな2〜3サービスの例 (frontend → backend → db) のみ

B) OpenTelemetry Demo (Astronomy Shop) をモデルにした 10+ サービス相当のトポロジー

C) 両方 — minimal な例と realistic な例の双方

X) Other (please describe after [Answer]: tag below)

[Answer]: C

---

## Question 13: 非機能要件 — パフォーマンス・スケール

本拡張に期待する処理スケール (1 k6 ランナーあたり) はどの程度ですか?

A) 〜100 RPS (動作確認/PoC レベル)

B) 〜1,000 RPS (典型的な負荷テスト)

C) 〜10,000 RPS 以上 (本格的なロードテスト)

X) Other (please describe after [Answer]: tag below)

[Answer]: B

---

## Question 14: テストレベル

実装に期待する自動テストの範囲は?

A) Unit tests のみ (主要パッケージ)

B) Unit tests + Integration tests (実際の OTel Collector に対する送信検証)

C) Unit + Integration + E2E (k6 を実行してCollector経由でバックエンド検証まで)

X) Other (please describe after [Answer]: tag below)

[Answer]: B

---

## Question 15: Security Extensions

セキュリティ拡張ルールをこのプロジェクトに適用しますか？

A) Yes — すべての SECURITY ルールをブロッキング制約として適用する (本番品質のアプリケーション向け推奨)

B) No — SECURITY ルールをスキップする (PoC、プロトタイプ、実験プロジェクト向け)

X) Other (please describe after [Answer]: tag below)

[Answer]: B

---

## Question 16: Resiliency Extensions

レジリエンシ・ベースラインをこのプロジェクトに適用しますか？

**この拡張の内容**: AWS Well-Architected Framework (Reliability Pillar) に基づいた、設計時の方向性ベストプラクティス (フォールトトレランス、可観測性、可用性、回復性など 15 領域) を要件・設計・コードに反映します。

**この拡張ではないもの**: ワークロードを本番レディにするものではなく、可用性/RTO/RPO 目標を保証するものでもありません。あくまでスタート地点としてのスキャフォルドです。

A) Yes — レジリエンシ・ベースラインを適用する (ビジネスクリティカルなワークロード向け推奨)

B) No — スキップする (PoC、プロトタイプ、実験プロジェクト向け)

X) Other (please describe after [Answer]: tag below)

[Answer]: B

---

## Question 17: Property-Based Testing Extension

プロパティベーステスト (PBT) ルールをこのプロジェクトで強制しますか？

A) Yes — PBT ルールをすべてブロッキング制約として適用する (ビジネスロジック、データ変換、シリアライゼーション、ステートフルコンポーネントを持つプロジェクト向け推奨)

B) Partial — pure 関数とシリアライゼーションのラウンドトリップのみ PBT を適用 (アルゴリズム複雑性が限定的なプロジェクト向け)

C) No — PBT ルールをスキップ (シンプルな CRUD アプリ、UI のみのプロジェクト、または重要なビジネスロジックを持たない統合レイヤー向け)

X) Other (please describe after [Answer]: tag below)

[Answer]: A
