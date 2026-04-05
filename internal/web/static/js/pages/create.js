// Create agent view.
import { UI } from '../ui.js';
import { escapeHtml } from '../format.js';

export function renderCreate(app, agents) {
  const folders = [...new Set(agents.map(a => a.cwd).filter(Boolean))];

  app.innerHTML = UI.header('New Agent',
    UI.btn('&larr; Back', { variant: 'ghost', onclick: "Dashboard.showList()" })
  ) + `
    <div class="create-form-card">
      <div class="form-group">
        <label class="form-label">Folder</label>
        <input id="create-folder" class="action-input" style="width:100%" placeholder="/path/to/repo" list="folder-suggestions">
        <datalist id="folder-suggestions">
          ${folders.map(f => `<option value="${escapeHtml(f)}">`).join('')}
        </datalist>
        <div class="form-hint" id="folder-hint"></div>
      </div>
      <div class="form-group">
        <label class="form-label">Skill</label>
        <select id="create-skill" class="action-input" style="width:100%">
          <option value="">Default</option>
          <option value="feature">feature</option>
          <option value="bugfix">bugfix</option>
          <option value="refactor">refactor</option>
          <option value="test">test</option>
          <option value="docs">docs</option>
        </select>
      </div>
      <div class="form-group">
        <label class="form-label">Message (optional)</label>
        <textarea id="create-message" class="action-input" style="width:100%;min-height:80px;resize:vertical" placeholder="What should the agent do?"></textarea>
      </div>
      <div style="margin-top:8px">${UI.btn('Create Agent', { variant: 'primary', onclick: "Dashboard.createAgent()" })}</div>
    </div>
  `;

  const folderInput = document.getElementById('create-folder');
  const folderHint = document.getElementById('folder-hint');
  if (folderInput && folderHint) {
    folderInput.addEventListener('input', () => {
      const val = folderInput.value.trim();
      if (!val) {
        folderHint.textContent = '';
        folderHint.className = 'form-hint';
      } else if (!val.startsWith('/')) {
        folderHint.textContent = 'Path should be absolute (start with /)';
        folderHint.className = 'form-hint form-hint-error';
      } else if (folders.length > 0 && folders.includes(val)) {
        folderHint.textContent = 'Known folder';
        folderHint.className = 'form-hint form-hint-ok';
      } else {
        folderHint.textContent = '';
        folderHint.className = 'form-hint';
      }
    });
  }
}
