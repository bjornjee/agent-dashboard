// HTTP API helpers.

let navAbort = null;

export async function api(method, path, body) {
  const opts = {
    method,
    headers: { 'X-Requested-With': 'dashboard' },
  };
  if (body) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  const resp = await fetch(path, opts);
  if (resp.status === 401) {
    window.location.href = '/auth/login';
    return null;
  }
  return resp.json();
}

export function get(path) { return api('GET', path); }
export function post(path, body) { return api('POST', path, body); }

export function cancelNav() {
  if (navAbort) { navAbort.abort(); navAbort = null; }
}

export function newNavSignal() {
  cancelNav();
  navAbort = new AbortController();
  return navAbort.signal;
}
