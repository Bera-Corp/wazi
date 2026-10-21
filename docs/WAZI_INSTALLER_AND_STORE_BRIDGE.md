# Wazi Universal Installer, Native Registration and Store Bridge

> **Imported completed technical research report | 21 October 2026 | proposed scope.** This report describes a phased cross-platform distribution layer. Windows/macOS installers and store partnerships are **not** in the existing Wazi pre-MVP. Its original research-viewer citation tokens are not portable to GitHub; consult the primary-source index below. See [Wazi master plan](WAZI_MASTER_PLAN.md), [hackathon execution plan](HACKATHON_2026_EXECUTION_PLAN.md), and [preview/insider program](PREVIEW_AND_INSIDER_PROGRAM.md).


## Executive finding

The installer idea is technically sound, but it needs one architectural distinction to avoid fighting the operating systems:

> **Wazi should standardise the installation experience and installation intent, while translating that intent into the native registration, packaging and security mechanisms of Windows, macOS and Linux.**

Trying to make Windows, macOS and Linux literally install software in the same way would create fragile software. Making the **Wazi interface, manifest, security disclosures, component selection, evidence and lifecycle model consistent** while letting platform adapters perform the actual installation is viable.

This produces three distribution channels from one application definition:

```text
 wazi.package.yaml
 │
 Wazi Distribution Compiler
 │
 ┌────────────────────┼────────────────────┐
 │ │ │
 ▼ ▼ ▼
 Wazi Distribution Native Artefacts Store Bridge
 Experience / Packages Exports
 │ │ │
 same Wazi UI Windows: MSI/MSIX Microsoft Store
 same permissions macOS: APP/DMG/PKG WinGet
 same security Linux: Flatpak/RPM Mac App Store
 same review DEB/AppImage Homebrew
 same receipts Flathub
```

The current `Bera-Corp/wazi` repository is deliberately narrower: its README defines Wazi as a marketplace and deployment system for self-hosted open-source applications, initially targeting Linux x86_64 hosts using Docker/Compose, and explicitly puts Windows/macOS native packaging outside the first MVP. That is the correct short-term boundary. The desktop-installer work should become a **parallel specification and second distribution layer**, not cause the current Linux deployment MVP to expand uncontrollably.

The architecture already in the repository is unusually compatible with this extension. Wazi separates its web client, control plane, context engine, policy engine, planner gateway and restricted host agent, while explicitly denying AI a generic shell and requiring signed, versioned manifests. The desktop installer can reuse exactly that philosophy: **declarative package → policy decision → typed native capability → evidence → rollback**.

The biggest discovery from the platform research is that your proposed identical Wazi installation wizard should belong to **Wazi/direct distribution**, not be assumed to appear inside every downstream app store. Microsoft and Apple both impose installation behaviour that can conflict directly with a third-party wizard. Microsoft Store policy version 7.20, effective 22 October 2026, requires traditional MSI/EXE Store submissions to install silently; Apple requires Mac App Store applications to be self-contained, sandboxed and submitted using Apple-supported packaging rather than third-party installers. That leads to a cleaner product concept:

**Wazi Setup**
The consistent human-facing installer.

**Wazi Package Standard**
The open declarative format describing what installation means.

**Wazi Native Adapters**
Windows, macOS and Linux translators.

**Wazi Registry**
The cross-platform record of what was installed, how, from where and under which permissions.

**Wazi Store Bridge**
The tooling that determines whether an application is ready for Microsoft Store, Mac App Store, WinGet, Homebrew, Flathub and eventually other stores.

## What the operating systems actually require

The phrase “register the application with the operating system” means something materially different on each desktop platform. Wazi should therefore expose one logical operation — `register_application` — but implement it differently on each OS.

| Platform | What “installed application” means | What Wazi should do |
|---|---|---|
| Windows | Installer/product registration, Apps & Features/Add or Remove Programs information, Start Menu integration, package identity where applicable | MSI/MSIX or carefully authored EXE installer; register uninstall metadata, identity, icons and optional associations |
| macOS | Proper `.app` bundle containing `Info.plist`, normally placed in `/Applications` or `~/Applications`; Launch Services discovers application identity | Install signed/notarised app bundle in an application directory; preserve bundle ID and signature; avoid inventing a Windows-style registry |
| Linux | No single universal registry; application launchers use freedesktop `.desktop` entries, software centres use AppStream metadata, package managers maintain their own databases | Install the selected package format and ensure desktop entry, icon and AppStream metadata are correctly installed |

