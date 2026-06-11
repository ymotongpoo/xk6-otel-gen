# Integration Test Instructions

本書は xk6-otel-gen project の **integration test (cross-unit + Docker-based)** を集約する。

参照: 各 unit の NFR-R / NFR-D の integration section。

---

## 1. Integration Test Scope

各 unit に独立した integration test harness を配置:

| Unit | Path | Build tag | 内容 |
|---|---|---|---|
| U4 | `exporter/integration/` | `-tags=integration` | Pipeline → Collector OTLP → file_exporter file 読み取り、3-signal correlation |
| U3 | `synth/integration/` | `-tags=integration` | synth → U4 Pipeline → Collector、trace_id correlation 確認 |
| U2 | `journey/integration/` | `-tags=integration` | Engine → synth → U4 Pipeline → Collector、cascade pattern + recovery 確認 |
| U5 | `k6otelgen/integration/` | `-tags=integration` | xk6 build → 実 k6 run → Collector、JS module の `runJourney` flow |
| U6 | `k6output/integration/` | `-tags=integration` | xk6 build → 実 k6 run with `--out otel-gen=...` → k6.* metric が Collector に届く |

U1 / U7 / U8 は integration test なし (U7 は generator quality を他 unit が利用、U8 は static deliverable)。

---

## 2. Prerequisites for Integration Tests

| Tool | 用途 |
|---|---|
| Docker Engine | Collector container 起動 |
| `docker compose` | (U3/U4 の test) |
| `xk6` | (U5/U6 の test) k6 binary build |
| `kind` (任意) | k6output integration の full Kubernetes test |

各 test の helper (`integration/helpers.go`) は **prerequisite が無ければ skip** する設計:
- `requireDocker(t)` — Docker daemon が unreachable なら skip
- `requireXK6(t)` — xk6 が PATH にないなら skip

→ CI 環境で必要 tools を install すれば実行、ローカル開発で tools 無くても`go test` で fail しない。

---

## 3. Running Integration Tests

### 3.1 All integration tests

```bash
go test -tags=integration -race -count=1 ./...
```

### 3.2 Per-unit

```bash
go test -tags=integration ./exporter/integration/...
go test -tags=integration ./synth/integration/...
go test -tags=integration ./journey/integration/...
go test -tags=integration ./k6otelgen/integration/...
go test -tags=integration ./k6output/integration/...
```

### 3.3 Default `go test` で skip 確認

```bash
go test ./...
```

`-tags=integration` なしでは `//go:build integration` ガードにより integration test は build されない (file 自体が compile 対象外)。

---

## 4. Docker Collector Lifecycle

各 integration test は per-test Collector を起動:

```text
1. testStart:
   - helpers.StartCollector(t, configDir) — docker compose up
   - wait for OTLP port (4317) ready
2. test body:
   - Pipeline.New(...) で OTLP に送信
   - ForceFlush + Shutdown
3. helpers.ReadCollectorXxx(t) — file_exporter 出力 JSON を読み取り
4. assert
5. testEnd:
   - cleanup() — docker compose down
```

Collector image (各 integration test で pinned):
- `otel/opentelemetry-collector-contrib:0.154.0` (現時点)

---

## 5. xk6 Integration Tests (U5 / U6)

### 5.1 xk6 build inline in test

```go
// In integration/helpers.go
func buildK6Binary(t *testing.T, xk6Path, modulePath, outputDir string) string {
    binPath := filepath.Join(outputDir, "k6")
    cmd := exec.Command(xk6Path, "build", "--with", modulePath+"=.")
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    require.NoError(t, cmd.Run())
    return binPath
}
```

- xk6 が PATH にあれば実行、無ければ skip
- 出力 binary は `t.TempDir()` 配下

### 5.2 k6 run shell out

```go
cmd := exec.Command(k6Bin, "run", scriptPath, "--out", "otel-gen=endpoint="+collectorAddr+",insecure=true")
output, _ := cmd.CombinedOutput()
require.Equal(t, 0, cmd.ProcessState.ExitCode())
```

### 5.3 Collector output verification

各 test は `/var/log/otel/{traces,metrics,logs}.json` (Collector の file_exporter 出力) を読み取って:
- expected service.name が出現するか
- expected trace_id が複数 signal に出現 (correlation)
- error.type / k6.* attribute が正しく付与されているか

---

## 6. CI Integration Test Workflow (推奨)

`.github/workflows/integration.yml` (Build and Test stage で配置):

```yaml
name: Integration Tests

on:
  schedule:
    - cron: '0 4 * * *'  # nightly
  workflow_dispatch:

jobs:
  integration:
    runs-on: ubuntu-latest
    timeout-minutes: 30
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
          cache: true
      - name: Install xk6
        run: go install go.k6.io/xk6/cmd/xk6@latest
      - name: Pull Collector image
        run: docker pull otel/opentelemetry-collector-contrib:0.154.0
      - name: Run integration tests
        run: go test -tags=integration -race -count=1 -timeout=20m ./...
```

- nightly cron で重い integration test を実行
- PR 上では unit test のみ run
- 手動 trigger (`workflow_dispatch`) でも実行可

---

## 7. Cross-unit Correlation Test

U2 → U3 → U4 → U6 の full pipeline を verify する E2E test。U5/U6 の integration test 内で実施:

```text
journey.Execute → synth signals → exporter.Pipeline → Collector file_exporter
                                                    ↓
                                             k6 native metrics
                                                    ↓
                                             k6output.AddMetricSamples
                                                    ↓
                                             Same Pipeline → Collector
```

verify:
- 同じ trace_id が trace + metric + log の 3 signal で表現される (U3 synth が `synth.cascaded` attribute 含む)
- `service.name="<simulated service>"` (synth) と `service.name="xk6-otel-gen-runner"` (k6 native) が 別 Resource で OTLP 経由
- cascade test pattern が trace に表現される (U2 cascade emit)

---

## 8. Examples k8s Integration (cluster-available CI 環境)

`kubectl apply --dry-run=server -k examples/*/k8s/` を CI で実行:

```yaml
- name: Set up kind cluster
  uses: helm/kind-action@v1
  with:
    cluster_name: xk6-otel-gen-test
    version: v0.20.0
- name: Validate manifests
  run: |
    kubectl apply --dry-run=server -k examples/minimal/k8s/
    kubectl apply --dry-run=server -k examples/astroshop/k8s/
```

cluster 起動が重いので nightly のみ。client-side dry-run は PR 上で実施。

---

## 9. Skip 動作 (graceful degradation)

整理:

| Prerequisite missing | 挙動 |
|---|---|
| Docker daemon | `t.Skip("docker unavailable")` |
| `xk6` not in PATH | `t.Skip("xk6 not installed")` |
| `kind` not installed (k8s test) | `t.Skip("kind not available")` |

→ CI で nightly job が tools 不在で fail しないよう、SKIP の判定が conservative。

---

## 10. Common Issues

| Issue | Solution |
|---|---|
| `docker compose up` で port conflict | `lsof -i :4317` で確認、過去の test container を clean up |
| Collector file_exporter が空 | ForceFlush + Shutdown を test 内で明示呼び出し、append-oriented file の race を回避 |
| xk6 build slow on CI | actions cache `~/.xk6-cache` を活用、build time を分摊 |
| k6 process が exit せず hang | --duration / --max-iterations を script に設定、CI で timeout 設定 |
| trace_id correlation fail | context.Context 経由の trace 伝搬が機能しているか確認、`go.opentelemetry.io/otel/propagation` 非使用前提 |
