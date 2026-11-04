# Wazi interface system

Locked for the Phase 5 Hallmark redesign, 5 November 2026.

- Brand authority: the existing transparent Wazi wordmark and W app icon under services/internal/controlplane/assets. Preserve their shapes and green overlap.
- Palette authority: existing tokens.css and context.css. Forest navigation, near-white canvas, green primary action, neutral borders, restrained warning and error colours. Every colour is a root token.
- Typography: existing IBM Plex Sans / Segoe UI body and Familjen Grotesk display stacks, with system fallbacks. No remote font dependency. Labels, data and controls use the body stack.
- App shell: compact dark top bar, real route navigation, capped 1440px content width. No sidebar: this product has few destinations. Private routes are overview /, operate /context, alpha /alpha, and package workflow /context#package-studio-title. Catalog routes remain /apps and /dashboard in their separate service.
- Layout: app screens, not marketing pages. Tables, ledgers, inline forms and progressive disclosure. Use 8px panel corners, 6px inputs, 16px body, clear 14px labels, instant focus rings and 44px hit areas.
- Signature: release readiness ledger showing exact local/VPS evidence coverage, measured numerator/denominator, and unresolved work. No invented completion gauges, trend lines, uptime claims, user identities or successful actions.
- Four states for data regions: loading, empty, error and populated. Every write uses the real private API and explicit confirmation where needed. Preview is read-only and visibly labelled.
- Preserve every existing form ID, script hook, API and route. New alpha functionality may add routes and controls; do not remove existing workflows.
- Colour, fonts, focus, reduced motion and container rules are shared in assets/workspace.css and tokens.css. Each page owns its composition stylesheet. The catalog imports the same shared asset through its static mapping.
- Do not turn implementation constraints into large product copy blocks. Put advanced JSON, digests and protocol detail behind disclosure.