### Windows

Windows Installer already has a formal mechanism for registering software with Add/Remove Programs. MSI properties such as product name, installation location, publisher information, support links, icon and uninstall information populate the relevant Windows Installer registration data and uninstall registry entries. Microsoft specifically notes that an unregistered application will not appear in Add/Remove Programs. That means Wazi should **not manually invent registry keys when a proper MSI or MSIX can own them**. The package generator should emit the native registration information and allow Windows Installer or MSIX deployment to maintain it.

For a Wazi-managed Windows application, the logical mapping would be:

| Wazi field | Windows output |
|---|---|
| `app.id` | Package identity / upgrade identity |
| `app.name` | Display name |
| `publisher.name` | Publisher |
| `version` | Product/package version |
| `icon` | Installed-app icon |
| `install.scope` | Per-user or per-machine |
| `install.location` | MSI installation directory when relocation is supported |
| `uninstall` | MSI/MSIX lifecycle registration |
| `associations` | Supported Windows package/shell registrations |
| `command_aliases` | MSIX execution alias where appropriate |

For newer packaged applications, MSIX gives Wazi a much stronger route because Windows package manifests can declare application identity and OS integrations. Windows also supports manifest extensions such as application execution aliases. There is one important limitation to the “choose where to install” UX: **not every Windows package model should expose arbitrary paths**. Traditional MSI installations can be authored to permit target-directory selection; MSIX is much more OS-managed. Wazi therefore needs a capability such as:

```yaml
install:
 relocatable: true
```

rather than assuming every application can safely be placed wherever the user chooses.

### macOS

There is no Windows-equivalent global application registry that Wazi should edit.

A macOS application is fundamentally an application bundle. Its `Contents/Info.plist` describes how macOS interprets the bundle, while the executable and resources live in the standard bundle hierarchy. For ordinary direct-distributed software, placing an application in the Applications folder makes it discoverable in macOS's application browsing experience. Apple's current Mac user documentation specifically instructs users to drag non-App-Store applications to the Applications folder in order for them to appear in the system's Apps interface. So the Wazi macOS adapter should think in terms of:

```text
signed MyApp.app
 │
 ├── valid bundle identifier
 ├── Info.plist
 ├── executable
 ├── resources
 └── entitlements
 │
 ▼
 /Applications/MyApp.app
 │
 ▼
 discovered by macOS
```

Apple does expose Launch Services APIs, but Wazi should treat Launch Services as the operating-system discovery layer rather than maintain a separate imitation of the Windows registry. Complex direct-distribution software can instead use a signed `.pkg`. Apple's installer distribution format can display licence material and expose selectable installation choices through its Distribution XML model. That gives Wazi two macOS paths:

| Application | Recommended Wazi artefact |
|---|---|
| Simple self-contained GUI application | Signed/notarised `.app`, normally delivered in a DMG |
| Application requiring privileged helpers or additional installed resources | Signed/notarised `.pkg` |
| Mac App Store application | Store-specific application bundle submitted through Apple's process |

A DMG does not normally behave like the Windows setup wizard you described. The established pattern is a window asking the user to drag the application to Applications; Tauri's own macOS distribution documentation uses that model. The consistent Wazi wizard therefore has to be its **own signed installer application** if you want an identical interface on macOS.

### Linux

Linux is the platform where the idea of a single “application registry” is least accurate.

Freedesktop's Desktop Entry Specification defines `.desktop` files describing how graphical applications are launched and displayed in application menus. Desktop entry IDs derive from files installed in XDG application directories. The XDG Base Directory Specification defines standard per-user and system-level data locations; user-specific data defaults to `~/.local/share`, while system locations conventionally include `/usr/local/share` and `/usr/share`. For an ordinary graphical Linux application, Wazi should ensure at minimum:

```text
application executable/package
 +
org.example.App.desktop
 +
application icon
 +
org.example.App.metainfo.xml
```

The `.desktop` file makes the application visible to desktops such as GNOME and KDE; AppStream provides richer metadata used by Linux software-centre interfaces. The latest AppStream specification explicitly describes itself as infrastructure for distro-agnostic software centres and unified component metadata. For consumer-facing Linux desktop applications, **Flatpak should be a major Wazi target**, because it combines packaging, desktop integration and a meaningful sandbox permission model. Flathub requires a Flatpak manifest, validated AppStream metadata, a desktop file for graphical applications and appropriate icons. ## The Wazi installation experience

The strongest implementation is not “replace MSI, PKG and Flatpak with one new installer format”.

