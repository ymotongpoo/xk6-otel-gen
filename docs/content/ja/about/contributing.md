---
title: コントリビュート
weight: 2
---

変更は修正対象のパッケージやサンプルに限定し、関連するテストを実行し、コミットメッセージ
には Conventional Commits を使用してください。

```bash
go test ./...
go test -race -count=1 ./...
golangci-lint run
```

| 変更の種類 | 期待されるチェック |
|---|---|
| トポロジパーサー | `go test ./topology/...` |
| ジャーニーエンジン | `go test ./journey/...` |
| サンプル | `go test ./test/examples/...` と `kustomize build examples/<name>/k8s/` |
| k6 連携 | `xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.` |
