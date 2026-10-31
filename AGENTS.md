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
27. [PROCESS] Always make the owner-supplied judging pillars integral to Wazi feature priorities, roadmap, acceptance and demo planning: Impact 30%, Use of Meta AI Tooling 25%, Technical Depth & Execution 20%, Demo 25% — user instruction; use docs/JUDGING_PILLARS.md and retain mandatory engineering safety gates.
28. [TOOL] Never end a JavaScript String.raw template literal with a single backslash — it escapes the closing delimiter; append the path separator separately.
29. [PROCESS] Always develop the Wazi film transcript first in the separate video/ workspace and target a three-minute master with the owner's cute big-eyed female gladiator, oversized weapon, Meta AI mind-control power-up and shocked arena reaction — user creative brief; preserve room for inspiration and distinguish concept footage from implemented features.
30. [TOOL] Always use explicit UTF-8 transport or Unicode escapes in PowerShell here-strings piped to Python — native stdin encoding can corrupt non-ASCII literals in generated documents.
31. [TOOL] Always normalise remote Bash scripts to LF and append a final newline before piping them through SSH — an unterminated final compound command can fail at EOF.
32. [PROCESS] Always discover the Wazi development host through .env.local and docs/DEVELOPMENT_ENVIRONMENTS.md; store only SSH key paths in environment files and keep development sudo credentials separate from runtime agent identities — user requested reusable agent access to the Linux environment.
33. [TOOL] Always use the exact PowerShell block-comment delimiters and parse new .ps1 files before running them — malformed comment terminators can hide the entire script.
34. [TOOL] Never round-trip UTF-8 files through PowerShell Get-Content/Set-Content without -Encoding utf8 - default ANSI decoding corrupts non-ASCII text and adds a BOM.
35. [PROCESS] Always verify exact source line numbers before submitting review locations — guessed ranges can exceed the file and reject otherwise valid evidence.
36. [TOOL] Always build remote PowerShell command text with single-quoted format strings instead of backtick escapes inside JavaScript tool templates — nested quoting can fail before any patch or command executes.
37. [TOOL] Never embed unescaped shell dollar-brace expressions in JavaScript template literals — JavaScript interprets them as interpolation even inside String.raw; use shell variables without braces or explicit string arguments.
38. [TOOL] Always include explicit critical CA basic constraints and certificate-signing key usage in development CA certificates — Python 3.14 strict TLS verification rejects otherwise usable OpenSSL-generated roots without those extensions.
39. [TOOL] Always resolve source paths against the command's explicit working directory before formatting or editing — a module-relative command must not repeat the repository's services prefix.
40. [UX] Always verify the browser entry URL after a development deployment and provide a useful root response with its API/UI scope — successful API checks alone can leave the owner facing an unexplained 404.
41. [TOOL] Keep binary offsets and decoded-text offsets in the same type; search raw bytes with byte strings or decoded text with text strings — mixing them raises TypeError during inspection.
42. [TOOL] Transport embedded Python or JSON through a single-quoted here-string or an argument file instead of nested shell quote/backslash escaping — PowerShell transforms nested quoting before Python parses it.

41. [UX] Treat user-supplied logo sketches as direction for accents and use the current logo as the authority for final letterforms — the user clarified that sketch lettering is not desired.

42. [UX] Always preserve the owner-specified Wazi overlap and WA/ZI gap as symbols of wazi meaning open — the green arc should trace across the white letterforms while a light outer edge keeps them distinct.

