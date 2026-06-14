---
title: Security
weight: 1
---

This project distributes source code and examples, not prebuilt k6 binaries.
Build your own k6 binary with xk6 so the final artifact is produced in your
environment from audited inputs.

| Security choice | Rationale |
|---|---|
| No prebuilt binary | Avoids asking users to trust an opaque load-testing executable |
| Pinned demo images | Kubernetes examples use explicit image tags |
| Synthetic data only | Examples do not require production credentials or user data |
| OTLP TLS options | Configure secure endpoints through JS options or environment variables |

Example production-style endpoint:

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

The certificate files are read during pipeline validation and startup so
missing files, malformed PEM data, incomplete client certificate/key pairs, and
certificate options combined with `insecure: true` fail before traffic starts.
Header values are never included in JS-module configuration logs.
