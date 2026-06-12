import otelgen from "k6/x/otel-gen";

export const options = {
  vus: 10,
  duration: "30s",
};

export function setup() {
  otelgen.configure({
    endpoint: "localhost:4317",
    protocol: "grpc",
    insecure: true,
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
  // Pipeline shutdown is handled by the otel-gen k6 output Stop hook.
}
