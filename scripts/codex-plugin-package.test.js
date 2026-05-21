#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const REPO = path.resolve(__dirname, '..');

function readJson(filePath) {
  return JSON.parse(fs.readFileSync(filePath, 'utf8'));
}

function skillNames(root) {
  return fs.readdirSync(root, { withFileTypes: true })
    .filter(entry => entry.isDirectory())
    .map(entry => entry.name)
    .sort();
}

function relativeFiles(root) {
  const files = [];

  function walk(current) {
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const fullPath = path.join(current, entry.name);
      if (entry.isDirectory()) {
        walk(fullPath);
      } else if (entry.isFile()) {
        files.push(path.relative(root, fullPath));
      }
    }
  }

  walk(root);
  return files.sort();
}

function readCodexSkill(name) {
  return fs.readFileSync(path.join(REPO, `adapters/codex/skills/${name}/SKILL.md`), 'utf8');
}

describe('codex plugin package', () => {
  it('keeps adapters named by supported harness', () => {
    const adapters = fs.readdirSync(path.join(REPO, 'adapters'), { withFileTypes: true })
      .filter(entry => entry.isDirectory())
      .map(entry => entry.name)
      .sort();

    assert.deepEqual(adapters, ['claude-code', 'codex']);
  });

  it('publishes a Codex marketplace entry that points at the Codex adapter', () => {
    const marketplace = readJson(path.join(REPO, '.agents/plugins/marketplace.json'));
    const plugin = marketplace.plugins.find(entry => entry.name === 'agent-dashboard');

    assert.ok(plugin, 'agent-dashboard plugin entry should exist');
    assert.equal(plugin.source.source, 'local');
    assert.equal(plugin.source.path, './adapters/codex');
    assert.deepEqual(plugin.policy, {
      installation: 'AVAILABLE',
      authentication: 'ON_INSTALL',
    });
    assert.equal(plugin.category, 'Engineering');
  });

  it('has a Codex manifest under the Codex adapter', () => {
    const manifest = readJson(path.join(REPO, 'adapters/codex/.codex-plugin/plugin.json'));
    const releaseManifest = readJson(path.join(REPO, '.release-please-manifest.json'));

    assert.equal(manifest.name, 'agent-dashboard');
    assert.equal(manifest.version, releaseManifest['.']);
    assert.equal(manifest.skills, './skills/');
    assert.equal(manifest.hooks, './hooks/plugin-hooks.json');
    assert.equal(manifest.interface.developerName, 'bjornjee');
    assert.equal(manifest.interface.category, 'Engineering');
  });

  it('uses plugin-local Codex hook commands, not Claude adapter or global hooks', () => {
    const hooks = readJson(path.join(REPO, 'adapters/codex/hooks/plugin-hooks.json'));
    const commands = Object.values(hooks.hooks)
      .flatMap(entries => entries)
      .flatMap(entry => entry.hooks)
      .map(hook => hook.command);

    assert.ok(commands.length > 0, 'expected hook commands');
    for (const command of commands) {
      assert.match(command, /\$\{PLUGIN_ROOT\}/);
      assert.doesNotMatch(command, /adapters\/claude-code/);
      assert.doesNotMatch(command, /\$HOME\/\.codex\/hooks/);
    }
  });

  it('registers Codex plugin subagent lifecycle events', () => {
    const hooks = readJson(path.join(REPO, 'adapters/codex/hooks/plugin-hooks.json')).hooks;
    assert.ok(hooks.SubagentStart, 'SubagentStart hook should be registered');
    assert.ok(hooks.SubagentStop, 'SubagentStop hook should be registered');
  });

  it('packages the agent-dashboard skills inside the Codex plugin root', () => {
    const codexSkills = path.join(REPO, 'adapters/codex/skills');
    const claudeSkills = path.join(REPO, 'adapters/claude-code/skills');

    assert.deepEqual(skillNames(codexSkills), skillNames(claudeSkills));
    assert.deepEqual(relativeFiles(codexSkills), relativeFiles(claudeSkills));
  });

  it('uses Codex skill references in Codex-packaged skill content', () => {
    const codexSkills = path.join(REPO, 'adapters/codex/skills');
    const skillReference = '(?:feature|fix|refactor|chore|implement|investigate|pr|rca)';
    const agentDashboardSlash = new RegExp(`/agent-dashboard:${skillReference}\\b`);
    const bareSlash = new RegExp(`(^|[^\\w$])/${skillReference}\\b`);

    for (const relativeFile of relativeFiles(codexSkills)) {
      const text = fs.readFileSync(path.join(codexSkills, relativeFile), 'utf8');
      assert.doesNotMatch(text, agentDashboardSlash, `${relativeFile} should not use Claude agent-dashboard slash references`);
      assert.doesNotMatch(text, bareSlash, `${relativeFile} should not use bare Claude slash references`);
    }

    assert.match(
      fs.readFileSync(path.join(codexSkills, 'feature/SKILL.md'), 'utf8'),
      /\$agent-dashboard:feature\b/,
    );
  });

  it('does not publish Claude-only workflow primitives in Codex skill content', () => {
    const codexSkills = path.join(REPO, 'adapters/codex/skills');
    const forbidden = [
      ['EnterPlanMode', /\bEnterPlanMode\b/],
      ['ExitPlanMode', /\bExitPlanMode\b/],
      ['AskUserQuestion', /\bAskUserQuestion\b/],
      ['Agent() subagent calls', /\bAgent\s*\(/],
      ['dangerouslyDisableSandbox', /\bdangerouslyDisableSandbox\b/],
      ['Claude Agent run_in_background argument', /\brun_in_background\b/],
      ['Claude plan directory', /~\/\.claude\/plans/],
      ['Claude projects directory', /~\/\.claude\/projects/],
      ['Claude Code plan approval', /\bCC'?s plan-mode|\bClaude Code\b.*\bplan\b/i],
      ['codex-delegate slash command', /\/codex-delegate\b/],
      ['codex setup slash command', /\/codex:setup\b/],
    ];

    for (const relativeFile of relativeFiles(codexSkills)) {
      const text = fs.readFileSync(path.join(codexSkills, relativeFile), 'utf8');
      for (const [label, pattern] of forbidden) {
        assert.doesNotMatch(text, pattern, `${relativeFile} should not use ${label}`);
      }
    }
  });

  it('documents Codex-native planning and delegation primitives', () => {
    const feature = fs.readFileSync(path.join(REPO, 'adapters/codex/skills/feature/SKILL.md'), 'utf8');
    const implement = fs.readFileSync(path.join(REPO, 'adapters/codex/skills/implement/SKILL.md'), 'utf8');

    assert.match(feature, /\/plan\b/);
    assert.match(feature, /<proposed_plan>[\s\S]*<\/proposed_plan>/);
    assert.match(feature, /\brequest_user_input\b/);
    assert.match(feature, /\brequire_escalated\b/);
    assert.match(feature, /\.feature-plan-path/);
    assert.match(implement, /\bspawn_agent\b/);
    assert.match(implement, /\bworker\b/);
  });

  it('documents a complete Codex request_user_input schema in feature', () => {
    const feature = fs.readFileSync(path.join(REPO, 'adapters/codex/skills/feature/SKILL.md'), 'utf8');

    assert.match(feature, /request_user_input\(\{/);
    assert.match(feature, /\bid:\s*["'][^"']+["']/);
    assert.match(feature, /\bheader:\s*["'][^"']+["']/);
    assert.match(feature, /\bquestion:\s*["'][^"']+["']/);
    assert.match(feature, /\boptions:\s*\[/);
  });

  it('forces Codex feature planning before interview or plan drafting', () => {
    const feature = fs.readFileSync(path.join(REPO, 'adapters/codex/skills/feature/SKILL.md'), 'utf8');
    const phase2 = feature.slice(feature.indexOf('### Phase 2: Plan'));

    const planMode = phase2.indexOf('Enter Codex Plan Mode');
    const research = phase2.indexOf('Research');
    const interview = phase2.indexOf('request_user_input');

    assert.notEqual(planMode, -1, 'feature skill must explicitly enter Codex Plan Mode');
    assert.ok(planMode < research, 'Plan Mode must be active before research');
    assert.ok(planMode < interview, 'Plan Mode must be active before request_user_input');
    assert.doesNotMatch(phase2, /research first, interview second, plan mode third/i);
  });

  it('keeps Codex worktree setup stampable by dashboard hooks', () => {
    const branchPrefixes = { feature: 'feat', fix: 'fix', refactor: 'refactor' };

    for (const [skillName, branchPrefix] of Object.entries(branchPrefixes)) {
      const text = fs.readFileSync(path.join(REPO, `adapters/codex/skills/${skillName}/SKILL.md`), 'utf8');

      assert.doesNotMatch(
        text,
        /mkdir -p[^\n]+&&\s*git worktree add/,
        `${skillName} must not hide git worktree add behind a chained shell command`,
      );
      assert.match(
        text,
        /git worktree add[^\n]+as its own `exec_command` tool call/,
        `${skillName} must require standalone git worktree add so hooks can pin the worktree`,
      );
      assert.match(
        text,
        /stamp-worktree\.js/,
        `${skillName} must still run the explicit stamp helper after creating the worktree`,
      );
      assert.match(
        text,
        new RegExp(`git worktree add -b ${branchPrefix}/<name> \\.\\./worktrees/<app>/<name> main`),
        `${skillName} must put -b before the worktree path so hooks can observe the branch`,
      );
    }
  });

  it('keeps read-only Codex skills free of mutating git setup commands', () => {
    const readOnlySkills = ['investigate', 'rca'];
    const forbidden = [
      ['checking out branches', /\bgit checkout\b/],
      ['pulling remotes', /\bgit pull\b/],
      ['stashing changes', /\bgit stash\b/],
      ['switching branches', /\bswitch branches\b/i],
    ];

    for (const skillName of readOnlySkills) {
      const text = readCodexSkill(skillName);
      for (const [label, pattern] of forbidden) {
        assert.doesNotMatch(text, pattern, `${skillName} must stay read-only and avoid ${label}`);
      }
    }
  });

  it('pairs Codex spawn_agent instructions with wait_agent unless explicitly fire-and-forget', () => {
    const codexSkills = path.join(REPO, 'adapters/codex/skills');

    for (const relativeFile of relativeFiles(codexSkills)) {
      const text = fs.readFileSync(path.join(codexSkills, relativeFile), 'utf8');
      if (!/\bspawn_agent\b/.test(text)) continue;

      assert.match(
        text,
        /\bwait_agent\b|fire-and-forget/i,
        `${relativeFile} mentions spawn_agent but does not say how results are consumed`,
      );
    }
  });

  it('uses only Codex-supported subagent roles in skill text', () => {
    const codexSkills = path.join(REPO, 'adapters/codex/skills');
    const unsupportedRoles = [
      ['refactor-cleaner', /\brefactor-cleaner\b/],
      ['planner', /\bplanner\b/],
      ['general-purpose', /\bgeneral-purpose\b/],
      ['Plan subagent', /\bPlan subagent\b/],
    ];

    for (const relativeFile of relativeFiles(codexSkills)) {
      const text = fs.readFileSync(path.join(codexSkills, relativeFile), 'utf8');
      for (const [label, pattern] of unsupportedRoles) {
        assert.doesNotMatch(text, pattern, `${relativeFile} should not reference unsupported Codex role ${label}`);
      }
    }
  });

  it('requires confirmation and untracked-only safeguards before destructive PR cleanup', () => {
    const pr = readCodexSkill('pr');

    assert.match(pr, /\buntracked only\b/i);
    assert.match(pr, /\brequest_user_input\b|\bconfirm/i);
    assert.match(pr, /\bgit status --porcelain\b/);
    assert.match(pr, /\brm -rf\b/);
  });

  it('keeps Codex hard-rules blocks concise and executable', () => {
    const codexSkills = path.join(REPO, 'adapters/codex/skills');

    for (const relativeFile of relativeFiles(codexSkills)) {
      const text = fs.readFileSync(path.join(codexSkills, relativeFile), 'utf8');
      const match = text.match(/<codex_skill_must>\n([\s\S]*?)\n<\/codex_skill_must>/);
      if (!match) continue;

      const lines = match[1].split('\n').filter(line => /^\d+\./.test(line.trim()));
      assert.ok(lines.length <= 6, `${relativeFile} has too many hard rules for reliable Codex adherence`);
      for (const line of lines) {
        assert.ok(line.length <= 260, `${relativeFile} hard rule is too long: ${line}`);
      }
    }
  });
});
