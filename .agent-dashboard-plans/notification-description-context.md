# Improve Adapter Notification Descriptions

## Summary
Research found the adapter desktop notification hooks already include the current git branch in the notification subtitle, but the body/description does not reliably identify the agent or show the actual user-facing Ask. The change makes question/input notifications self-identifying in the compact macOS notification body.

## Key Changes
- Update adapter desktop notification formatting for both supported plugin adapters: Claude and Codex.
- For agent question/input notifications, build the body as: `<agent>/<branch>: <ask>`.
- Claude source: extract the first `AskUserQuestion` question from the alerting transcript tool input when a `Stop` notification is caused by `AskUserQuestion`.
- Codex source: read the sidecar agent state for `session_id` and use `pending_question.questions[0].question` for `request_user_input`/elicitation notifications when available.
- Agent source: use the existing sidecar `target` label, such as `main:0.1`.
- Branch source: keep using the existing `git branch --show-current` lookup from `cwd`, compacted to the final path segment.
- Preserve existing fallback behavior for non-question notifications, unreadable asks, and subtitles.

## Test Plan
- Verification profile: Targeted.
- Proof command: `npm test --prefix adapters/claude-code && npm test --prefix adapters/codex/hooks`.

## Assumptions
- “Ask” means the first pending question text from `AskUserQuestion` or Codex `request_user_input`.
- The notification description/body should include agent, branch, and Ask, even though branch remains in the subtitle.
- The default body format is context first, Ask second: `<agent>/<branch>: <ask>`.
