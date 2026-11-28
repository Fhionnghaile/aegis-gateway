import http from "k6/http";
import { check } from "k6";

// Ramp load to find the throughput and show 429 back-pressure (not OOM).
// Defaults are the full test (peak 80k req/s). CI runs a lighter profile: PEAK=5000 HOLD=30s.
const PEAK = parseInt(__ENV.PEAK || "80000");
const HOLD = __ENV.HOLD || "60s";
export const options = {
  scenarios: {
    ramp: { executor: "ramping-arrival-rate", startRate: Math.min(1000, PEAK), timeUnit: "1s",
            preAllocatedVUs: 200, maxVUs: 1000,
            stages: [ {target: Math.round(PEAK / 4), duration: "30s"}, {target: PEAK, duration: HOLD}, {target: 0, duration: "10s"} ] },
  },
  // The run FAILS if any response is a 5xx or anything other than 202/429.
  thresholds: { checks: ["rate==1.0"] },
};
const body = JSON.stringify({ source: "edr", type: "login_fail", severity: "high" });
export default function () {
  const r = http.post("http://localhost:8080/events", body, { headers: { "Content-Type": "application/json" } });
  check(r, { "202 or 429 (never 5xx)": (res) => res.status === 202 || res.status === 429 });
}