It is:

> **One Wazi installation protocol and UX, implemented by platform-specific native adapters.**

The same installer window can appear on Windows, macOS and Linux because the *front end* is portable. What changes underneath it is the execution adapter.

A sensible component architecture is:

```text
┌────────────────────────────────────────────────────┐
│ Wazi Setup │
│ │
│ React/TypeScript interface │
│ │
│ Verify → Terms → Permissions → Options → Review │
│ → Install → Verify → Complete │
└─────────────────────────┬──────────────────────────┘
 │
 typed IPC only
 │
┌─────────────────────────▼──────────────────────────┐
│ Wazi Installation Engine │
│ │
│ manifest verifier │
│ dependency resolver │
│ policy evaluator │
│ transaction planner │
│ evidence recorder │
└──────────┬─────────────────┬─────────────────┬─────┘
 │ │ │
 ▼ ▼ ▼
 Windows adapter macOS adapter Linux adapter
 MSI / MSIX APP / PKG Flatpak
 ARP / aliases Applications XDG entries
 UAC boundary notarisation AppStream
```

For the GUI shell, Tauri is a realistic candidate because its current tooling targets Windows, macOS and Linux and can build platform-specific installers and bundles. Its supported distribution paths include MSI/NSIS on Windows, app bundles/DMGs/App Store builds on macOS, and AppImage, Debian, RPM, Flatpak and other Linux mechanisms. Tauri alone should **not** be treated as the universal packager for arbitrary Wazi applications. Tauri packages Tauri applications. Wazi needs its own packaging layer capable of describing and wrapping applications written in Electron, Qt,.NET, Rust, Go, Python, Java and other stacks.

Qt Installer Framework demonstrates that a single installer codebase can expose custom pages, components, licences and elevated operations across Windows, Linux and macOS, although Qt deliberately gives installers the native look and feel of the host platform rather than forcing an identical appearance. For Wazi, the stronger branding decision is therefore:

**Use a Wazi-owned UI shell for consistency.
Use native packaging underneath for correctness.**

### The Wazi Setup flow

The installation window should use the same information architecture on all supported operating systems.

| Screen | User-facing purpose | Technical purpose |
|---|---|---|
| Identity | “What am I installing?” | Publisher, app ID, version, provenance and signing verification |
| Trust | “Is this application safe enough for me?” | Vulnerabilities, Wazi assessment, maintenance status and artefact signature |
| Licence | “What terms govern this?” | SPDX licence, upstream terms, third-party licences |
| Permissions | “What can this application access?” | Normalised permissions and OS enforcement status |
| Installation mode | Default / Full / Custom | Component resolver |
| Location | Select install/data destination when supported | Adapter-specific path policy |
| Integrations | Shortcuts, file types, startup, network exposure | Native OS registrations |
| Review | Exact changes before mutation | Signed installation plan |
| Install | Progress | Transaction execution |
| Verify | Application health | Registration, launch and integrity checks |
| Complete | Launch/open/manage | Wazi receipt creation |

The “Default / Full / Custom” model is viable, but should describe **components**, not promise low-level behaviour the host platform cannot provide.

A useful interpretation is:

**Default** installs the maintainer-recommended configuration.

**Full** installs all recognised optional modules that the package author has declared compatible.

**Custom** allows the user to choose optional components, data locations, integrations and supported permissions.

The location selector should appear only when `relocatable: true`. A Flatpak, MSIX or distro-native Linux package should not pretend to support arbitrary installation paths simply to preserve visual consistency.

That is an important principle for Wazi generally:

> **same mental model, not fake equivalence.**

## The Wazi package manifest and application registry

The key to making the installation framework portable is a richer Wazi distribution manifest.

It should not contain executable shell instructions. It should contain declarative intent that platform adapters translate into approved native actions.

A first-generation specification could resemble:

