# U5 k6otelgen — Tech Stack Decisions

本書は U5 (`k6otelgen/`) が依存するパッケージ・採用された代替案・却下された案を確定する。

---

## 1. 依存モジュール (Production code)

### 1.1 採用一覧

| Module | Version | 用途 | 必要性 |
|---|---|---|---|
| `go.k6.io/k6/js/modules` | latest stable | k6 module SDK (RootModule / ModuleInstance / VU / Exports) | 必須 |
| `github.com/grafana/sobek` | k6 推移依存 | JS runtime (value 変換 / FunctionCall / Object construction) | 必須 (k6 が pin、direct import 可) |
| `github.com/ymotongpoo/xk6-otel-gen/topology` | (local) | Schema / Validate / ApplyFaults / ConfigError | 必須 |
| `github.com/ymotongpoo/xk6-otel-gen/exporter` | (local) | Config / ConfigFromEnv / GetShared / Pipeline / Stats / ConfigError | 必須 |
| `github.com/ymotongpoo/xk6-otel-gen/synth` | (local) | NewDefault / Synthesizer | 必須 |
| `github.com/ymotongpoo/xk6-otel-gen/journey` | (local) | NewEngine / BuildPlan / Execute / PlanError / ExecuteError | 必須 |
| stdlib `sync`, `os`, `path/filepath`, `time` | Go 1.25 | sync.Once, file load, path resolution | 必須 |

### 1.2 採用しないモジュール (production)

- `go.opentelemetry.io/otel/*` — synth / exporter 経由のみ参照
- 直接 sobek runtime 構築 (`sobek.New()`) — k6 SDK 経由のみ
- 別 JS engine (goja 等) — k6 が sobek を採用 (近年 goja → sobek 移行)、合わせる

### 1.3 検証

`go list -deps ./k6otelgen/...` の出力に許可外の `go.opentelemetry.io/otel/*` direct import が含まれないことを CI で sanity check。

---

## 2. テスト依存 (Test-only)

| Module | Version | 用途 |
|---|---|---|
| `pgregory.net/rapid` | latest stable | PBT (TP-U5-1〜3) |
| `github.com/stretchr/testify` | latest stable | assertion |
| `go.k6.io/k6/js/modulestest` (or `modulestest`) | k6 推移依存 | sobek runtime + modules.VU mock |
| `github.com/ymotongpoo/xk6-otel-gen/testutil/generators` | (local) | ValidConfigureOpts / ValidLoadPath / ValidService 等 |

### 2.1 JS runtime test strategy (Q10=A)

`modulestest.NewRuntime(t)` で sobek runtime + modules.VU を構築。U5 internal API を test runtime 経由で実行:

```go
func TestLoad_HappyPath(t *testing.T) {
    t.Parallel()
    rt := modulestest.NewRuntime(t)
    err := rt.SetupModuleSystem(map[string]any{
        "k6/x/otel-gen": k6otelgen.New(),
    }, ...)
    require.NoError(t, err)

    _, err = rt.RunOnEventLoop(`
        const otelgen = require("k6/x/otel-gen");
        otelgen.load("./testdata/topology.yaml");
    `)
    require.NoError(t, err)
}
```

### 2.2 Mock 戦略

- **synth.Synthesizer mock**: U2 で作成した `mockSynth` (helpers_test.go) を再利用は不可 (test package 内なので外から見えない) → U5 内で同等の mock を作成
- **journey.Engine の挙動 verify**: BuildPlan が呼ばれたか / Execute に正しい ctx が渡るかは U5 mock で確認、Engine 実装は U2 で test 済
- 必要なら U5 内 `*RootModule` の field を test-only injection で書き換え (NFR Design で具体策確定)

---

## 3. Integration test 依存

`k6otelgen/integration/` で使用:
- **xk6** — k6 binary を本拡張込みで build するツール
- Docker Engine + `docker compose`
- `otel/opentelemetry-collector-contrib:<pinned-tag>` (U2/U3/U4 と同 tag)
- 実 `k6 run --out otel-gen=... script.js` 実行
- Collector の file_exporter 出力を assert

`-tags=integration` build tag で default `go test` から除外。

---

## 4. Version 戦略

### 4.1 k6 SDK version pinning

- `go.k6.io/k6` は **stable major** を pin
- k6 minor / patch update に dependabot で追従
- breaking change (major) があれば手動対応 phase を別 PR で

### 4.2 sobek

k6 が pin する version に従う (direct dependency にすることも可能だが、必要性が出るまでは transitive のみ)。

### 4.3 Go toolchain

- `go.mod`: `go 1.25`
- U1〜U4 と整合

### 4.4 transitive dependency

`go mod tidy` で管理、`replace` 不使用、vendor なし。

