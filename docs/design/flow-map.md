# Flow map — agent-dashboard web UI

Three primary user flows define the surface area of this redesign. Each flow lists the steps a user takes, the visitor goal at each step, the decision points, and the failure/exit states. One screenshot per step will be captured for the grader in Gate 1.

Scope: List + Detail + Create. Usage view is out of this PR (rendered with the new tokens but otherwise untouched). Light theme deferred.

Form factor: mobile-first at 390×844; desktop at 1280×800 = centered narrow column (≤ 640px content width) with the same surface chrome.

---

## Flow A — Land → pick a running agent → read its transcript → reply

| Step | Visitor goal | Render shows | Decision point | Failure exit |
|---|---|---|---|---|
| **A1** | "Which of my agents is doing what right now?" | Agent list grouped by state. Each row: agent name, branch, model, elapsed, cost, status dot. Section headers: `BLOCKED`, `WAITING`, `RUNNING`, `REVIEW`, `PR`, `MERGED`. | Tap a row → A2. Tap "New" CTA → Flow B. | Empty list = empty-state prose: "No agents running — tap **Chat** to start one." |
| **A2** | "What is this agent doing? What did it just say?" | Detail view: top app bar (back + agent name + machine sub-line + spinner if running + kebab), Chat/Activity/Diff/Plan tabs, chat scroll with flat-prose messages. Sticky composer at bottom. | Switch tabs to inspect activity/diff/plan. Tap composer → A3. Tap back → A1. | Agent in `error` state shows error pill at top of chat; composer becomes "Type a reply…". |
| **A3** | "Send a reply / approval / new instruction." | Composer expands. Send button activates when non-empty. | Tap send → reply renders right-aligned as a small dark pill; assistant response streams in flat prose below. | Send-while-running queues; queued indicator appears at top of composer. |

**A1 visitor invariant:** the first eye-stop must be the agent that needs human input — `BLOCKED` group at the top, ahead of `RUNNING`.

**A2 visitor invariant:** the last assistant message must be in the first viewport when the page loads. No scrolling-to-find-the-latest.

---

## Flow B — Land → "+ New" → spawn agent

| Step | Visitor goal | Render shows | Decision point | Failure exit |
|---|---|---|---|---|
| **B1** | "Start a new agent in this folder with this prompt." | Create view: top app bar (back + "New agent"), large display title "What should we work on?" + composer with placeholder "Do anything", folder picker below composer, harness chip (`claude` / `codex`), optional settings disclosure. | Type prompt + pick folder → tap send → B2. Tap back → A1. | Required folder unset = send disabled; helper text "Pick a folder to spawn in." |
| **B2** | "Did it spawn? Where can I see it?" | Brief inline confirmation, then auto-redirect to the new agent's detail view (Flow A2). | — | Spawn error = inline error card under composer with the failure reason; composer stays populated. |

**B1 visitor invariant:** the composer is the visual centre. Folder + harness are secondary chrome, not primary UI.

---

## Flow C — Navigate between primary destinations

Codex mobile does **not** use a bottom tab bar. The actual pattern is a floating bottom dock with **Search** + a single primary action (e.g., "Chat"). Less-frequent destinations live behind the top-bar kebab.

The dashboard adopts that pattern verbatim:

| Step | Visitor goal | Render shows | Decision point | Failure exit |
|---|---|---|---|---|
| **C1** | "Open something else (Usage / Settings)." | On Agents list (A1) only: floating bottom dock = `Search agents` pill (left) + white `+ New` CTA pill (right). Top-bar kebab opens a sheet with `Usage`, `Settings`, `Notifications`. | Tap kebab → sheet. Tap an item → corresponding view. | Sheet dismisses on outside-tap or back gesture. |

**Resolution of the earlier "bottom tab bar" decision:** the Codex mobile reference uses a floating dock with primary CTAs, not a 3-tab persistent bar. We honor what the reference actually shows. Usage + Settings move into the kebab. This is one decision I'm taking against the original interview answer because the reference material contradicts it — calling it out explicitly here so the grader and the next implementer have the rationale.

---

## Page-state inventory (the grader will look at one screenshot per row)

| ID | Page | State | Viewport |
|---|---|---|---|
| A1-m | Agent list | Mixed state groups, 3+ agents | 390×844 |
| A1-d | Agent list | Same data | 1280×800 |
| A1-empty | Agent list | No agents running | 390×844 |
| A2-m | Agent detail | RUNNING, mid-transcript, Chat tab | 390×844 |
| A2-tools | Agent detail | RUNNING, transcript with tool-call blocks | 390×844 |
| A2-composer | Agent detail | Composer focused, keyboard implied | 390×844 |
| A2-d | Agent detail | Same data | 1280×800 |
| B1-m | Create | Empty composer, folder unset | 390×844 |
| B1-filled | Create | Composer with prompt, folder set, harness chosen | 390×844 |
| C1-kebab | Kebab sheet | Open over A1-m | 390×844 |

Ten target screenshots. Baseline (current state) only has the 4 already captured — `current-list-mobile`, `current-detail-mobile`, `current-create-mobile`, `current-list-desktop`. The grader scores the baseline against these four; the gap to the full inventory above is the implementation work.