```yaml
schema: wazi.dev/package/v1

application:
 id: io.example.notes
 name: Example Notes
 version: 1.4.0
 publisher:
 name: Example Organisation
 verified: true

source:
 repository: github:example/notes
 release: v1.4.0

licensing:
 expression: AGPL-3.0-or-later
 mode: disclosure
 notices:
 - THIRD_PARTY_NOTICES.txt

installation:
 modes:
 default:
 components:
 - core

 full:
 components:
 - core
 - language-packs
 - shell-integration

 custom: true

 scope:
 supported:
 - user
 - system

components:
 core:
 required: true

 language-packs:
 required: false

 shell-integration:
 required: false

permissions:
 network:
 outbound: required
 inbound: none

 filesystem:
 application_data: read-write
 arbitrary_user_files: user-mediated

 camera: none
 microphone: none

integrations:
 launcher: true
 start_menu: true
 file_associations:
 - ".note"

 autostart:
 supported: false

platforms:
 windows:
 artefact: example-notes-x64.msi
 relocatable: true

 macos:
 artefact: ExampleNotes.app
 relocatable: false

 linux:
 preferred: flatpak
 app_id: io.example.notes

security:
 sbom: sbom.cdx.json
 provenance: provenance.json
 vulnerability_policy: standard
 signed_release: required

distribution:
 microsoft_store:
 enabled: true

 winget:
 enabled: true

 mac_app_store:
 assess: true

 homebrew:
 enabled: true

 flathub:
 enabled: true
```

SPDX expressions are an appropriate representation for the `licensing.expression` field because SPDX formally supports simple and compound software licence expressions such as `MIT`, `GPL-2.0-or-later`, `Apache-2.0 OR MIT`, and combinations using `AND`, `OR` and `WITH`. ### Do not force click-through acceptance for every open-source package

This point matters.

Your Wazi installer can certainly show licences and terms before installation, but **open-source licensing should normally be treated as transparent disclosure rather than automatically imposing a Wazi click-wrap contract**.

The Open Source Definition explicitly rejects licences that require a particular interface or explicit click-wrap mechanism as a prerequisite to the licence itself, because such requirements would interfere with redistribution through non-GUI channels. The manifest should therefore distinguish:

```yaml
licensing:
 mode: disclosure
```

from:

```yaml
licensing:
 mode: explicit_acceptance
 terms_id: commercial-eula-2026-01
```

The second would apply only where the upstream publisher actually requires a separate enforceable EULA or terms agreement.

That also gives Wazi the ability to record:

```text
terms version
application version
user/account
timestamp
installation ID
acceptance mode
```

without claiming that every open-source licence legally requires explicit assent.

### The Wazi Registry

This is where your “application registry” idea should become something genuinely new.

Wazi should maintain its own **cross-platform installation registry**, independently of whatever the OS maintains.

For every installation:

```text
Wazi Installation Receipt
├── application ID
├── publisher identity
├── version
├── source repository
├── release artefact digest
├── installation mechanism
├── native package ID
├── installation scope
├── actual installation location
├── application-data locations
├── native registration status
├── permissions requested
├── permissions actually enforced
├── SBOM digest
├── vulnerability assessment
├── installation timestamp
├── update channel
├── rollback checkpoint
├── Wazi package version
└── ownership / managing store
```

This solves an important cross-platform problem: Wazi can always answer **what it installed and how to remove or repair it**, even though each operating system represents installation differently.

Native registration then becomes an adapter output:

| Wazi installation operation | Windows | macOS | Linux |
|---|---|---|---|
| Register app identity | MSI/MSIX identity | Bundle identifier | Desktop/AppStream ID |
| Register installed programme | Windows Installer / Apps registration | Place `.app` in Applications | Package DB / Flatpak + desktop entry |
| Register launcher | Start Menu/package integration | App bundle discoverability | `.desktop` |
| Register icon | Package/shortcut metadata | Bundle resources | Icon theme/AppStream |
| Register file type | Windows associations | Bundle document types | MIME/Desktop entry |
| Register uninstall | MSI/MSIX lifecycle | Package/app removal recipe | Flatpak/dpkg/rpm/etc. |
| Wazi receipt | Wazi registry | Wazi registry | Wazi registry |

Wazi should never modify a signed upstream application just to add its own metadata. Apple explicitly states that changes to a signed application bundle invalidate its code signature, and notarisation will reject a modified signed bundle. Authenticode similarly exists to verify both Windows publisher identity and integrity after signing. Wazi therefore needs two packaging modes:

**Publisher-integrated mode:** Wazi packaging occurs before final publisher signing.

**Upstream-preserving mode:** Wazi leaves an already-signed artefact unchanged and adds external Wazi metadata and receipts.

That is the correct model for eventually supporting major open-source projects without asking them to abandon their release processes.

## The Store Bridge

This is the strongest extension to your concept.

Wazi should not merely tell developers, “You can publish here.”

It should be able to say:

> **Your Wazi package is 92% ready for Microsoft Store, passes WinGet requirements, passes Homebrew structural checks, and needs three changes before it is eligible for the Mac App Store.**