---

## 5. 代替案 (Rejected)

### 5.1 goja 直接採用 (rejected)

- 案: `github.com/dop251/goja` を直接 import して JS evaluation
- 却下理由: k6 が sobek に移行済 (goja から fork した sobek が k6 標準)。sobek を採用しないと k6 module SDK と signature 不一致

### 5.2 Pipeline を JS-side で持つ (rejected)

- 案: `otelgen.pipeline = exporter.GetShared(...)` のような露出
- 却下理由: U4 の責務、U5 から JS 経由で pipeline 操作する API は無意味 (Shutdown は U6 が担当)

### 5.3 全方向の Path traversal check (rejected)

- 案: `load(path)` で `filepath.Clean` + `..` 拒否 + symlink follow チェック
- 却下理由: Q8=A で k6 SDK sandbox に委譲、重複は冗長。security boundary は k6 binary レベル

### 5.4 JS API を class-based (rejected)

- 案: `new otelgen.Otelgen(opts)` のような JS class
- 却下理由: k6 慣例は function-based + handle pattern。class-based は k6 user の expectation と齟齬

### 5.5 Configure を複数回許可 (rejected)

- 案: `Configure` を merge で複数回呼べる
- 却下理由: state 確定 timing が曖昧、test difficult。Q5=A で single-shot 採用

### 5.6 Self-metric を atomic counter で持つ (rejected)

- 案: `RootModule.Stats{LoadCalls, ConfigureCalls, RunJourneyCalls}` を atomic counter
- 却下理由: Q6=A、U2/U3 と同方針で frontend layer は薄く保つ

### 5.7 Pipeline shutdown を teardown() JS-side で明示 (rejected)

- 案: JS の teardown function で `otelgen.shutdown()` を呼ぶ
- 却下理由: Q8=A、U6 Output.Stop が k6 lifecycle で正しい timing、JS user に shutdown を意識させない

---

## 6. CI / Lint 統合

### 6.1 必須 CI ジョブ

| ジョブ | コマンド | DoD blocking? |
|---|---|---|
| Build | `go build ./k6otelgen/...` | Yes |
| Unit test (race) | `go test -race -count=1 ./k6otelgen/...` | Yes |
| Coverage | `go test -cover ./k6otelgen/...` ≥ 80% | Yes |
| Lint | `golangci-lint run ./k6otelgen/...` | Yes |
| `go vet` | `go vet ./k6otelgen/...` | Yes |
| Bench (regression) | `go test -bench=. ./k6otelgen/...` | informational (NFR-U5-6 は soft) |
| Integration (xk6 + Docker) | `go test -tags=integration ./k6otelgen/integration/...` | nightly + manual trigger |

### 6.2 lint rules

`.golangci.yml` で project 共通設定 (U2/U3/U4 と同じ):
- `revive` (GoDoc 網羅)
- `govet`
- `staticcheck`
- `errcheck`
- `unused`

---

## 7. Cross-unit dependency summary

```text
U5 (k6otelgen) imports:
  - go.k6.io/k6/js/modules (k6 SDK)
  - github.com/grafana/sobek (JS runtime, optional direct)
  - github.com/ymotongpoo/xk6-otel-gen/{topology,exporter,synth,journey}
  - stdlib (sync, os, path/filepath, time)

U5 does NOT import:
  - go.opentelemetry.io/otel/*  (synth/exporter 経由)
  - dop251/goja                  (sobek 採用)

U5 is imported by:
  - cmd/xk6 (build target for U8)  — k6 binary build に link される
  - k6otelgen/integration/         — integration test
```

---

## 8. Migration / Upgrade Notes

### 8.1 k6 SDK major upgrade

- `go.k6.io/k6/js/modules.Module` / `modules.Instance` interface 変更時、`module.go` / `instance.go` の signature 更新
- breaking change を release notes で確認、test green 維持

### 8.2 sobek API 変更

- sobek の `runtime.Runtime` 等の API 変更時、`instance.go` の jsXxx wrapper を修正
- k6 SDK が sobek を抽象化しているので影響は小さいはず

### 8.3 topology / synth / journey / exporter interface 変更

- 各 unit の SemVer に従う (post-v1)
- U5 は薄い frontend なので modification も小さい

---

## 9. Open questions for Future revisit

| 質問 | 想定 trigger |
|---|---|
| Multiple topology 同時 load サポート | user demand があれば (現状 1 topology) |
| `otelgen.shutdown()` JS API 明示提供 | `--out` flow に依存しない使い方が増えたら |
| Configure の partial merge (複数回呼び出し) | static config が JS-side で動的変更したい用途が出たら |
| JS-side observability hook (event subscription) | journey 実行 progress を JS 側で受け取る要望が出たら |
