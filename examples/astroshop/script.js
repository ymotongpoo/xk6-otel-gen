import otelgen from "k6/x/otel-gen";

export const options = {
  scenarios: {
    // Rate-based executors cap how fast journeys are generated. astroshop
    // journeys are larger (many spans each), so the rates are lower than the
    // minimal example to keep total backend volume near a ~1000 telemetry/s
    // budget. Without a rate cap, k6 runs at full CPU speed, overflowing the
    // exporter queue and dropping spans — most often the root span, which is
    // enqueued last. See the README "Throughput, batching, and dropped root
    // spans" section.
    browse: {
      executor: "constant-arrival-rate",
      rate: 150,
      timeUnit: "1s",
      duration: "60s",
      preAllocatedVUs: 30,
      maxVUs: 150,
      exec: "browse",
    },
    checkout: {
      executor: "constant-arrival-rate",
      rate: 50,
      timeUnit: "1s",
      duration: "60s",
      preAllocatedVUs: 20,
      maxVUs: 100,
      exec: "checkout",
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

// Call load() inside the iteration functions, not in setup(): k6
// JSON-serializes setup() return values, which strips the handle's methods.
// load() parses the YAML only once and returns the cached handle afterwards.

export function browse() {
  const topology = otelgen.load("./topology.yaml");
  topology.runRandomJourney();
}

export function checkout() {
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
