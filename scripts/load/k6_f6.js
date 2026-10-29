// k6 — prueba de carga F6 de Tiendas-Track.
//
// Escenarios:
//   ingest : VUS vehículos concurrentes reportando GPS (RNF-06).
//   ws     : suscripción tiempo real y latencia hasta recibir el evento (RNF-04).
//
// Características: 5% de reintentos duplicados exactos (idempotencia) y 1% de
// payloads inválidos inyectados que deben devolver 4xx (nunca 5xx).
//
// Variables de entorno:
//   BASE_URL  backend (def. http://localhost:8082)
//   VUS       vehículos concurrentes del escenario ingest (def. 100)
//   DURATION  duración del escenario ingest (def. 2m)
//   WS_VUS    conexiones WS concurrentes (def. min(VUS, 20))
//   WS_DURATION duración del escenario ws (def. DURATION)
//   AUTH_EMAIL / AUTH_PASSWORD credenciales de proveedor
//
// El rate limiting de producción (RATE_LIMIT_TELEMETRY_PER_MIN) está pensado
// para proteger la API, no para medir capacidad: para esta prueba arranca el
// backend con RATE_LIMIT_TELEMETRY_PER_MIN=0 (ver README).

import http from 'k6/http';
import ws from 'k6/ws';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const BASE_URL = (__ENV.BASE_URL || 'http://localhost:8082').replace(/\/$/, '');
const VUS = Number(__ENV.VUS || 100);
const DURATION = __ENV.DURATION || '2m';
const WS_VUS = Number(__ENV.WS_VUS || Math.min(VUS, 20));
const WS_DURATION = __ENV.WS_DURATION || DURATION;
const AUTH_EMAIL = __ENV.AUTH_EMAIL || 'proveedor@tiendas.dev';
const AUTH_PASSWORD = __ENV.AUTH_PASSWORD || 'Proveedor123';

const unexpected5xx = new Rate('unexpected_5xx');
const duplicateConfirmed = new Counter('duplicate_confirmed');
const invalidRejected = new Counter('invalid_rejected');
const wsEventLatency = new Trend('ws_event_latency', true);
const wsConnected = new Counter('ws_connected');

export const options = {
  scenarios: {
    ingest: {
      executor: 'constant-vus',
      vus: VUS,
      duration: DURATION,
      exec: 'ingestScenario',
      tags: { scenario: 'ingest' },
    },
    ws: {
      executor: 'constant-vus',
      vus: WS_VUS,
      duration: WS_DURATION,
      exec: 'wsScenario',
      startTime: '5s',
      tags: { scenario: 'ws' },
    },
  },
  thresholds: {
    // RNF-06: p95 de ingesta válida por debajo de 500 ms.
    'http_req_duration{kind:ingest_valid}': ['p(95)<500'],
    // RNF-04: p95 de entrega WS por debajo de 500 ms.
    'ws_event_latency': ['p(95)<500'],
    // Menos del 1% de respuestas 5xx no esperadas.
    'unexpected_5xx': ['rate<0.01'],
    // Los duplicados y los 422 inyectados deben comportarse según contrato.
    'checks{kind:duplicate}': ['rate>0.95'],
    'checks{kind:invalid}': ['rate>0.95'],
  },
};

// setup: login de proveedor y flota visible. Se ejecuta una sola vez.
export function setup() {
  const login = http.post(
    `${BASE_URL}/api/v1/auth/login`,
    JSON.stringify({ email: AUTH_EMAIL, password: AUTH_PASSWORD }),
    { headers: { 'Content-Type': 'application/json' }, tags: { kind: 'setup_login' } }
  );
  if (login.status !== 200) {
    throw new Error(`login falló: status=${login.status} body=${login.body}`);
  }
  const body = login.json();
  if (!body.access_token) {
    throw new Error('login sin access_token en la respuesta');
  }

  const vehicles = http.get(`${BASE_URL}/api/v1/vehicles?limit=200`, {
    headers: { Authorization: `Bearer ${body.access_token}` },
    tags: { kind: 'setup_vehicles' },
  });
  if (vehicles.status !== 200) {
    throw new Error(`listar vehículos falló: status=${vehicles.status}`);
  }
  const ids = (vehicles.json().items || []).map((v) => v.id).filter(Boolean);
  if (ids.length === 0) {
    throw new Error('no hay vehículos visibles: ejecuta scripts/load/seed_load_vehicles.sh');
  }
  return { token: body.access_token, vehicles: ids };
}

