"use strict";
const byId = (id) => document.getElementById(id);
const state = {
  applications: [],
  user: null,
  consent: null,
  returnProject: null,
};
function node(tag, text, className) {
  const el = document.createElement(tag);
  if (text !== undefined) el.textContent = text;
  if (className) el.className = className;
  return el;
}
async function api(path, body) {
  const options = { credentials: "same-origin", headers: {} };
  if (body !== undefined) {
    options.method = "POST";
    options.headers["Content-Type"] = "application/json";
    options.headers["X-CSRF-Token"] = state.user?.csrf || "";
    options.body = JSON.stringify(body);
  }
  const response = await fetch(path, options);
  const result = await response.json();
  if (!response.ok)
    throw new Error(result.error || "The request could not be completed.");
  return result;
}
const familyName = {
  "linux-engine": "Linux Docker",
  "windows-wsl2": "Windows / WSL2",
  "macos-desktop": "macOS Docker",
};
function available(app) {
  return app.recipes.filter((recipe) => !recipe.revoked);
}
function matching(app, values) {
  return available(app).filter(
    (recipe) =>
      (!values.release || recipe.releaseVersion === values.release) &&
      recipe.environments.some(
        (e) =>
          (!values.environment ||
            e.environment.family === values.environment) &&
          (!values.operation || e.operations.includes(values.operation)),
      ),
  );
}
function parameters() {
  return new URLSearchParams(location.search);
}
function render() {
  const values = Object.fromEntries(new FormData(byId("filters")));
  const recipesFor = (app) => matching(app, values);
  const selected = state.applications.filter((app) => {
    const text = (
      app.name +
      " " +
      app.description +
      " " +
      app.category
    ).toLowerCase();
    return (
      (!values.q || text.includes(values.q.toLowerCase())) &&
      (!values.category || app.category === values.category) &&
      (!values.availability ||
        (values.availability === "deferred"
          ? app.deferred
          : values.availability === "available"
            ? recipesFor(app).length > 0
            : recipesFor(app).length === 0))
    );
  });
  selected.sort(
    values.sort === "stars"
      ? (a, b) => (b.github?.stars ?? -1) - (a.github?.stars ?? -1)
      : (a, b) => a.name.localeCompare(b.name),
  );
  const result = byId("results");
  result.replaceChildren();
  result.setAttribute("aria-busy", "false");
  byId("count").textContent = selected.length + " projects";
  if (!selected.length) {
    const empty = node("div", undefined, "empty");
    empty.append(
      node("h3", "No matching projects"),
      node(
        "p",
        "Try another category or search. Missing coverage is an opportunity to contribute.",
      ),
    );
    result.append(empty);
  }
  for (const app of selected) {
    const row = node("article", undefined, "app");
    const heading = node("div");
    heading.append(node("div", app.category, "category"), node("h3", app.name));
    const popularity = app.github
      ? Intl.NumberFormat(undefined, {
          notation: "compact",
          maximumFractionDigits: 1,
        }).format(app.github.stars) + " GitHub stars"
      : "Stars not fetched";
    row.append(
      heading,
      node("div", popularity, "stars"),
      node("p", app.description),
    );
    const packages = recipesFor(app);
    const specific = values.environment || values.release || values.operation;
    const label = app.deferred
      ? "Live qualification deferred"
      : packages.length
        ? packages.length + " matching signed recipe(s)"
        : specific
          ? "No matching signed setup; help verify this environment"
          : app.pilotEvidence
            ? "Pilot-tested on Linux; portable release pending"
            : "No portable Wazi package published";
    row.append(
      node(
        "div",
        label +
          (app.metadataStale ? "; GitHub metadata unverified or stale" : ""),
        "package-state",
      ),
    );
    const button = node(
      "button",
      packages.length ? "View setup" : "View project",
    );
    button.type = "button";
    button.dataset.application = app.applicationId;
    button.setAttribute("aria-label", button.textContent + " for " + app.name);
    button.addEventListener("click", () => {
      state.returnProject = app.applicationId;
      const p = parameters();
      p.set("app", app.applicationId);
      history.pushState(null, "", "?" + p);
      showDetails(app);
    });
    row.append(button);
    result.append(row);
  }
}
function showDetails(app) {
  byId("library").hidden = true;
  byId("workspace").hidden = true;
  byId("details").hidden = false;
  const region = byId("detail-body");
  region.replaceChildren(node("h1", app.name));
  region.firstChild.id = "detail-title";
  region.firstChild.tabIndex = -1;
  region.firstChild.focus();
  region.append(node("p", app.description));
  const upstream = node("a", "Upstream repository");
  upstream.href = "https://github.com/" + app.repository;
  upstream.rel = "noreferrer";
  region.append(upstream);
  const documentation = node("p");
  const docsLink = node("a", "Upstream documentation");
  docsLink.href = "https://github.com/" + app.repository + "#readme";
  docsLink.rel = "noreferrer";
  documentation.append(docsLink);
  region.append(documentation);
  const summary = node("dl");
  for (const [key, value] of [
    ["Category", app.category],
    [
      "Latest observed GitHub release",
      (app.github?.latestRelease ||
        (app.github?.releaseObservedAt
          ? "No published GitHub release observed"
          : "Not fetched")) + (app.github?.releaseStale ? " (stale)" : ""),
    ],
    [
      "Upstream license",
      app.github?.license || "Not fetched; verify upstream before reuse",
    ],
    [
      "Metadata checked",
      app.metadataObservedAt
        ? new Date(app.metadataObservedAt * 1000).toLocaleString()
        : "Not fetched",
    ],
    [
      "Community outcomes",
      String(app.reportedOutcomes) + " unverified reports",
    ],
  ])
    summary.append(node("dt", key), node("dd", value));
  region.append(summary);
  const packages = available(app);
  if (app.pilotEvidence) {
    const proof = app.pilotEvidence;
    const section = node("section", undefined, "record");
    section.append(
      node("h2", "Previously tested in Wazi"),
      node("p", proof.releaseVersion + " / " + proof.environment),
      node("p", proof.operations.join(", ")),
      node(
        "p",
        "Verified " +
          new Date(proof.verifiedAt).toLocaleString() +
          ". " +
          proof.limitations,
      ),
      node("p", proof.evidenceDigest),
    );
    region.append(section);
  }
  if (!packages.length) {
    region.append(
      node(
        "h2",
        app.deferred
          ? "Qualification is deferred"
          : "Contribute the first Wazi setup",
      ),
      node(
        "p",
        "No portable recipe is published here. This does not mean the project has never been deployed elsewhere. A starter is an inert checklist, not an installer.",
      ),
    );
    const starter = node("a", "Download a discovery starter", "download");
    starter.href =
      "/api/starter?application=" + encodeURIComponent(app.applicationId);
    region.append(starter);
  }
  for (const recipe of packages) {
    const section = node("section", undefined, "record");
    section.append(
      node("h2", recipe.recipeId + " / " + recipe.releaseVersion),
      node(
        "p",
        "Signed package. Verify its signature locally and obtain approval before installation.",
      ),
    );
    section.append(
      node("p", "Recipe contributors: " + recipe.authors.join(", ")),
    );
    for (const env of recipe.environments)
      section.append(
        node(
          "p",
          familyName[env.environment.family] +
            " / " +
            env.environment.architecture +
            ": " +
            env.operations.join(", "),
        ),
      );
    const link = node("a", "Download signed package", "download");
    link.href = "/api/downloads/" + recipe.artifact;
    section.append(link);
    region.append(section);
  }
}
async function workspace() {
  byId("library").hidden = true;
  byId("details").hidden = true;
  byId("workspace").hidden = false;
  const tab = parameters().get("tab") || "applications";
  document.querySelectorAll("[data-tab]").forEach((el) => {
    if (el.dataset.tab === tab) el.setAttribute("aria-current", "page");
    else el.removeAttribute("aria-current");
  });
  byId("contribution-panel").hidden = tab !== "contributions" || !state.user;
  byId("empty-workspace").hidden = tab === "contributions" && !!state.user;
  byId("workspace-status").textContent = state.user
    ? "Signed in as " +
      state.user.login +
      ". Community identity grants no access to your hosts."
    : "Sign in to manage your contributions. Host operations remain in your private control plane.";
  byId("empty-title").textContent =
    tab === "evidence"
      ? "Evidence stays attached to its environment"
      : "Your hosts stay under your control";
  byId("empty-description").textContent =
    "This catalog has no host execution connection. Existing applications and jobs remain in your private Wazi workbench; no installations are simulated here.";
  if (tab === "contributions" && state.user) {
    byId("contribute").checked = !!state.consent?.enabled;
    byId("license-consent").checked = !!state.consent?.enabled;
    const region = byId("contributions");
    region.replaceChildren(node("p", "Loading your contributions."));
    try {
      const response = await api("/api/contributions");
      region.replaceChildren();
      if (!response.contributions.length && !response.candidates.length)
        region.append(
          node(
            "p",
            "No contributions submitted yet. Enable and configure your local outbox to share eligible outcomes.",
          ),
        );
      for (const report of response.contributions) {
        const row = node("article", undefined, "record");
        row.append(
          node(
            "h3",
            report.state === "withdrawn"
              ? "Withdrawn contribution"
              : report.body.applicationId,
          ),
          node("p", report.digest),
          node("p", report.state),
        );
        if (report.state !== "withdrawn") {
          const button = node("button", "Withdraw public evidence");
          button.type = "button";
          button.addEventListener("click", async () => {
            try {
              await api("/api/contributions/withdraw", { id: report.id });
              await workspace();
            } catch (error) {
              byId("workspace-status").textContent = error.message;
            }
          });
          row.append(button);
        }
        region.append(row);
      }
      for (const candidate of response.candidates) {
        const row = node("article", undefined, "record");
        row.append(
          node(
            "h3",
            candidate.index.recipeId + " / " + candidate.index.releaseVersion,
          ),
          node("p", candidate.state),
          node("p", candidate.digest),
        );
        const remove = node("button", "Withdraw candidate");
        remove.type = "button";
        remove.addEventListener("click", async () => {
          try {
            await api("/api/recipe-candidates/withdraw", { id: candidate.id });
            await workspace();
          } catch (error) {
            byId("workspace-status").textContent = error.message;
          }
        });
        row.append(remove);
        region.append(row);
      }
    } catch (error) {
      region.replaceChildren(node("p", error.message, "error"));
    }
  }
}
function route() {
  const dashboard = location.pathname === "/dashboard";
  for (const [id, active] of [
    ["library-nav", !dashboard],
    ["dashboard-nav", dashboard],
  ]) {
    if (active) byId(id).setAttribute("aria-current", "page");
    else byId(id).removeAttribute("aria-current");
  }
  if (dashboard) workspace();
  else {
    const selected = state.applications.find(
      (a) => a.applicationId === parameters().get("app"),
    );
    if (selected) showDetails(selected);
    else {
      byId("details").hidden = true;
      byId("workspace").hidden = true;
      byId("library").hidden = false;
      render();
      if (state.returnProject) {
        [...document.querySelectorAll(".app button")]
          .find((button) => button.dataset.application === state.returnProject)
          ?.focus();
        state.returnProject = null;
      }
    }
  }
}
async function load() {
  byId("load-error").hidden = true;
  byId("retry").hidden = true;
  try {
    const [catalog, identity] = await Promise.all([
      api("/api/applications"),
      api("/api/me"),
    ]);
    state.applications = catalog.applications;
    state.user = identity.user;
    state.consent = identity.consent;
    byId("signin").hidden = !!state.user;
    byId("identity").hidden = !state.user;
    byId("identity").textContent = state.user?.login || "";
    byId("logout").hidden = !state.user;
    if (!identity.signInConfigured && !state.user) {
      byId("signin").textContent = "Sign-in not configured";
      byId("signin").removeAttribute("href");
    }
    for (const category of [
      ...new Set(state.applications.map((a) => a.category)),
    ].sort()) {
      const option = node("option", category);
      option.value = category;
      byId("category").append(option);
    }
    const p = parameters();
    for (const el of byId("filters").elements)
      if (el.name && p.has(el.name)) el.value = p.get(el.name);
    route();
  } catch (error) {
    byId("results").replaceChildren();
    byId("results").setAttribute("aria-busy", "false");
    byId("load-error").textContent = error.message;
    byId("load-error").hidden = false;
    byId("retry").hidden = false;
    byId("count").textContent = "Catalog unavailable";
  }
}
byId("filters").addEventListener("submit", (event) => event.preventDefault());
byId("filters").addEventListener("input", () => {
  const p = new URLSearchParams(new FormData(byId("filters")));
  history.replaceState(null, "", "/apps?" + p);
  render();
});
byId("retry").addEventListener("click", load);
byId("close-detail").addEventListener("click", () => {
  const p = parameters();
  p.delete("app");
  history.pushState(null, "", "/apps?" + p);
  route();
});
document.querySelectorAll("[data-tab]").forEach((button) =>
  button.addEventListener("click", () => {
    history.pushState(null, "", "/dashboard?tab=" + button.dataset.tab);
    route();
  }),
);
byId("consent-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    state.consent = await api("/api/consent", {
      enabled: byId("contribute").checked,
      version: "1.0.0",
      licenseAccepted: byId("license-consent").checked,
    });
    byId("workspace-status").textContent = state.consent.enabled
      ? "Automatic contribution intake enabled. Configure your local outbox separately."
      : "Contribution intake disabled. Connected workers cancel pending sends at their next consent check.";
  } catch (error) {
    byId("workspace-status").textContent = error.message;
  }
});
byId("logout").addEventListener("click", async () => {
  try {
    await api("/auth/logout", {});
    location.assign("/apps");
  } catch (error) {
    byId("workspace-status").textContent = error.message;
  }
});
window.addEventListener("popstate", route);
load();
byId("device-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    await api("/api/device/approve", {
      userCode: byId("device-code").value.toUpperCase(),
    });
    byId("device-code").value = "";
    byId("workspace-status").textContent =
      "Worker authorized for contribution submission only.";
  } catch (error) {
    byId("workspace-status").textContent = error.message;
  }
});
byId("revoke-workers").addEventListener("click", async () => {
  try {
    await api("/api/agents/revoke", {});
    byId("workspace-status").textContent =
      "All contribution worker identities revoked.";
  } catch (error) {
    byId("workspace-status").textContent = error.message;
  }
});
byId("candidate-upload").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    const file = byId("candidate-file").files[0];
    if (!file || file.size > 4 * 1024 * 1024)
      throw new Error("Choose a candidate ZIP under 4 MiB.");
    const headers = { "Content-Type": "application/zip" };
    headers["X-CSRF-Token"] = state.user?.csrf || "";
    const response = await fetch("/api/recipe-candidates", {
      method: "POST",
      credentials: "same-origin",
      headers,
      body: file,
    });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || "Candidate rejected.");
    byId("workspace-status").textContent =
      "Candidate quarantined. Independent qualification and review are required.";
    byId("candidate-file").value = "";
    await workspace();
  } catch (error) {
    byId("workspace-status").textContent = error.message;
  }
});
