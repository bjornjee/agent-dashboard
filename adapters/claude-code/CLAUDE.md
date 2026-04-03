# adapters/claude-code

## Version bumping

When any file under `adapters/claude-code/` is changed, you MUST bump the patch version in both:

1. `adapters/claude-code/.claude-plugin/plugin.json` — the `"version"` field
2. `.claude-plugin/marketplace.json` — the `"version"` field inside the `plugins` array

Both files must stay in sync with the same version string.