function vehicleFor(data) {
  // Cada VU tiende a un vehículo distinto; si hay menos vehículos que VUs se
  // reutiliza el vehículo pero device_seq es único por VU/iteración.
  return data.vehicles[(__VU - 1) % data.vehicles.length];
}

function track(status, kind) {
  if (status >= 500) {
    unexpected5xx.add(true, { kind });
  } else {
    unexpected5xx.add(false, { kind });
  }
}

export function ingestScenario(data) {
  const vehicleID = vehicleFor(data);
  const seq = __VU * 1000000 + __ITER;
  const now = new Date().toISOString();
  const lat = 4.65 + ((__VU % 100) / 10000);
  const lon = -74.05 - ((__VU % 100) / 10000);

  const payload = {
    vehicle_id: vehicleID,
    time: now,
    lat,
    lon,
    speed_kph: 35,
    accuracy_m: 8,
    heading: 90,
    device_seq: seq,
  };
  const headers = {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${data.token}`,
  };

  // 1% de errores inyectados (más un gatillo determinista para que el smoke
  // test corto siempre los ejercite): lat fuera de rango → 422, nunca 5xx.
  if ((__ITER === 0 && __VU === 1) || Math.random() < 0.01) {
    const bad = Object.assign({}, payload, { lat: 999 });
    const res = http.post(`${BASE_URL}/api/v1/telemetry/points`, JSON.stringify(bad), {
      headers,
      tags: { kind: 'invalid' },
    });
    track(res.status, 'invalid');
    check(res, { 'payload inválido → 422': (r) => r.status === 422 }, { kind: 'invalid' });
    if (res.status === 422) invalidRejected.add(1);
    sleep(0.2);
    return;
  }

  const res = http.post(`${BASE_URL}/api/v1/telemetry/points`, JSON.stringify(payload), {
    headers,
    tags: { kind: 'ingest_valid' },
  });
  track(res.status, 'ingest_valid');
  check(res, { 'ingesta válida → 201': (r) => r.status === 201 }, { kind: 'ingest_valid' });

  // 5% de duplicados: se reenvía EXACTAMENTE el mismo payload (misma PK
  // time+vehicle_id+device_seq) y el servidor debe responder duplicate=true.
  if (Math.random() < 0.05) {
    const dup = http.post(`${BASE_URL}/api/v1/telemetry/points`, JSON.stringify(payload), {
      headers,
      tags: { kind: 'duplicate' },
    });
    track(dup.status, 'duplicate');
    let isDuplicate = false;
    try {
      isDuplicate = dup.status === 201 && dup.json().duplicate === true;
    } catch (_) {
      isDuplicate = false;
    }
    check(dup, { 'duplicado confirmado (201, duplicate=true)': () => isDuplicate }, { kind: 'duplicate' });
    if (isDuplicate) duplicateConfirmed.add(1);
  }

  sleep(0.9 + Math.random() * 0.2);
}

export function wsScenario(data) {
  const vehicleID = vehicleFor(data);
  const channel = `vehicle:${vehicleID}`;
  const seq = __VU * 1000000 + __ITER;
  const wsURL = BASE_URL.replace(/^http/, 'ws') + `/ws?access_token=${encodeURIComponent(data.token)}`;

  const res = ws.connect(wsURL, { tags: { kind: 'ws' } }, (socket) => {
    let started = 0;

    socket.on('open', () => {
      wsConnected.add(1);
      socket.send(JSON.stringify({ action: 'subscribe', channel }));
    });

    socket.on('message', (raw) => {
      let msg;
      try {
        msg = JSON.parse(raw);
      } catch (_) {
        return;
      }
      if (msg.type === 'subscribed') {
        // Un punto nuevo tras suscribirse: el evento debe llegar al canal.
        started = Date.now();
        const point = {
          vehicle_id: vehicleID,
          time: new Date().toISOString(),
          lat: 4.66,
          lon: -74.06,
          speed_kph: 30,
          device_seq: seq,
        };
        http.post(`${BASE_URL}/api/v1/telemetry/points`, JSON.stringify(point), {
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${data.token}`,
          },
          tags: { kind: 'ws_ingest' },
        });
      }
      if (msg.type === 'event' && msg.channel === channel && started > 0) {
        wsEventLatency.add(Date.now() - started);
        started = 0;
        socket.close();
      }
    });

    socket.on('error', () => socket.close());
    socket.setTimeout(() => socket.close(), 5000);
  });

  check(res, { 'handshake WS 101': (r) => r && r.status === 101 }, { kind: 'ws' });
  sleep(1);
}
