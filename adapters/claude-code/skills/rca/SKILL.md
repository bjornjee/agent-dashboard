---
name: rca
description: Root cause analysis for process crashes, server deaths, and unexplained system failures using macOS logs, session forensics, and code tracing
when_to_use: when a process died unexpectedly, a server crashed, the tmux server vanished, an agent exited mid-task, or any system-level failure with no clear cause. Use when the user says "what killed X", "RCA", "post-mortem", or "why did Y crash". NOT for bugs with clear stack traces (use /fix) or feature design (use /feature).
version: 1.0.0
disable-model-invocation: true
---

Root cause analysis for a system-level failure. **Gather ALL evidence before reasoning about the cause. No exceptions.**

Incident description: $ARGUMENTS

## Instructions

Follow these phases strictly in order. Do NOT speculate or reason about root cause until Phase 5. Every phase has a gate.

---

### Phase 1: Scope the Incident

1. Parse the incident description — what died? (process, tmux server, container, service, etc.)
2. Establish the **time window**: ask the user or derive from context when the failure was noticed and when things last worked.
3. Identify what was running at the time — check:
   - `tmux list-sessions` / `tmux list-panes -a` (if tmux is back up)
   - Shell history with timestamps to reconstruct user activity:
     ```
     tail -500 ~/.zsh_history | while IFS= read -r line; do
       if echo "$line" | grep -q "^: [0-9]"; then
         ts=$(echo "$line" | sed 's/^: \([0-9]*\):.*/\1/')
         cmd=$(echo "$line" | sed 's/^: [0-9]*:[0-9]*;//')
         dt=$(date -r "$ts" "+%Y-%m-%d %H:%M:%S" 2>/dev/null)
         echo "$dt  $cmd"
       fi
     done
     ```
   - Filter to the time window around the incident.

4. Identify **all active agents/processes** — check Claude Code session logs:
   ```
   ls -lt ~/.claude/projects/<project-dir>/*.jsonl | head -15
   ```
   Cross-reference modification times with the incident window.

**Gate:** Time window is established. List of active processes/sessions at the time is known.

---

### Phase 2: System-Level Evidence (macOS Unified Log)

Run ALL of the following. Do not skip any — even if you expect empty results, the absence of evidence is evidence.

#### 2a. Target process events
```
log show --start "<start>" --end "<end>" --style compact \
  --predicate 'process == "<target-process>"'
```
Replace `<target-process>` with the crashed process (e.g., "tmux", "tmux-server", "agent-dashboard"). Run for each relevant process name.

#### 2b. Code signature / AMFI failures
```
log show --start "<start>" --end "<end>" --style compact \
  --predicate 'subsystem == "com.apple.AMFI" OR eventMessage CONTAINS "AMFI" OR eventMessage CONTAINS "code signature" OR eventMessage CONTAINS "CMS blob"'
```

#### 2c. Process termination signals
```
log show --start "<start>" --end "<end>" --style compact \
  --predicate 'eventMessage CONTAINS "SIGKILL" OR eventMessage CONTAINS "SIGTERM" OR eventMessage CONTAINS "SIGHUP" OR eventMessage CONTAINS "taskgated"'
```

#### 2d. Memory pressure (jetsam)
```
log show --start "<start>" --end "<end>" --style compact \
  --predicate 'category == "jetsam" OR eventMessage CONTAINS "memorystatus" OR eventMessage CONTAINS "jetsam"'
```

#### 2e. Power management (sleep/wake)
```
log show --start "<start>" --end "<end>" --style compact \
  --predicate 'subsystem == "com.apple.powerd" OR eventMessage CONTAINS "Wake reason" OR eventMessage CONTAINS "DarkWake" OR eventMessage CONTAINS "caffeinate"'
```

#### 2f. Kernel events
```
log show --start "<start>" --end "<end>" --style compact \
  --predicate 'sender == "kernel"' | head -50
```

#### 2g. Crash reports
```
find ~/Library/Logs/DiagnosticReports /Library/Logs/DiagnosticReports \
  -name "*<process-name>*" -newer <reference-file> 2>/dev/null
```

#### 2h. Application-level crash logs
Check for any crash/diagnostic files the application writes (e.g., `crash.log`, `debug-keys.log`, state files).

**Gate:** All 8 log categories checked. Results recorded with exact timestamps. Note which categories returned empty.

---

### Phase 3: Session Forensics

For each Claude Code session active during the incident window:

1. **Extract all Bash tool calls** — these are the commands Claude actually executed:
   ```python
   python3 << 'PYEOF'
   import json, os

   fpath = "<session-jsonl-path>"
   with open(fpath) as f:
       for line in f:
           try:
               obj = json.loads(line)
               if obj.get('type') == 'assistant':
                   content = obj.get('message', {}).get('content', [])
                   if isinstance(content, list):
                       for block in content:
                           if isinstance(block, dict) and block.get('type') == 'tool_use' and block.get('name') == 'Bash':
                               cmd = block.get('input', {}).get('command', '')
                               print(f'CMD: {cmd[:400]}')
                               print()
           except:
               pass
   PYEOF
   ```

2. **Flag dangerous commands** — search for:
   - `kill`, `pkill`, `killall` (process termination)
   - `tmux kill-*` (tmux destruction)
   - `rm -rf`, `git clean`, `git reset --hard` (destructive ops)
   - `signal`, `SIGKILL`, `SIGTERM` (signal sending)
   - Any command referencing the crashed process

3. **Extract subagent launches** — check for Agent tool calls, especially background agents:
   ```python
   # Same pattern but filter for block.get('name') == 'Agent'
   # Check run_in_background, prompt content
   ```

