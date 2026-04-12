// Service Worker for Agent Dashboard PWA
const CACHE_NAME = 'agent-dashboard-v1';
const STATIC_ASSETS = [
  '/',
  '/index.html',
  '/app.js',
  '/style.css',
  '/manifest.json',
];

// Install: cache static shell
self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => cache.addAll(STATIC_ASSETS))
  );
  self.skipWaiting();
});

// Activate: clean old caches
self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k)))
    )
  );
  self.clients.claim();
});

// Notification click: focus tab and navigate to agent
self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const agentId = event.notification.data && event.notification.data.agentId;
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clients) => {
      for (const client of clients) {
        if (client.url.includes(self.location.origin)) {
          return client.focus().then((focusedClient) => {
            if (agentId && focusedClient) focusedClient.postMessage({ type: 'navigate-agent', agentId });
          });
        }
      }
      // No existing tab — open a new one
      self.clients.openWindow('/');
    })
  );
});

// Fetch: network-first for API, cache-first for static
self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);

  // Never cache API calls or SSE
  if (url.pathname.startsWith('/api/') || url.pathname === '/events') {
    event.respondWith(fetch(event.request));
    return;
  }

  // Cache-first for static assets
  event.respondWith(
    caches.match(event.request).then((cached) => {
      if (cached) {
        // Update cache in background
        fetch(event.request).then((response) => {
          if (response.ok) {
            caches.open(CACHE_NAME).then((cache) => cache.put(event.request, response));
          }
        });
        return cached;
      }
      return fetch(event.request);
    })
  );
});
