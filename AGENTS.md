# agentGo Repository Guidelines

## Communication

- Use Chinese for discussion and Markdown documents by default.
- Prefer plain Markdown for plans, specs, and explanations. Do not introduce
  HTML unless explicitly requested.
- Keep `AGENTS.md` focused on agent collaboration rules. Project background,
  architecture details, and design discussion should live under `docs/`.

## Workflow

- For architecture changes, boundary refactors, or non-trivial feature work,
  write a Chinese spec first under `docs/superpowers/specs/`.
- After the spec is approved, write an implementation plan under
  `docs/superpowers/plans/` before changing code.
- Once the user approves the plan, move directly into implementation instead of
  reopening the same design discussion.

## Development

- Use TDD by default. Start from a failing test when adding or changing
  behavior.
- Use `apply_patch` for manual file edits.
- Format Go code after edits with `gofmt -w` or an approved formatter.
- Run targeted tests for the packages you touched before finishing.

## Docs And References

- Use `docs/Crush_AGENTS.md` as a reference source, not as a template to copy
  blindly.
- Use [docs/development-conventions.md](/root/agentGo/docs/development-conventions.md)
  for current layering, interface, persistence, and event-boundary rules.
- When discussion converges, persist the result as a Markdown document instead
  of leaving the decision only in chat history.