Call this **Wazi Store Bridge** or **Wazi Distribution Readiness**.

A single Wazi release could be evaluated against multiple target profiles:

```text
Example Notes 1.4.0

Wazi READY
Windows direct READY
Microsoft Store MSIX READY
WinGet READY
macOS direct READY
Mac App Store BLOCKED
Homebrew Cask READY
Flatpak READY
Flathub 2 ISSUES
```

### Microsoft Store

Microsoft currently supports both MSIX applications and traditional Win32 MSI/EXE applications in the Microsoft Store. Microsoft recommends MSIX for tighter Store integration, while existing EXE/MSI applications may be submitted through a hosted installer URL. The October 2026 Microsoft Store policy is especially relevant to Wazi. Traditional installer submissions must, among other requirements, be signed, come from a versioned immutable URL, be standalone rather than downloader stubs, install only the intended product and perform the installation without displaying their own installer UI; UAC is allowed. Therefore:

```text
Wazi website installation
 → Wazi Setup wizard allowed

Microsoft Store installation
 → Store/native silent path
 → Wazi installer wizard skipped
```

This is not a weakness. It validates the need to separate Wazi's **installation contract** from its **installation UI**.

Wazi should generate either:

```text
MSIX submission package
```

or:

```text
signed offline MSI/EXE
+ Store listing metadata
+ certification evidence
```

Microsoft also provides a submission API for MSI/EXE applications that can automate later submissions after initial Partner Center prerequisites and an initial submission are established. That means the long-term Wazi flow can become:

```text
wazi store prepare microsoft
wazi store validate microsoft
wazi store submit microsoft
```

with the developer explicitly connecting their own Partner Center account.

### WinGet

WinGet is almost an ideal target for Wazi-generated export.

Its public repository uses structured YAML manifests containing package identity, version, publisher, licence, installer URL, installer SHA-256 and installer type. WinGet submissions are automatically validated, installers are expected to work non-interactively, and Microsoft recommends that package and publisher names correlate with the Windows Apps & Features registration information so upgrades can be identified correctly. Most of this information is already information Wazi should possess.

Therefore:

```bash
wazi export winget
```

could generate the complete multi-file WinGet manifest and run the relevant validations before opening a submission PR.

### Mac App Store

This is where expectations must be controlled carefully.

Apple's Mac App Store model conflicts with several features of the Wazi direct installer.

Mac App Store applications must be appropriately sandboxed, must be packaged and submitted using Apple-supported technologies, must be self-contained single-app bundles and may not use third-party installers. They cannot request root escalation, present their own licence screen at launch or use an independent update mechanism. Apple separately confirms that App Sandbox is mandatory for Mac App Store distribution, while Developer ID/notarised applications distributed outside the Store can use a different security model. So Wazi's role is not:

> “Wrap every Wazi application and send it to Apple.”

It is:

> **“Analyse whether the application can be transformed into a valid Mac App Store product, and automate everything that is legitimately automatable.”**

A readiness report might say:

```text
Mac App Store readiness: BLOCKED

PASS valid bundle identifier
PASS universal or supported architecture
PASS application icon
PASS signing configuration
FAIL App Sandbox disabled
FAIL application writes outside permitted container
FAIL bundled updater conflicts with App Store update model
FAIL application installs additional executable code
```

Some failures can be fixed through configuration.

Others require application architecture changes.

Apple's App Store Connect API can automate app metadata, provisioning-related operations and submission workflows, while builds can be uploaded using supported mechanisms such as Xcode or Transporter. Wazi can therefore eventually automate much of the submission workflow while still leaving App Review in Apple's control.

### Homebrew

Homebrew is not a conventional commercial app store. It is a package-management ecosystem with community-maintained acceptance rules.

Homebrew Casks support application bundles, PKGs, installers, binaries and other artefacts. A Cask normally includes version, SHA-256, URL, name, description and homepage metadata plus at least one installation artefact. Its `app` stanza specifically moves a macOS `.app` into `/Applications`. Homebrew provides standard validation commands including install, uninstall, audit and style checks. That makes a Wazi export practical:

```bash
wazi export homebrew
```

producing:

```ruby
cask "example-notes" do
 version "1.4.0"
 sha256 "..."

 url "..."
 name "Example Notes"
 desc "..."
 homepage "..."

 app "Example Notes.app"
end
```

followed by automated audit and installation testing.

Wazi could open the upstream Homebrew PR, but Homebrew maintainers would still decide whether it meets their acceptance policy. ### Flathub

