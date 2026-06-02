// Service Worker for Agent Dashboard PWA
const CACHE_NAME = 'agent-dashboard-v13';
const STATIC_ASSETS = [
  '/',
  '/index.html',
  '/app.js',
  '/style.css',
  '/manifest.json',
  '/js/notify.js',
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

// Fetch strategy
//   /api/, /events       → network only (live data)
//   *.js, *.css, *.html  → network-first with cache fallback (so a
//                          freshly-deployed style.css / app.js wins
//                          immediately, but offline still works)
//   everything else      → cache-first with background revalidation
self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);

  if (url.pathname.startsWith('/api/') || url.pathname === '/events') {
    event.respondWith(fetch(event.request));
    return;
  }

  const isCode = /\.(?:js|css|html)$/.test(url.pathname) || url.pathname === '/';
  if (isCode) {
    event.respondWith(
      fetch(event.request).then((response) => {
        if (response.ok) {
          const clone = response.clone();
          caches.open(CACHE_NAME).then((cache) => cache.put(event.request, clone));
        }
        return response;
      }).catch(() => caches.match(event.request))
    );
    return;
  }

  // Cache-first with background revalidation for everything else.
  event.respondWith(
    caches.match(event.request).then((cached) => {
      if (cached) {
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
