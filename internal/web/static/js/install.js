// PWA install lifecycle. Owns:
//   - beforeinstallprompt capture (toggles body.can-install so the sidebar
//     row appears).
//   - promptInstall(): triggers the deferred prompt on user gesture.
//   - maybeShowIOSHint(): iOS Safari has no install API, so we surface the
//     manual "Share -> Add to Home Screen" path once.
//   - consumeNewAgentShortcut(): the manifest shortcut deep-links to
//     /?action=new-agent. Strip the param and route to the create view.

const IOS_HINT_KEY = 'ios-install-dismissed';

let deferredPrompt = null;

window.addEventListener('beforeinstallprompt', (e) => {
  e.preventDefault();
  deferredPrompt = e;
  document.body.classList.add('can-install');
});

window.addEventListener('appinstalled', () => {
  deferredPrompt = null;
  document.body.classList.remove('can-install');
});

export async function promptInstall() {
  if (!deferredPrompt) return false;
  deferredPrompt.prompt();
  const choice = await deferredPrompt.userChoice;
  deferredPrompt = null;
  document.body.classList.remove('can-install');
  return choice.outcome === 'accepted';
}

export function isIOS() {
  return /iPad|iPhone|iPod/.test(navigator.userAgent) && !window.MSStream;
}

export function isStandalone() {
  return window.matchMedia('(display-mode: standalone)').matches
      || window.navigator.standalone === true;
}

export function maybeShowIOSHint(showModal) {
  if (!isIOS() || isStandalone()) return;
  try {
    if (localStorage.getItem(IOS_HINT_KEY)) return;
  } catch {
    return;
  }
  showModal(
    'Install Agent Dashboard',
    'Tap the Share button, then "Add to Home Screen" to install this app.',
    () => { try { localStorage.setItem(IOS_HINT_KEY, '1'); } catch {} },
    { confirmLabel: 'Got it', cancelLabel: 'Later' }
  );
}

export function consumeNewAgentShortcut(navigateTo) {
  const params = new URLSearchParams(location.search);
  if (params.get('action') !== 'new-agent') return false;
  params.delete('action');
  const qs = params.toString();
  const url = location.pathname + (qs ? '?' + qs : '') + location.hash;
  history.replaceState(history.state, '', url);
  navigateTo('create', null, true);
  return true;
}
