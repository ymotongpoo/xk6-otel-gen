---
title: xk6-otel-gen
layout: hextra-home
# type: docs makes Hextra root the left sidebar at the site home so every
# top-level section is listed on every docs page (see the English _index.md).
type: docs
# cascade type:docs to descendants so leaf pages use docs/single.html (with the
# sidebar) instead of the default single.html (see the English _index.md).
cascade:
  type: docs
---

{{< hextra/hero-badge >}}
  <div class="hx-w-2 hx-h-2 hx-rounded-full hx-bg-primary-400"></div>
  Apache-2.0 · Go 1.25+
{{< /hextra/hero-badge >}}

<div class="hx-mt-6 hx-mb-6">
{{< hextra/hero-headline >}}
  OpenTelemetry のトレース・メトリクス・&nbsp;<br class="sm:hx-block hx-hidden" />ログを生成
{{< /hextra/hero-headline >}}
</div>

<div class="hx-mb-12">
{{< hextra/hero-subtitle >}}
  宣言的な YAML トポロジから OpenTelemetry シグナルを生成する k6 拡張機能。&nbsp;<br class="sm:hx-block hx-hidden" />実サービスを用意せずに、マイクロサービスのグラフ・ジャーニー・障害をモデル化できます。
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx-mb-6">
{{< hextra/hero-button text="はじめる" link="getting-started" >}}
</div>

<div class="hx-mt-6"></div>

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="宣言的なトポロジ"
    subtitle="サービス間のエッジ・ジャーニー・障害を YAML で記述。実バックエンドは不要です。"
  >}}
  {{< hextra/feature-card
    title="OTLP エクスポート"
    subtitle="トレース・メトリクス・ログを OTLP/gRPC または OTLP/HTTP で、任意の Collector や SaaS エンドポイントに送信します。"
  >}}
  {{< hextra/feature-card
    title="k6 ネイティブ"
    subtitle="k6 スクリプトから合成テレメトリを生成し、otel-gen 出力で k6 の出力メトリクスを転送します。"
  >}}
{{< /hextra/feature-grid >}}
