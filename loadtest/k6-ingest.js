import http from "k6/http";
import { check } from "k6";

// Ramp load to find the throughput and show 429 back-pressure (not OOM).
export const options = {
  scenarios: {
    ramp: { executor: "ramping-arrival-rate", startRate: 1000, timeUnit: "1s",
            preAllocatedVUs: 200, maxVUs: 1000,
            stages: [ {target: 20000, duration: "30s"}, {target: 80000, duration: "60s"}, {target: 0, duration: "10s"} ] },
  },
};
const body = JSON.stringify({ source: "edr", type: "login_fail", severity: "high" });
export default function () {
  const r = http.post("http://localhost:8080/events", body, { headers: { "Content-Type": "application/json" } });
  check(r, { "202 or 429 (never 5xx)": (res) => res.status === 202 || res.status === 429 });
}
