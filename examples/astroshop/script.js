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

  const topology = otelgen.load("./topology.yaml");
  return { topology };
}

export function browse(data) {
  data.topology.runRandomJourney();
}

export function checkout(data) {
  data.topology.runJourney("checkout");
}

export function teardown() {
  // Pipeline shutdown is handled by the otel-gen k6 output Stop hook.
}
