---
title: 設定
weight: 2
---

エクスポーターの設定は 3 つの面（JS API・k6 出力の `--out` 引数・環境変数）から行えます。
この文書では設定できる項目をすべて記載します。

設定は優先順位に従ってマージされます。上の行ほど優先され、下の行を上書きします。

| 優先度 | ソース | 例 |
|---:|---|---|
| 1 | JS API | `otelgen.configure({ endpoint: "localhost:4317" })` |
| 2 | `--out` 引数 | `--out otel-gen=endpoint=localhost:4317,protocol=grpc` |
| 3 | 環境変数 | `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318` |
| 4 | 既定値 | `localhost:4317`、gRPC、insecure は false |

## 設定可能な項目（一覧）

各オプションを、3 つの設定面（JS API の `configure()` キー、`--out otel-gen=` 引数キー、
環境変数）と対応づけた一覧です。`—` はその面では設定できないことを表します。

| オプション | JS API | `--out` 引数 | 環境変数 | 型 | 既定値 | 説明 |
|---|---|---|---|---|---|---|
| ベースエンドポイント | `endpoint` | `endpoint` | `OTEL_EXPORTER_OTLP_ENDPOINT` | string | `localhost:4317` | 全シグナル共通の OTLP エンドポイント |
| トレース用エンドポイント | `tracesEndpoint` | — | `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | string | — | トレース専用の上書き |
| メトリクス用エンドポイント | `metricsEndpoint` | `metricsEndpoint` | `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` | string | — | メトリクス専用の上書き |
| ログ用エンドポイント | `logsEndpoint` | — | `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` | string | — | ログ専用の上書き |
| プロファイル用エンドポイント | `profilesEndpoint` | — | — | string | — | Pyroscope のプロファイル取り込みエンドポイント（profile エクスポートを有効化） |
| プロトコル | `protocol` | `protocol` | `OTEL_EXPORTER_OTLP_PROTOCOL` | enum | `grpc` | `grpc` / `http`（環境変数では `http/protobuf` も可） |
| insecure（TLS 無効） | `insecure` | `insecure` | `OTEL_EXPORTER_OTLP_INSECURE` | bool | `false` | TLS を無効にする |
| CA 証明書 | `caCert` | `caCert` | `OTEL_EXPORTER_OTLP_CERTIFICATE` | string（パス） | — | サーバ検証用 CA 証明書 |
| クライアント証明書 | `clientCert` | `clientCert` | `OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE` | string（パス） | — | mTLS クライアント証明書 |
| クライアント鍵 | `clientKey` | `clientKey` | `OTEL_EXPORTER_OTLP_CLIENT_KEY` | string（パス） | — | mTLS クライアント鍵 |
| ヘッダー | `headers` | `headers` | `OTEL_EXPORTER_OTLP_HEADERS` | map | — | 追加の OTLP ヘッダー（面ごとに記法が異なる） |
| 圧縮 | `compression` | `compression` | `OTEL_EXPORTER_OTLP_COMPRESSION` | enum | `""`（なし） | `""` または `gzip` |
| タイムアウト | `timeout` | `timeout` | `OTEL_EXPORTER_OTLP_TIMEOUT` | duration | `10s` | エクスポートのタイムアウト |
| バッチサイズ | `batchSize` | `batchSize` | — | int | `512` | 1 リクエストあたりの最大スパン数 |
| バッチタイムアウト | `batchTimeout` | `batchTimeout` | — | duration | `1s` | バッチをフラッシュするまでの最大待機時間 |
| キュー上限 | `maxQueueSize` | `maxQueueSize` | — | int | `2048` | 破棄が始まる前にバッファするスパン数 |
| サンプラー | `sampler` | — | `OTEL_TRACES_SAMPLER` | enum | `always_on` | `always_on` / `always_off` / `traceidratio` |
| サンプラー引数 | `samplerArg` | — | `OTEL_TRACES_SAMPLER_ARG` | number | `1` | `traceidratio` の比率 `[0,1]` |
| リソース属性の上書き | `resourceOverrides` | — | — | map | — | リソース属性を上書き・追加 |
| 出力キューサイズ | — | `queueSize` | — | int | `100` | k6 出力の内部キュー（`[10, 10000]`、出力専用） |

{{< callout type="info" >}}
環境変数のうち `HEADERS`、`PROTOCOL`、`COMPRESSION`、`TIMEOUT`、`INSECURE`、`CERTIFICATE`、
`CLIENT_CERTIFICATE`、`CLIENT_KEY` には、シグナル別の変種
（`OTEL_EXPORTER_OTLP_TRACES_*`、`OTEL_EXPORTER_OTLP_METRICS_*`、`OTEL_EXPORTER_OTLP_LOGS_*`）
があり、シグナル別が基底（`OTEL_EXPORTER_OTLP_*`）より優先されます。
{{< /callout >}}

代表的な JS 設定:

```javascript
otelgen.configure({
  endpoint: "localhost:4317",
  protocol: "grpc",
  insecure: true,
  caCert: "/etc/otel/ca.pem",
  clientCert: "/etc/otel/client.pem",
  clientKey: "/etc/otel/client-key.pem",
  headers: { "x-demo": "minimal" },
  compression: "gzip",
  timeout: "10s",
  // バッチ/キューの余裕。括弧内は既定値。後述の「スループット、バッチ、
  // 破棄されるルートスパン」を参照してください。
  batchSize: 2048,        // (既定 512)
  batchTimeout: "1s",     // (既定 1s)
  maxQueueSize: 16384,    // (既定 2048)
  sampler: "traceidratio",
  samplerArg: 0.1,
});
```

## 各設定の詳細

### エンドポイント

エクスポーターを宛先に向ける方法は 2 つあり、
[OTLP エクスポーター仕様](https://opentelemetry.io/docs/specs/otel/protocol/exporter/#endpoint-urls-for-otlphttp)
に従います。

1. **ベースエンドポイント** — 単一の `endpoint` を設定します。HTTP の場合、シグナルごとの
   パスが自動的に付加されます（`v1/traces`、`v1/metrics`、`v1/logs`）。例えば
   `https://otlp-gateway.example.com/otlp` はトレースを
   `https://otlp-gateway.example.com/otlp/v1/traces` に送信します。gRPC や `host:port`
   形式のエンドポイントはそのまま使われます。