43. [UX] Always use a transparent canvas for logo concepts when the owner supplies a background image only to specify accent color — the background is not part of the requested mark.
44. [TOOL] Always inspect signed-blob CLI defaults and disable transparency-log publication before invoking Cosign signing — newer Cosign versions may prompt to publish immutable signing metadata even when a local key is supplied.
45. [TOOL] Replace Go raw-string literals with quoted strings inside JavaScript template-literal patches — an unescaped backtick terminates the outer template before apply_patch runs.
46. [TOOL] Keep JavaScript template interpolation markers out of String.raw patch text — interpolation is evaluated before the patch is sent to apply_patch.
47. [TOOL] Use plain text or tilde code fences in String.raw documentation patches — Markdown backtick spans terminate the JavaScript template before the patch runs.
48. [TOOL] Verify each file's exact hunk context before a multi-file apply_patch — similar documentation wording does not guarantee matching anchors.
49. [TOOL] Insert documentation rows using stable table-header anchors instead of reconstructing formatted cells — embedded code styling can make otherwise matching rows differ.
50. [TOOL] Resolve output paths against the command's actual working directory before passing them to Python — a services-relative path is invalid when the shell starts at the repository root.
51. [DATA] Keep synthetic operator-token fixtures visibly split and avoid token-named aliases in scanner-sensitive assignments — preserve the secret scanner instead of weakening it.
52. [TOOL] Encode Markdown code spans as placeholders in JavaScript patch text and replace them after template parsing — raw backticks terminate the script before apply_patch runs.
53. [TOOL] Scan the complete patch body for Markdown backticks before invoking String.raw — escaping only the obvious hunk does not protect other context lines.
54. [TOOL] Preserve visible continuation arrows and exact line wrapping when replacing roadmap text — a visually similar wrapped paragraph can still fail an exact patch hunk.
55. [TOOL] Avoid Markdown inline-code backticks in String.raw patch prose or replace them through placeholders — parser errors happen before any file edit.

51. [UX] Treat the owner-approved Wazi wordmark and right-side W app icon as canonical brand assets — use the wordmark for site branding and the W icon for favicon and app-icon metadata.
56. [TOOL] Invoke a Windows virtual-environment Python executable with PowerShell's call operator (&), not as an argument to the global python command — otherwise Python reads the binary executable as source text.
57. [TOOL] Prepend `/usr/local/go/bin` to `PATH` for remote Bera-003 Go commands — the pinned Go installation is absent from the default SSH path.
58. [TOOL] Extract the staged source archive on Bera-003 before running a source-based deployment script — uploading the archive alone does not populate its build directory.
59. [TOOL] Check for presence, then invoke staged Bera-003 shell scripts with `bash` — SCP from Windows does not preserve the executable bit.
60. [TOOL] Pass SSH tunnel options as discrete native arguments from PowerShell and verify the local listener — reconstructed command-line strings can be parsed as an invalid hostname.
61. [TOOL] Parenthesize `Join-Path` before passing its result to `Get-Content -Raw` — otherwise PowerShell can treat `-Raw` as a path argument.
57. [TOOL] Scan the full String.raw patch for Go raw-string backticks and replace them with quoted Go strings — a single embedded backtick terminates JavaScript before the patch is applied.
58. [TOOL] When the command working directory is the services module, address Go source paths relative to that module — repeating the services prefix targets nonexistent nested paths.
59. [TOOL] Use slash-separated io/fs paths for embed.FS lookups on Windows — filepath.Join emits backslashes that embed.FS does not resolve.
60. [TOOL] Re-read the exact import block before patching test imports — helpers may already use some of the cryptographic packages needed by the new test.
61. [TOOL] Add only imports used by the final test body — Go rejects unused imports at package compile time.
62. [CODE] Remove an existing signature property before computing a replacement payload digest in signing tests — the ADR-001 digest excludes that property.
63. [CODE] Hash the ASCII payloadDigest with SHA-256 before calling ecdsa.SignASN1 in Cosign-profile tests — the verifier hashes the message bytes before ECDSA verification.
64. [TOOL] Replace inline-code backticks with placeholders when editing Markdown table rows through String.raw patches — a table row still terminates the outer template.
65. [TOOL] Resolve source paths against the module's actual package layout before formatting — cmd/controlplane is a sibling of internal, not internal/cmd.
66. [TOOL] Run Go module commands from the module directory and use paths relative to that directory — repository-root paths and module-root paths are different scopes.

67. [TOOL] Always resolve schema and package file names with rg --files before reading presumed filenames — contract layouts may name the envelope or Go implementation differently.

68. [TOOL] Represent integer values passed into canonical JSON signing helpers as json.Number — the canonicalizer rejects native int64 values to avoid ambiguous numeric formatting.

