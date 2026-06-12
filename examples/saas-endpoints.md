# SaaS OTLP Endpoint Configuration Examples

This document shows how to configure xk6-otel-gen to send synthesized
OpenTelemetry signals to managed OTLP backends instead of the local
LGTM-lite stack shipped under `examples/minimal/k8s/` and
`examples/astroshop/k8s/`.

Two backends are covered:

1. **Grafana Cloud** — direct OTLP gateway, HTTP/protobuf with basic auth.
2. **Google Cloud Observability** (Trace / Metrics / Logging) — via a local
   OpenTelemetry Collector sidecar that handles OAuth2 / Application
   Default Credentials before re-exporting upstream.

The k6 script side stays the same in both patterns. Only the
`otelgen.configure(...)` block (or `--out otel-gen=...` args), and any
upstream Collector config, differ.

---

## 1. Grafana Cloud (direct OTLP gateway)

Grafana Cloud exposes a single OTLP gateway endpoint per stack that accepts
traces, metrics, and logs over HTTP/protobuf with HTTP basic
authentication. xk6-otel-gen can send to it directly.

### 1.1 Get the endpoint and credentials

1. Sign in to Grafana Cloud (<https://grafana.com/>) and open your stack.
2. From the **Stack details** page, copy the **OTLP endpoint** URL. It
   looks like `https://otlp-gateway-prod-us-central-0.grafana.net/otlp`.
   The region segment (`prod-us-central-0`, `prod-eu-west-2`, …) depends
   on where your stack lives.
3. Create or reuse an **API token** with the `MetricsPublisher` role (or
   a more permissive token if you also need traces and logs).
4. Capture your **Instance ID** (also called *user* in the OTLP doc).
   It's a numeric string visible on the same details page.

### 1.2 Encode the credential

OTLP basic auth uses a single header value:

```text
Authorization: Basic base64(<instance_id>:<api_token>)
```

Compute it once, e.g.:

```bash
INSTANCE_ID=1234567
API_TOKEN=glc_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
echo -n "${INSTANCE_ID}:${API_TOKEN}" | base64
# → MTIzNDU2NzpnbGNfeHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eA==
```

Treat this string as a secret. Pass it to k6 via an environment variable
rather than committing it.

### 1.3 k6 script configuration

```javascript
import otelgen from "k6/x/otel-gen";

export const options = {
  vus: 10,
  duration: "30s",
};

export function setup() {
  otelgen.configure({
    endpoint: __ENV.GRAFANA_OTLP_ENDPOINT,   // https://otlp-gateway-...grafana.net/otlp
    protocol: "http",
    insecure: false,
    headers: {
      Authorization: `Basic ${__ENV.GRAFANA_OTLP_BASIC_AUTH}`,
    },
    timeout: "30s",
    batchSize: 512,
    batchTimeout: "5s",
    maxQueueSize: 4096,
  });
}

export default function () {
  // load() must run here, not in setup(): k6 JSON-serializes setup()
  // return values, which strips the handle's methods. The parsed topology
  // is cached, so per-iteration calls add no overhead.
  const topology = otelgen.load("./topology.yaml");
  topology.runJourney("checkout");
}
```

### 1.4 Run

```bash
export GRAFANA_OTLP_ENDPOINT="https://otlp-gateway-prod-us-central-0.grafana.net/otlp"
export GRAFANA_OTLP_BASIC_AUTH="MTIzNDU2NzpnbGNfeHh4eHh4..."

./k6 run \
  --out "otel-gen=endpoint=${GRAFANA_OTLP_ENDPOINT},protocol=http,headers=Authorization:Basic ${GRAFANA_OTLP_BASIC_AUTH}" \
  examples/minimal/script.js
```

The `--out` flag mirrors the JS configuration so you can use either path.
The JS API takes precedence (see `## Configuration` in the project
README).

### 1.5 Verify

In Grafana Cloud, open **Explore** and select the **Traces** datasource
(Tempo) — you should see spans from the simulated services within a
minute. Metrics (Mimir) and logs (Loki) populate similarly.

If nothing shows up:

- Use the base gateway URL ending in `/otlp`. For HTTP, xk6-otel-gen
  appends `/v1/traces`, `/v1/metrics`, `/v1/logs` automatically per the
  OTLP exporter spec — do not append those manually. If you need a signal
  to go elsewhere, set `tracesEndpoint` / `metricsEndpoint` / `logsEndpoint`
  (or the `OTEL_EXPORTER_OTLP_{SIGNAL}_ENDPOINT` env var), which are used
  as-is.
- `protocol: "http"` is required because Grafana Cloud's OTLP gateway is
  HTTP/protobuf. The gRPC endpoint is hosted on a different domain — use
  it only if you have a contract specifying gRPC.
- Check `otelgen.stats()` from `teardown()` (or another JS hook) — if
  `tracesFailed` is non-zero, the export is being rejected. Most often
  the cause is a malformed `Authorization` header (whitespace, missing
  `Basic` prefix, or the wrong base64 alphabet).

---

## 2. Google Cloud Observability (via local Collector)

Google Cloud Observability — Trace, Metrics, Logging — accepts OTLP, but
authentication uses OAuth2 with Application Default Credentials. The
k6 process is not the right place to hold long-lived service-account
credentials, and the GCP OTLP endpoint requires gRPC with TLS plus an
OAuth2 bearer that is re-minted every hour.

**Recommended pattern**: keep xk6-otel-gen pointed at a local
OpenTelemetry Collector, and have that Collector re-export to GCP using
its dedicated `googlecloud` exporter. The k6 side stays portable and the
credentials stay on whatever node is running the Collector.

### 2.1 Cluster setup

Provision a service account that can write telemetry:

```bash
PROJECT_ID="your-gcp-project"

gcloud iam service-accounts create xk6-otel-gen-writer \
  --project="${PROJECT_ID}" \
  --display-name="xk6-otel-gen telemetry writer"

# Grant write roles. Adjust to the subset of signals you actually use.
for role in \
    roles/cloudtrace.agent \
    roles/monitoring.metricWriter \
    roles/logging.logWriter
do
  gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
    --member="serviceAccount:xk6-otel-gen-writer@${PROJECT_ID}.iam.gserviceaccount.com" \
    --role="${role}"
done

# Create a key (rotate regularly).
gcloud iam service-accounts keys create ./gcp-sa-key.json \
  --iam-account="xk6-otel-gen-writer@${PROJECT_ID}.iam.gserviceaccount.com"
```

Store `gcp-sa-key.json` as a Kubernetes secret so the Collector pod can
read it from a mount path:

```bash
kubectl -n xk6-otel-gen-demo create secret generic gcp-sa-key \
  --from-file=key.json=./gcp-sa-key.json
```

### 2.2 Collector configuration

Replace `examples/minimal/k8s/otel-collector-config.yaml` (or fork it
into `examples/minimal-gcp/`) with the following. The receiver section
is identical to the local stack — only the exporters and pipelines
change.

```yaml
receivers:
  otlp:
    protocols:
      grpc: { endpoint: 0.0.0.0:4317 }
      http: { endpoint: 0.0.0.0:4318 }

processors:
  batch: {}
  resourcedetection:
    detectors: [env, gcp]
    timeout: 5s
    override: false

exporters:
  googlecloud:
    project: ${env:GOOGLE_CLOUD_PROJECT}
    # Trace uses the project's Cloud Trace bucket by default.
    # Metric and log paths are also handled by the same exporter.
    log:
      default_log_name: xk6-otel-gen

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch, resourcedetection]
      exporters: [googlecloud]
    metrics:
      receivers: [otlp]
      processors: [batch, resourcedetection]
      exporters: [googlecloud]
    logs:
      receivers: [otlp]
      processors: [batch, resourcedetection]
      exporters: [googlecloud]
```

The `googlecloud` exporter is part of the
`otel/opentelemetry-collector-contrib` image, which is already pinned
by the examples.

### 2.3 Collector Deployment patch

Patch the Collector Deployment so it mounts the service-account key and
sets `GOOGLE_APPLICATION_CREDENTIALS`:

```yaml
# kustomize patch for examples/minimal-gcp/k8s/collector.yaml
spec:
  template:
    spec:
      containers:
        - name: otel-collector
          env:
            - name: GOOGLE_APPLICATION_CREDENTIALS
              value: /var/secrets/google/key.json
            - name: GOOGLE_CLOUD_PROJECT
              value: your-gcp-project
          volumeMounts:
            - name: gcp-sa-key
              mountPath: /var/secrets/google
              readOnly: true
      volumes:
        - name: gcp-sa-key
          secret:
            secretName: gcp-sa-key
```

If you run on GKE or GCE with Workload Identity / metadata server, you
can drop both the secret mount and the `GOOGLE_APPLICATION_CREDENTIALS`
env var — the Collector picks up the metadata-server token
automatically.

### 2.4 k6 script

No vendor-specific changes. The script keeps pointing at the local
Collector:

```javascript
import otelgen from "k6/x/otel-gen";

export const options = { vus: 10, duration: "30s" };

export function setup() {
  otelgen.configure({
    endpoint: "localhost:4317",
    protocol: "grpc",
    insecure: true,
  });
}

export default function () {
  const topology = otelgen.load("./topology.yaml");
  topology.runJourney("checkout");
}
```

### 2.5 Run

```bash
# Forward the in-cluster Collector to localhost:4317
kubectl -n xk6-otel-gen-demo port-forward svc/otel-collector 4317:4317 &

./k6 run examples/minimal/script.js \
  --out "otel-gen=endpoint=localhost:4317,protocol=grpc,insecure=true"
```

### 2.6 Verify

- In Cloud Console, open **Trace explorer** for your project. Spans
  appear under `service.name=<simulated service>` and
  `service.name=xk6-otel-gen-runner` (the k6 runner's own metrics).
- For metrics, open **Cloud Monitoring → Metrics Explorer** and search
  for `workload.googleapis.com/k6.*` (k6 native counters) or any of the
  semantic-convention names emitted by the simulated services.
- For logs, open **Logs Explorer** and filter
  `logName=projects/${PROJECT_ID}/logs/xk6-otel-gen` (the
  `default_log_name` set in the exporter).

Common failure modes:

- `PermissionDenied` from the exporter → the service account is missing
  one of `cloudtrace.agent`, `monitoring.metricWriter`,
  `logging.logWriter`.
- `ResourceExhausted` → Cloud Trace ingestion quota per minute. Reduce
  `vus`, increase `batchTimeout`, or contact GCP to raise the quota.
- Metrics not appearing → check `resourcedetection` is enabled. Without
  it, GCP rejects time series that lack a known monitored-resource type.

---

## 3. Cost guard rails

Both SaaS backends charge per ingested data point, span, or log line.
With xk6-otel-gen you can easily emit hundreds of thousands of spans in
a single load test. Recommended guard rails before pointing at a paid
backend:

- Set `options.duration` and `options.vus` deliberately. A 30-second,
  10-VU run is roughly 200–1000 spans per second per service. Multiply
  by your simulated topology depth to estimate the bill.
- Run the same script against the local LGTM-lite stack first to catch
  configuration errors that would otherwise burn ingest quota.
- For Grafana Cloud, watch the **Billing** dashboard during the run —
  the meter updates in near-real time.
- For Google Cloud, set a Budget alert on Cloud Trace, Cloud Monitoring,
  and Cloud Logging line items before scaling tests above a development
  size.

---

## 4. Switching back to the local stack

To return to the local Tempo/Loki/Prometheus/Grafana stack shipped under
`examples/minimal/k8s/` or `examples/astroshop/k8s/`:

- Replace the SaaS endpoint URL with `localhost:4317`
  (`protocol: "grpc"`, `insecure: true`).
- Re-apply the unmodified Collector ConfigMap so the file_exporter /
  Tempo / Loki / Prometheus targets are restored.
- Remove any vendor-specific environment variables and secrets.