Flathub should be treated as one of Wazi's first-class Linux export targets.

Its current requirements include a Flatpak manifest, appropriate dependencies, AppStream metadata, a desktop file for graphical apps and icons. Flathub also expects builds to satisfy its packaging and metadata validation rules. Wazi could generate:

```text
io.example.Notes.yaml
io.example.Notes.metainfo.xml
io.example.Notes.desktop
icons/
flathub.json
```

and run the Flatpak/Flathub lint tooling before submission.

This is particularly attractive because Flatpak's sandbox model also maps cleanly onto your Wazi permissions interface. ### What a Microsoft or Apple partnership should mean

Do not make the business plan dependent on negotiating a special fast-track agreement.

A better sequence is:

```text
Wazi manifest standard
 ↓
Wazi validation profiles
 ↓
High-quality downstream packages
 ↓
Measurably lower packaging/review failures
 ↓
Large maintainer ecosystem
 ↓
Then pursue formal ecosystem relationships
```

Microsoft and Apple retain their own certification/review authority. Microsoft performs certification checks before publishing Store applications, and Apple reviews App Store submissions. A future partnership could still be valuable for technical support, co-marketing, documentation alignment, developer education or recognised Wazi conformance tooling.

But Wazi should be valuable before any such agreement exists.

The commercial pitch to platform owners later becomes stronger:

> “Thousands of applications already use Wazi. Our conformance profile produces correctly structured, signed and pre-tested submissions. We would like to make that bridge officially supported.”

That is materially stronger than asking them to recognise an unproven installer format.

## Permissions, security and trust

The installation wizard gives Wazi an opportunity to build one of the best permission-disclosure systems available on desktop operating systems.

The critical distinction is between:

**Declared permission**
The developer says the application uses it.

**Observed permission**
Wazi testing saw the application attempt to use it.

**Enforced permission**
The OS sandbox actually prevents access unless it is permitted.

Those three states must never be conflated.

### Cross-platform permission vocabulary

A Wazi package could declare:

```yaml
permissions:
 network:
 outbound:
 level: required
 inbound:
 level: none

 files:
 user_selected:
 level: optional

 downloads:
 level: read-write

 entire_home:
 level: none

 devices:
 camera: optional
 microphone: none
 bluetooth: none
 usb: none

 system:
 autostart: none
 privileged_execution: none
```

The Wazi UI could translate that into:

| Permission | Application says | OS enforcement |
|---|---|---|
| Internet access | Required | Sandboxed |
| Selected files | Optional | User-mediated |
| Camera | Optional | OS consent |
| Microphone | Not requested | Blocked |
| Administrator access | Not requested | Not granted |

### Windows mapping

Windows packaged applications can declare capabilities in their package manifest, including access to privacy-sensitive resources. Microsoft notes that users receiving applications from the Store are notified of declared package capabilities. There is an important limitation: traditional/full-trust desktop applications do not receive the same sandbox enforcement as AppContainer applications. Microsoft explicitly distinguishes AppContainer applications from medium-integrity full-trust desktop apps. Wazi must therefore display something such as:

```text
Filesystem access
Declared: broad
Enforcement: not sandbox-enforced by Windows for this package type
```

rather than showing a fake toggle that Wazi cannot enforce.

### macOS mapping

App Sandbox restricts application access to system resources and user data. Sandboxed applications explicitly declare the privileges they need, including network access, camera, microphone, USB, Bluetooth and selected file locations. This maps extremely well to a Wazi permission manifest for sandboxed applications.

Again, Wazi must differentiate a sandboxed Mac application from a conventional Developer ID application that is not running inside App Sandbox.

### Linux mapping

Flatpak provides the strongest immediate fit for your Android-like permission idea.

Flatpak applications begin with tightly restricted access to host files, network resources, devices and host services, then receive additional access through manifest permissions and portals. Portals can provide user-mediated access to files and other resources without granting blanket filesystem access. This means a Wazi-controlled Flatpak installation could genuinely allow users to understand and, in some situations, later modify the application's sandbox permissions.

### Installer privilege architecture

The current Wazi repository already adopts the correct security philosophy: narrow typed capabilities instead of arbitrary shell execution, explicit policy decisions for mutations and signed manifests with evidence.

The desktop installer should retain that boundary.

The Wazi GUI should **never run permanently as administrator/root**.

Instead:

```text
Unprivileged Wazi Setup UI
 │
 │ signed typed request
 ▼
Minimal privileged helper
 │
 ├── create directory
 ├── copy verified payload
 ├── invoke approved native installer
 ├── register approved integration
 └── remove approved installation
```

