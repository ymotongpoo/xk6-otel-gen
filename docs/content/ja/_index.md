---
title: xk6-otel-gen
# type: docs makes this home page render with Hextra's docs layout (sidebar +
# TOC), and roots the sidebar at the site home so every top-level section is
# listed on every page.
type: docs
# cascade type:docs to all descendants so every page uses the docs layout.
cascade:
  type: docs
---

`xk6-otel-gen` は、宣言的な YAML トポロジから OpenTelemetry のトレース・メトリクス・ログを
合成する [k6](https://k6.io/) 拡張機能です。実際のマイクロサービスを構築・デプロイすること
なく、サービス構成・ユーザージャーニー・障害を記述するだけで、相関の取れた OpenTelemetry
シグナルを生成し、任意の Collector やバックエンドへ送信できます。

## 解決する課題

オブザーバビリティのパイプライン、バックエンド、ダッシュボード、アラートを検証するには、
通常テレメトリを生成する実サービス群が必要です。しかし、そのためだけに多数のマイクロ
サービスを立ち上げ、負荷をかけ、現実的な障害を再現するのは、手間もコストもかかります。

`xk6-otel-gen` は、この「テレメトリの送り手」を合成で置き換えます。YAML でサービスのグラフ
とユーザージャーニーを宣言すれば、k6 がそれを実行して、相関の取れたトレース・メトリクス・
ログを生成します。実サービスは不要です。

## 主な特徴

- **宣言的なトポロジ** — サービス間のエッジ・ジャーニー・障害を YAML で記述。実バックエンドは不要です。
- **相関の取れたシグナル** — トレース・メトリクス・ログを、トレースコンテキストを保ったまま生成します。
- **障害注入** — レイテンシ増加・エラー率の上書き・接続断・クラッシュを確率的に再現できます。
- **OTLP エクスポート** — OTLP/gRPC・OTLP/HTTP で、任意の Collector や SaaS（Grafana Cloud など）へ送信します。
- **k6 ネイティブ** — k6 のエグゼキューターで生成レートやスケールを制御し、k6 の出力メトリクスも転送できます。

## こんな用途に

- オブザーバビリティ・バックエンド（Tempo、Prometheus、Loki、Grafana など）の検証やデモ。
- Collector のパイプラインやサンプリング設定の動作確認。
- 実サービスなしで、現実的なデータを使ってダッシュボードやアラートを作り込む。
- バックエンドの取り込み能力やスケールのベンチマーク。

## 仕組み

1. サービス・ジャーニー・障害を 1 つの YAML トポロジに記述します。
2. この拡張機能を組み込んだ k6 バイナリをビルドします。
3. k6 スクリプトからトポロジを読み込み、ジャーニーを実行します。各ジャーニーの実行が
   1 本のトレース（と関連するメトリクス・ログ）になります。
4. テレメトリは OTLP で Collector やバックエンドへ送信されます。

## 次のステップ

- [はじめに]({{< relref "/getting-started" >}}) — 機能の概要と最初の実行。
- [クイックスタート]({{< relref "/getting-started/quick-start" >}}) — k6 をビルドして合成トラフィックを流す。
- [トポロジ YAML リファレンス]({{< relref "/reference/topology" >}}) — 設定可能な全項目。
- [設定]({{< relref "/reference/configuration" >}}) — エクスポーター/k6 の全オプション。