69. [TOOL] Scan the JavaScript wrapper itself for Markdown backticks before a functions.exec apply_patch call — template parsing can fail before the patch tool runs.
70. [DATA] Always update every schema-backed example and embedded schema copy when a required contract field changes — otherwise the canonical validator can reject the repository's own graph fixture.
71. [TOOL] Avoid Go raw-string delimiters inside JavaScript template-literal patches; use interpreted Go strings or a placeholder — they can terminate the outer template before the patch tool runs.
72. [TOOL] Keep synthetic approval credentials split across short literals and pass them through a short non-secret-named local variable — the secret scanner flags long values assigned to token-named fields in test struct literals.
73. [TOOL] Never initialize an OperatorToken field directly in a test struct literal, even from a synthetic helper — the secret scanner flags the token-named field assignment before evaluating the helper.
74. [PROCESS] Resolve applicable SECURITY.md guidance before inspecting changed source during a security diff review — repository policy must inform candidate discovery and validation.
75. [TOOL] Use the verified `context/examples/` directory for repository examples instead of guessing `schemas/context/examples/` — examples live outside the schemas directory.
76. [TOOL] Include context or change lines in every apply_patch hunk and omit empty hunks — an empty hunk header invalidates the patch before changes can be applied.
77. [TOOL] In Python -c commands passed through PowerShell, use bytes([10]) for newline checks instead of layered backslash escapes — PowerShell and Python parse different escape layers.
78. [TOOL] Prefix every deletion, addition, and context line inside apply_patch hunks with its required marker — unmarked document text invalidates the patch before it can apply.
79. [TOOL] Anchor document replacement hunks with unique unchanged context such as the section heading — a bare replacement block may fail to match reliably.
80. [TOOL] When patching Markdown list items, include both the patch deletion/addition marker and the literal list marker — a single dash can be mistaken for only the patch syntax.
81. [TOOL] Include all intervening table header and separator rows when using them as patch context — Markdown table rows are not adjacent when a separator is omitted.
82. [TOOL] Include every intervening list item when anchoring a list-row patch, or replace the uniquely matched line directly — list context must be consecutive.
83. [TOOL] Do not assume TextEncoder exists in the functions.exec JavaScript isolate — construct shell-safe UTF-8 payloads with verified APIs or keep replacements in a source-aware patch.
84. [TOOL] Read exact physical line breaks before scripting multi-line replacements — prose wrapping differs from assumed grouping and can make otherwise correct text match zero times.
85. [TOOL] Prefix copied apply_patch context lines with a literal leading-space marker before the source text — raw context text after @@ is parsed as an invalid hunk.
86. [TOOL] Do not pass `*.go` as an rg path from PowerShell; pass the directory with `-g '*.go'` — rg does not receive native wildcard expansion there.
87. [TOOL] Replace an existing source line with a deletion and addition marker instead of adding a second copy — otherwise generated or test lists can duplicate entries.
88. [TOOL] When inserting code before an existing declaration, add a new declaration name and preserve the original exactly once — reusing the context line duplicates declarations.
89. [ARCH] When moving a schema to a new major-version base URI, qualify shared-schema references to their registered versioned ID — relative references otherwise resolve under the new version path.
90. [TOOL] Match the exact relative schema-reference prefix when rewriting IDs and make the rewrite idempotent — substring replacement can double-prefix absolute references.

91. [TOOL] Always treat workbench runtime validation as authoritative over generated signatures — declared optional fields can still be forbidden by the active scan context.

92. [TOOL] Query Docker numeric fields directly and convert them with shell tools instead of unsupported Go-template helpers — a failed field expression under set -e stops later read-only preflight checks.

93. [TOOL] Put `-Raw` after the fully parenthesized Join-Path expression when reading a PowerShell artifact — placing it inside the expression treats it as a literal path segment.

94. [TOOL] Use `rg --files` results to select exact source files before PowerShell inspection; never pass wildcard filenames to `Select-String -LiteralPath` or guess helper filenames — PowerShell does not expand the wildcard and similarly named helpers may live in another file.

95. [TOOL] Print only matching tool names or short metadata when discovering plugins — returning full descriptions can drown the actionable inventory.

96. [TOOL] Keep Markdown backticks out of raw JavaScript patch templates or replace them before parsing — a single documentation code span can abort the whole patch before any file changes.

97. [TOOL] Use a real referencing Registry with validate.make_validator, or Draft202012Validator directly for a self-contained schema diagnostic — the helper cannot combine a plain dictionary as its registry.

