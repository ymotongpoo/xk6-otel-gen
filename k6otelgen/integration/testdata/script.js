import otelgen from "k6/x/otel-gen";

export const options = {
  vus: 1,
  iterations: 1,
};

const topology = otelgen.load("./topology.yaml");

otelgen.configure({
  endpoint: __ENV.OTEL_ENDPOINT || "localhost:4317",
  protocol: "grpc",
  insecure: true,
  batchSize: 1,
  batchTimeout: "100ms",
  maxQueueSize: 8,
});

export default function () {
  topology.runJourney("home");
}
