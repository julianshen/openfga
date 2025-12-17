import http from 'k6/http';
import { check, group } from 'k6';
import { Trend } from 'k6/metrics';

const latencyValKey = new Trend('latency_valkey');
const latencyPostgres = new Trend('latency_postgres');
const latencyMySQL = new Trend('latency_mysql');

export const options = {
  scenarios: {
    postgres: {
      executor: 'per-vu-iterations',
      exec: 'postgres',
      vus: 10,
      iterations: 1,
      maxDuration: '10m',
    },
    mysql: {
      executor: 'per-vu-iterations',
      exec: 'mysql',
      vus: 10,
      iterations: 1,
      maxDuration: '10m',
    },
    valkey: {
      executor: 'per-vu-iterations',
      exec: 'valkey',
      vus: 10,
      iterations: 1,
      maxDuration: '10m',
    },
  },
};

const PAGE_SIZE = 50;
const MAX_PAGES = 2000; // Go up to 100,000 items deep

function runPagination(name, url, trend) {
  let continuationToken = "";

  group(name, function () {
    for (let i = 0; i < MAX_PAGES; i++) {
      let endpoint = `${url}/stores?page_size=${PAGE_SIZE}`;
      if (continuationToken) {
        endpoint += `&continuation_token=${encodeURIComponent(continuationToken)}`;
      }

      const res = http.get(endpoint);

      check(res, {
        'status is 200': (r) => r.status === 200,
      });

      if (res.status === 200) {
        trend.add(res.timings.duration, { page: i.toString() });
        const body = res.json();
        continuationToken = body.continuation_token;

        // Stop if no more pages
        if (!continuationToken) {
          console.log(`[${name}] No more pages at iteration ${i}`);
          break;
        }
      } else {
        console.error(`[${name}] Error: ${res.status} ${res.body}`);
        break;
      }
    }
  });
}

export function postgres() {
  runPagination('Postgres', 'http://localhost:8081', latencyPostgres);
}

export function mysql() {
  runPagination('MySQL', 'http://localhost:8082', latencyMySQL);
}

export function valkey() {
  runPagination('Valkey', 'http://localhost:8083', latencyValKey);
}
