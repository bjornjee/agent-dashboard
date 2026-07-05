# Environment setup

Set up the dev environment in the worktree.

1. Auto-detect project type from project files (highest match wins):

   | Priority | Signal | Type |
   |----------|--------|------|
   | 1 | `react-native` in package.json dependencies | Mobile |
   | 2 | `next`, `vite`, or `webpack` in package.json | Web |
   | 3 | `requirements.txt`, `pyproject.toml`, or `setup.py` | Python |
   | 4 | `go.mod` | Go |
   | 5 | `Dockerfile` or `docker-compose.yml` | Containerized |

2. Install dependencies appropriate for the project type (e.g. `pip install`, `npm install`, `go mod download`). Configure ports, create emulators/simulators as needed. For browser UI work, allocate worktree-local Playwright/server/profile/output resources per `ui-automation.md` in this directory.
3. Symlink large source-content directories (`data/`, `datasets/`, `evals/`, `models/`, `artifacts/`) from the source repo rather than copying. NEVER symlink build outputs or per-project caches (`.next/`, `dist/`, `build/`, `out/`, `target/`, `.turbo/`, `.cache/`, `.parcel-cache/`, `.vite/`, `__pycache__/`, `.pytest_cache/`, `.gradle/`, `.venv/`, `node_modules/`) — they bake absolute paths and corrupt across worktrees, and must be regenerated per-worktree.
4. On success, write a sentinel file: `touch .env-setup-done`
   On failure, write the error: `echo "<error message>" > .env-setup-failed`
