// Settings view — configure the default harness and other dashboard prefs.
import { UI } from '../ui.js';
import { get } from '../api.js';

const HARNESS_OPTIONS = [
  { value: 'claude', label: 'claude' },
  { value: 'pi', label: 'pi (supports openai/codex models)' },
  { value: 'codex', label: 'codex' },
];

let currentSettings = null;

export async function renderSettings(app) {
  app.innerHTML = UI.header('Settings', {
    actions: [{ label: '&larr; Back', onclick: 'Dashboard.showList()' }],
  }) + `
    <div class="create-form-card">
      <div class="form-group">
        <label class="form-label">Default Harness</label>
        <select id="settings-harness" class="action-input" style="width:100%">
          ${HARNESS_OPTIONS.map(o => `<option value="${o.value}">${o.label}</option>`).join('')}
        </select>
        <div class="form-hint">Used when the new-agent form leaves <em>Harness</em> blank. Per-harness settings (<code>[harness.pi]</code>, <code>[harness.codex]</code>) still come from <code>settings.toml</code>.</div>
      </div>
      <div style="margin-top:8px">${UI.btn('Save', { variant: 'primary', onclick: 'Dashboard.saveSettings(event)' })}</div>
      <div class="form-hint" style="margin-top:12px">Saving rewrites <code>settings.toml</code> in canonical form &mdash; comments and unknown keys will be lost. Edit the file directly to preserve them.</div>
    </div>
  `;

  currentSettings = await get('/api/settings');
  const sel = document.getElementById('settings-harness');
  if (sel && currentSettings && currentSettings.Harness && currentSettings.Harness.Default) {
    sel.value = currentSettings.Harness.Default;
  }
}

// Read the form and produce the Settings payload to POST. Merges into the
// last-fetched settings struct so we don't accidentally blank fields the
// page doesn't expose yet.
export function buildSettingsPayload() {
  const sel = document.getElementById('settings-harness');
  const merged = currentSettings ? JSON.parse(JSON.stringify(currentSettings)) : {};
  merged.Harness = merged.Harness || {};
  merged.Harness.Default = sel ? sel.value : 'claude';
  return merged;
}
