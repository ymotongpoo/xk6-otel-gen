---
title: セキュリティ
weight: 1
---

本プロジェクトはソースコードとサンプルを配布するものであり、ビルド済みの k6 バイナリは
配布しません。最終的な成果物が監査済みの入力からあなたの環境で生成されるよう、xk6 で
自分の k6 バイナリをビルドしてください。

| セキュリティ上の選択 | 理由 |
|---|---|
| ビルド済みバイナリなし | 不透明な負荷試験用実行ファイルを信頼するよう利用者に求めずに済む |
| デモイメージのピン留め | Kubernetes のサンプルは明示的なイメージタグを使用 |
| 合成データのみ | サンプルは本番の認証情報やユーザーデータを必要としない |
| OTLP の TLS オプション | JS オプションまたは環境変数でセキュアなエンドポイントを設定可能 |

本番想定のエンドポイントの例:

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

証明書ファイルはパイプライン検証と起動時に読み込まれるため、ファイルの欠落、不正な PEM
データ、クライアント証明書/鍵ペアの不備、`insecure: true` との証明書オプション併用は、
トラフィック開始前に失敗します。ヘッダーの値が JS モジュールの設定ログに含まれることは
ありません。
