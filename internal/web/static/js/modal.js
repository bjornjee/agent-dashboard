// Modal dialog and toast notifications.
import { escapeHtml } from './format.js';

let modalSeq = 0;

export function showModal(title, message, onConfirm, opts) {
  const o = opts || {};
  const confirmLabel = o.confirmLabel || 'Confirm';
  const cancelLabel = o.cancelLabel || 'Cancel';
  const allowedVariants = new Set(['primary', 'danger', 'secondary', 'ghost']);
  const confirmVariant = allowedVariants.has(o.confirmVariant) ? o.confirmVariant : 'primary';
  const initialFocus = o.initialFocus === 'cancel' ? 'cancel' : 'confirm';
  const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  const id = ++modalSeq;
  const titleId = `modal-title-${id}`;
  const messageId = `modal-message-${id}`;
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  overlay.innerHTML = `
    <div class="modal" role="dialog" aria-modal="true" aria-labelledby="${titleId}" aria-describedby="${messageId}" tabindex="-1">
      <div class="modal-title" id="${titleId}">${escapeHtml(title)}</div>
      <div class="modal-message" id="${messageId}">${escapeHtml(message)}</div>
      <div class="modal-actions">
        <button class="ui-modal-btn ui-modal-btn--ghost" id="modal-cancel">${escapeHtml(cancelLabel)}</button>
        <button class="ui-modal-btn ui-modal-btn--${escapeHtml(confirmVariant)}" id="modal-confirm">${escapeHtml(confirmLabel)}</button>
      </div>
    </div>
  `;
  document.body.appendChild(overlay);
  const dialog = overlay.querySelector('.modal');
  const cancelBtn = overlay.querySelector('#modal-cancel');
  const confirmBtn = overlay.querySelector('#modal-confirm');
  let busy = false;

  function restoreFocus() {
    if (document.querySelector('.modal-overlay')) return;
    if (opener && typeof opener.focus === 'function') {
      try { opener.focus(); } catch {}
    }
  }

  function close() {
    overlay.remove();
    restoreFocus();
  }

  function setBusy(nextBusy) {
    busy = nextBusy;
    cancelBtn.disabled = nextBusy;
    confirmBtn.disabled = nextBusy;
  }

  cancelBtn.addEventListener('click', () => {
    if (!busy) close();
  });
  confirmBtn.addEventListener('click', async (e) => {
    if (busy) return;
    setBusy(true);
    try {
      await onConfirm(e);
      close();
    } catch (err) {
      setBusy(false);
      throw err;
    }
  });
  overlay.addEventListener('click', (e) => {
    if (!busy && e.target === overlay) close();
  });
  overlay.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      if (!busy) close();
      return;
    }
    if (e.key !== 'Tab') return;
    const focusable = [cancelBtn, confirmBtn].filter((el) => !el.disabled);
    if (focusable.length === 0) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  });
  requestAnimationFrame(() => {
    const target = initialFocus === 'cancel' ? cancelBtn : confirmBtn;
    if (target && typeof target.focus === 'function') target.focus();
    else if (dialog && typeof dialog.focus === 'function') dialog.focus();
  });
}

export function toast(msg, type) {
  const variant = type === 'error' ? 'error' : 'success';
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
