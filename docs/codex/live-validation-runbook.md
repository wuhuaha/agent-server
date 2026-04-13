# Live Validation Runbook

This runbook standardizes live validation outside the fast local and CI command surface.
Use it when the target change needs a running worker, a running `agentd`, browser interaction, or archived comparison artifacts.

## Goals

- keep live validation repeatable without widening the default CI surface
- keep archived artifacts comparable across local runs
- separate quick local smoke runs from comparison-worthy baseline runs

## Artifact Roots

Use one of these roots:

- `artifacts/live-smoke/YYYYMMDD/<profile>/`
  - quick local validation
  - okay to overwrite or rerun frequently
- `artifacts/live-baseline/YYYYMMDD/<profile>/`
  - comparison-worthy runs you expect to reference later in docs, issues, or roadmap work
  - do not reuse casually across unrelated stacks

`YYYYMMDD` should be the run date in local machine time.

## Canonical Profile Names

Prefer these profile directory names:

- `desktop-full`
- `desktop-regression`
- `desktop-server-endpoint-preview`
- `rtos-mock`
- `samples`
- `web-h5-manual`

If you need a variant, append a short suffix instead of inventing a new root shape, for example:

- `desktop-regression-deepseek`
- `rtos-mock-barge-in`
- `web-h5-manual-mimo`

## Canonical Files In A Profile Root

Keep these names stable when the run owns the full stack:

- `report.json`
- `input.wav` when a synthetic or copied input sample is part of the run
- `agentd.log`
- `agentd.err.log`
- `worker.log`
- `worker.err.log`

For `web-h5-manual`, the profile root should also keep these manual-evidence files:

- `capture.json`
- `manual-checklist.md`
- `server/healthz.txt`
- `server/info.json`
- `server/realtime.json`
- `pages/`
- `screenshots/`
- `exports/`
- `logs/`

The runner and RTOS mock then create one `run_<id>/` directory below the profile root for replay-friendly artifacts.

## Runner Artifact Layout

Desktop runner:

- root:
  - `report.json`
  - `run_<id>/`
- inside `run_<id>/scenario-name/`:
  - `events.json`
  - `response.txt`
  - `scenario.json`
  - `received-audio.wav` when audio arrived

RTOS mock:

- root:
  - `report.json`
  - optional top-level `received-audio.wav`
  - `run_<id>/`
- inside `run_<id>/`:
  - `events.json`
  - `response.txt`
  - `run.json`
  - `received-audio.wav` when audio arrived

## Canonical Commands

Desktop smoke against an already running local server:

```bash
mkdir -p artifacts/live-smoke/$(date +%Y%m%d)/desktop-full
PYTHONPATH=clients/python-desktop-client/src python3 -m agent_server_desktop_client.runner \
  --scenario full \
  --http-base http://127.0.0.1:8080 \
  --output artifacts/live-smoke/$(date +%Y%m%d)/desktop-full/report.json \
  --save-rx-dir artifacts/live-smoke/$(date +%Y%m%d)/desktop-full
```

Desktop archived regression baseline:

```bash
mkdir -p artifacts/live-baseline/$(date +%Y%m%d)/desktop-regression
PYTHONPATH=clients/python-desktop-client/src python3 -m agent_server_desktop_client.runner \
  --scenario regression \
  --http-base http://127.0.0.1:8080 \
  --wav artifacts/live-baseline/$(date +%Y%m%d)/samples/input.wav \
  --output artifacts/live-baseline/$(date +%Y%m%d)/desktop-regression/report.json \
  --save-rx-dir artifacts/live-baseline/$(date +%Y%m%d)/desktop-regression
```

RTOS mock archived baseline:

```bash
mkdir -p artifacts/live-baseline/$(date +%Y%m%d)/rtos-mock
PYTHONPATH=clients/python-desktop-client/src python3 -m agent_server_desktop_client.rtos_mock \
  --http-base http://127.0.0.1:8080 \
  --wav artifacts/live-baseline/$(date +%Y%m%d)/samples/input.wav \
  --output artifacts/live-baseline/$(date +%Y%m%d)/rtos-mock/report.json \
  --save-rx-dir artifacts/live-baseline/$(date +%Y%m%d)/rtos-mock
```

Windows one-command smokes now default to:

- `scripts/smoke-funasr.ps1`
  - `artifacts/live-smoke/YYYYMMDD/desktop-full/`
- `scripts/smoke-rtos-mock.ps1`
  - `artifacts/live-smoke/YYYYMMDD/rtos-mock/`

Linux one-command smokes now default to the same profile roots:

- `scripts/smoke-funasr.sh`
  - `artifacts/live-smoke/YYYYMMDD/desktop-full/`
- `scripts/smoke-rtos-mock.sh`
  - `artifacts/live-smoke/YYYYMMDD/rtos-mock/`

If no `--wav` is provided, the Linux helpers generate a local silence `input.wav` so the stack can still be exercised end to end without an external sample file.

Web/H5 manual evidence scaffolding:

```bash
./scripts/web-h5-manual-capture.sh --mode built-in
./scripts/web-h5-manual-capture.sh --mode both --standalone-base http://127.0.0.1:18081
```

That helper creates the canonical `web-h5-manual` artifact root, fetches server and page snapshots, and writes `manual-checklist.md` so screenshots, console exports, and WAV exports land in predictable locations.

## Runbook Selection

Choose the smallest live path that proves the change:

- transport or runtime regression check:
  - desktop `full`
- comparison-worthy quality or migration slice:
  - desktop `regression`
- hidden endpoint preview tuning:
  - desktop `server-endpoint-preview`
- RTOS-facing interruption or session behavior:
  - `rtos-mock`
- browser-only interaction or UX check:
  - `web-h5-manual`

## Machine Caveats On This Host

On this WSL2 machine, live listeners and live probes may need the same unrestricted execution context.
This especially applies when:

- starting `agentd` or workers on local ports
- probing those listeners from the same Codex session
- running Docker-backed validation

Repository code checks can still run inside the default sandbox.
Live stack validation should assume the listener and the probe need the same network context unless proven otherwise.

## Recording Rule

When a live run is important enough to influence roadmap, protocol, or architecture decisions:

- record the artifact root in `plan.md` or the relevant architecture note
- summarize the outcome in `.codex/change-log.md`
- capture any caveat in `.codex/issues-and-resolutions.md`

Avoid scattering one-off artifact paths across unrelated docs when the runbook root can be referenced once.
