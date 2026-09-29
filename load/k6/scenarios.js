import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';

const deniedCount = new Counter('decisions_denied');
const allowedRate = new Rate('decisions_allowed_rate');

// Configure via environment: BASE_URL, API_KEY, CLIENT_PREFIX
const BASE_URL    = __ENV.BASE_URL    || 'http://localhost:8080';
const API_KEY     = __ENV.API_KEY     || 'replace-with-api-key';
const CLIENT_PFX  = __ENV.CLIENT_PFX || 'loadclient';

// ---- Scenario 1: Same-key contention ---
// 500 VUs all hammering the same client_id — tests Redis atomic correctness.
export const options = {
  scenarios: {
    same_key_contention: {
      executor: 'constant-vus',
      vus: 500,
      duration: '30s',
      tags: { scenario: 'same_key' },
    },
    many_key_throughput: {
      executor: 'ramping-vus',
      startTime: '35s',
      stages: [
        { duration: '10s', target: 200 },
        { duration: '20s', target: 200 },
        { duration: '5s', target: 0 },
      ],
      tags: { scenario: 'many_keys' },
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<25'],   // < 25ms p95
    http_req_failed:   ['rate<0.01'],  // < 1% errors
  },
};

const headers = {
  'Content-Type': 'application/json',
  'Authorization': `Bearer ${API_KEY}`,
};

// Same-key contention scenario function
export default function () {
  const scenario = __ENV.K6_SCENARIO_NAME || 'same_key_contention';

  let clientID;
  if (scenario === 'same_key_contention') {
    clientID = 'shared-load-client';
  } else {
    clientID = `${CLIENT_PFX}-${__VU}`;
  }

  const payload = JSON.stringify({ client_id: clientID, cost: 1 });
  const res = http.post(`${BASE_URL}/v1/decisions`, payload, { headers });

  const ok = check(res, {
    'status 200': (r) => r.status === 200,
    'has allowed field': (r) => JSON.parse(r.body).allowed !== undefined,
  });

  if (!ok) return;

  const body = JSON.parse(res.body);
  if (body.allowed) {
    allowedRate.add(1);
  } else {
    deniedCount.add(1);
  }

  sleep(0.001);
}
