# Agent Instructions

Read this entire file before starting any task. Treat this as your Constitution.

## Self-Correcting Rules Engine

This file contains a growing ruleset that improves over time. **At session start, read the entire "Learned Rules" section before doing anything.**

### How it works

1. When the user corrects you or you make a mistake, **immediately append a new rule** to the "Learned Rules" section at the bottom of this file.
2. Rules are numbered sequentially and written as clear, imperative instructions.
3. Format: `N. [CATEGORY] Never/Always do X — because Y.`
4. Categories: `[STYLE]`, `[CODE]`, `[ARCH]`, `[TOOL]`, `[PROCESS]`, `[DATA]`, `[UX]`, `[OTHER]`
5. Before starting any task, scan all rules below for relevant constraints.
6. If two rules conflict, the higher-numbered (newer) rule wins.
7. Never delete rules. If a rule becomes obsolete, append a new rule that supersedes it.

### When to add a rule

- User explicitly corrects your output ("no, do it this way")
- User rejects a file, approach, or pattern
- You hit a bug caused by a wrong assumption about this codebase
- User states a preference ("always use X", "never do Y")

### Rule format example

```
14. [CODE] Always use `bun` instead of `npm` — user preference, bun is installed globally.
15. [STYLE] Never add emojis to commit messages — project convention.
16. [ARCH] API routes live in `src/server/routes/`, not `src/api/` — existing codebase pattern.
```

---

## Learned Rules

<!-- New rules are appended below this line. Do not edit above this section. -->
17. [TOOL] Assign PowerShell report collections before formatting and parenthesize pipeline results before using -join — compound blocks and operator binding can otherwise fail during read-only inspection.
18. [TOOL] Delimit PowerShell variables before literal colons in interpolated strings, such as ${path}: — PowerShell can parse an adjacent colon as part of a drive-qualified variable reference.
19. [TOOL] Preserve Windows backslashes when embedding paths in JavaScript here-strings; prefer String.raw for path literals — extra escaping can silently target a nonexistent path.
20. [TOOL] Avoid raw backticks inside functions.exec template literals; use a normal JavaScript string or escape them — nested templates can fail before the command runs.
21. [TOOL] Reconstruct absolute paths from the complete reported relative path before inspecting a file — omitting an intermediate directory makes a valid target appear missing.
22. [TOOL] Verify an archive format header before parsing embedded package metadata — container framing differs from plain JSON offsets and naive reads can misidentify ownership.
23. [TOOL] Always pass directories with rg -g filters instead of wildcard file paths in PowerShell — native commands can receive unexpanded wildcard paths and fail with invalid filename errors.

24. [TOOL] After any bulk scripted text replacement, check every touched file's length before continuing - a failed read can write empty files that look like successful edits.
25. [PROCESS] Never claim a safety guarantee from a schema or validator without an adversarial record that fails because of it - documented intent is not enforcement.
26. [TOOL] Always use verified context in every apply_patch hunk and inspect file state after a failed multi-file patch — a placeholder anchor can fail after earlier files have already changed.