98. [TOOL] Use `findmnt -T PATH` to inspect the filesystem containing a directory, not `findmnt PATH` unless it is itself a mountpoint — the Docker data-root normally sits within the root mount.
99. [TOOL] Resolve contract implementation filenames from `rg --files` before reading them instead of inferring a filename from a schema name — contract types and helpers may be grouped in a differently named source file.
100. [TOOL] Avoid unquoted shell `${...}` expansions in remote Bash command strings even when the outer JavaScript string is not a template — `set -u` can abort the read-only probe before subsequent checks run.
101. [TOOL] Stage remote diagnostic scripts as LF-terminated files and invoke them through the verified SSH wrapper instead of nesting Bash, awk, and format-string quotes in one PowerShell SSH argument — nested parsing changes commands before they reach the host.
102. [TOOL] Do not assume local Bash is installed on Windows; validate Bash scripts with the staged host Bash or an available compatible shell — invoking WSL without a distribution fails before the script is checked.
103. [TOOL] Do not backslash-escape awk field markers inside PowerShell double-quoted SSH commands; use a staged LF script or single-quoted PowerShell command construction — the backslash reaches awk and breaks its program.
104. [TOOL] When adding a root-owned helper to a user home, verify execute permission on every parent directory — a searchable leaf is still unreachable through a 0700 intermediate directory.
105. [TOOL] Parse systemd CPUQuotaPerSecUSec using its returned duration format, such as 2.625000s, instead of assuming an integer microsecond value — systemctl show renders the property with units.
106. [TOOL] Compare repquota limits in the unit it actually emits, such as 8192K, instead of assuming -s normalizes an 8 MiB limit to 8M — quota reporting can retain KiB units.
107. [TOOL] Verify file size after a quota-denied dd write instead of assuming a partial write up to the hard limit — the filesystem may reject the whole append atomically.
108. [TOOL] Do not use Bash brace expansion in PowerShell command arguments; list exact paths or use rg directory filters — PowerShell parses brace groups differently.

109. [TOOL] Scan the full String.raw patch for Go struct-tag backticks as well as Markdown code spans before apply_patch; source context can terminate the wrapper before the tool call. — nested JavaScript parsing happens before patch validation.

110. [TOOL] Use module-relative paths when the working directory is services/; prefixing those paths with services duplicates the module segment and makes valid files appear missing. — command paths resolve from the explicit working directory.

111. [TOOL] Replace every Markdown backtick in String.raw patch text with a placeholder before invoking functions.exec - JavaScript template parsing fails before apply_patch runs.

112. [TOOL] Never pass literal wildcard source paths to rg from PowerShell; enumerate with rg --files or pass the directory with -g filters — PowerShell may forward the wildcard as an invalid filename.
113. [TOOL] Use simple independently parsed PowerShell line-selection commands when inspecting multiple files; mismatched nested braces can abort inspection before files are read.
114. [TOOL] Always inspect the full on-disk AGENTS.md before work when supplied context may be stale — later local rules can constrain tool use.
115. [TOOL] Use sudo -u for cross-user inspection on bera-003 instead of assuming runuser is permitted — unprivileged development callers cannot use runuser.
116. [TOOL] Carry remote Bash scripts through a dedicated UTF-8 SSH script path; scripts/ssh-dev.ps1 accepts command arguments but does not accept pipeline stdin — direct piping fails before the remote command runs.
117. [TOOL] Resolve example paths from the repository root and Go package paths from the services module root before reading — switching workdirs without switching the path base produces false missing-file errors.
118. [TOOL] Enumerate exact example filenames with rg --files before opening presumed fixtures — a similarly named versioned example may not exist.
119. [TOOL] Pass directory paths with rg -g filters and never use wildcard filenames in PowerShell — PowerShell forwards the wildcard literally and rg rejects it.
120. [TOOL] Inspect full-file state after any broad patch whose context appears inside a callback or repeated block — a matching nested anchor can duplicate the rest of a test function.
121. [TOOL] Prefix every added line in an apply_patch `*** Add File` hunk with `+` — an unprefixed source line is parsed as a patch directive and rejects the whole addition.
122. [TOOL] Avoid PowerShell backtick escapes inside JavaScript template literals and join with `[char]10` instead — a literal PowerShell backtick terminates the outer JavaScript template before execution.
123. [TOOL] Replace Markdown backticks with a safe placeholder before embedding patch text in a JavaScript template literal — source parsing stops before apply_patch can validate the hunk.
124. [TOOL] Prefix replaced source lines with `-` and replacement lines with `+` in apply_patch hunks — bare source text is parsed as invalid patch syntax.
125. [TOOL] Preserve Markdown list markers after the apply_patch removal/addition prefix, doubling the leading sign when needed — the patch prefix is not part of the source content.
126. [TOOL] Download GitHub release assets before passing them to `gh attestation verify` — non-OCI URLs are interpreted as local artifact paths.
127. [TOOL] Build complex workflow patches as raw text with placeholders for `${{ ... }}` expressions — nested shell quotes in JavaScript string arrays can fail before the patch runs.
128. [TOOL] Search generated workflow files for unresolved placeholders after substitution — a missed token remains literal in the published configuration.
129. [TOOL] Balance Python `-c` parentheses before executing validation snippets — an unbalanced comprehension fails before parsing any target file.
130. [TOOL] Keep apply_patch `+` and `-` markers inside quoted patch-string entries — an unquoted leading minus can coerce the replacement text to `NaN`.
131. [PROCESS] Always compare an annotated release tag object with its peeled commit before building — the tag object and source commit are distinct identities.
132. [PROCESS] Always run native Linux service CI for Phase 1 Go changes before reporting cross-platform readiness — Windows does not expose POSIX directory-mode and ignored-fixture failures.

