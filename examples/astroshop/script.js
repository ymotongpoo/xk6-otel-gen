import otelgen from "k6/x/otel-gen";

export const options = {
  scenarios: {
    browse: {
      executor: "constant-vus",
      vus: 20,
      duration: "60s",
      exec: "browse",
    },
    checkout: {
      executor: "constant-vus",
      vus: 5,
      duration: "60s",
      exec: "checkout",
    },
  },
};

export function setup() {
  otelgen.configure({
    endpoint: "localhost:4317",
    protocol: "grpc",
    insecure: true,
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
  // Pipeline shutdown is handled by the otel-gen k6 output Stop hook.
}