There should be no operation such as:

```text
execute arbitrary command as root
```

That would undermine the entire Wazi trust model.

Before installation, Wazi should verify artefact digest, publisher signature where available, package signature, SBOM association and manifest signature. After installation, it should verify native registration, executable identity and application health.

The project's existing security principles already require pinned artefacts, SBOMs, vulnerability scanning, isolated tests, least privilege, encrypted state, rollback and emergency catalogue revocation. The desktop distribution layer should inherit those rather than define a second security model.

## Project structure and roadmap

This installer work should be added to Wazi as a coherent subsystem, not folded indiscriminately into the current Docker deployment engine.

A strong eventual repository structure is:

```text
wazi/
├── apps/
│ ├── web/
│ ├── desktop/
│ │ └── wazi-setup/
│ └── developer-portal/
│
├── cmd/
│ ├── wazi/
│ └── wazi-pack/
│
├── schemas/
│ ├── application/
│ ├── deployment/
│ ├── package/
│ │ └── v1/
│ ├── permissions/
│ ├── receipt/
│ └── distribution-readiness/
│
├── packaging/
│ ├── compiler/
│ ├── windows/
│ │ ├── msi/
│ │ ├── msix/
│ │ └── registration/
│ ├── macos/
│ │ ├── app/
│ │ ├── dmg/
│ │ └── pkg/
│ └── linux/
│ ├── flatpak/
│ ├── deb/
│ ├── rpm/
│ └── appimage/
│
├── distribution/
│ ├── microsoft-store/
│ ├── winget/
│ ├── mac-app-store/
│ ├── homebrew/
│ └── flathub/
│
├── registry/
│ ├── receipts/
│ ├── inventory/
│ └── reconciliation/
│
├── permissions/
│ ├── vocabulary/
│ ├── windows/
│ ├── macos/
│ └── linux/
│
└── docs/
 ├── UNIVERSAL_INSTALLER_AND_STORE_BRIDGE.md
 ├── PACKAGE_SPECIFICATION.md
 ├── PERMISSIONS_MODEL.md
 └── STORE_READINESS_PROFILES.md
```

The existing Wazi repository does not yet need all of these directories. Its current roadmap deliberately proceeds through contracts/security boundaries, a deterministic Linux deployment slice, context retrieval, controlled AI packaging, lifecycle operations and a five-to-ten-application private alpha.

The installer vision should therefore have two time horizons.

### Hackathon scope

For the Meta hackathon, attempting genuine production installers for three operating systems would reduce the probability of delivering a convincing working product.

The strongest demonstration is narrower:

| Hackathon component | Implementation |
|---|---|
| Wazi Package Manifest v1 | Real |
| Wazi Setup interface | Real |
| Default / Full / Custom UX | Real |
| Permission summary | Real |
| Security and vulnerability information | Real |
| Installation plan | Real |
| One actual deployment/install adapter | Real |
| Cross-platform package compiler architecture | Real |
| Windows export | Generate/validate metadata or artefact where feasible |
| macOS export | Generate readiness output rather than promise complete Store packaging |
| Linux registration | Real `.desktop`/AppStream or existing Wazi deployment path |
| Microsoft Store readiness profile | Real rule evaluator |
| Mac App Store readiness profile | Real rule evaluator |
| WinGet manifest generation | Real |
| Homebrew Cask generation | Real |
| Flathub metadata generation | Real or partially real |
| Store partnerships | Roadmap only |

This gives the demo a very strong narrative:

```text
GitHub repository
 │
 ▼
Wazi analyses project
 │
 ├── dependency/security analysis
 ├── permissions analysis
 ├── package identity
 └── application metadata
 │
 ▼
Wazi Package generated
 │
 ▼
┌─────────────────────────────────────────┐
│ Wazi Setup │
│ │
│ Verified publisher │
│ Security assessment │
│ Permissions │
│ Installation mode │
│ Exact changes │
└──────────────────┬──────────────────────┘
 │
 ▼
 Install / Deploy
 │
 ▼
 Health check
 │
 ▼
 Wazi Registry
 │
 ▼
 Distribution Readiness Report
 │
 ┌──────────┼──────────┐
 ▼ ▼ ▼
 WinGet Homebrew Flathub
 ready ready 1 issue

 Microsoft Store: 2 requirements remaining
 Mac App Store: architecture changes required
```

