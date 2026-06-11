# Build Instructions

本書は xk6-otel-gen project の **全 build target と手順** を集約する。

参照: 各 unit の NFR-R / NFR-D `tech-stack-decisions.md` ファイル。

---

## 1. Prerequisites

| Tool | Min version | 用途 |
|---|---|---|
| Go | 1.25+ | Go module build (全 unit) |
| xk6 | latest (k6 が pin する version) | k6 binary build with extension |
| Docker | latest stable | xk6 build container backend、integration test 用 Collector |
| kubectl | 1.27+ | examples の k8s manifest dry-run / apply |
| kind (任意) | 0.20+ | local Kubernetes cluster |
| `kustomize` (kubectl 同梱で OK) | latest | examples/*/k8s/ build |
| `golangci-lint` | latest stable | lint + SPDX header enforce |
| `lychee` (任意) | latest | README link check |

---

## 2. Go Module Build

### 2.1 Full project build

```bash
go build ./...
```

- 全 package を build
- 失敗時: missing dependency / signature mismatch / SDK version conflict 等を確認

### 2.2 Specific unit build

```bash
go build ./topology/...      # U1
go build ./journey/...       # U2
go build ./synth/...         # U3
go build ./exporter/...      # U4
go build ./k6otelgen/...     # U5
go build ./k6output/...      # U6
go build ./testutil/...      # U7
go build ./cmd/...           # U8 cmd (xk6-otel-gen-schema)
```

### 2.3 Dependency 確認

```bash
go mod tidy
go mod verify
go list -m all
```

---

## 3. xk6 Build (k6 extension binary)

### 3.1 Local build

```bash
# xk6 install (1 回のみ)
go install go.k6.io/xk6/cmd/xk6@latest

# Build k6 binary with this extension
xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.
```

- 出力: `./k6` (current directory)
- `=.` で local checkout を使用 (release tag を使う場合は `=github.com/ymotongpoo/xk6-otel-gen@v0.1.0`)

### 3.2 Build with specific Go version

```bash
xk6 build --with github.com/ymotongpoo/xk6-otel-gen=. --output ./k6
```

### 3.3 Build script for examples README

各 example の README は user に次のコマンドを示す:

```bash
xk6 build --with github.com/ymotongpoo/xk6-otel-gen
```

---

## 4. cmd/xk6-otel-gen-schema/ Build

```bash
go build -o xk6-otel-gen-schema ./cmd/xk6-otel-gen-schema
```

- 出力: `./xk6-otel-gen-schema` standalone binary
- 用途: JSON Schema export for editor integration

```bash
# Usage:
./xk6-otel-gen-schema -output schema.json
```

---

## 5. Kubernetes Manifest Build

### 5.1 minimal

```bash
kustomize build examples/minimal/k8s/ > /tmp/minimal.yaml
kubectl apply --dry-run=client -f /tmp/minimal.yaml
```

### 5.2 astroshop

```bash
kustomize build examples/astroshop/k8s/ > /tmp/astroshop.yaml
kubectl apply --dry-run=client -f /tmp/astroshop.yaml
```

### 5.3 Server-side validation (cluster available 時)

```bash
kubectl apply --dry-run=server -k examples/minimal/k8s/
kubectl apply --dry-run=server -k examples/astroshop/k8s/
```

---

## 6. Build Verification Checklist

各 commit / PR で確認:

- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` clean
- [ ] `golangci-lint run` clean (SPDX header enforce 含む)
- [ ] `xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.` succeeds
- [ ] `go build -o /tmp/xk6-otel-gen-schema ./cmd/xk6-otel-gen-schema` succeeds
- [ ] `kustomize build examples/minimal/k8s/` succeeds
- [ ] `kustomize build examples/astroshop/k8s/` succeeds

---

## 7. Build Caching (CI 最適化)

- Go module cache: `~/go/pkg/mod`
- Go build cache: `~/.cache/go-build`
- xk6 cache: `~/.xk6-cache` (xk6 設定次第)

GitHub Actions では `actions/setup-go@v5` の cache 機能を利用。

---

## 8. Reproducibility Notes

### 8.1 Pinned tool versions

- Go: 1.25 (project minimum, exact patch level は user 環境)
- OTel Go SDK: `go.opentelemetry.io/otel/*` の version は `go.mod` で固定
- k6 SDK: `go.k6.io/k6` も `go.mod` で固定

### 8.2 Container image tags (examples)

- Collector: `otel/opentelemetry-collector-contrib:0.154.0`
- Tempo: `grafana/tempo:3.0.0`
- Loki: `grafana/loki:3.7.2`
- Prometheus: `prom/prometheus:v3.12.0`
- Grafana: `grafana/grafana:13.0.2`

これらは `.github/dependabot.yml` で monthly bump、bump 時に re-deploy を再検証。

---

## 9. Common Build Issues

| Issue | Solution |
|---|---|
| `xk6: command not found` | `go install go.k6.io/xk6/cmd/xk6@latest` |
| `xk6 build` 失敗 with import cycle | local checkout を使う際は path `=.` 指定、module path mismatch を確認 |
| Docker daemon unreachable | Docker Desktop / colima / podman を起動、`docker info` で確認 |
| kustomize 失敗 with "load restrictions" | manifest 内 `../` 参照を avoid、k8s/ 内 local copy を使用 (NFR-D §3.2 参照) |
| golangci-lint goheader fail | 該当 `.go` ファイルに `// SPDX-License-Identifier: Apache-2.0` 追加 |
