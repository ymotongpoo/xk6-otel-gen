import otelgen from "k6/x/otel-gen";

export const options = {
  scenarios: {
    checkout: {
      // Cap the generation rate. Without a rate-based executor, k6 runs the
      // journey at full CPU speed (10k+ iterations/s), which floods the
      // exporter queue: spans are dropped — most often the root span, which
      // ends last and is enqueued last — and Tempo shows
      // "<root span not yet received>". See the README "Throughput, batching,
      // and dropped root spans" section.
      executor: "constant-arrival-rate",
      // The checkout journey emits 3 spans per iteration, so 300 iterations/s
      // is ~900 spans/s — within a conservative ~1000 telemetry/s budget.
      rate: 300,
      timeUnit: "1s",
      duration: "30s",
      preAllocatedVUs: 20,
      maxVUs: 100,
    },
  },
};

export function setup() {
  otelgen.configure({
    endpoint: "localhost:4317",
    protocol: "grpc",
    insecure: true,
    // Generous batch/queue headroom so transient bursts do not drop spans.
    // Defaults are batchSize 512, batchTimeout 1s, maxQueueSize 2048.
    batchSize: 2048,
    batchTimeout: "1s",
    maxQueueSize: 16384,
  });
}

export default function () {
  // Call load() here, not in setup(): k6 JSON-serializes setup() return
  // values, which strips the handle's methods. load() parses the YAML only
  // once and returns the cached handle on subsequent calls.
  const topology = otelgen.load("./topology.yaml");
  topology.runJourney("checkout");
}

export function teardown() {
  // Flush queued telemetry — notably each trace's root span, which ends last
  // and would otherwise be dropped at process exit. This makes trace delivery
  // independent of whether the otel-gen output is enabled; when it is, the
  // output's Stop hook still performs the final pipeline shutdown.
  otelgen.flush();
}
