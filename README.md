# Wazi

<p align="center">
  <img src="services/internal/controlplane/assets/wazi-wordmark-open.png" alt="Wazi wordmark" width="620">
</p>

**Wazi means open in Swahili.** Wazi helps you discover, package and operate open-source applications on your own server. Signed packages, compatibility checks and explicit approvals connect the catalog to a private deployment workbench.

**Current stage: private alpha buildout.** Phases 2–5 have provisional 90% code
estimates. Phase 1's bounded Gitea pilot is accepted within its recorded scope.
These estimates are not acceptance scores. Static review has two unresolved
findings; alpha reliability, owner review, restore coverage and platform
qualification still need evidence. See the [Phase 1–5 closeout ledger](docs/PHASE1_5_CLOSEOUT.md),
[buildout checkpoint](docs/BUILDOUT_90_2026-11-05.md) and
[live Linux integration record](docs/LINUX_INTEGRATION_2026-11-04.md).

[Download logos and favicons](services/internal/controlplane/assets/wazi-brand-assets.zip) | [Brand asset guide](graphics/README.md)

## Try the interface locally

The preview shows the actual overview, workbench, catalog and alpha progress pages. It binds only to your computer, rejects deployment actions and does not install applications. An empty progress view means no evidence has been supplied; it is not a successful alpha run.

Install Git and Python **3.11**, then clone the repository:

~~~sh
git clone https://github.com/Bera-Corp/wazi.git
cd wazi
~~~

**Windows PowerShell**

~~~powershell
py -3.11 -m venv .venv
.\.venv\Scripts\python.exe -m pip install -r requirements-dev.txt
.\.venv\Scripts\python.exe scripts/ui_preview.py
~~~

**Linux or macOS**

~~~sh
python3.11 -m venv .venv
.venv/bin/python -m pip install -r requirements-dev.txt
.venv/bin/python scripts/ui_preview.py
~~~

Open [Alpha progress](http://127.0.0.1:3688/alpha), [Workbench](http://127.0.0.1:3688/context), [Catalog](http://127.0.0.1:3688/apps) or [Overview](http://127.0.0.1:3688/). Stop the server with Ctrl+C. If the port is busy, add `--port 3689` and use that port in the URL. Preview sign-in and private host operations require a configured live service.

The preview creates a local catalog database under `artifacts/ui-preview/`. It does not connect to the VPS. To display exported alpha evidence, use `--snapshot PATH` with the report JSON described in the [alpha operations guide](docs/PHASE5_ALPHA_OPERATIONS.md).

## Use a real host

The qualified deployment target is **Linux x86_64 with Docker Engine and Compose**. Windows/WSL2 and macOS preparation code does not establish native deployment qualification. The browser preview works independently of that qualification.

A live deployment needs a configured control plane and agent, paired device identity, CA and client certificates, signed packages, an approved resource budget and an operator session. Application changes follow the configured policy and approval flow. The repository is not a one-command production installer.

For an existing authorized development host, follow [Development environments](docs/DEVELOPMENT_ENVIRONMENTS.md) and its linked deployment records. Configure `.env.local` from the repository template using SSH key **paths**, never private key contents. The owner's configured Windows connection can then be checked with:

~~~powershell
.\scripts\ssh-dev.ps1 -Check
~~~

That helper targets the documented development host; it is not a general VPS installer. Keep credentials, keys, operator codes and local evidence databases out of Git.

The normal product journey is: browse a package, choose a qualified host, inspect compatibility and the proposed plan, approve the change, then follow execution and health. Backups, recovery and upgrade actions require their own supported package procedures. A catalog entry alone does not prove that an application is qualified or installed.

## Work on the code

Use Python 3.11 with the pinned dependencies above. Go service development uses **Go 1.23**, as declared in `services/go.mod`. Docker is needed for actual runtime qualification, not for the UI preview.

Contract validation, from the repository root:

~~~powershell
.\.venv\Scripts\python.exe scripts/validate.py
~~~

On Linux or macOS use `.venv/bin/python` instead. Go service checks run from the module directory:

~~~sh
cd services
go test ./...
go build ./cmd/...
~~~

Native Linux qualification and release CI remain separate from Windows checks. Running these commands does not establish live deployment, restore or security acceptance. See the [roadmap](docs/MVP_ROADMAP.md) for the remaining gates.

## Find the right documentation

| Need | Read |
|---|---|
| Understand components and trust boundaries | [Architecture](docs/ARCHITECTURE.md), [context engineering](docs/CONTEXT_ENGINEERING_PLAN.md) |
| See current implementation scope and gaps | [Buildout checkpoint](docs/BUILDOUT_90_2026-11-05.md), [roadmap](docs/MVP_ROADMAP.md), [backlog](docs/BACKLOG.md) |
| Record cohorts, reliability and support work | [Phase 5 alpha operations](docs/PHASE5_ALPHA_OPERATIONS.md) |
| Connect to the authorized development host | [Development environments](docs/DEVELOPMENT_ENVIRONMENTS.md) |
| Inspect recorded live outcomes | [Phase 1 evidence](docs/PHASE1_LIVE_EVIDENCE.md), [Phase 2 evidence](docs/PHASE2_LIVE_EVIDENCE.md) |
| Prepare the demonstration | [Judging pillars](docs/JUDGING_PILLARS.md), [video workspace](video/README.md) |

## Branding and licensing

Use the approved wordmark for Wazi branding and the W app icon for favicons. The [graphics guide](graphics/README.md) describes the supplied transparent PNGs, SVGs and icon sizes; the [downloadable package](services/internal/controlplane/assets/wazi-brand-assets.zip) keeps them together.

Wazi's public distribution license is still pending; see [LICENSE-DECISION.md](LICENSE-DECISION.md). Upstream applications retain their own licenses. Do not treat catalog inclusion as permission to redistribute their code or images.
