# Wazi brand assets

This directory documents the brand files used by the Wazi control plane.

## Canonical files

| Asset | Source file | Use |
|---|---|---|
| Transparent Wazi wordmark | services/internal/controlplane/assets/wazi-wordmark-open.png | Site and README branding |
| Multi-resolution favicon | services/internal/controlplane/assets/favicon.ico | Browser tab and app icon |
| Downloadable brand kit | services/internal/controlplane/assets/wazi-brand-assets.zip | The two canonical files and this usage note |

The control plane embeds its asset directory and serves these files below
the /assets/ path. The downloadable package is therefore available at
/assets/wazi-brand-assets.zip after the control plane is deployed.

The PNG wordmark and ICO favicon remain the source of truth. Rebuild the
package with scripts/package-brand-assets.ps1 after either source asset or
the package note changes. Keep the wordmark's letterforms and open arc intact.
