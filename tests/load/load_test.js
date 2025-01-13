import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export let options = {
    stages: [
        { duration: '2m', target: 100 }, // Ramp up to 100 users
        { duration: '5m', target: 100 }, // Stay at 100 users
        { duration: '2m', target: 200 }, // Ramp up to 200 users
        { duration: '5m', target: 200 }, // Stay at 200 users
        { duration: '2m', target: 0 },   // Ramp down to 0 users
    ],
    thresholds: {
        'http_req_duration': ['p(95)<500'], // 95% of requests should be below 500ms
        'errors': ['rate<0.1'],             // Error rate should be below 10%
    },
};

const BASE_URL = __ENV.API_URL || 'http://localhost:8080';
let token = '';

export function setup() {
    // Login and get token
    const loginRes = http.post(`${BASE_URL}/api/auth/login`, JSON.stringify({
        username: 'testuser',
        password: 'testpass'
    }), {
        headers: { 'Content-Type': 'application/json' },
    });

    check(loginRes, {
        'login successful': (r) => r.status === 200,
    });

    token = loginRes.json('token');
    return { token };
}

export default function(data) {
    const headers = {
        'Authorization': `Bearer ${data.token}`,
        'Content-Type': 'application/json',
    };

    // Test GET endpoints
    const requests = {
        'get users': {
            method: 'GET',
            url: `${BASE_URL}/api/users`,
        },
        'get residents': {
            method: 'GET',
            url: `${BASE_URL}/api/residents`,
        },
        'get expenses': {
            method: 'GET',
            url: `${BASE_URL}/api/expenses`,
        },
    };

    const responses = http.batch(Object.values(requests).map(req => ({
        ...req,
        headers,
    })));

    responses.forEach((res, index) => {
        const name = Object.keys(requests)[index];
        check(res, {
            [`${name} status was 200`]: (r) => r.status === 200,
        }) || errorRate.add(1);
    });

    sleep(1);
}

export function teardown(data) {
    // Cleanup if needed
} 