That is far more compelling than showing three generic setup executables.

### Product alpha

After the hackathon, the sequence should be:

**Wazi Package Specification.** Stabilise application identity, components, licences, permissions, installation intent, native integrations and receipts.

**Linux desktop adapter.** Flatpak first for graphical consumer applications, alongside the existing Docker/self-hosted deployment layer.

**Windows adapter.** Support existing upstream MSI/EXE first, then MSIX generation and deeper Windows integration.

**macOS direct-distribution adapter.** Application bundle inspection, Developer ID signing/notarisation workflow, Applications installation and PKG support.

**Wazi Setup.** Ship the same installer experience across the three platforms once each adapter can guarantee correct installation and uninstall semantics.

**Store Bridge.** Add validated exporters and readiness profiles for WinGet, Homebrew and Flathub first, followed by Microsoft Store and Mac App Store workflows.

**Submission automation.** Connect developer-owned Microsoft Partner Center and App Store Connect accounts through their supported APIs. Microsoft exposes a submission API for appropriate Store applications, while Apple exposes App Store Connect APIs for app metadata and submission automation. **Ecosystem partnerships.** Approach distribution platforms only after Wazi can demonstrate that its package standard consistently produces policy-compliant, tested submissions.

### Strategic result

This addition materially improves the overall Wazi thesis.

The project stops being merely:

> **An easier way to install open-source applications.**

It becomes:

> **An open application distribution standard that carries a project from repository, through packaging and security verification, into installation, native operating-system integration, lifecycle management and eventually downstream app stores.**

The important architectural boundary is this:

```text
Source repository
 │
 ▼
Repository Standard
human + agent maintainability
 │
 ▼
Wazi Package Standard
application identity + components + permissions
 │
 ▼
Security / SBOM / vulnerability evidence
 │
 ▼
Wazi Marketplace
discovery + reviews + maintainers
 │
 ├────────────────────────────┐
 ▼ ▼
Wazi Setup Wazi Cloud Deployment
desktop installation private VM / self-hosting
 │ │
 ▼ ▼
Native OS integration managed service lifecycle
 │
 ▼
Wazi Registry
 │
 ▼
Store Bridge
 │
 ├── Microsoft Store
 ├── WinGet
 ├── Mac App Store
 ├── Homebrew
 └── Flathub
```

The underlying insight is strong: **Wazi should not attempt to replace Windows Installer, Apple application bundles, Flatpak, WinGet, Homebrew or Flathub. It should become the common layer that understands all of them.**

That creates a plausible path towards the international standard originally envisioned: developers describe an application once in a rich Wazi specification; Wazi validates it, presents it consistently to users, installs it correctly on supported environments, records what happened, and tells maintainers exactly what remains before the same application can graduate to larger distribution channels.

---

## Primary-source reference index (reviewed 2026-10-21)

- [Microsoft: distribute Win32 apps through Microsoft Store](https://learn.microsoft.com/en-us/windows/apps/distribute-through-store/how-to-distribute-your-win32-app-through-microsoft-store)
- [Microsoft: MSI/EXE package requirements, including silent installers](https://learn.microsoft.com/en-us/windows/apps/publish/publish-your-app/msi/app-package-requirements)
- [Microsoft: MSIX application packaging](https://learn.microsoft.com/en-us/windows/msix/overview)
- [Microsoft: installed programs/uninstall registry data](https://learn.microsoft.com/en-us/windows/win32/msi/uninstall-registry-key)
- [Apple: bundle identifiers](https://developer.apple.com/documentation/bundleresources/information-property-list/cfbundleidentifier)
- [Apple: notarizing outside-store software](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution)
- [Apple: App Sandbox](https://developer.apple.com/documentation/xcode/configuring-the-macos-app-sandbox)
- [Apple: App Store Connect](https://developer.apple.com/help/app-store-connect/)
- [Freedesktop Desktop Entry Specification](https://specifications.freedesktop.org/desktop-entry/latest-single/)
- [AppStream metadata](https://www.freedesktop.org/software/appstream/docs/)
- [Flatpak documentation](https://docs.flatpak.org/en/latest/)
- [Flathub submission and review](https://docs.flathub.org/docs/for-app-authors/submission)
- [Homebrew: adding software](https://docs.brew.sh/Adding-Software-to-Homebrew)
- [Homebrew Cask Cookbook](https://docs.brew.sh/Cask-Cookbook)
- [WinGet publishing packages](https://learn.microsoft.com/en-us/windows/package-manager/package/repository)
