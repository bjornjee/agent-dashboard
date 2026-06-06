// Modal dialog and toast notifications.
import { escapeHtml } from './format.js';

export function showModal(title, message, onConfirm, opts) {
  const o = opts || {};
  const confirmLabel = o.confirmLabel || 'Confirm';
  const cancelLabel = o.cancelLabel || 'Cancel';
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  overlay.innerHTML = `
    <div class="modal">
      <div class="modal-title">${escapeHtml(title)}</div>
      <div class="modal-message">${escapeHtml(message)}</div>
      <div class="modal-actions">
        <button class="ui-modal-btn ui-modal-btn--ghost" id="modal-cancel">${escapeHtml(cancelLabel)}</button>
        <button class="ui-modal-btn ui-modal-btn--primary" id="modal-confirm">${escapeHtml(confirmLabel)}</button>
      </div>
    </div>
  `;
  document.body.appendChild(overlay);
  overlay.querySelector('#modal-cancel').addEventListener('click', () => overlay.remove());
  overlay.querySelector('#modal-confirm').addEventListener('click', async (e) => {
    await onConfirm(e);
    overlay.remove();
  });
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) overlay.remove();
  });
}

export function toast(msg, type) {
  const variant = type === 'error' || type === 'warn' ? type : 'success';
  const el = document.createElement('div');
  el.className = 'ui-toast ui-toast--' + variant;
  el.setAttribute('role', variant === 'error' ? 'alert' : 'status');
  el.innerHTML = `<span class="ui-toast__dot" aria-hidden="true"></span><span class="ui-toast__text">${escapeHtml(msg)}</span>`;
  document.body.appendChild(el);
  requestAnimationFrame(() => el.classList.add('ui-toast--visible'));
  setTimeout(() => {
    el.classList.remove('ui-toast--visible');
    setTimeout(() => el.remove(), 220);
  }, 2400);
}