2. **シグナルごとのエンドポイント** — `tracesEndpoint`、`metricsEndpoint`、`logsEndpoint`
   を設定します。これらはパス補完なしで **そのまま** 使われ、該当シグナルについてはベースの
   `endpoint` より優先されます。

| 設定面 | ベース | シグナルごと |
|---|---|---|
| JS API | `endpoint` | `tracesEndpoint`、`metricsEndpoint`、`logsEndpoint` |
| `--out` 引数 | `endpoint` | `metricsEndpoint`（この出力はメトリクスのみを発行） |
| 環境変数 | `OTEL_EXPORTER_OTLP_ENDPOINT` | `OTEL_EXPORTER_OTLP_{TRACES,METRICS,LOGS}_ENDPOINT` |

エンドポイントは `host:port` 形式、または `scheme://host[:port]` 形式である必要があります。

```javascript
otelgen.configure({
  // ベースエンドポイント: HTTP では v1/{signal} が付加される。
  endpoint: "https://otlp-gateway-prod-us-central-0.grafana.net/otlp",
  protocol: "http",
  // シグナルごとの任意の上書き（そのまま使われ、パス補完なし）:
  // tracesEndpoint: "https://traces.example.com/v1/traces",
  // metricsEndpoint: "https://metrics.example.com/v1/metrics",
  // logsEndpoint: "https://logs.example.com/v1/logs",
});
```

