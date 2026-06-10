import http from "k6/http";
import { sleep } from "k6";
import otelgen from "k6/x/otel-gen";

export const options = {
  vus: 1,
  iterations: 2,
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
  http.get("http://127.0.0.1:1/health", {
    timeout: "100ms",
    tags: { name: "local_healthcheck" },
  });
  sleep(0.1);
}
