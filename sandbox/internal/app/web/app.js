const state = {
  blueprint: null,
  selection: "runtime-lite",
  language: "python3",
  lastResult: null,
  policies: null,
  sandboxes: [],
  activeSandboxId: "",
  events: [],
  files: [],
  resolvedEndpoint: null,
};

const examples = {
  python3: {
    preload: "import json",
    code: [
      "payload = {",
      "    'service': 'zgi-sandbox',",
      "    'mode': 'compat-preview',",
      "    'steps': ['run', 'inspect', 'iterate']",
      "}",
      "print(json.dumps(payload, indent=2))",
    ].join("\n"),
  },
  nodejs: {
    preload: "",
    code: [
      "const payload = {",
      "  service: 'zgi-sandbox',",
      "  mode: 'compat-preview',",
      "  steps: ['run', 'inspect', 'iterate']",
      "};",
      "console.log(JSON.stringify(payload, null, 2));",
    ].join("\n"),
  },
};

document.addEventListener("DOMContentLoaded", async () => {
  bindStaticActions();
  await loadBlueprint();
  await refreshPolicies();
  await refreshSandboxes();
  setLanguage("python3");
  applyURLState();
  selectEntity(state.selection, { scroll: false });
  exposeDebugHooks();
});

async function api(method, path, body) {
  const response = await fetch(path, {
    method,
    headers: {
      "Content-Type": "application/json",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  const payload = await response.json();
  if (!response.ok || payload.code !== 0) {
    throw new Error(payload.message || "request failed");
  }
  return payload.data;
}

async function loadBlueprint() {
  state.blueprint = await api("GET", "/api/blueprint");
  document.getElementById("hero-subtitle").textContent = state.blueprint.subtitle;
  renderHighlights(state.blueprint.highlights);
  renderNavigation(state.blueprint.navigation);
  renderPanelGrid("runtime-grid", state.blueprint.runtimes);
  renderPanelGrid("module-grid", state.blueprint.modules);
  renderPanelGrid("phase-strip", state.blueprint.phases);
}

function bindStaticActions() {
  document.querySelectorAll("[data-entity]").forEach((button) => {
    button.addEventListener("click", () => selectEntity(button.dataset.entity));
  });

  document.querySelectorAll("[data-language]").forEach((button) => {
    button.addEventListener("click", () => setLanguage(button.dataset.language));
  });

  document.getElementById("run-button").addEventListener("click", runSample);
  document.getElementById("reset-button").addEventListener("click", () => {
    setLanguage(state.language);
    setResultState("idle");
    updatePlaygroundNote();
  });
  document
    .getElementById("create-sandbox-button")
    .addEventListener("click", createSandbox);
  document
    .getElementById("refresh-sandboxes-button")
    .addEventListener("click", refreshSandboxes);
  document
    .getElementById("renew-sandbox-button")
    .addEventListener("click", renewSandbox);
  document
    .getElementById("delete-sandbox-button")
    .addEventListener("click", deleteSandbox);
  document
    .getElementById("resolve-endpoint-button")
    .addEventListener("click", resolveEndpoint);
  document
    .getElementById("sandbox-profile")
    .addEventListener("change", syncCreateDefaults);
  document
    .getElementById("run-command-button")
    .addEventListener("click", runCommand);
  document.getElementById("save-file-button").addEventListener("click", saveFile);
  document.getElementById("load-file-button").addEventListener("click", loadFile);
  document
    .getElementById("delete-file-button")
    .addEventListener("click", deleteFile);
  document
    .getElementById("refresh-events-button")
    .addEventListener("click", refreshEvents);
  document
    .getElementById("refresh-policies-button")
    .addEventListener("click", refreshPolicies);
  document
    .getElementById("event-type-filter")
    .addEventListener("change", refreshEvents);
}

function setLanguage(language) {
  state.language = language;
  const example = examples[language];
  document.getElementById("preload-input").value = example.preload;
  document.getElementById("code-input").value = example.code;

  document.querySelectorAll("[data-language]").forEach((button) => {
    button.classList.toggle("is-active", button.dataset.language === language);
  });
}

function applyURLState() {
  const url = new URL(window.location.href);
  const requestedLanguage = url.searchParams.get("language");
  if (requestedLanguage && examples[requestedLanguage]) {
    setLanguage(requestedLanguage);
  }

  const selectedEntity = url.searchParams.get("select");
  if (selectedEntity) {
    state.selection = selectedEntity;
  }

  if (url.searchParams.get("autorun") === "1") {
    window.setTimeout(() => {
      runSample();
    }, 250);
  }
}

function renderHighlights(items) {
  const strip = document.getElementById("highlight-strip");
  strip.innerHTML = "";
  items.forEach((item) => {
    const pill = document.createElement("div");
    pill.className = "highlight-pill";
    pill.textContent = item;
    strip.appendChild(pill);
  });
}

function renderNavigation(items) {
  const nav = document.getElementById("side-nav");
  nav.innerHTML = "";
  items.forEach((item) => {
    const button = document.createElement("button");
    button.className = "nav-button";
    button.textContent = item.label;
    button.type = "button";
    button.dataset.entity = item.ID || item.id;
    button.addEventListener("click", () => selectEntity(button.dataset.entity));
    nav.appendChild(button);
  });
}

function renderPanelGrid(targetID, panels) {
  const container = document.getElementById(targetID);
  container.innerHTML = "";
  panels.forEach((panel) => {
    const button = document.createElement("button");
    button.className =
      targetID === "runtime-grid"
        ? "runtime-tile"
        : targetID === "module-grid"
          ? "module-button"
          : "phase-card";
    button.type = "button";
    button.dataset.entity = panel.id;
    button.dataset.anchor = panel.id;
    button.innerHTML = `
      <p class="eyebrow">${escapeHTML(panel.category)}</p>
      <h4>${escapeHTML(panel.label)}</h4>
      <p>${escapeHTML(panel.summary)}</p>
    `;
    button.addEventListener("click", () => selectEntity(panel.id));
    container.appendChild(button);
  });
}

function selectEntity(entityID, options = { scroll: true }) {
  if (!state.blueprint) {
    state.selection = entityID;
    return;
  }

  state.selection = entityID;
  syncActiveState(entityID);

  const panel = lookupPanel(entityID);
  if (!panel) {
    return;
  }

  document.getElementById("inspector-title").textContent = panel.label;
  document.getElementById("inspector-category").textContent = panel.category;
  document.getElementById("inspector-summary").textContent = panel.summary;
  renderInspectorList("inspector-capabilities", panel.capabilities);
  renderInspectorList("inspector-actions", panel.actions);
  renderMetrics(panel.metrics);

  if (options.scroll) {
    const target = document.querySelector(`[data-anchor="${entityID}"]`);
    if (target) {
      target.scrollIntoView({ behavior: "smooth", block: "center" });
      target.classList.add("focus-ring");
      window.setTimeout(() => target.classList.remove("focus-ring"), 900);
    }
  }
}

function syncActiveState(entityID) {
  document.querySelectorAll("[data-entity]").forEach((node) => {
    node.classList.toggle("is-active", node.dataset.entity === entityID);
  });
}

function lookupPanel(entityID) {
  if (entityID === "playground") {
    return {
      id: "playground",
      label: "Execution Lab",
      category: "Console",
      summary:
        "A live operator surface for compat execution, session execution, command dispatch, file edits, endpoint preview, and right-side result validation.",
      capabilities: [
        "Run compat execution without a sandbox",
        "Switch to session exec automatically when a sandbox is selected",
        "Inspect code, command, file, and endpoint results from one console",
      ],
      actions: [
        "Run the current code sample",
        "Create a session or interactive sandbox below",
        "Validate operator workflows before shipping",
      ],
      metrics: [
        { label: "Compat endpoint", value: "/v1/sandbox/run" },
        {
          label: "Active mode",
          value: state.activeSandboxId ? "session exec" : "compat exec",
        },
        {
          label: "Endpoint preview",
          value: state.resolvedEndpoint ? state.resolvedEndpoint.port : "inactive",
        },
      ],
    };
  }

  return [
    ...(state.blueprint?.runtimes || []),
    ...(state.blueprint?.modules || []),
    ...(state.blueprint?.phases || []),
  ].find((panel) => panel.id === entityID);
}

function renderInspectorList(targetID, items) {
  const list = document.getElementById(targetID);
  list.innerHTML = "";
  items.forEach((item) => {
    const element = document.createElement("li");
    element.textContent = item;
    list.appendChild(element);
  });
}

function renderMetrics(metrics) {
  const target = document.getElementById("inspector-metrics");
  target.innerHTML = "";
  metrics.forEach((metric) => {
    const item = document.createElement("div");
    item.innerHTML = `<span>${escapeHTML(metric.label)}</span><strong>${escapeHTML(String(metric.value))}</strong>`;
    target.appendChild(item);
  });
}

async function refreshPolicies() {
  state.policies = await api("GET", "/v1/policies");
  populatePolicyControls();
  renderPolicyGrid();
  refreshInspectorSelection();
}

function populatePolicyControls() {
  const networkSelect = document.getElementById("network-policy-select");
  const dependencySelect = document.getElementById("dependency-profile-select");

  networkSelect.innerHTML = "";
  (state.policies.network_profiles || []).forEach((item) => {
    const option = document.createElement("option");
    option.value = item.name;
    option.textContent = item.name;
    networkSelect.appendChild(option);
  });

  dependencySelect.innerHTML = "";
  (state.policies.dependency_profiles || []).forEach((item) => {
    const option = document.createElement("option");
    option.value = item.name;
    option.textContent = item.name;
    dependencySelect.appendChild(option);
  });

  syncCreateDefaults();
}

function syncCreateDefaults() {
  const profile = document.getElementById("sandbox-profile").value;
  const networkSelect = document.getElementById("network-policy-select");
  const networkEnabled = document.getElementById("sandbox-network-enabled");

  networkSelect.value =
    profile === "interactive" ? "interactive-preview" : "deny-by-default";
  networkEnabled.checked = profile === "interactive";
}

function renderPolicyGrid() {
  const grid = document.getElementById("policy-grid");
  grid.innerHTML = "";

  const cards = [
    {
      title: "Runtime profiles",
      body: state.policies.runtime_profiles.map((item) => item.name).join(", "),
      note: state.policies.runtime_profiles.map((item) => item.isolation).join(" / "),
    },
    {
      title: "Network defaults",
      body: state.policies.network_profiles.map((item) => item.name).join(", "),
      note: "Policy-selected outbound access",
    },
    {
      title: "Dependency mode",
      body: state.policies.dependency_policy.mode,
      note: state.policies.dependency_profiles.map((item) => item.name).join(", "),
    },
    {
      title: "Execution limits",
      body: `${state.policies.limits.max_workers} workers / ${state.policies.limits.default_timeout}s timeout`,
      note: `${state.policies.limits.max_active_sandboxes} active sandboxes / ${state.policies.limits.max_file_size_kb} KB max file size`,
    },
  ];

  cards.forEach((card) => {
    const item = document.createElement("div");
    item.className = "policy-card";
    item.innerHTML = `
      <span class="editor-label">${escapeHTML(card.title)}</span>
      <strong>${escapeHTML(card.body)}</strong>
      <p>${escapeHTML(card.note)}</p>
    `;
    grid.appendChild(item);
  });
}

async function refreshSandboxes() {
  const payload = await api("GET", "/v1/sandboxes");
  state.sandboxes = payload.items || [];

  if (!state.sandboxes.find((item) => item.id === state.activeSandboxId)) {
    state.activeSandboxId = state.sandboxes[0]?.id || "";
    state.resolvedEndpoint = null;
  }

  renderSandboxList();
  renderActiveSandboxMetrics();
  renderEndpointShell();
  updatePlaygroundNote();
  refreshInspectorSelection();
  await refreshEvents();
  await refreshFileTree();
}

async function createSandbox() {
  const profile = document.getElementById("sandbox-profile").value;
  const created = await api("POST", "/v1/sandboxes", {
    runtime_profile: profile,
    ttl_seconds: profile === "interactive" ? 600 : 300,
    network_enabled: document.getElementById("sandbox-network-enabled").checked,
    network_policy: document.getElementById("network-policy-select").value,
    dependency_profile: document.getElementById("dependency-profile-select").value,
    workspace_binding: document.getElementById("workspace-binding-input").value,
  });

  state.activeSandboxId = created.id;
  state.resolvedEndpoint = null;
  await refreshSandboxes();
  selectEntity("module-lifecycle");
}

async function renewSandbox() {
  const sandbox = currentSandbox();
  if (!sandbox) {
    return showActionError("Select a sandbox before renewing it.");
  }

  await api("POST", `/v1/sandboxes/${sandbox.id}/renew-expiration`, {
    ttl_seconds: sandbox.runtime_profile === "interactive" ? 600 : 300,
  });
  await refreshSandboxes();
}

async function deleteSandbox() {
  const sandbox = currentSandbox();
  if (!sandbox) {
    return showActionError("Select a sandbox before deleting it.");
  }

  await api("DELETE", `/v1/sandboxes/${sandbox.id}`);
  state.activeSandboxId = "";
  state.resolvedEndpoint = null;
  await refreshSandboxes();
}

async function resolveEndpoint() {
  const sandbox = currentSandbox();
  if (!sandbox) {
    return showActionError("Create or select an interactive sandbox before resolving an endpoint.");
  }
  if (sandbox.runtime_profile !== "interactive") {
    return showActionError("Endpoint resolution is only available for interactive sandboxes.");
  }

  const port = document.getElementById("endpoint-port-input").value || "3000";
  state.resolvedEndpoint = await api(
    "GET",
    `/v1/sandboxes/${encodeURIComponent(sandbox.id)}/endpoints/${encodeURIComponent(port)}`,
  );
  renderEndpointShell();
  await refreshEvents();
}

function renderSandboxList() {
  const shell = document.getElementById("sandbox-list");
  shell.innerHTML = "";

  if (state.sandboxes.length === 0) {
    shell.appendChild(
      emptyMessage(
        "No sandboxes yet. Create one to unlock session exec, files, observer queries, and interactive endpoint previews.",
      ),
    );
    return;
  }

  state.sandboxes.forEach((sandbox) => {
    const item = document.createElement("div");
    item.className = "list-item";

    const meta = document.createElement("div");
    meta.innerHTML = `
      <span class="editor-label">${escapeHTML(sandbox.runtime_profile)}</span>
      <strong>${escapeHTML(sandbox.id)}</strong>
      <p>${escapeHTML(sandbox.status)} · policy ${escapeHTML(sandbox.network_policy)} · deps ${escapeHTML(sandbox.dependency_profile)} · expires ${escapeHTML(formatDate(sandbox.expires_at))}</p>
    `;

    const button = document.createElement("button");
    button.type = "button";
    button.textContent = sandbox.id === state.activeSandboxId ? "Active" : "Select";
    button.disabled = sandbox.id === state.activeSandboxId;
    button.addEventListener("click", async () => {
      state.activeSandboxId = sandbox.id;
      state.resolvedEndpoint = null;
      renderSandboxList();
      renderActiveSandboxMetrics();
      renderEndpointShell();
      updatePlaygroundNote();
      await refreshEvents();
      await refreshFileTree();
      refreshInspectorSelection();
    });

    item.append(meta, button);
    shell.appendChild(item);
  });
}

function renderActiveSandboxMetrics() {
  const grid = document.getElementById("active-sandbox-metrics");
  grid.innerHTML = "";

  const sandbox = currentSandbox();
  const metrics = sandbox
    ? [
        { label: "Active sandbox", value: sandbox.id },
        { label: "Runtime", value: sandbox.runtime_profile },
        { label: "Status", value: sandbox.status },
        { label: "TTL", value: `${sandbox.ttl_seconds}s` },
        { label: "Network", value: sandbox.network_enabled ? sandbox.network_policy : "disabled" },
        { label: "Dependencies", value: sandbox.dependency_profile },
      ]
    : [
        { label: "Active sandbox", value: "none" },
        { label: "Runtime", value: "-" },
        { label: "Status", value: "idle" },
        { label: "TTL", value: "-" },
        { label: "Network", value: "-" },
        { label: "Dependencies", value: "-" },
      ];

  metrics.forEach((metric) => {
    const item = document.createElement("div");
    item.className = "status-card";
    item.innerHTML = `<span class="editor-label">${escapeHTML(metric.label)}</span><strong>${escapeHTML(metric.value)}</strong>`;
    grid.appendChild(item);
  });
}

function renderEndpointShell() {
  const shell = document.getElementById("endpoint-shell");
  shell.innerHTML = "";

  const sandbox = currentSandbox();
  if (!sandbox) {
    shell.appendChild(emptyMessage("Select a sandbox to inspect endpoint routing."));
    return;
  }
  if (sandbox.runtime_profile !== "interactive") {
    shell.appendChild(emptyMessage("Switch to an interactive sandbox to resolve preview endpoints."));
    return;
  }
  if (!state.resolvedEndpoint) {
    shell.appendChild(emptyMessage("No endpoint resolved yet. Pick a port and resolve it."));
    return;
  }

  const item = document.createElement("div");
  item.className = "list-item";
  item.innerHTML = `
    <div>
      <span class="editor-label">Interactive endpoint</span>
      <strong>${escapeHTML(state.resolvedEndpoint.url)}</strong>
      <p>port ${escapeHTML(state.resolvedEndpoint.port)} · status ${escapeHTML(state.resolvedEndpoint.status)}</p>
    </div>
  `;
  shell.appendChild(item);
}

function updatePlaygroundNote() {
  const note = document.querySelector(".playground-note");
  const sandbox = currentSandbox();
  note.textContent = sandbox
    ? `Code runs through /v1/exec/code inside ${sandbox.id}. Network policy is ${sandbox.network_policy}, dependency profile is ${sandbox.dependency_profile}, and workspace/file actions are active.`
    : "No sandbox selected. Code runs through the compat endpoint until you create or select a sandbox.";
}

async function runSample() {
  selectEntity("playground", { scroll: true });
  setResultState("running");
  const runButton = document.getElementById("run-button");
  const resetButton = document.getElementById("reset-button");
  runButton.disabled = true;
  resetButton.disabled = true;
  const originalLabel = runButton.textContent;
  runButton.textContent = "Running...";

  const payload = {
    language: state.language,
    preload: document.getElementById("preload-input").value,
    code: document.getElementById("code-input").value,
    enable_network: document.getElementById("network-toggle").checked,
  };

  try {
    const result = state.activeSandboxId
      ? await api("POST", "/v1/exec/code", {
          sandbox_id: state.activeSandboxId,
          ...payload,
        })
      : await api("POST", "/v1/sandbox/run", payload);

    state.lastResult = result;
    updateResult(result);
    setResultState("success");
    await refreshEvents();
    await refreshFileTree();
  } catch (error) {
    updateResult({
      stdout: "",
      error: String(error),
      exit_code: -1,
      duration_ms: 0,
      truncated: false,
    });
    setResultState("error");
  } finally {
    runButton.disabled = false;
    resetButton.disabled = false;
    runButton.textContent = originalLabel;
  }
}

async function runCommand() {
  const sandbox = currentSandbox();
  if (!sandbox) {
    return showActionError("Create or select a sandbox before running commands.");
  }

  selectEntity("module-exec");
  setResultState("running");

  try {
    const result = await api("POST", "/v1/exec/command", {
      sandbox_id: sandbox.id,
      command: document.getElementById("command-input").value,
    });
    updateResult(result);
    setResultState("success");
    await refreshEvents();
    await refreshFileTree();
  } catch (error) {
    showActionError(String(error));
  }
}

async function saveFile() {
  const sandbox = currentSandbox();
  if (!sandbox) {
    return showActionError("Create or select a sandbox before saving files.");
  }

  await api("POST", "/v1/files/upload", {
    sandbox_id: sandbox.id,
    path: document.getElementById("file-path-input").value,
    content: document.getElementById("file-content-input").value,
    encoding: "utf-8",
  });
  await refreshFileTree();
  await refreshEvents();
}

async function loadFile() {
  const sandbox = currentSandbox();
  if (!sandbox) {
    return showActionError("Create or select a sandbox before loading files.");
  }

  const result = await api(
    "GET",
    `/v1/files/download?sandbox_id=${encodeURIComponent(sandbox.id)}&path=${encodeURIComponent(document.getElementById("file-path-input").value)}`,
  );
  document.getElementById("file-content-input").value = result.content;
  await refreshEvents();
}

async function deleteFile() {
  const sandbox = currentSandbox();
  if (!sandbox) {
    return showActionError("Create or select a sandbox before deleting files.");
  }

  await api(
    "DELETE",
    `/v1/files?sandbox_id=${encodeURIComponent(sandbox.id)}&path=${encodeURIComponent(document.getElementById("file-path-input").value)}`,
  );
  await refreshFileTree();
  await refreshEvents();
}

async function refreshFileTree() {
  const shell = document.getElementById("file-list");
  if (!state.activeSandboxId) {
    state.files = [];
    shell.innerHTML = "";
    shell.appendChild(emptyMessage("Select a sandbox to inspect its workspace files."));
    return;
  }

  const payload = await api(
    "GET",
    `/v1/files/tree?sandbox_id=${encodeURIComponent(state.activeSandboxId)}`,
  );
  state.files = payload.items || [];
  renderFileList();
}

function renderFileList() {
  const shell = document.getElementById("file-list");
  shell.innerHTML = "";

  if (state.files.length === 0) {
    shell.appendChild(emptyMessage("The active sandbox workspace is empty."));
    return;
  }

  state.files.forEach((file) => {
    const item = document.createElement("div");
    item.className = "list-item";

    const meta = document.createElement("div");
    meta.innerHTML = `
      <span class="editor-label">${escapeHTML(file.is_directory ? "directory" : "file")}</span>
      <strong>${escapeHTML(file.path)}</strong>
      <p>${escapeHTML(`${file.size} bytes · ${file.mode}`)}</p>
    `;

    const button = document.createElement("button");
    button.type = "button";
    button.textContent = "Use";
    button.addEventListener("click", () => {
      document.getElementById("file-path-input").value = file.path;
      if (!file.is_directory) {
        loadFile();
      }
    });

    item.append(meta, button);
    shell.appendChild(item);
  });
}

async function refreshEvents() {
  const shell = document.getElementById("event-list");
  if (!state.activeSandboxId) {
    state.events = [];
    shell.innerHTML = "";
    shell.appendChild(emptyMessage("Select a sandbox to inspect observer events."));
    return;
  }

  const type = document.getElementById("event-type-filter").value;
  const payload = await api(
    "GET",
    `/v1/observer/events?sandbox_id=${encodeURIComponent(state.activeSandboxId)}&limit=8&type=${encodeURIComponent(type)}`,
  );
  state.events = payload.events || [];
  renderEventList();
}

function renderEventList() {
  const shell = document.getElementById("event-list");
  shell.innerHTML = "";

  if (state.events.length === 0) {
    shell.appendChild(emptyMessage("No observer events match the current filter for the active sandbox."));
    return;
  }

  state.events.forEach((event) => {
    const item = document.createElement("div");
    item.className = "event-item";
    const meta = event.metadata
      ? Object.entries(event.metadata)
          .map(([key, value]) => `${key}=${value}`)
          .join(" · ")
      : "no metadata";
    item.innerHTML = `
      <strong>${escapeHTML(event.type)}</strong>
      <p>${escapeHTML(event.message)}</p>
      <p>${escapeHTML(meta)}</p>
      <div class="event-meta">
        <span>${escapeHTML(event.sandbox_id || "global")}</span>
        <span>${escapeHTML(formatDate(event.created_at))}</span>
      </div>
    `;
    shell.appendChild(item);
  });
}

function currentSandbox() {
  return state.sandboxes.find((item) => item.id === state.activeSandboxId);
}

function updateResult(result) {
  document.getElementById("stdout-output").textContent =
    result.stdout || "No stdout output.";
  document.getElementById("stderr-output").textContent =
    result.error || "No stderr output.";
  document.getElementById("result-exit").textContent = String(result.exit_code);
  document.getElementById("result-duration").textContent = `${result.duration_ms} ms`;
  document.getElementById("result-truncation").textContent = result.truncated
    ? "truncated"
    : "complete";
}

function setResultState(status) {
  const label = document.getElementById("result-state");
  label.textContent = status;
  label.dataset.state = status;
}

function showActionError(message) {
  updateResult({
    stdout: "",
    error: message,
    exit_code: -1,
    duration_ms: 0,
    truncated: false,
  });
  setResultState("error");
}

function emptyMessage(text) {
  const item = document.createElement("div");
  item.className = "list-item";
  item.innerHTML = `<div><span class="editor-label">State</span><strong>${escapeHTML(text)}</strong></div>`;
  return item;
}

function formatDate(value) {
  try {
    return new Date(value).toLocaleString();
  } catch {
    return value;
  }
}

function refreshInspectorSelection() {
  if (state.selection) {
    selectEntity(state.selection, { scroll: false });
  }
}

function exposeDebugHooks() {
  window.zgiSandbox = {
    selectEntity,
    setLanguage,
    runSample,
    refreshPolicies,
    refreshSandboxes,
    refreshEvents,
    resolveEndpoint,
    state,
  };
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}
