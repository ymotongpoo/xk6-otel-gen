---
title: xk6-otel-gen
layout: hextra-home
# type: docs makes Hextra root the left sidebar at the site home so every
# top-level section (getting-started, usage, reference, examples, about) is
# listed on every docs page. Without it, the sidebar only shows the current
# page's own section. The hextra-home layout still renders the landing hero.
type: docs
# cascade type:docs to every descendant page so leaf pages (e.g.
# reference/configuration) use Hextra's layouts/docs/single.html — which renders
# the left sidebar — instead of the default layouts/single.html, which disables
# it. This is what kept the sidebar from appearing on individual article pages.
cascade:
  type: docs
---

{{< hextra/hero-badge >}}
  <div class="hx-w-2 hx-h-2 hx-rounded-full hx-bg-primary-400"></div>
  Apache-2.0 · Go 1.25+
{{< /hextra/hero-badge >}}

<div class="hx-mt-6 hx-mb-6">
{{< hextra/hero-headline >}}
  Synthesize OpenTelemetry&nbsp;<br class="sm:hx-block hx-hidden" />traces, metrics, and logs
{{< /hextra/hero-headline >}}
</div>

<div class="hx-mb-12">
{{< hextra/hero-subtitle >}}
  A k6 extension that generates OpenTelemetry signals from a declarative&nbsp;<br class="sm:hx-block hx-hidden" />YAML topology — model microservice graphs, journeys, and faults without real services.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx-mb-6">
{{< hextra/hero-button text="Get Started" link="getting-started" >}}
</div>

<div class="hx-mt-6"></div>

- **Declarative topology** — model service edges, journeys, and faults in YAML. No real backends required.
- **OTLP egress** — send traces, metrics, and logs over OTLP/gRPC or OTLP/HTTP to any collector or SaaS endpoint.
- **k6-native** — drive synthetic telemetry from k6 scripts and forward k6 output metrics through the otel-gen output.