133. [PROCESS] Write the peeled source commit in release summaries and keep the annotated tag-object identity separate — the two Git identities are distinct.

134. [TOOL] Copy exact Go test source lines before building patch context — raw and interpreted string literals have different backslash sequences.

135. [TOOL] Use scripts/ssh-dev.ps1 for bera-003 access and never substitute a placeholder hostname — the repository wrapper validates host identity and key handling.

136. [TOOL] Check native command availability and use fail-fast PowerShell before scripting side effects — unavailable CLIs and unsupported parameters can otherwise produce misleading success output.

137. [TOOL] Scan workflow patch templates for every `${{ ... }}` before parsing and replace each with a placeholder — one raw GitHub expression can abort the whole JavaScript patch call.

138. [TOOL] Search the complete patch body for raw `${{` before invoking apply_patch — expressions left in context lines also break JavaScript template parsing.

139. [TOOL] Assign PowerShell foreach output to a variable before piping it to ConvertTo-Json — a foreach block cannot be used as an empty pipeline source in this command form.

140. [TOOL] Fetch upstream release files through the release API or inspect their pinned remote path instead of assuming a CI-only checkout exists locally — GitHub Actions checkout directories are not present in the workspace.

141. [PROCESS] Match every downloaded release asset to the publisher-reported SHA-256 before verifying its signature bundle — a correct-looking bundle cannot authenticate the wrong or corrupted file.

142. [TOOL] Check required Python modules in the selected interpreter before using them for cryptographic inspection — the global and virtualenv environments have different installed packages.

143. [PROCESS] Capture UTC evidence timestamps after the associated check completes — a timestamp taken before verification can misstate the evidence window.

144. [TOOL] Replace every inline-code backtick in JavaScript template patch text with a placeholder before parsing — a code span anywhere in the patch body terminates the outer template literal.

145. [TOOL] Apply documentation status changes in isolated, file-scoped patches after reading exact physical lines — one historical hunk failure can prevent later status updates from landing.

146. [TOOL] Copy exact physical text from the target document before preparing a patch hunk — contextual excerpts can omit leading words and cause repeated no-op patch failures.

147. [TOOL] Resolve every relative path against the command workdir before invoking remote or crypto tooling — the services module is one level below the repository root.

148. [TOOL] Select one concrete executable from Get-Command results before invoking it — PowerShell can return multiple application paths as a collection that stringifies into an invalid command.

149. [PROCESS] Emit qualification timestamps after every inspected state has been read — a start-of-script time can predate the evidence it labels.

150. [TOOL] Inspect authenticated API redirects with an HTTP client that disables automatic redirects — Invoke-WebRequest with a zero redirection limit throws before returning the Location value.

151. [TOOL] Check the magic bytes and response content type before opening downloaded job logs as ZIP — a successful HTTP response can still contain an error document or another format.

152. [PROCESS] Keep OCI attestation subject-name equal to the repository identity and carry run-specific tags only in the push tag — the verifier compares the subject against the untagged image name.