**`protocol`** は `grpc`（既定）または `http`（OTLP/HTTP/protobuf）です。環境変数では
`http/protobuf` も受け付けます。

{{< callout type="warning" >}}
**破壊的変更（シグナルごとのエンドポイント対応）:** URL 形式のベースエンドポイント
（`scheme://` を含むもの）は、HTTP の場合に `v1/{signal}` が付加されるようになりました。
以前は URL パスがそのまま送信されていました。旧来の挙動に依存していた場合
（例: `endpoint: "https://host:4318/v1/traces"` の設定）は、その値を該当する
シグナルごとのキー（`tracesEndpoint`、こちらはそのまま使われます）へ移してください。
{{< /callout >}}

### プロファイル用エンドポイント

**`profilesEndpoint`** は継続的プロファイリングのエクスポートを有効にします。
[Pyroscope](https://grafana.com/oss/pyroscope/)（または Grafana Cloud Profiles）の
取り込みベース URL を設定すると、[`profile`]({{< relref "/reference/topology" >}}) を宣言した
オペレーションが合成 pprof フレームグラフをそこへ送ります。未設定の場合、profile 生成は no-op です。

これは **profile 専用の独立した HTTP エンドポイント** であり、OTLP のシグナル別エンドポイント
ではありません。`OTEL_EXPORTER_OTLP_*` の規則には従わず（`v1/profiles` の付加や環境変数は
ありません）、JS API からのみ設定します。値は `host:port` または `scheme://host[:port]` で
ある必要があります。後述の `headers`・TLS（`caCert` / `clientCert` / `clientKey`）・
`compression`・`timeout` は profile クライアントと共有されるため、Grafana Cloud Profiles でも
同じ `Authorization` ヘッダーで動作します。

```javascript
otelgen.configure({
  endpoint: "localhost:4317",       // トレース/メトリクス/ログ用 OTLP
  profilesEndpoint: "http://localhost:4040",  // Pyroscope 取り込み
});
```

### 認証と TLS

- **`insecure`** — `true` で TLS を無効にします（平文）。証明書オプションと同時には
  指定できません。
- **`caCert`** — サーバ証明書を検証するための CA 証明書（PEM）のパス。システムの証明書
  プールに追加されます。
- **`clientCert` / `clientKey`** — mTLS 用のクライアント証明書と鍵のパス。**必ずセットで**
  指定してください。一方だけの指定は検証エラーになります。
- **`headers`** — OTLP リクエストに付与する追加ヘッダー。キーは `[A-Za-z0-9_-]+` に一致し、
  値は空でないことが必要です。記法は設定面ごとに異なります。
  - JS API: オブジェクト `{ "x-key": "value" }`
  - `--out` 引数: `headers=key1:value1;key2:value2`（`;` 区切り、`:` で key と value を区切る）
  - 環境変数: `OTEL_EXPORTER_OTLP_HEADERS=key1=value1,key2=value2`（`,` 区切り、`=` 区切り、値は URL デコードされる）
- **`compression`** — `""`（なし、既定）または `gzip`。

証明書ファイルはパイプライン検証と起動時に読み込まれるため、ファイルの欠落、不正な PEM
データ、クライアント証明書/鍵ペアの不備、`insecure: true` との併用は、トラフィック開始前に
失敗します。TLS の最小バージョンは 1.2 です。ヘッダーの値が JS モジュールの設定ログに含まれる
ことはありません。

```javascript
otelgen.configure({
  endpoint: "otel-collector.example.internal:4317",
  protocol: "grpc",
  insecure: false,
  caCert: "/etc/otel/ca.pem",
  clientCert: "/etc/otel/client.pem",
  clientKey: "/etc/otel/client-key.pem",
  headers: { authorization: "Bearer ${TOKEN}" },
});
```

### バッチとキュー

- **`timeout`** — 1 回の OTLP エクスポート呼び出しのタイムアウト。既定 `10s`。
  JS API では数値はミリ秒、または Go の duration 文字列（例: `"10s"`）。`--out` 引数は
  duration 文字列。環境変数 `OTEL_EXPORTER_OTLP_TIMEOUT` はミリ秒です。
- **`batchSize`** — 1 回の OTLP エクスポートリクエストに含める最大スパン数。`maxQueueSize`
  以下である必要があります。既定 `512`。
- **`batchTimeout`** — スパンがフラッシュされるまで待つ最大時間。既定 `1s`。
- **`maxQueueSize`** — 破棄が始まる前にバッファするスパン数。既定 `2048`。

#### スループット、バッチ、破棄されるルートスパン

シンセサイザーはジャーニーのイテレーションごとに 1 本のトレースを、k6 がイテレーションを
回す速さで生成します。`constant-vus` エグゼキューターでシンクタイムなしの場合、単一の VU
でも **毎秒 10,000 イテレーション以上** を生成でき、これはほとんどのバックエンド、あるいは
OTLP エクスポーターが取り込める量をはるかに超えます。

生成がエクスポートを上回ると、トレースの `BatchSpanProcessor` のキューが埋まり、
**スパンが破棄されます**。この破棄はエクスポーターより *前* で発生するため、
`otelgen.stats().tracesFailed` には **カウントされません**。重要なのは、トレースの
**ルートスパンはすべての子スパンの後に終了する** ため最後にキューへ入る点で、キューが
あふれたときに最初に犠牲になります。その結果バックエンドは子スパンを受け取るのに
ルートを受け取れず、Grafana Tempo ではトレース一覧の Service 列に
`<root span not yet received>` と表示されます。

これに対処する独立した制御が 2 つあります。

**1. 生成レートを抑えます。** レートベースのエグゼキューターを使い、CPU の全速力ではなく
取り込み可能な一定レートでジャーニーを生成します。

```javascript
export const options = {
  scenarios: {
    checkout: {
      executor: "constant-arrival-rate",
      rate: 300,            // ジャーニー/秒。× ジャーニーあたりのスパン数 ≈ バックエンドのスパンレート
      timeUnit: "1s",
      duration: "30s",
      preAllocatedVUs: 20,
      maxVUs: 100,
    },
  },
};
```

`rate × ジャーニーあたりのスパン数` がバックエンドの取り込み予算に収まるように `rate` を
選びます。同梱の例はおよそ **毎秒 1,000 スパン** を目標としています。最小構成のジャーニーは
3 スパンを生成するため、`rate: 300` で実行します。

**2. エクスポーターのキューとバッチのサイズを調整します。** バッチプロセッサーにバーストを
破棄せず吸収できる余裕を与えます。

| オプション | 既定 | 余裕を持たせた値 | 効果 |
|---|---:|---:|---|
| `maxQueueSize` | 2048 | 16384 | 破棄が始まる前にバッファできるスパン数。破棄を止めるにはまずこれを上げます。 |
| `batchSize` | 512 | 2048 | OTLP エクスポートリクエストあたりの最大スパン数。`maxQueueSize` 以下である必要があります。 |
| `batchTimeout` | 1s | 1s | スパンがフラッシュされるまで待つ最大時間。小さくすると、ルートスパンが子スパンに遅れる時間が短くなります。 |

最終バッチ（最新のルートスパンを含む）がプロセス終了前に確実に送信されるよう、必ず
`teardown()` で `otelgen.flush()` を呼び出してください（[使い方]({{< relref "/usage" >}})を参照）。

### サンプリング

- **`sampler`** — `always_on`（既定）/ `always_off` / `traceidratio` のいずれか。
- **`samplerArg`** — `traceidratio` で使われ、`[0,1]` の範囲でなければなりません。既定 `1`。

不正なサンプラーの環境変数値はパイプライン検証で失敗し、元の `OTEL_TRACES_SAMPLER` の値と
許可される値の集合がエラーメッセージに含まれます。

サンプリングはトレースにのみ適用されます。メトリクスとログは引き続き発行され、ログは
トレースサンプラーがスパンを破棄した場合でもアクティブなトレースコンテキストを保持します。

### エグゼンプラー

ヒストグラムメトリクスには **エグゼンプラー**（`trace_id` / `span_id`）が付与され、Grafana
などのバックエンドからメトリクスのデータポイントを対応するトレースへたどれます。メーター
プロバイダーは OTel SDK 既定の `TraceBasedFilter` を使うため、測定がサンプリング済みの
スパンコンテキスト内で記録されると自動的にエグゼンプラーが付きます（設定は不要）。付与は
サンプリングに依存するため、`sampler: "always_off"` ではエグゼンプラーは付きません。

### リソース属性

- **`resourceOverrides`** — リソース属性を上書き・追加するマップ（JS API 専用）。キーは空で
  なく、値は文字列に変換できる必要があります。

```javascript
otelgen.configure({
  endpoint: "localhost:4317",
  resourceOverrides: {
    "deployment.environment": "staging",
    "service.version": "1.2.3",
  },
});
```

### k6 出力固有

- **`queueSize`** — `otel-gen` k6 出力の内部キューサイズ。`[10, 10000]` の範囲で、既定は
  `100`。`--out` 引数でのみ設定できます。

```bash
./k6 run script.js --out otel-gen=endpoint=localhost:4317,protocol=grpc,insecure=true,queueSize=100
./k6 run script.js --out otel-gen=endpoint=otel.example.com:4317,protocol=grpc,caCert=/etc/otel/ca.pem,clientCert=/etc/otel/client.pem,clientKey=/etc/otel/client-key.pem
```

## 組み込みメトリクス

JS モジュールはジャーニー実行後に、エクスポーターのカウンタをネイティブの k6 メトリクス
として公開します: `otel_gen_traces_exported`、`otel_gen_traces_failed`、
`otel_gen_metrics_exported`、`otel_gen_metrics_failed`、`otel_gen_logs_exported`、
`otel_gen_logs_failed`、`otel_gen_queue_drops`。キューの破棄は JS モジュールの
パイプラインメトリクスにスコープされ、`otel-gen` k6 出力は最終的なキュー破棄数を
`Stop()` でログ出力します。

## SaaS の OTLP エンドポイントへの送信

同じ `configure(...)` / `--out otel-gen=...` の仕組みは、マネージドな OpenTelemetry
エンドポイントに対しても動作します。ベンダーごとの詳しい手順は
[examples/saas-endpoints.md](https://github.com/ymotongpoo/xk6-otel-gen/blob/main/examples/saas-endpoints.md)
を参照してください。

**Grafana Cloud（OTLP ゲートウェイ、HTTP/protobuf）**:

```javascript
otelgen.configure({
  endpoint: "https://otlp-gateway-prod-us-central-0.grafana.net/otlp",
  protocol: "http",
  insecure: false,
  headers: {
    // base64("<instance_id>:<api_token>")
    Authorization: `Basic ${__ENV.GRAFANA_CLOUD_OTLP_TOKEN}`,
  },
});
```

**Google Cloud Observability（サイドカー Collector 経由）** — Google の OTLP 受信は
OAuth2 / ADC を必要とするため、推奨されるパターンは xk6-otel-gen をローカルの Collector に
向けたままにし、その Collector が認証を処理して `telemetry.googleapis.com` へ再エクスポート
する構成です。k6 側は変更不要です（`endpoint: "localhost:4317"`）。

各ベンダー向けのコピー&ペースト可能な Collector 設定は
[examples/saas-endpoints.md](https://github.com/ymotongpoo/xk6-otel-gen/blob/main/examples/saas-endpoints.md)
にあります。
