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

  const topology = otelgen.load("./topology.yaml");
  return { topology };
}

export default function (data) {
  data.topology.runJourney("checkout");
}

export function teardown() {
  // Pipeline shutdown is handled by the otel-gen k6 output Stop hook.
}
