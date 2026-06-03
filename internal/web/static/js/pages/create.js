// Create agent view — Codex composer anatomy: centered hero + single
// composer card with inline harness/skill pills, folder pill below, and
// recent-folder action cards. Spawn POST is unchanged — same IDs.
import { UI } from '../ui.js';
import { ICONS } from '../icons.js';
import { Theme } from '../theme.js';
import { escapeHtml } from '../format.js';
import { get } from '../api.js';

// Path basename. Trims trailing slashes so '/repo/alpha/' → 'alpha'.
function basename(p) {
  if (!p) return '';
  const trimmed = String(p).replace(/\/+$/, '');
  const i = trimmed.lastIndexOf('/');
  return i < 0 ? trimmed : trimmed.slice(i + 1);
}

// Recent-folder summary for the action-card row. Counts agents per cwd,
// sorts by count desc (insertion order preserved on ties), caps at `limit`.
// Pure — exported for unit tests.
export function buildRecentFolders(agents, limit = 3) {
  if (!Array.isArray(agents) || agents.length === 0) return [];
  const counts = new Map();
  for (const a of agents) {
    const cwd = a && a.cwd;
    if (!cwd) continue;
    counts.set(cwd, (counts.get(cwd) || 0) + 1);
  }
  const out = [];
  for (const [cwd, count] of counts) out.push({ cwd, count, label: basename(cwd) });
  out.sort((a, b) => b.count - a.count);
  return out.slice(0, limit);
}

// Folder-pill label. Pure — exported for unit tests.
export function formatFolderLabel(p) {
  const b = basename(p);
  return b || 'Work in a project';
}

const SEND_ARROW = `<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round"><path d="M12 19V5M5 12l7-7 7 7"/></svg>`;
const CHEVRON_DOWN = `<svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9l6 6 6-6"/></svg>`;

function actionCard(folder) {
  const path = escapeHtml(folder.cwd);
  const label = escapeHtml(folder.label || folder.cwd);
  const sub = `${folder.count} agent${folder.count === 1 ? '' : 's'} · ${path}`;
  return `<button class="create-action" type="button" data-folder="${path}">
    <span class="create-action__icon">${ICONS.folder}</span>
    <span class="create-action__title">${label}</span>
    <span class="create-action__sub">${sub}</span>
  </button>`;
}

export function renderCreate(app, agents) {
  const agentFolders = [...new Set((agents || []).map(a => a && a.cwd).filter(Boolean))];
  const recents = buildRecentFolders(agents || []);

  app.innerHTML = `
    ${UI.appBar({ back: true, title: 'New agent', trailing: [Theme.trailingEntry()] })}
    <div class="create-shell">
      <h1 class="create-hero">What should we work on?</h1>

      <div class="create-composer">
        <textarea
          class="create-composer__input"
          id="create-message"
          rows="3"
          placeholder="Do anything"
          oninput="UI.composerAutoSize(this)"
          onkeydown="if((event.metaKey||event.ctrlKey)&&event.key==='Enter'){const b=document.getElementById('create-spawn');if(b&&!b.disabled){event.preventDefault();Dashboard.createAgent(event);}}"></textarea>

        <div class="create-composer__toolbar">
          <div class="create-composer__lead">
            <button class="create-composer__icon" type="button" aria-label="More" tabindex="-1">${ICONS.attach}</button>
            <label class="create-composer__pill" title="Harness">
              ${ICONS.gear}
              <select id="create-harness" aria-label="Harness">
                <option value="">Default</option>
                <option value="claude">Claude Code</option>
                <option value="codex">Codex CLI</option>
              </select>
              ${CHEVRON_DOWN}
            </label>
          </div>
          <div class="create-composer__trail">
            <label class="create-composer__pill" title="Skill">
              <select id="create-skill" aria-label="Skill">
                <option value="">Skill</option>
              </select>
              ${CHEVRON_DOWN}
            </label>
            <button class="create-composer__send" id="create-spawn" type="button" aria-label="Spawn" onclick="Dashboard.createAgent(event)" disabled>${SEND_ARROW}</button>
          </div>
        </div>
      </div>

      <label class="create-folder-pill" for="create-folder">
        ${ICONS.folder}
        <input
          id="create-folder"
          class="create-folder-pill__input"
          type="text"
          placeholder="Work in a project"
          list="folder-suggestions"
          autocomplete="off"
          spellcheck="false"
          aria-label="Project folder">
        ${CHEVRON_DOWN}
      </label>
      <datalist id="folder-suggestions">
        ${agentFolders.map(f => `<option value="${escapeHtml(f)}">`).join('')}
      </datalist>
      <div class="create-hint" id="folder-hint">Pick a folder to spawn in.</div>

      ${recents.length ? `<div class="create-actions">${recents.map(actionCard).join('')}</div>` : ''}
    </div>
  `;

  get('/api/skills').then(skills => {
    if (!Array.isArray(skills)) return;
    const sel = document.getElementById('create-skill');
    if (!sel) return;
    for (const s of skills) {
      const opt = document.createElement('option');
      opt.value = s;
      opt.textContent = s;
      sel.appendChild(opt);
    }
  });

  get('/api/suggestions').then(suggestions => {
    if (!suggestions || !Array.isArray(suggestions)) return;
    const datalist = document.getElementById('folder-suggestions');
    if (!datalist) return;
    const seen = new Set(suggestions);
    const merged = [...suggestions];
    for (const f of agentFolders) if (!seen.has(f)) merged.push(f);
    datalist.innerHTML = merged.map(f => `<option value="${escapeHtml(f)}">`).join('');
  });

  const folderInput = document.getElementById('create-folder');
  const folderPill = folderInput ? folderInput.closest('.create-folder-pill') : null;
  const folderHint = document.getElementById('folder-hint');
  const spawnBtn = document.getElementById('create-spawn');

  function updateFolderState() {
    if (!folderInput || !folderHint || !spawnBtn || !folderPill) return;
    const val = folderInput.value.trim();
    const empty = val.length === 0;
    folderPill.classList.toggle('create-folder-pill--empty', empty);
    if (empty) {
      folderHint.textContent = 'Pick a folder to enable spawn.';
      folderHint.className = 'create-hint';
      spawnBtn.disabled = true;
      spawnBtn.title = 'Pick a folder first';
    } else if (!val.startsWith('/')) {
      folderHint.textContent = 'Path should be absolute (start with /)';
      folderHint.className = 'create-hint create-hint--error';
      spawnBtn.disabled = true;
      spawnBtn.title = 'Path must be absolute';
    } else if (agentFolders.length > 0 && agentFolders.includes(val)) {
      folderHint.textContent = 'Known folder';
      folderHint.className = 'create-hint create-hint--ok';
      spawnBtn.disabled = false;
      spawnBtn.title = 'Spawn (Cmd/Ctrl+Enter)';
    } else {
      folderHint.textContent = '';
      folderHint.className = 'create-hint';
      spawnBtn.disabled = false;
      spawnBtn.title = 'Spawn (Cmd/Ctrl+Enter)';
    }
  }

  if (folderInput) folderInput.addEventListener('input', updateFolderState);

  // Recent-folder cards: one-tap to fill the folder input. Wired after
  // render so onclick strings don't have to escape paths into HTML.
  for (const btn of document.querySelectorAll('.create-action')) {
    btn.addEventListener('click', () => {
      const folder = btn.getAttribute('data-folder') || '';
      if (!folderInput || !folder) return;
      folderInput.value = folder;
      folderInput.dispatchEvent(new Event('input'));
      const msg = document.getElementById('create-message');
      if (msg) msg.focus();
    });
  }

  updateFolderState();
}
