---
title: 設定
weight: 2
---

設定は優先順位に従ってマージされます。上の行ほど優先され、下の行を上書きします。

| 優先度 | ソース | 例 |
|---:|---|---|
| 1 | JS API | `otelgen.configure({ endpoint: "localhost:4317" })` |
| 2 | `--out` 引数 | `--out otel-gen=endpoint=localhost:4317,protocol=grpc` |
| 3 | 環境変数 | `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318` |
| 4 | 既定値 | `localhost:4317`、gRPC、insecure は false |

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
  timeout: "10s",
  // バッチ/キューの余裕。括弧内は既定値。以下の値は持続的な負荷向けに余裕を持たせて
  // います。後述の「スループット、バッチ、破棄されるルートスパン」を参照してください。
  batchSize: 2048,        // (既定 512)
  batchTimeout: "1s",     // (既定 1s)
  maxQueueSize: 16384,    // (既定 2048)
  sampler: "traceidratio",
  samplerArg: 0.1,
});
```

`sampler` には `always_on`、`always_off`、`traceidratio` を指定できます。
`samplerArg` は `traceidratio` で使われ、`[0,1]` の範囲でなければなりません。不正な
サンプラーの環境変数値はパイプライン検証で失敗し、元の `OTEL_TRACES_SAMPLER` の値と
許可される値の集合がエラーメッセージに含まれます。

## スループット、バッチ、破棄されるルートスパン

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

最終バッチ(最新のルートスパンを含む)がプロセス終了前に確実に送信されるよう、必ず
`teardown()` で `otelgen.flush()` を呼び出してください([使い方]({{< relref "/usage" >}})を参照)。

## エンドポイントの解決

エクスポーターを宛先に向ける方法は 2 つあり、
[OTLP エクスポーター仕様](https://opentelemetry.io/docs/specs/otel/protocol/exporter/#endpoint-urls-for-otlphttp)
に従います。

1. **ベースエンドポイント** — 単一の `endpoint` を設定します。HTTP の場合、シグナルごとの
   パスが自動的に付加されます(`v1/traces`、`v1/metrics`、`v1/logs`)。例えば
   `https://otlp-gateway.example.com/otlp` はトレースを
   `https://otlp-gateway.example.com/otlp/v1/traces` に送信します。gRPC や `host:port`
   形式のエンドポイントはそのまま使われます(SDK が独自にシグナルごとのパスを適用します)。
2. **シグナルごとのエンドポイント** — `tracesEndpoint`、`metricsEndpoint`、`logsEndpoint`
   を設定します。これらはパス補完なしで **そのまま** 使われ、該当シグナルについてはベースの
   `endpoint` より優先されます。

| 設定面 | ベース | シグナルごと |
|---|---|---|
| JS API | `endpoint` | `tracesEndpoint`、`metricsEndpoint`、`logsEndpoint` |
| `--out` 引数 | `endpoint` | `metricsEndpoint`(この出力はメトリクスのみを発行) |
| 環境変数 | `OTEL_EXPORTER_OTLP_ENDPOINT` | `OTEL_EXPORTER_OTLP_{TRACES,METRICS,LOGS}_ENDPOINT` |

```javascript
otelgen.configure({
  // ベースエンドポイント: HTTP では v1/{signal} が付加される。
  endpoint: "https://otlp-gateway-prod-us-central-0.grafana.net/otlp",
  protocol: "http",
  // シグナルごとの任意の上書き(そのまま使われ、パス補完なし):
  // tracesEndpoint: "https://traces.example.com/v1/traces",
  // metricsEndpoint: "https://metrics.example.com/v1/metrics",
  // logsEndpoint: "https://logs.example.com/v1/logs",
});
```

{{< callout type="warning" >}}
**破壊的変更(シグナルごとのエンドポイント対応):** URL 形式のベースエンドポイント
(`scheme://` を含むもの)は、HTTP の場合に `v1/{signal}` が付加されるようになりました。
以前は URL パスがそのまま送信されていました。旧来の挙動に依存していた場合
(例: `endpoint: "https://host:4318/v1/traces"` の設定)は、その値を該当する
シグナルごとのキー(`tracesEndpoint`、こちらはそのまま使われます)へ移してください。
{{< /callout >}}

TLS 証明書のオプションは、JS(`caCert`、`clientCert`、`clientKey`)、同じキーの `--out`
引数、または OTEL 環境変数で指定できます。環境変数は
`OTEL_EXPORTER_OTLP_CERTIFICATE`、`OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE`、
`OTEL_EXPORTER_OTLP_CLIENT_KEY` で、`OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE` のような
シグナル固有のものも含みます。`clientCert` と `clientKey` は必ずセットで設定してください。
証明書オプションは `insecure: true` と併用できません。

サンプリングはトレースにのみ適用されます。メトリクスとログは引き続き発行され、ログは
トレースサンプラーがスパンを破棄した場合でもアクティブなトレースコンテキストを保持します。

## 組み込みメトリクス

JS モジュールはジャーニー実行後に、エクスポーターのカウンタをネイティブの k6 メトリクス
として公開します: `otel_gen_traces_exported`、`otel_gen_traces_failed`、
`otel_gen_metrics_exported`、`otel_gen_metrics_failed`、`otel_gen_logs_exported`、
`otel_gen_logs_failed`、`otel_gen_queue_drops`。キューの破棄は JS モジュールの
パイプラインメトリクスにスコープされ、`otel-gen` k6 出力は最終的なキュー破棄数を
`Stop()` でログ出力します。

代表的な出力設定:

```bash
./k6 run script.js --out otel-gen=endpoint=localhost:4317,protocol=grpc,insecure=true,queueSize=100
./k6 run script.js --out otel-gen=endpoint=otel.example.com:4317,protocol=grpc,caCert=/etc/otel/ca.pem,clientCert=/etc/otel/client.pem,clientKey=/etc/otel/client-key.pem
```

## SaaS の OTLP エンドポイントへの送信

同じ `configure(...)` / `--out otel-gen=...` の仕組みは、マネージドな OpenTelemetry
エンドポイントに対しても動作します。ベンダーごとの詳しい手順は
[examples/saas-endpoints.md](https://github.com/ymotongpoo/xk6-otel-gen/blob/main/examples/saas-endpoints.md)
を参照してください。

**Grafana Cloud(OTLP ゲートウェイ、HTTP/protobuf)**:

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

**Google Cloud Observability(サイドカー Collector 経由)** — Google の OTLP 受信は
OAuth2 / ADC を必要とするため、推奨されるパターンは xk6-otel-gen をローカルの Collector に
向けたままにし、その Collector が認証を処理して `telemetry.googleapis.com` へ再エクスポート
する構成です。k6 側は変更不要です(`endpoint: "localhost:4317"`)。

各ベンダー向けのコピー&ペースト可能な Collector 設定は
[examples/saas-endpoints.md](https://github.com/ymotongpoo/xk6-otel-gen/blob/main/examples/saas-endpoints.md)
にあります。
