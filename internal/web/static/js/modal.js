// Modal dialog and toast notifications.
import { UI } from './ui.js';
import { escapeHtml } from './format.js';

export function showModal(title, message, onConfirm) {
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  overlay.innerHTML = `
    <div class="modal">
      <div class="modal-title">${escapeHtml(title)}</div>
      <div class="modal-message">${escapeHtml(message)}</div>
      <div class="modal-actions">
        ${UI.btn('Cancel', { variant: 'ghost', id: 'modal-cancel' })}
        ${UI.btn('Confirm', { variant: 'danger', id: 'modal-confirm' })}
      </div>
    </div>
  `;
  document.body.appendChild(overlay);
  overlay.querySelector('#modal-cancel').addEventListener('click', () => overlay.remove());
  overlay.querySelector('#modal-confirm').addEventListener('click', () => {
    overlay.remove();
    onConfirm();
  });
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) overlay.remove();
  });
}

export function toast(msg, type) {
  const el = document.createElement('div');
  el.className = 'toast ' + (type || '');
  el.textContent = msg;
  document.body.appendChild(el);
  requestAnimationFrame(() => el.classList.add('visible'));
  setTimeout(() => {
    el.classList.remove('visible');
    setTimeout(() => el.remove(), 300);
  }, 2500);
}
