# AI Agent Guidelines

This document contains rules that apply specifically to AI coding assistants and autonomous agents working in this repository. Engineering standards and architectural decisions are documented in [`engineering-standards.md`](engineering-standards.md) and [`ml-engine.md`](ml-engine.md); do not duplicate them here.

---

## Mandatory reading order

Before proposing or applying any change, read these files in full:

1. [`README.md`](../README.md) — project overview, startup contract, architecture diagram
2. [`docs/engineering-standards.md`](engineering-standards.md) — all engineering practices and domain rules
3. [`docs/ml-engine.md`](ml-engine.md) — ML pipeline, feature vector, walk-forward CV, conviction calibration

---

## Source of truth

Documentation is the source of truth. When documentation and code disagree, surface the discrepancy explicitly — do not silently pick one side and proceed.

---

## Documentation language

All documentation (`README.md`, files under `docs/`) and all AI configuration files must be written in English. If content in another language is found, flag it before acting.

---

## Operational rules

- Propose a plan and surface any ambiguities before applying changes. Do not make structural edits silently.
- Do not infer architectural intent from code alone when authoritative documentation exists.
- Match the comment density, naming conventions, and idiom of the surrounding code.

---

## Off-limits without explicit approval

The following changes require the human author to explicitly request them. Do not introduce them as a side effect of another task:

| Change | Reason |
|---|---|
| New CLI flags or startup parameters | Startup contract is single-parameter by design (see `README.md`) |
| New direct dependencies | Must pass the four-point checklist in `engineering-standards.md` § Dependency Minimisation |
| Modifications to an already-applied schema migration | Prohibited — add a new version instead (see `engineering-standards.md` § Schema Migrations) |
| New signal types | Requires a new `Type` constant and a `New*` constructor in `internal/signal/signal.go` |
| Sharpe Ratio as a performance metric | Prohibited; Sortino Ratio is the canonical metric (see `ml-engine.md`) |