4. **Identify the LAST command before the crash** — cross-reference the session's final tool call timestamp with the system log timestamps from Phase 2.

**Gate:** Every active session's commands are extracted. The last command before the crash is identified with timestamp.

---

### Phase 4: Code Path Analysis

Trace the code paths that were active at the time of the crash:

1. **What was the last command doing?** — if it was a build/test/run command, check:
   - Does it spawn subprocesses? (`exec.Command`, `go test`, `npm test`)
   - Do those subprocesses interact with the crashed system? (e.g., tmux commands, port binding)
   - Are the spawned binaries code-signed? (critical on macOS)

2. **Check the crashed application's code** for:
   - All interactions with the system that died (e.g., `grep -rn "tmux" internal/ cmd/`)
   - Cleanup/teardown code that could cascade (e.g., session cleanup, pane cleanup)
   - Signal handlers and their behavior
   - Panic/crash recovery mechanisms
   - Race conditions in concurrent code

3. **Instrument production runners to trace leaked subprocess calls** — when tests crash an external system (tmux, databases, etc.), add debug logging to every production runner that spawns subprocesses. Log to a persistent file so the output survives the crash:
   ```go
   // Add to every production runner method that calls exec.Command
   func debugLogExec(caller, name string, args []string) {
       f, _ := os.OpenFile("/tmp/test-exec.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
       if f != nil {
           defer f.Close()
           fmt.Fprintf(f, "[%s] %s %v\n", caller, name, args)
       }
   }
   ```
   Run tests, then inspect the log to identify which packages and code paths are leaking real subprocess calls. An empty log file (or no file created) confirms zero leaks. This is especially useful when subprocess calls are indirect — e.g., an HTTP handler calls a function that calls a package function that eventually hits `exec.Command` through 3+ layers of indirection.

4. **Check configuration** that affects crash behavior:
   - tmux: `exit-empty`, `exit-unattached`, `destroy-unattached`
   - Process supervisors: launchd plists, systemd units
   - Application-level restart/respawn logic

5. **Check recent code changes** in the area:
   ```
   git log --oneline --since="1 week ago" -- <relevant-paths>
   ```

**Gate:** Code paths from the last command to the crash point are traced. Configuration that affects cascading failures is documented.

---

### Phase 5: Root Cause Analysis

**Only now may you reason about the cause.** Build the argument from evidence, not speculation.

**No exceptions:**
- Don't reason about the cause in Phases 1–4. Collect first; reason later.
- Don't skip a Phase 2 log category because "I'm sure it's not that". Empty results are evidence too.
- Don't shortcut to a likely cause without ruling out alternatives in step 4.
- Don't claim a root cause with gaps in the timeline. State the gap as an unknown instead.

1. **Construct the event chain** — a timestamped sequence from the trigger to the final failure. Every link must cite evidence from Phases 2-4:
   - `HH:MM:SS.mmm` — [source: unified log / session log / crash log] — event description

2. **Identify the root cause** — the earliest event in the chain that, if prevented, would have avoided the failure. Distinguish:
   - **Root cause**: the underlying defect or condition
   - **Contributing factors**: things that made it worse (e.g., `exit-empty on`)
   - **Symptoms**: what the user observed

3. **Verify the chain is complete** — are there gaps? If yes, state them explicitly as unknowns.

4. **Rule out alternatives** — for each plausible alternative cause, cite the evidence that eliminates it:
   | Alternative | Ruled out by |
   |-------------|-------------|
   | Example: manual kill-server | No kill-server in any session log |
   | Example: sleep/wake SIGKILL | No powerd sleep events in window |

---

### Phase 6: Report

Present a structured report:

#### Timeline
A table with timestamp, event, and evidence source for every link in the chain.

#### Root Cause
One paragraph explaining what happened and why, with evidence citations.

#### Contributing Factors
Bullet list of conditions that enabled or worsened the failure.

#### Evidence Summary
| Category | Result |
|----------|--------|
| AMFI/codesign | Found / Not found |
| Jetsam | Found / Not found |
| Sleep/wake | Found / Not found |
| Signals | Found / Not found |
| Session commands | Suspicious / Clean |
| Crash reports | Found / Not found |

#### Ruled-Out Alternatives
Table from Phase 5.

#### Recommended Fix
Concrete, minimal actions to prevent recurrence. Reference specific files and line numbers.

**Gate:** Report delivered. Every claim cites evidence. Unknowns are stated explicitly.

---

### Transition to implementation

This skill is read-only. If the user asks to implement a fix based on your findings, **do not start editing files**. Instead, hand off to the appropriate skill:

- Bug fix or prevention → suggest `/fix <description>`
- Structural change to prevent recurrence → suggest `/refactor <description>`
- New safeguard or feature → suggest `/feature <description>`

These skills handle branch/worktree setup, TDD, review, and delivery. Starting implementation inline from `/rca` skips those gates.

---

## Red Flags — STOP

If you catch yourself saying or thinking any of these, pause and re-read the relevant phase:

- "I see the cause already, skip to the report" → no. Phase 2 collects evidence the report cites. Skipping it means an uncited report.
- "AMFI/jetsam/sleep is unlikely, skip those queries" → run them anyway. Empty result is evidence.
- "I'll start fixing the cause inline" → wrong skill. This is read-only. Hand off to `/fix` after the report.
- "The session log doesn't have the smoking gun, the cause must be code" → check the other log categories first. The smoking gun is rarely where you expect.
- "I'll write a one-line summary instead of the structured report" → the structure exists because RCAs without it get re-relitigated. Use it.
- "Gap in the timeline? I'll fill it with a likely event" → no. Mark it as unknown. Speculation in an RCA report is worse than gaps.
