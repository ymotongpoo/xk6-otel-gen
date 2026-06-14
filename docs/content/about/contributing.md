---
title: Contributing
weight: 2
---

Keep changes scoped to the package or example being modified, run the relevant
tests, and use Conventional Commits for commit messages.

```bash
go test ./...
go test -race -count=1 ./...
golangci-lint run
```

| Change type | Expected check |
|---|---|
| Topology parser | `go test ./topology/...` |
| Journey engine | `go test ./journey/...` |
| Examples | `go test ./test/examples/...` and `kustomize build examples/<name>/k8s/` |
| k6 integration | `xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.` |
