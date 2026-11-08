// k6 load test for AiR staging.
//
// k6 run docs/ops/loadtest.js \
//   -e BASE_URL=https://staging.example \
//   -e SERVICE_KEY=... \
//   -e SESSION_ID=23:1 \
//   -e TOOL=get_current_time

import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const SERVICE_KEY = __ENV.SERVICE_KEY || '';
const SESSION_ID = __ENV.SESSION_ID || '23:1';
const TOOL = __ENV.TOOL || 'get_current_time';

function mcpHeaders() {
  const headers = {
    'Content-Type': 'application/json',
    'X-Session-ID': SESSION_ID,
  };
  if (SERVICE_KEY) {
    headers['X-Service-Key'] = SERVICE_KEY;
  }
  return headers;
}

function mcpCall(name, args) {
  const body = JSON.stringify({
    jsonrpc: '2.0',
    id: '1',
    method: 'tools/call',
    params: { name, arguments: args },
  });
  return http.post(`${BASE_URL}/int/mcp`, body, {
    headers: mcpHeaders(),
    tags: { scenario: 'mcp_tool_call', tool: name },
  });
}

export const options = {
  scenarios: {
    readyz: {
      executor: 'constant-arrival-rate',
      exec: 'readyz',
      rate: 50,
      timeUnit: '1s',
      duration: '2m',
      preAllocatedVUs: 20,
      maxVUs: 60,
    },
    tools_list: {
      executor: 'constant-arrival-rate',
      exec: 'toolsList',
      rate: 50,
      timeUnit: '1s',
      duration: '2m',
      preAllocatedVUs: 20,
      maxVUs: 60,
    },
    tool_call: {
      executor: 'constant-arrival-rate',
      exec: 'toolCall',
      rate: 20,
      timeUnit: '1s',
      duration: '2m',
      preAllocatedVUs: 10,
      maxVUs: 40,
    },
  },
  thresholds: {
    'http_req_failed': ['rate<0.001'],
    'http_req_duration{scenario:readyz}': ['p(95)<100'],
    'http_req_duration{scenario:tools_list}': ['p(95)<200'],
    'http_req_duration{scenario:tool_call}': ['p(95)<300'],
  },
};

export function readyz() {
  const res = http.get(`${BASE_URL}/readyz`, { tags: { scenario: 'readyz' } });
  check(res, { 'readyz 200': (r) => r.status === 200 });
}

export function toolsList() {
  const body = JSON.stringify({ jsonrpc: '2.0', id: '1', method: 'tools/list', params: {} });
  const res = http.post(`${BASE_URL}/int/mcp`, body, {
    headers: mcpHeaders(),
    tags: { scenario: 'tools_list' },
  });
  check(res, { 'tools/list 200': (r) => r.status === 200 });
}

export function toolCall() {
  const res = mcpCall(TOOL, {});
  check(res, { 'tools/call 200': (r) => r.status === 200 });
  sleep(0.01);
}
