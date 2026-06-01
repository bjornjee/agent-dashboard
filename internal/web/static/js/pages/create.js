// Create agent view — Codex flat-prose form with display headline + sticky spawn.
import { UI } from '../ui.js';
import { ICONS } from '../icons.js';
import { escapeHtml } from '../format.js';
import { get } from '../api.js';

export function renderCreate(app, agents) {
  const agentFolders = [...new Set(agents.map(a => a.cwd).filter(Boolean))];

  app.innerHTML = `
    ${UI.appBar({ back: true, title: 'New agent' })}
    <div class="create-display">What should we work on?</div>
    <div class="create-form">
      <textarea
        class="ui-input ui-input--multiline create-message"
        id="create-message"
        rows="4"
        placeholder="Do anything"
        oninput="UI.composerAutoSize(this)"></textarea>

      ${UI.sectionLabel('Folder')}
      <div class="create-field">
        <input
          id="create-folder"
          class="ui-input"
          type="text"
          placeholder="/path/to/repo"
          list="folder-suggestions">
        <datalist id="folder-suggestions">
          ${agentFolders.map(f => `<option value="${escapeHtml(f)}">`).join('')}
        </datalist>
        <div class="create-hint" id="folder-hint"></div>
      </div>

      ${UI.sectionLabel('Harness')}
      <div class="create-field">
        <select id="create-harness" class="ui-input">
          <option value="">Default (settings.toml)</option>
          <option value="claude">Claude Code</option>
          <option value="codex">Codex CLI</option>
        </select>
        <div class="create-hint">Codex reads <code>[harness.codex]</code> from settings.toml.</div>
      </div>

      ${UI.sectionLabel('Skill')}
      <div class="create-field">
        <select id="create-skill" class="ui-input">
          <option value="">Default</option>
        </select>
      </div>

      <button class="create-spawn" id="create-spawn" onclick="Dashboard.createAgent(event)" disabled>Spawn</button>
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
  const folderHint = document.getElementById('folder-hint');
  const spawnBtn = document.getElementById('create-spawn');

  function updateFolderState() {
    if (!folderInput || !folderHint || !spawnBtn) return;
    const val = folderInput.value.trim();
    if (!val) {
      folderHint.textContent = 'Pick a folder to spawn in.';
      folderHint.className = 'create-hint';
      spawnBtn.disabled = true;
    } else if (!val.startsWith('/')) {
      folderHint.textContent = 'Path should be absolute (start with /)';
      folderHint.className = 'create-hint create-hint--error';
      spawnBtn.disabled = true;
    } else if (agentFolders.length > 0 && agentFolders.includes(val)) {
      folderHint.textContent = 'Known folder';
      folderHint.className = 'create-hint create-hint--ok';
      spawnBtn.disabled = false;
    } else {
      folderHint.textContent = '';
      folderHint.className = 'create-hint';
      spawnBtn.disabled = false;
    }
  }

  if (folderInput) folderInput.addEventListener('input', updateFolderState);
  updateFolderState();
}