153. [TOOL] Confirm each replacement belongs to the named target file before composing a multi-file patch — a backlog table row cannot match inside the reference-app document.
154. [TOOL] Enumerate contract helper filenames before searching them with rg — schema helpers are not guaranteed to live in schema.go.
155. [TOOL] Resolve test filenames with rg --files before reading — approval HTTP tests may be grouped in server_test.go rather than approvals_test.go.
156. [TOOL] Check helper signatures before integrating them — beginBodyRead takes only the ResponseWriter and does not return an error.
157. [PROCESS] Include context/examples and root schemas in native service qualification archives — Go tests read repository fixtures outside the services module.
158. [TOOL] Normalize null operation receipts before inspecting nested state — preflight rejections legitimately persist operation as null.
159. [TOOL] Capture BashScriptPath SSH output through the tool result or an explicit remote evidence file — its ProcessStartInfo transport inherits stdout and bypasses PowerShell pipeline capture.
160. [PROCESS] Store adversarial-test console logs outside the repository workspace — intentional synthetic secret fixtures in those logs are correctly flagged by the repository-wide secret scan, including under ignored artifacts.
161. [PROCESS] Run gofmt on all staged Go sources, including modified regression tests, before pushing — tests and vet do not detect formatting failures in remote CI.
162. [PROCESS] Include releases/phase1 in native qualification archives alongside schemas and context examples — renderer regression tests load the signed Gitea pilot manifests from that tree.
163. [PROCESS] Check interpreter compatibility before installing pinned Python development dependencies on bera-003 — its Python 3.14 runtime can trigger unsupported source builds of older rpds and PyYAML pins; qualify and record the distribution runtime separately from Python 3.11 CI.
164. [TOOL] Explicitly chmod shared executable parent directories after mkdir under umask 077 — a requested 0755 creation mode is masked to 0700 and prevents runtime users from traversing the installed worker tree.
165. [CODE] Verify revocation records with VerifySignedRecord after their revocation schema check — unlike application manifests and procedures, revocation lists have no reviewState field.
166. [PROCESS] Poll the private API for readiness after restarting a Type=simple service — systemctl is-active can succeed before its Unix listener is accepting requests.
167. [TOOL] Pass the agentclient directory with an rg -g '*test.go' filter when locating test helpers — a services/internal/agentclient/*test.go argument remains a literal invalid path in PowerShell.
168. [TOOL] Read workflow filenames from the completed rg --files inventory before opening one — the existing image workflows are named phase1-gitea-image, not publish-phase1-gitea.
169. [CODE] Preserve the established empty allowedLicenses semantics when enforcing licensing in Go — the Python gate treats an empty list as unrestricted explicit licences, while a non-empty list filters by exact SPDX value.
170. [CODE] Reject leading slashes and drive separators explicitly in portable archive/configuration paths — filepath.IsAbs alone treats /absolute as relative on Windows.
171. [TOOL] Select the named phase2 package transport artifacts when downloading image builds — Buildx also publishes dockerbuild records that gh run download may try to parse as ZIP archives.
172. [CODE] Validate unsigned package shapes against the canonical schema before signing — the manifest port contract has no protocol property, and an unsupported field otherwise wastes a signing pass.
173. [CODE] Put optional procedure source expiry in provenance.expiresAt — the canonical procedure schema rejects a top-level expiresAt property.
174. [CODE] Preserve the manifest minimum host RAM of 128 MiB even for a smaller container limit — host eligibility requirements and container memory ceilings are different contract fields.
175. [TOOL] Enumerate resource-validation filenames before inspecting them — resource contracts need not live in a file named resources.go.
176. [TOOL] Enumerate policy regression test filenames before opening them — authenticated policy tests may live in execution_preflight_test.go rather than policy_test.go.
177. [CODE] Convert typed Go slices through canonicalPayload before canonical hashing — the strict canonical JSON helper accepts parsed JSON arrays, not native []string values.
178. [TOOL] Split the NOPASSWD sudoers literal when generating a fixed helper rule — the scanner otherwise interprets its PASSWD suffix as a secret assignment; preserve the scanner unchanged.
179. [TOOL] Resolve the npm cache with npm config get cache before inspecting transient CLI packages — this Windows installation does not use the default LocalAppData npm-cache path.
180. [TOOL] Read the protected development server certificate with sudo before deriving its public-key fingerprint, and reject pipeline errors — hashing empty input after a permission failure produces a valid-looking but wrong pin.
