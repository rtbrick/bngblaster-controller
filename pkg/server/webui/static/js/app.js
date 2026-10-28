// BNG Blaster Controller — Web UI application logic.
// Vanilla JS, no build step, no framework: this file is served as-is by the
// embedded controller binary.
(function () {
  'use strict';

  //= ========================================================================
  // API client
  //= ========================================================================
  // apiFetch marks every request as coming from the web UI, which lets the
  // controller keep its polling out of the log (-log-ui=false).
  function apiFetch(url, opts) {
    const o = Object.assign({}, opts);
    o.headers = Object.assign({ 'X-BNGBlaster-Client': 'webui' }, o.headers);
    return fetch(url, o);
  }

  const API = {
    async fetchJSON(url, opts) {
      const res = await apiFetch(url, opts);
      if (res.status === 204 || res.status === 202) return null;
      const text = await res.text();
      let body = null;
      if (text) {
        try { body = JSON.parse(text); } catch (e) { body = text; }
      }
      if (!res.ok) {
        const msg = (body && body.message) ? body.message : (typeof body === 'string' ? body : res.statusText);
        const err = new Error(msg || ('HTTP ' + res.status));
        err.status = res.status;
        err.body = body;
        throw err;
      }
      return body;
    },
    version() { return this.fetchJSON('/api/v1/version'); },
    schema() { return this.fetchJSON('/api/v1/schema'); },
    interfaces() { return this.fetchJSON('/api/v1/interfaces'); },
    // detail=true returns [{name, status}] in one request instead of the
    // plain name array plus one status request per instance.
    instances(detail) {
      return this.fetchJSON('/api/v1/instances' + (detail ? '?detail=true' : ''));
    },
    status(name) { return this.fetchJSON('/api/v1/instances/' + encodeURIComponent(name)); },
    create(name, config) {
      return this.fetchJSON('/api/v1/instances/' + encodeURIComponent(name), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(config),
      });
    },
    getConfig(name) {
      // config.json is served as a plain file (also used for the download
      // link), so it doesn't follow the {status,message} JSON error contract
      // used elsewhere and can't go through fetchJSON as-is. no-store: the
      // editor must show what was just saved, never a cached older copy
      // (e.g. from a proxy or an older controller without Cache-Control).
      return apiFetch('/api/v1/instances/' + encodeURIComponent(name) + '/config.json', { cache: 'no-store' }).then((res) => {
        if (!res.ok) throw new Error('HTTP ' + res.status);
        return res.json();
      });
    },
    delete(name) {
      return this.fetchJSON('/api/v1/instances/' + encodeURIComponent(name), { method: 'DELETE' });
    },
    start(name, runningConfig) {
      return this.fetchJSON('/api/v1/instances/' + encodeURIComponent(name) + '/_start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(runningConfig),
      });
    },
    stop(name) {
      return this.fetchJSON('/api/v1/instances/' + encodeURIComponent(name) + '/_stop', { method: 'POST' });
    },
    kill(name) {
      return this.fetchJSON('/api/v1/instances/' + encodeURIComponent(name) + '/_kill', { method: 'POST' });
    },
    command(name, command, args) {
      return this.fetchJSON('/api/v1/instances/' + encodeURIComponent(name) + '/_command', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ command: command, arguments: args || {} }),
      });
    },
    streams(name, offset, limit, filters) {
      const params = new URLSearchParams({ offset: String(offset), limit: String(limit) });
      Object.entries(filters || {}).forEach(([k, v]) => {
        if (v !== undefined && v !== null && v !== '') params.set(k, v);
      });
      return this.fetchJSON('/api/v1/instances/' + encodeURIComponent(name) + '/_streams?' + params.toString());
    },
    sessions(name, offset, limit, filters) {
      const params = new URLSearchParams({ offset: String(offset), limit: String(limit) });
      Object.entries(filters || {}).forEach(([k, v]) => {
        if (v !== undefined && v !== null && v !== '') params.set(k, v);
      });
      return this.fetchJSON('/api/v1/instances/' + encodeURIComponent(name) + '/_sessions?' + params.toString());
    },
    host() { return this.fetchJSON('/api/v1/host'); },
    hostInterfaces() { return this.fetchJSON('/api/v1/host/interfaces'); },
    // Aggregates session-counters, the three interface commands and
    // test-info into one cached response, instead of five separate
    // control-socket round-trips per poll per open browser tab.
    overview(name) {
      return this.fetchJSON('/api/v1/instances/' + encodeURIComponent(name) + '/_overview');
    },
    logs(name, offset, limit) {
      return this.fetchJSON('/api/v1/instances/' + encodeURIComponent(name) + '/_logs?offset=' + offset + (limit ? '&limit=' + limit : ''));
    },
    files(name) {
      return this.fetchJSON('/api/v1/instances/' + encodeURIComponent(name) + '/_files');
    },
    fileDownloadURL(name, file) {
      return '/api/v1/instances/' + encodeURIComponent(name) + '/_files/' + encodeURIComponent(file);
    },
    generators() { return this.fetchJSON('/api/v1/generators'); },
    generate(name, tool, request) {
      return this.fetchJSON('/api/v1/instances/' + encodeURIComponent(name) + '/_generate/' + encodeURIComponent(tool), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(request),
      });
    },
    upload(name, file) {
      const fd = new FormData();
      fd.append('file', file);
      return apiFetch('/api/v1/instances/' + encodeURIComponent(name) + '/_upload', { method: 'POST', body: fd })
        .then((res) => {
          if (!res.ok) return res.text().then((t) => { throw new Error(t || res.statusText); });
        });
    },
  };

  //= ========================================================================
  // Small DOM / a11y helpers
  //= ========================================================================
  const $ = (sel, root) => (root || document).querySelector(sel);
  const $all = (sel, root) => Array.from((root || document).querySelectorAll(sel));
  const el = (tag, attrs, children) => {
    const node = document.createElement(tag);
    Object.entries(attrs || {}).forEach(([k, v]) => {
      if (v === undefined || v === null) return;
      if (k === 'class') node.className = v;
      else if (k === 'text') node.textContent = v;
      else if (k.startsWith('on') && typeof v === 'function') node.addEventListener(k.slice(2), v);
      else node.setAttribute(k, v);
    });
    (children || []).forEach((c) => { if (c) node.appendChild(c); });
    return node;
  };

  let idCounter = 0;
  const nextId = (prefix) => prefix + '-' + (++idCounter);

  // The "instance is not running" paths overwrite these placeholders in
  // place, so the original wording is captured once up front to be able to
  // restore it when the view is reset for another instance.
  const DEFAULT_EMPTY_TEXT = {};

  //= ========================================================================
  // Polling
  //= ========================================================================
  // Every recurring refresh in this UI goes through schedulePoll, which skips
  // ticks while the browser tab is in the background. Without that, a
  // forgotten tab keeps hammering the controller's unix control socket
  // indefinitely - and the data it fetches is not being looked at anyway.
  // Becoming visible again runs each active poll immediately, so the view is
  // current by the time the user has finished switching to it rather than up
  // to one interval stale.
  const activePolls = new Set();

  function schedulePoll(fn, intervalMs) {
    const poll = {
      fn: fn,
      id: setInterval(() => { if (!document.hidden) fn(); }, intervalMs),
    };
    activePolls.add(poll);
    return poll;
  }

  function cancelPoll(poll) {
    if (!poll) return null;
    clearInterval(poll.id);
    activePolls.delete(poll);
    return null;
  }

  document.addEventListener('visibilitychange', () => {
    if (document.hidden) return;
    activePolls.forEach((poll) => poll.fn());
  });

  const TOAST_TIMEOUT_MS = { error: 10000, success: 4000, info: 6000 };

  // Shows a message as a visible toast. #global-status alone is a
  // screen-reader-only live region, so anything announced through it used to
  // be completely invisible to sighted users - which meant a failed stop,
  // kill, delete or command reported nothing at all on screen.
  function toast(message, kind) {
    const region = $('#toast-region');
    if (!region) return;
    const node = el('div', { class: 'toast ' + (kind || 'info') }, [
      el('div', { class: 'toast-message', text: message }),
    ]);
    const dismiss = () => {
      if (node.parentNode) node.parentNode.removeChild(node);
      clearTimeout(timer);
    };
    node.appendChild(el('button', {
      type: 'button', class: 'toast-dismiss', 'aria-label': 'Dismiss notification',
      text: '\u2715', onclick: dismiss,
    }));
    region.appendChild(node);
    // Errors linger noticeably longer: they usually carry something the user
    // needs to read and act on, rather than a confirmation they can ignore.
    const timer = setTimeout(dismiss, TOAST_TIMEOUT_MS[kind] || TOAST_TIMEOUT_MS.info);
    // Never let the stack grow without bound during a burst of failures.
    while (region.childElementCount > 5) region.removeChild(region.firstChild);
  }

  // Announces a message to assistive technology *and* shows it on screen.
  // kind is 'error' | 'success' | 'info' (default 'info').
  function announce(message, kind) {
    const region = $('#global-status');
    region.textContent = '';
    // Force screen readers to re-announce even if the text is identical.
    window.requestAnimationFrame(() => { region.textContent = message; });
    toast(message, kind);
  }

  // Runs an action that talks to the controller, reporting both outcomes.
  // Without this every caller had to remember its own try/catch; the ones
  // that forgot (stop, kill) turned a failure into a silent unhandled
  // promise rejection and left the UI showing the wrong state.
  async function withFeedback(action, successMessage, failureMessage) {
    try {
      const result = await action();
      if (successMessage) announce(successMessage, 'success');
      return result;
    } catch (e) {
      announce(failureMessage + ': ' + e.message, 'error');
      return undefined;
    }
  }

  // Sets (or clears) a dialog's inline status/alert box with consistent
  // error/success styling. Pass an empty message to clear it back to an
  // invisible, unstyled state.
  function setStatusMessage(el, message, kind) {
    el.textContent = message || '';
    el.className = message ? ('form-status' + (kind ? ' ' + kind : '')) : '';
  }

  // A start/save failure can be the raw (possibly multi-line) stderr output
  // of the bngblaster process itself - e.g. a JSON config validation error.
  // That deserves more than a line of plain text quietly sitting in a small
  // alert box: a clear heading plus a monospace, line-break-preserving
  // block makes it obvious something failed and keeps the actual reason
  // readable instead of being squashed onto one line.
  function showDetailedError(box, title, message) {
    box.innerHTML = '';
    box.className = 'form-status error';
    box.appendChild(el('div', { class: 'form-status-title', text: title }));
    box.appendChild(el('pre', { class: 'form-status-detail', text: message }));
  }

  function escapeHtml(str) {
    return String(str).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }

  //= ========================================================================
  // Dialog handling (native <dialog>, with focus restore)
  //= ========================================================================
  let lastFocusedBeforeDialog = null;
  function openDialog(id) {
    const dialog = document.getElementById(id);
    lastFocusedBeforeDialog = document.activeElement;
    dialog.showModal();
    const focusable = dialog.querySelector('input, select, textarea, button');
    if (focusable) focusable.focus();
  }
  function closeDialog(id) {
    const dialog = document.getElementById(id);
    if (dialog.open) dialog.close();
    if (lastFocusedBeforeDialog && document.contains(lastFocusedBeforeDialog)) {
      lastFocusedBeforeDialog.focus();
    }
  }
  document.addEventListener('click', (ev) => {
    const trigger = ev.target.closest('[data-close-dialog]');
    if (trigger) closeDialog(trigger.getAttribute('data-close-dialog'));
  });

  function confirmAction(message) {
    return new Promise((resolve) => {
      $('#dialog-confirm-message').textContent = message;
      openDialog('dialog-confirm');
      const dialog = $('#dialog-confirm');
      let confirmed = false;
      const onConfirm = () => { confirmed = true; closeDialog('dialog-confirm'); };
      const onClose = () => {
        dialog.removeEventListener('close', onClose);
        $('#btn-confirm-ok').removeEventListener('click', onConfirm);
        resolve(confirmed);
      };
      $('#btn-confirm-ok').addEventListener('click', onConfirm);
      dialog.addEventListener('close', onClose);
    });
  }

  //= ========================================================================
  // Application state
  //= ========================================================================
  const state = {
    instances: [],
    interfaces: [],
    schema: null,
    currentInstance: null,
    instanceCommands: {}, // name -> normalized command list
    // Drives the whole instance detail view: header badge on every tab, plus
    // the Overview sections while that tab is visible.
    overviewTimer: null,
    stream: { instance: null, total: 0, rowHeight: 34, buffer: 8, pending: false, timer: null, pollTimer: null, filters: {}, detailFlowId: null, detailTimer: null },
    session: { instance: null, total: 0, rowHeight: 34, buffer: 8, pending: false, timer: null, pollTimer: null, filters: {}, detailSessionId: null, detailTimer: null },
    // selected holds the keys (bgpSessionKey) of the sessions ticked for
    // "Send update", so the selection survives the poll re-renders; items
    // is the last bgp-sessions result. update is the open send dialog: its
    // target sessions and the ones skipped by the last send.
    bgp: { pollTimer: null, detailKey: null, detailTimer: null, selected: new Set(), items: [], update: null },
    // pending holds the member interfaces with an enable/disable request in
    // flight, so their button stays disabled across poll re-renders.
    lag: { pollTimer: null, pending: new Set() },
    // ISIS and OSPF tabs: the instance the selects were filled for, and the
    // last database read, kept for re-filtering without a new request.
    isis: { pollTimer: null, dbTimer: null, instance: null, config: null, database: [], dbOffset: 0 },
    ospf: { pollTimer: null, dbTimer: null, instance: null, config: null, database: [], dbOffset: 0 },
    // The entry shown in the ISIS/OSPF detail dialog, see showRoutingDetail.
    routingDetail: null,
    // Generate tab: the form and the history belong to one instance (its
    // config prefills the form) and are reset for the next one.
    generate: {
      instance: null, tools: null, tool: null, fields: [], files: [], config: null,
      running: false, busy: false, history: [],
    },
    // generations maps instance -> the identity of the run.log it last read,
    // so a restarted instance (which recreates the file) is detected.
    log: { instance: null, offsets: {}, generations: {}, paused: false, mode: 'collapsed', manualSelect: false, timer: null },
    downloads: { instance: null },
    uploads: { instance: null },
  };

  //= ========================================================================
  // Dashboard
  //= ========================================================================
  async function refreshVersion() {
    try {
      const v = await API.version();
      $('#version-badge').textContent = 'controller ' + v['bngblasterctrl-version'] + ' · bngblaster ' + v['bngblaster-version'];
    } catch (e) { /* non-fatal */ }
  }

  async function loadInstances() {
    let instances = [];
    try {
      instances = await API.instances(true) || [];
    } catch (e) {
      announce('Failed to load instances: ' + e.message, 'error');
      return;
    }
    state.instances = instances.map((i) => ({ name: i.name, status: i.status }));
    renderInstancesTable();
    populateInstanceSelects();
  }

  // Instance list filter: status (all/running/stopped) and a name search.
  // Applied on every render, so it survives the periodic refresh, and kept
  // per browser (best effort, the list works without it).
  const INSTANCE_FILTER_KEY = 'bngblaster.instanceFilter';
  const instanceFilter = { status: 'all', text: '' };
  try {
    const saved = JSON.parse(localStorage.getItem(INSTANCE_FILTER_KEY) || 'null');
    if (saved && ['all', 'running', 'stopped'].includes(saved.status)) instanceFilter.status = saved.status;
    if (saved && typeof saved.text === 'string') instanceFilter.text = saved.text;
  } catch (e) { /* storage unavailable */ }

  function saveInstanceFilter() {
    try { localStorage.setItem(INSTANCE_FILTER_KEY, JSON.stringify(instanceFilter)); } catch (e) { /* ignore */ }
  }

  function syncInstanceFilterControls() {
    $('#instances-search').value = instanceFilter.text;
    $all('[data-status-filter]').forEach((button) => {
      button.setAttribute('aria-pressed', String(button.dataset.statusFilter === instanceFilter.status));
    });
  }

  function setInstanceFilter(changes) {
    Object.assign(instanceFilter, changes);
    saveInstanceFilter();
    syncInstanceFilterControls();
    renderInstancesTable();
  }

  $all('[data-status-filter]').forEach((button) => {
    button.addEventListener('click', () => setInstanceFilter({ status: button.dataset.statusFilter }));
  });
  $('#instances-search').addEventListener('input', (ev) => {
    instanceFilter.text = ev.target.value;
    saveInstanceFilter();
    renderInstancesTable();
  });
  $('#btn-instances-clear-filter').addEventListener('click', () => {
    setInstanceFilter({ status: 'all', text: '' });
    $('#instances-search').focus();
  });
  syncInstanceFilterControls();

  // Case-insensitive; several words must all appear in the name.
  function instanceMatchesFilter(inst) {
    const running = inst.status === 'started';
    if (instanceFilter.status === 'running' && !running) return false;
    if (instanceFilter.status === 'stopped' && running) return false;
    const name = inst.name.toLowerCase();
    return instanceFilter.text.toLowerCase().split(/\s+/).filter(Boolean).every((word) => name.includes(word));
  }

  function renderInstancesTable() {
    const tbody = $('#instances-tbody');
    tbody.innerHTML = '';
    const total = state.instances.length;
    const runningCount = state.instances.filter((i) => i.status === 'started').length;
    $('#instances-count-all').textContent = String(total);
    $('#instances-count-running').textContent = String(runningCount);
    $('#instances-count-stopped').textContent = String(total - runningCount);
    const visible = state.instances.filter(instanceMatchesFilter);
    const filtered = visible.length !== total;
    $('#instances-empty').hidden = total > 0;
    $('#instances-no-match').hidden = total === 0 || visible.length > 0;
    $('#instances-table').hidden = visible.length === 0;
    const summary = filtered ? 'Showing ' + visible.length + ' of ' + total : '';
    if ($('#instances-filter-summary').textContent !== summary) $('#instances-filter-summary').textContent = summary;
    visible.forEach((inst) => {
      const running = inst.status === 'started';
      const actions = el('td', { class: 'actions-cell' }, [
        el('button', {
          class: 'btn btn-sm', type: 'button', text: 'Open',
          onclick: () => openInstance(inst.name),
        }),
        running
          ? el('button', { class: 'btn btn-sm', type: 'button', text: 'Stop', onclick: () => doStop(inst.name) })
          : el('button', { class: 'btn btn-sm btn-primary', type: 'button', text: 'Start', onclick: () => openStartDialog(inst.name) }),
        running ? el('button', { class: 'btn btn-sm btn-danger', type: 'button', text: 'Kill', onclick: () => doKill(inst.name) }) : null,
        !running ? el('button', { class: 'btn btn-sm', type: 'button', text: 'Edit', onclick: () => openInstanceDialog(inst.name) }) : null,
        // Also while running: a capture can be started and stopped during
        // the test, and its pcap downloaded without stopping the instance.
        el('button', { class: 'btn btn-sm', type: 'button', text: 'Download', onclick: () => showDownloads(inst.name, running) }),
        !running ? el('button', { class: 'btn btn-sm', type: 'button', text: 'Upload', onclick: () => showUploads(inst.name) }) : null,
        !running ? el('button', { class: 'btn btn-sm btn-danger', type: 'button', text: 'Delete', onclick: () => doDelete(inst.name) }) : null,
      ]);
      const row = el('tr', {}, [
        el('th', { scope: 'row', text: inst.name }),
        el('td', {}, [el('span', { class: 'status-pill ' + (running ? 'started' : 'stopped'), text: running ? 'started' : 'stopped' })]),
        actions,
      ]);
      tbody.appendChild(row);
    });
  }

  function formatFileSize(bytes) {
    if (!Number.isFinite(bytes)) return '';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let size = bytes;
    let unit = 0;
    while (size >= 1024 && unit < units.length - 1) { size /= 1024; unit++; }
    return (unit === 0 ? String(size) : size.toFixed(size < 10 ? 2 : 1)) + ' ' + units[unit];
  }

  // Downloads dialog: lists the files present in an instance's result
  // folder (name, size, download button), mirroring the stream/session
  // detail dialogs instead of opening a separate browser tab/window.
  async function loadDownloads(showLoading) {
    const name = state.downloads.instance;
    if (!name) return;
    const empty = $('#downloads-empty');
    const error = $('#downloads-error');
    const table = $('#downloads-table');
    if (showLoading) {
      empty.hidden = true;
      error.hidden = true;
      table.hidden = true;
    }
    try {
      const files = await API.files(name);
      error.hidden = true;
      if (!files || files.length === 0) {
        table.hidden = true;
        empty.hidden = false;
        return;
      }
      empty.hidden = true;
      const tbody = $('#downloads-tbody');
      tbody.innerHTML = '';
      files.forEach((f) => {
        tbody.appendChild(el('tr', {}, [
          el('td', { text: f.name }),
          el('td', { text: formatFileSize(f.size) }),
          el('td', {}, [el('a', {
            class: 'btn btn-sm', href: API.fileDownloadURL(name, f.name), download: f.name, text: 'Download',
          })]),
        ]));
      });
      table.hidden = false;
    } catch (e) {
      table.hidden = true;
      empty.hidden = true;
      error.hidden = false;
      error.textContent = 'Failed to load files: ' + e.message;
    }
  }

  function showDownloads(name, running) {
    state.downloads.instance = name;
    $('#dialog-downloads-title').textContent = 'Downloads — ' + name;
    $('#downloads-running').hidden = !running;
    openDialog('dialog-downloads');
    loadDownloads(true);
  }

  function showUploads(name) {
    state.uploads.instance = name;
    $('#dialog-uploads-title').textContent = 'Uploads — ' + name;
    $('#upload-list').innerHTML = '';
    openDialog('dialog-uploads');
  }

  $('#btn-downloads-refresh').addEventListener('click', () => loadDownloads(true));

  function populateInstanceSelects() {
    const selects = [$('#logdock-instance-select')];
    selects.forEach((sel) => {
      const previous = sel.value;
      sel.innerHTML = '';
      if (state.instances.length === 0) {
        sel.appendChild(el('option', { value: '', text: 'No instances available' }));
        sel.disabled = true;
        return;
      }
      sel.disabled = false;
      state.instances.forEach((inst) => sel.appendChild(el('option', { value: inst.name, text: inst.name })));
      if (state.instances.some((i) => i.name === previous)) sel.value = previous;
    });
    if (!state.log.manualSelect && state.currentInstance) {
      $('#logdock-instance-select').value = state.currentInstance;
    }
    onLogInstanceChange();
  }

  async function doStop(name) {
    await withFeedback(() => API.stop(name), 'Stop signal sent to ' + name, 'Failed to stop ' + name);
    loadInstances();
  }
  async function doKill(name) {
    const ok = await confirmAction('Kill instance "' + name + '"? This sends SIGKILL immediately.');
    if (!ok) return;
    await withFeedback(() => API.kill(name), 'Kill signal sent to ' + name, 'Failed to kill ' + name);
    loadInstances();
  }
  async function doDelete(name) {
    const ok = await confirmAction('Delete instance "' + name + '" and all of its files? This cannot be undone.');
    if (!ok) return;
    const deleted = await withFeedback(
      () => API.delete(name).then(() => true), 'Deleted ' + name, 'Failed to delete ' + name);
    if (deleted && renderedInstance === name) renderedInstance = null;
    if (deleted && state.currentInstance === name) showDashboard();
    loadInstances();
  }

  //= ========================================================================
  // Start-instance dialog (RunningConfig)
  //= ========================================================================
  // Enum options mirrored from the "_start" endpoint's request body schema
  // (docs/swagger.yaml): report_flags, logging_flags and metric_flags.
  const REPORT_FLAGS = ['sessions', 'streams'];
  const LOGGING_FLAGS = [
    'debug', 'error', 'igmp', 'io', 'pppoe', 'info', 'pcap', 'ip', 'loss',
    'l2tp', 'dhcp', 'isis', 'ospf', 'ldp', 'bgp', 'tcp', 'lag', 'dpdk',
    'af_xdp', 'packet', 'http',
  ];
  const METRIC_FLAGS = ['session_counters', 'interfaces', 'access_interfaces', 'network_interfaces', 'a10nsp_interfaces', 'streams'];

  // Fills a .flags-grid container with one checkbox per flag. Guarded by
  // childElementCount since the dialog markup and flag lists are static -
  // this only ever needs to run once, not on every dialog open.
  function renderFlagCheckboxes(containerId, idPrefix, flags, defaultChecked) {
    const container = $('#' + containerId);
    if (!container || container.childElementCount) return;
    flags.forEach((flag) => {
      const id = idPrefix + '-' + flag;
      const cb = el('input', { type: 'checkbox', id, value: flag });
      cb.checked = defaultChecked.includes(flag);
      container.appendChild(el('div', { class: 'checkbox-field' }, [cb, el('label', { for: id, text: flag })]));
    });
  }

  function collectCheckedFlags(containerId) {
    return Array.from($('#' + containerId).querySelectorAll('input[type="checkbox"]'))
      .filter((cb) => cb.checked)
      .map((cb) => cb.value);
  }

  // Hides a flag sub-list while its enabling checkbox is unticked, so the
  // options read as belonging to it rather than always-on settings.
  function bindFlagsVisibility(toggleId, containerId) {
    const toggle = $('#' + toggleId);
    const container = $('#' + containerId);
    const update = () => { container.hidden = !toggle.checked; };
    toggle.addEventListener('change', update);
    update();
  }

  renderFlagCheckboxes('start-report-flags', 'start-report-flag', REPORT_FLAGS, []);
  renderFlagCheckboxes('start-logging-flags', 'start-logging-flag', LOGGING_FLAGS, []);
  renderFlagCheckboxes('start-metric-flags', 'start-metric-flag', METRIC_FLAGS, []);
  bindFlagsVisibility('start-opt-report', 'start-report-flags');
  bindFlagsVisibility('start-opt-logging', 'start-logging-flags');

  let startDialogTarget = null;
  let startDialogThen = null;
  function openStartDialog(name, andThen) {
    startDialogTarget = name;
    startDialogThen = andThen || null;
    setStatusMessage($('#start-instance-status'), '');
    $('#dialog-start-instance-title').textContent = 'Start Instance — ' + name;
    openDialog('dialog-start-instance');
  }
  function collectRunningConfig() {
    const sessionCount = parseInt($('#start-opt-session-count').value, 10) || 0;
    return {
      report: $('#start-opt-report').checked,
      report_flags: $('#start-opt-report').checked ? collectCheckedFlags('start-report-flags') : [],
      logging: $('#start-opt-logging').checked,
      logging_flags: $('#start-opt-logging').checked ? collectCheckedFlags('start-logging-flags') : [],
      pcap_capture: $('#start-opt-pcap').checked,
      session_count: sessionCount,
      stream_config: $('#start-opt-stream-config').value.trim(),
      metric_flags: collectCheckedFlags('start-metric-flags'),
    };
  }
  // _start answers only once the instance has come up or failed, which can
  // take minutes (large configurations, or waiting for the startup of other
  // instances). Meanwhile the dialog stays open, shows what the instance is
  // doing (test-info status and task, reported by newer bngblaster versions)
  // and offers to abort the startup.
  const startProgress = { name: null, timer: null, aborted: false };

  function setStartDialogBusy(busy) {
    $('#btn-start-instance-confirm').disabled = busy;
    $('#btn-start-instance-close').disabled = busy;
    $('#btn-start-instance-cancel').hidden = busy;
    $('#btn-start-instance-abort').hidden = !busy;
    $('#btn-start-instance-abort').disabled = false;
    $all('#dialog-start-instance .dialog-body input').forEach((input) => { input.disabled = busy; });
    const progress = $('#start-instance-progress');
    progress.hidden = !busy;
    if (busy) progress.textContent = 'Starting…';
  }

  async function pollStartProgress() {
    const name = startProgress.name;
    let info = null;
    try {
      const resp = await API.command(name, 'test-info');
      info = resp && resp['test-info'];
    } catch (e) {
      // No control socket yet (or an older bngblaster that opens it only
      // once up): keep showing the generic text.
    }
    if (startProgress.name !== name || !info) return;
    let text = info.status === 'stopping' ? 'Stopping' : 'Starting';
    if (info.task) text += ' · ' + prettyLabel(info.task);
    if (info['startup-lock-pid']) {
      text += ' (waiting for the startup of another instance, PID ' + info['startup-lock-pid'] + ')';
    }
    const progress = $('#start-instance-progress');
    // Only touch the live region when the text changes, so screen readers
    // announce each task once rather than on every poll.
    if (progress.textContent !== text + '…') progress.textContent = text + '…';
  }

  // Closing the dialog while starting would leave no way to abort.
  $('#dialog-start-instance').addEventListener('cancel', (ev) => {
    if (startProgress.name) ev.preventDefault();
  });

  $('#btn-start-instance-confirm').addEventListener('click', async () => {
    const name = startDialogTarget;
    const cfg = collectRunningConfig();
    setStatusMessage($('#start-instance-status'), '');
    startProgress.name = name;
    startProgress.aborted = false;
    setStartDialogBusy(true);
    startProgress.timer = schedulePoll(pollStartProgress, 500);
    try {
      await API.start(name, cfg);
      announce('Started ' + name, 'success');
      closeDialog('dialog-start-instance');
      if (startDialogThen) startDialogThen();
    } catch (e) {
      if (startProgress.aborted || e.status === 409) {
        announce('Start of ' + name + ' aborted', 'info');
        closeDialog('dialog-start-instance');
      } else {
        showDetailedError($('#start-instance-status'), 'Could not start instance', e.message);
      }
    } finally {
      startProgress.timer = cancelPoll(startProgress.timer);
      startProgress.name = null;
      setStartDialogBusy(false);
      loadInstances();
      if (state.currentInstance === name) refreshInstanceStatus();
    }
  });

  // Abort asks the instance to tear down via test-stop, which restores the
  // interface configuration. Without a working control socket (not open
  // yet, or a bngblaster version without startup reporting) the instance
  // is killed instead. Either way the pending _start request then returns.
  $('#btn-start-instance-abort').addEventListener('click', async () => {
    const name = startProgress.name;
    if (!name) return;
    startProgress.aborted = true;
    $('#btn-start-instance-abort').disabled = true;
    $('#start-instance-progress').textContent = 'Aborting…';
    try {
      await API.command(name, 'test-stop');
    } catch (e) {
      try {
        await API.kill(name);
      } catch (killErr) {
        showDetailedError($('#start-instance-status'), 'Could not abort start', killErr.message);
        $('#btn-start-instance-abort').disabled = false;
      }
    }
  });

  //= ========================================================================
  // JSON Schema driven "New Instance" form
  //= ========================================================================
  function resolveSchema(node, root) {
    let n = node;
    let guard = 0;
    while (n && n.$ref && guard++ < 20) {
      n = pointerGet(root, n.$ref);
    }
    if (n && Array.isArray(n.allOf)) {
      const merged = Object.assign({}, n);
      delete merged.allOf;
      n.allOf.forEach((sub) => {
        const resolved = resolveSchema(sub, root);
        merged.properties = Object.assign({}, merged.properties, resolved.properties);
        merged.required = (merged.required || []).concat(resolved.required || []);
        if (!merged.type) merged.type = resolved.type;
      });
      n = merged;
    }
    return n || {};
  }

  // Detects the "single item OR array of that item" oneOf pattern used
  // throughout the bngblaster schema (network/access/a10nsp/links/lag,
  // routing protocol blocks, http/icmp/arp clients, ...). Returns the raw
  // (unresolved) item schema for the array alternative, or null.
  function oneOfArrayItems(rawSchema, root) {
    if (!rawSchema || !Array.isArray(rawSchema.oneOf) || rawSchema.oneOf.length !== 2) return null;
    const arrayAlt = rawSchema.oneOf.find((alt) => resolveSchema(alt, root).type === 'array');
    if (!arrayAlt) return null;
    const resolvedArrayAlt = resolveSchema(arrayAlt, root);
    return arrayAlt.items || resolvedArrayAlt.items || null;
  }

  function pointerGet(root, ref) {
    if (!ref.startsWith('#/')) return {};
    const parts = ref.slice(2).split('/').map((p) => p.replace(/~1/g, '/').replace(/~0/g, '~'));
    let node = root;
    for (const p of parts) {
      if (node == null) return {};
      node = node[p];
    }
    return node || {};
  }

  function isInterfaceField(key) {
    // "lag-interface" (on links) names a LAG group defined elsewhere in this
    // same config, not a host NIC, so it must never pull from the
    // host-interfaces dropdown.
    if (key === 'lag-interface') return false;
    return /(^|[-_])interface(name)?$/i.test(key);
  }

  // A lagInterface object's own "interface" property is the name of the
  // virtual LAG interface being created (e.g. "lag0") - it is picked by the
  // user, not selected from the host's real network interfaces, so it must
  // always render as a plain text field rather than the interfaces dropdown.
  function isLagInterfaceDefSchema(schema) {
    return !!(schema && schema.properties && schema.properties['lacp-min-active-links']);
  }

  // Protocol/unit acronyms used throughout the bngblaster schema that should
  // not be rendered with naive title-casing (e.g. "Pppoe", "Ipoe").
  const ACRONYMS = {
    a10nsp: 'A10NSP', arp: 'ARP', as: 'AS', bgp: 'BGP', cfm: 'CFM', csnp: 'CSNP',
    dhcp: 'DHCP', dhcpv6: 'DHCPv6', df: 'DF', dns1: 'DNS1', dns2: 'DNS2', dsl: 'DSL',
    http: 'HTTP', https: 'HTTPS', ia: 'IA', icmp: 'ICMP', id: 'ID', igmp: 'IGMP',
    io: 'IO', ip: 'IP', ip6cp: 'IP6CP', ipcp: 'IPCP', ipoe: 'IPoE', ipv4: 'IPv4',
    ipv6: 'IPv6', ipv6pd: 'IPv6PD', isis: 'ISIS', l1: 'L1', l2: 'L2', l2tp: 'L2TP',
    lacp: 'LACP', lag: 'LAG', lcp: 'LCP', ldp: 'LDP', ldra: 'LDRA', lsa: 'LSA',
    lsp: 'LSP', lsr: 'LSR', mac: 'MAC', mrt: 'MRT', mru: 'MRU', mtu: 'MTU',
    nat: 'NAT', ont: 'ONT', onu: 'ONU', ospf: 'OSPF', ospfv2: 'OSPFv2', ospfv3: 'OSPFv3',
    p2p: 'P2P', pon: 'PON', ppp: 'PPP', pppoe: 'PPPoE', pps: 'pps', psnp: 'PSNP',
    qinq: 'QinQ', rx: 'RX', sid: 'SID', sr: 'SR', tcp: 'TCP', tos: 'ToS', ttl: 'TTL',
    tun: 'TUN', tx: 'TX', udp: 'UDP', url: 'URL', vlan: 'VLAN',
  };

  function prettyWord(word) {
    if (!word) return word;
    const canonical = ACRONYMS[word.toLowerCase()];
    if (canonical) return canonical;
    return word.charAt(0).toUpperCase() + word.slice(1).toLowerCase();
  }

  // Renders a schema key or raw identifier as a human label, applying the
  // known protocol/unit acronyms (PPPoE, IPoE, VLAN, ...) instead of naive
  // per-word title-casing.
  function prettyLabel(text) {
    return String(text).split(/[\s_-]+/).filter(Boolean).map(prettyWord).join(' ');
  }

  function labelFor(key, schemaNode) {
    return (schemaNode && schemaNode.title) || prettyLabel(key);
  }

  // Builds a form field for a schema node. Returns { el, getValue } where
  // getValue() returns undefined when the field should be omitted from the
  // submitted document (untouched optional section, empty optional array...).
  function buildField(key, rawSchema, root, required, opts) {
    const arrayItems = oneOfArrayItems(rawSchema, root);
    if (arrayItems) {
      const arraySchema = { type: 'array', items: arrayItems, description: rawSchema.description };
      const field = buildArrayField(key, arraySchema, root, required, nextId('f'), labelFor(key, rawSchema));
      return {
        el: field.el,
        getValue: field.getValue,
        // The real config (this is the "single item OR array of that item"
        // oneOf pattern - network/access/a10nsp/links/lag, ...) may store a
        // single object rather than an array, since the schema allows
        // either. buildArrayField.setValue only understands arrays, so a
        // bare object here must be normalized to a one-item array first -
        // otherwise loading an existing single-interface config for editing
        // would silently discard it, and saving would then write it out
        // with that section missing entirely.
        setValue: (v) => field.setValue(v === undefined || v === null || Array.isArray(v) ? v : [v]),
      };
    }

    const schema = resolveSchema(rawSchema, root);
    const type = schema.type || (schema.enum ? 'string' : 'object');
    const id = nextId('f');
    const label = labelFor(key, schema);

    if (type === 'object' && schema.properties) {
      return buildObjectField(key, schema, root, required, id, label);
    }
    if (type === 'array') {
      return buildArrayField(key, schema, root, required, id, label);
    }
    if (schema.enum) {
      return buildEnumField(key, schema, required, id, label);
    }
    if (type === 'boolean') {
      return buildBooleanField(key, schema, required, id, label);
    }
    if (type === 'integer' || type === 'number') {
      return buildNumberField(key, schema, required, id, label, type === 'integer');
    }
    if (isInterfaceField(key) && !(opts && opts.plainInterface)) {
      return buildInterfaceField(key, schema, required, id, label);
    }
    return buildStringField(key, schema, required, id, label);
  }

  function fieldWrap(id, label, required, hint, control) {
    const wrap = el('div', { class: 'field' });
    wrap.appendChild(el('label', { for: id }, [
      document.createTextNode(label),
      required ? el('span', { class: 'required-mark', 'aria-hidden': 'true', text: '*' }) : null,
    ]));
    wrap.appendChild(control);
    if (hint) wrap.appendChild(el('span', { class: 'hint', text: hint }));
    return wrap;
  }

  // Leaf fields never bake a schema "default" into the generated config
  // unless the field is required: bngblaster already applies its own
  // defaults for any key that is simply absent, so silently emitting e.g.
  // "cfm-cc": false for a field nobody touched only adds noise and risks
  // diverging from the real default later. The default is still shown (as
  // placeholder text, or a "Default: ..." hint where a placeholder isn't
  // possible) purely for reference. Required fields keep the old
  // pre-filled behavior since the config needs that key present regardless.

  function buildStringField(key, schema, required, id, label) {
    const input = el('input', { type: 'text', id: id, required: required || null });
    if (schema.default !== undefined) {
      if (required) input.value = schema.default;
      else input.setAttribute('placeholder', String(schema.default));
    }
    if (schema.pattern) input.setAttribute('pattern', schema.pattern);
    const wrap = fieldWrap(id, label, required, schema.description, input);
    return {
      el: wrap,
      getValue: () => (input.value.trim() === '' ? undefined : input.value),
      setValue: (v) => { input.value = (v === undefined || v === null) ? '' : v; },
    };
  }

  function buildNumberField(key, schema, required, id, label, isInt) {
    const input = el('input', { type: 'number', id: id, required: required || null });
    if (schema.minimum !== undefined) input.setAttribute('min', schema.minimum);
    if (schema.maximum !== undefined) input.setAttribute('max', schema.maximum);
    if (isInt) input.setAttribute('step', '1');
    if (schema.default !== undefined) {
      if (required) input.value = schema.default;
      else input.setAttribute('placeholder', String(schema.default));
    }
    const wrap = fieldWrap(id, label, required, schema.description, input);
    return {
      el: wrap,
      getValue: () => {
        if (input.value.trim() === '') return undefined;
        const n = Number(input.value);
        return Number.isNaN(n) ? undefined : n;
      },
      setValue: (v) => { input.value = (v === undefined || v === null) ? '' : v; },
    };
  }

  function buildBooleanField(key, schema, required, id, label) {
    const input = el('input', { type: 'checkbox', id: id });
    const hasDefault = schema.default !== undefined;
    // Shown for reference either way, but only *counts* as an explicit
    // value once required, or once the user actually toggles it.
    input.checked = hasDefault && schema.default === true;
    let touched = !!required;
    input.addEventListener('change', () => { touched = true; });
    const wrap = el('div', { class: 'field checkbox-field' }, [
      input,
      el('label', { for: id, text: label }),
    ]);
    if (!required && hasDefault) wrap.appendChild(el('span', { class: 'hint', text: 'Default: ' + schema.default }));
    if (schema.description) wrap.appendChild(el('span', { class: 'hint', text: schema.description }));
    return {
      el: wrap,
      getValue: () => (touched ? input.checked : undefined),
      setValue: (v) => {
        const has = v !== undefined && v !== null;
        touched = has || !!required;
        input.checked = has ? !!v : (hasDefault && schema.default === true);
      },
    };
  }

  function buildEnumField(key, schema, required, id, label) {
    const select = el('select', { id: id, required: required || null });
    if (!required) select.appendChild(el('option', { value: '', text: '(not set)' }));
    schema.enum.forEach((v) => select.appendChild(el('option', { value: v, text: String(v) })));
    const hasDefault = schema.default !== undefined;
    if (hasDefault) select.value = schema.default;
    // Same reference-only treatment as buildBooleanField: a pre-selected
    // default shows what will apply, but doesn't count until touched.
    let touched = !!required;
    select.addEventListener('change', () => { touched = true; });
    const hint = (!required && hasDefault)
      ? ('Default: ' + schema.default + (schema.description ? ' — ' + schema.description : ''))
      : schema.description;
    const wrap = fieldWrap(id, label, required, hint, select);
    return {
      el: wrap,
      getValue: () => (touched && select.value !== '' ? select.value : undefined),
      setValue: (v) => {
        const has = v !== undefined && v !== null && schema.enum.some((e) => String(e) === String(v));
        touched = has || !!required;
        if (has) select.value = String(v);
        else select.value = hasDefault ? String(schema.default) : '';
      },
    };
  }

  function buildInterfaceField(key, schema, required, id, label) {
    const select = el('select', { id: id });
    if (!required) select.appendChild(el('option', { value: '', text: '(not set)' }));
    state.interfaces.forEach((iface) => select.appendChild(el('option', { value: iface.name, text: iface.name + (iface.mtu ? ' (mtu ' + iface.mtu + ')' : '') })));
    select.appendChild(el('option', { value: '__other__', text: 'Other (type manually)…' }));
    const manual = el('input', { type: 'text', id: id + '-manual', class: 'visually-hidden', 'aria-label': label + ' (manual value)' });
    select.addEventListener('change', () => {
      const isOther = select.value === '__other__';
      manual.classList.toggle('visually-hidden', !isOther);
      if (isOther) manual.focus();
    });
    const container = el('div', {}, [select, manual]);
    const wrap = fieldWrap(id, label, required, schema.description || 'Populated from the host network interfaces detected by the controller.', container);
    return {
      el: wrap,
      getValue: () => {
        if (select.value === '') return undefined;
        if (select.value === '__other__') return manual.value.trim() === '' ? undefined : manual.value.trim();
        return select.value;
      },
      setValue: (v) => {
        if (v === undefined || v === null || v === '') {
          select.value = ''; manual.value = ''; manual.classList.add('visually-hidden');
          return;
        }
        const hasOption = Array.from(select.options).some((o) => o.value === v);
        if (hasOption) {
          select.value = v;
          manual.value = ''; manual.classList.add('visually-hidden');
        } else {
          select.value = '__other__';
          manual.value = v; manual.classList.remove('visually-hidden');
        }
      },
    };
  }

  function buildObjectField(key, schema, root, required, id, label) {
    const requiredChildren = schema.required || [];
    const isLagInterfaceDef = isLagInterfaceDefSchema(schema);
    const body = el('div', { class: 'form-grid' });
    const children = Object.entries(schema.properties).map(([childKey, childSchema]) => {
      const opts = (childKey === 'interface' && isLagInterfaceDef) ? { plainInterface: true } : undefined;
      const field = buildField(childKey, childSchema, root, requiredChildren.includes(childKey), opts);
      body.appendChild(field.el);
      return [childKey, field];
    });
    const details = el('details', { open: required ? '' : null });
    details.appendChild(el('summary', {}, [document.createTextNode(label + (required ? ' *' : ''))]));
    if (schema.description) details.appendChild(el('p', { class: 'hint', text: schema.description }));
    details.appendChild(body);
    return {
      el: details,
      getValue: () => {
        const obj = {};
        let any = false;
        children.forEach(([childKey, field]) => {
          const v = field.getValue();
          if (v !== undefined) { obj[childKey] = v; any = true; }
        });
        if (!any && !required) return undefined;
        return obj;
      },
      setValue: (v) => {
        const has = v !== null && typeof v === 'object';
        children.forEach(([childKey, field]) => { if (field.setValue) field.setValue(has ? v[childKey] : undefined); });
        if (has) details.open = true;
      },
    };
  }

  function buildArrayField(key, schema, root, required, id, label) {
    const itemSchema = resolveSchema(schema.items || {}, root);
    const fieldset = el('fieldset', {});
    fieldset.appendChild(el('legend', { text: label + (required ? ' *' : '') }));
    if (schema.description) fieldset.appendChild(el('p', { class: 'hint', text: schema.description }));

    // Enumerated string arrays are rendered as a checkbox group (e.g. flags).
    if (itemSchema.type === 'string' && Array.isArray(itemSchema.enum)) {
      const boxes = itemSchema.enum.map((v) => {
        const cbId = nextId('f');
        const cb = el('input', { type: 'checkbox', id: cbId, value: v });
        fieldset.appendChild(el('div', { class: 'checkbox-field' }, [cb, el('label', { for: cbId, text: v })]));
        return cb;
      });
      return {
        el: fieldset,
        getValue: () => {
          const values = boxes.filter((b) => b.checked).map((b) => b.value);
          return values.length ? values : (required ? [] : undefined);
        },
        setValue: (arr) => {
          const values = (Array.isArray(arr) ? arr : []).map(String);
          boxes.forEach((b) => { b.checked = values.includes(b.value); });
        },
      };
    }

    const itemsContainer = el('div', {});
    fieldset.appendChild(itemsContainer);
    const items = [];

    function addItem(initialValue) {
      const field = buildField(key + ' item', schema.items || {}, root, false);
      if (initialValue !== undefined && field.setValue) field.setValue(initialValue);
      const row = el('div', { class: 'array-item' }, [
        field.el,
        el('button', {
          type: 'button', class: 'btn btn-sm', 'aria-label': 'Remove ' + label + ' item',
          text: 'Remove',
          onclick: () => { itemsContainer.removeChild(row); const i = items.indexOf(field); if (i >= 0) items.splice(i, 1); },
        }),
      ]);
      itemsContainer.appendChild(row);
      items.push(field);
    }

    fieldset.appendChild(el('button', {
      type: 'button', class: 'btn btn-sm', text: 'Add ' + label + ' item',
      onclick: () => addItem(),
    }));
    if ((schema.minItems || 0) > 0) {
      for (let i = 0; i < schema.minItems; i++) addItem();
    }

    return {
      el: fieldset,
      getValue: () => {
        const values = items.map((f) => f.getValue()).filter((v) => v !== undefined);
        return values.length ? values : (required ? [] : undefined);
      },
      setValue: (arr) => {
        itemsContainer.innerHTML = '';
        items.length = 0;
        (Array.isArray(arr) ? arr : []).forEach((v) => addItem(v));
      },
    };
  }

  // newInstanceFields holds the [key, field] pairs of the schema-driven form
  // when the schema loaded successfully; null when only JSON editing is
  // available (schema missing / failed to load / has no top-level
  // properties).
  let newInstanceFields = null;
  let configMode = 'form'; // 'form' | 'json'

  function debounce(fn, ms) {
    let t;
    return (...args) => { clearTimeout(t); t = setTimeout(() => fn(...args), ms); };
  }

  function collectFormJSON() {
    const obj = {};
    (newInstanceFields || []).forEach(([k, f]) => { const v = f.getValue(); if (v !== undefined) obj[k] = v; });
    return obj;
  }

  function applyJSONToForm(obj) {
    (newInstanceFields || []).forEach(([k, f]) => { if (f.setValue) f.setValue(obj ? obj[k] : undefined); });
  }

  // Switches the New Instance dialog between the schema-driven form and raw
  // JSON editing, synchronizing the two representations at the switch point
  // (rather than continuously, which would be surprising while typing).
  function switchConfigMode(mode) {
    const jsonStatus = $('#schema-json-status');
    if (mode === configMode) return;
    if (mode === 'json') {
      $('#schema-json-textarea').value = JSON.stringify(collectFormJSON(), null, 2);
      placeJSONEditorCaretInsideRoot($('#schema-json-textarea'));
      refreshJSONEditor();
      setStatusMessage(jsonStatus, '');
    } else {
      let parsed;
      try {
        const text = $('#schema-json-textarea').value.trim();
        parsed = text ? JSON.parse(text) : {};
      } catch (e) {
        setStatusMessage(jsonStatus, 'Cannot switch to the form view: invalid JSON (' + e.message + ').', 'error');
        return;
      }
      applyJSONToForm(parsed);
      setStatusMessage(jsonStatus, '');
    }
    configMode = mode;
    $('#config-mode-btn-form').setAttribute('aria-pressed', String(mode === 'form'));
    $('#config-mode-btn-json').setAttribute('aria-pressed', String(mode === 'json'));
    $('#schema-form-root').hidden = mode !== 'form';
    $('#schema-json-root').hidden = mode !== 'json';
    const nav = $('#schema-form-nav');
    if (nav) nav.hidden = mode !== 'form' || !nav.childElementCount;
    if (mode === 'json') $('#schema-json-textarea').focus();
  }
  $('#config-mode-btn-form').addEventListener('click', () => switchConfigMode('form'));
  $('#config-mode-btn-json').addEventListener('click', () => switchConfigMode('json'));

  // Hides the Form/JSON toggle and keeps only the JSON editor visible, used
  // when there is no usable schema to drive a form from.
  function forceJSONOnlyMode() {
    newInstanceFields = null;
    configMode = 'json';
    $('#config-mode-toggle').hidden = true;
    $('#schema-json-root').hidden = false;
    const nav = $('#schema-form-nav');
    if (nav) { nav.hidden = true; nav.innerHTML = ''; }
  }

  //= ========================================================================
  // JSON text editor: schema-aware syntax highlighting, live validation,
  // property/enum autocomplete and a cursor-position schema info panel for
  // the "Edit as JSON" view. Works even without state.schema (highlighting
  // and JSON syntax errors only); schema-driven features (validation beyond
  // syntax, autocomplete, info panel) activate once state.schema is set.
  //= ========================================================================
  function jsonTokenize(text) {
    const tokens = [];
    let i = 0;
    const n = text.length;
    const isDigitStart = (c) => c === '-' || (c >= '0' && c <= '9');
    while (i < n) {
      const c = text[i];
      if (c === ' ' || c === '\t' || c === '\n' || c === '\r') { i++; continue; }
      if (c === '{' || c === '}' || c === '[' || c === ']' || c === ':' || c === ',') {
        tokens.push({ type: 'punct', value: c, start: i, end: i + 1 });
        i++;
        continue;
      }
      if (c === '"') {
        const start = i;
        i++;
        let terminated = false;
        while (i < n) {
          if (text[i] === '\\') { i += 2; continue; }
          if (text[i] === '"') { i++; terminated = true; break; }
          if (text[i] === '\n') break;
          i++;
        }
        tokens.push({ type: 'string', start, end: i, terminated });
        continue;
      }
      if (isDigitStart(c)) {
        const start = i;
        i++;
        while (i < n && /[0-9.eE+-]/.test(text[i])) i++;
        tokens.push({ type: 'number', start, end: i });
        continue;
      }
      if (/[a-zA-Z_]/.test(c)) {
        const start = i;
        i++;
        while (i < n && /[a-zA-Z_0-9]/.test(text[i])) i++;
        const word = text.slice(start, i);
        const type = (word === 'true' || word === 'false') ? 'boolean' : (word === 'null' ? 'null' : 'ident');
        tokens.push({ type, value: word, start, end: i });
        continue;
      }
      tokens.push({ type: 'invalid', value: c, start: i, end: i + 1 });
      i++;
    }
    return tokens;
  }

  function decodeJSONStringToken(text, tok) {
    let raw = text.slice(tok.start, tok.end);
    if (!tok.terminated) raw += '"';
    try {
      return JSON.parse(raw);
    } catch (e) {
      return raw.slice(1, -1);
    }
  }

  // Lenient recursive-descent JSON parser with error recovery: instead of
  // throwing on the first problem (like JSON.parse), it records an error and
  // keeps going, so the rest of an in-progress edit still gets highlighted
  // and validated. Produces an AST annotated with source offsets, and tags
  // object-key string tokens with role:'key' (used to tell keys from string
  // values apart when rendering syntax highlighting).
  function jsonParseLenient(text) {
    const tokens = jsonTokenize(text);
    let pos = 0;
    const errors = [];
    const peek = () => tokens[pos];
    const next = () => tokens[pos++];
    const err = (tok, msg) => errors.push({ start: tok ? tok.start : text.length, end: tok ? tok.end : text.length, message: msg });

    function parseValue() {
      const tok = peek();
      if (!tok) { err(null, 'Unexpected end of input.'); return null; }
      if (tok.type === 'punct' && tok.value === '{') return parseObject();
      if (tok.type === 'punct' && tok.value === '[') return parseArray();
      if (tok.type === 'string') { next(); return { type: 'string', start: tok.start, end: tok.end, raw: tok }; }
      if (tok.type === 'number') { next(); return { type: 'number', start: tok.start, end: tok.end, value: Number(text.slice(tok.start, tok.end)) }; }
      if (tok.type === 'boolean') { next(); return { type: 'boolean', start: tok.start, end: tok.end, value: tok.value === 'true' }; }
      if (tok.type === 'null') { next(); return { type: 'null', start: tok.start, end: tok.end }; }
      err(tok, 'Unexpected token "' + (tok.value || text.slice(tok.start, tok.end)) + '".');
      next();
      return null;
    }
    function parseObject() {
      const open = next();
      const node = { type: 'object', start: open.start, end: open.end, entries: [] };
      let first = true;
      for (;;) {
        let tok = peek();
        if (!tok) { err(null, 'Unterminated object.'); break; }
        if (tok.type === 'punct' && tok.value === '}') { next(); node.end = tok.end; break; }
        if (!first) {
          if (tok.type === 'punct' && tok.value === ',') {
            next();
            tok = peek();
            if (tok && tok.type === 'punct' && tok.value === '}') err(tok, 'Trailing comma is not allowed.');
          } else {
            err(tok, 'Expected "," or "}".');
          }
        }
        first = false;
        if (!tok) break;
        if (tok.type === 'punct' && tok.value === '}') { next(); node.end = tok.end; break; }
        if (tok.type !== 'string') { err(tok, 'Expected a property name.'); next(); continue; }
        const keyTok = next();
        keyTok.role = 'key';
        const keyName = decodeJSONStringToken(text, keyTok);
        const colon = peek();
        if (colon && colon.type === 'punct' && colon.value === ':') next();
        else err(colon, 'Expected ":".');
        const valueNode = parseValue();
        node.entries.push({ key: keyName, keyStart: keyTok.start, keyEnd: keyTok.end, value: valueNode });
        node.end = valueNode ? valueNode.end : keyTok.end;
      }
      return node;
    }
    function parseArray() {
      const open = next();
      const node = { type: 'array', start: open.start, end: open.end, items: [] };
      let first = true;
      for (;;) {
        let tok = peek();
        if (!tok) { err(null, 'Unterminated array.'); break; }
        if (tok.type === 'punct' && tok.value === ']') { next(); node.end = tok.end; break; }
        if (!first) {
          if (tok.type === 'punct' && tok.value === ',') {
            next();
            tok = peek();
            if (tok && tok.type === 'punct' && tok.value === ']') err(tok, 'Trailing comma is not allowed.');
          } else {
            err(tok, 'Expected "," or "]".');
          }
        }
        first = false;
        if (tok && tok.type === 'punct' && tok.value === ']') { next(); node.end = tok.end; break; }
        const itemNode = parseValue();
        node.items.push(itemNode);
        node.end = itemNode ? itemNode.end : node.end;
      }
      return node;
    }

    let root = null;
    if (tokens.length) {
      root = parseValue();
      if (pos < tokens.length) err(peek(), 'Unexpected trailing content.');
    }
    return { root, errors, tokens };
  }

  function jsonSchemaTypesOf(schema) {
    if (!schema) return null;
    if (schema.type) return Array.isArray(schema.type) ? schema.type : [schema.type];
    return null;
  }

  // Validates a parsed AST node against a (possibly $ref/allOf/oneOf-array)
  // schema, appending {start, end, message} problems to `out`. Deliberately
  // covers the constraint kinds actually used by the bngblaster schema
  // (type, enum, required, additionalProperties, pattern/length, min/max,
  // array size) rather than the full JSON Schema spec.
  function validateJSONAgainstSchema(text, node, rawSchema, root, path, out) {
    if (!node || !rawSchema) return;
    const arrayAlt = oneOfArrayItems(rawSchema, root);
    if (arrayAlt) {
      if (node.type === 'array') { validateJSONAgainstSchema(text, node, { type: 'array', items: arrayAlt }, root, path, out); return; }
      validateJSONAgainstSchema(text, node, arrayAlt, root, path, out);
      return;
    }
    const schema = resolveSchema(rawSchema, root);
    const types = jsonSchemaTypesOf(schema);
    const actual = node.type;
    const label = path || '(root)';
    if (types) {
      const ok = types.includes(actual) || (actual === 'number' && types.includes('integer') && Number.isInteger(node.value));
      if (!ok) {
        const at = (actual === 'object' || actual === 'array') ? [node.start, node.start + 1] : [node.start, node.end];
        out.push({ start: at[0], end: at[1], message: label + ': expected ' + types.join(' or ') + ', got ' + actual + '.' });
        return;
      }
    }
    if (schema.enum) {
      const val = actual === 'string' ? decodeJSONStringToken(text, node.raw) : (actual === 'number' || actual === 'boolean' ? node.value : null);
      if (!schema.enum.some((e) => e === val)) {
        out.push({ start: node.start, end: node.end, message: label + ': must be one of ' + schema.enum.map((e) => JSON.stringify(e)).join(', ') + '.' });
      }
    }
    if (actual === 'string') {
      const val = decodeJSONStringToken(text, node.raw);
      if (schema.pattern) {
        try {
          if (!new RegExp(schema.pattern).test(val)) out.push({ start: node.start, end: node.end, message: label + ': does not match pattern ' + schema.pattern + '.' });
        } catch (e) { /* invalid pattern in the schema itself - nothing to check */ }
      }
      if (schema.minLength !== undefined && val.length < schema.minLength) out.push({ start: node.start, end: node.end, message: label + ': must be at least ' + schema.minLength + ' characters.' });
      if (schema.maxLength !== undefined && val.length > schema.maxLength) out.push({ start: node.start, end: node.end, message: label + ': must be at most ' + schema.maxLength + ' characters.' });
    }
    if (actual === 'number') {
      if (schema.minimum !== undefined && node.value < schema.minimum) out.push({ start: node.start, end: node.end, message: label + ': must be ≥ ' + schema.minimum + '.' });
      if (schema.maximum !== undefined && node.value > schema.maximum) out.push({ start: node.start, end: node.end, message: label + ': must be ≤ ' + schema.maximum + '.' });
    }
    if (actual === 'object') {
      const props = schema.properties || {};
      const required = schema.required || [];
      const seen = new Set();
      node.entries.forEach((entry) => {
        seen.add(entry.key);
        const childSchema = props[entry.key];
        if (!childSchema) {
          if (schema.additionalProperties === false) {
            out.push({ start: entry.keyStart, end: entry.keyEnd, message: label + ': unknown property "' + entry.key + '".' });
          }
          return;
        }
        if (entry.value) validateJSONAgainstSchema(text, entry.value, childSchema, root, path ? path + '.' + entry.key : entry.key, out);
      });
      required.forEach((key) => {
        if (!seen.has(key)) {
          const at = node.end > node.start ? [Math.max(node.start, node.end - 1), node.end] : [node.start, node.end];
          out.push({ start: at[0], end: at[1], message: label + ': missing required property "' + key + '".' });
        }
      });
    }
    if (actual === 'array') {
      const itemSchema = schema.items;
      if (schema.minItems !== undefined && node.items.length < schema.minItems) out.push({ start: node.start, end: node.start + 1, message: label + ': must have at least ' + schema.minItems + ' item(s).' });
      if (schema.maxItems !== undefined && node.items.length > schema.maxItems) out.push({ start: node.start, end: node.start + 1, message: label + ': must have at most ' + schema.maxItems + ' item(s).' });
      if (itemSchema) node.items.forEach((item, idx) => { if (item) validateJSONAgainstSchema(text, item, itemSchema, root, path + '[' + idx + ']', out); });
    }
  }

  // Walks the token stream up to `offset` with a small stack machine
  // (mirroring the object/array nesting) to determine the JSON path and
  // schema in scope at the cursor, tolerating an in-progress/invalid
  // document around the cursor itself (the token currently being typed,
  // "partial", is deliberately excluded from the walk).
  function computeJSONCursorContext(text, offset, rootSchema) {
    const tokens = jsonTokenize(text);
    const partial = tokens.find((t) => {
      if (t.type === 'punct') return false;
      if (t.start < offset && offset < t.end) return true;
      if (t.type === 'string' && !t.terminated && t.start < offset && offset <= t.end) return true;
      return false;
    }) || null;
    const consumed = tokens.filter((t) => t !== partial && t.end <= offset);

    const rootEff = rootSchema ? resolveSchema(rootSchema, rootSchema) : null;
    const stack = [{ kind: 'root', rawSchema: rootSchema, effective: rootEff, path: '', keys: null, index: 0 }];
    let pendingKey = null;
    let expect = 'value';

    function schemaForChild(top, key) {
      if (top.kind === 'object') {
        const props = (top.effective && top.effective.properties) || {};
        if (Object.prototype.hasOwnProperty.call(props, key)) return props[key];
        if (top.effective && top.effective.additionalProperties && typeof top.effective.additionalProperties === 'object') return top.effective.additionalProperties;
        return null;
      }
      if (top.kind === 'array') return (top.effective && top.effective.items) || null;
      return null;
    }

    consumed.forEach((t) => {
      const top = stack[stack.length - 1];
      if (t.type === 'punct') {
        if (t.value === '{' || t.value === '[') {
          const isObj = t.value === '{';
          const key = pendingKey;
          const idx = top.index || 0;
          const childRaw = top.kind === 'root' ? top.rawSchema : schemaForChild(top, key);
          let effChild = childRaw ? resolveSchema(childRaw, rootSchema) : null;
          const arrAlt = childRaw ? oneOfArrayItems(childRaw, rootSchema) : null;
          if (arrAlt) effChild = isObj ? resolveSchema(arrAlt, rootSchema) : { type: 'array', items: arrAlt };
          const childPath = top.kind === 'array' ? top.path + '[' + idx + ']' : (top.path ? top.path + '.' + key : (key || ''));
          stack.push({ kind: isObj ? 'object' : 'array', rawSchema: childRaw, effective: effChild, path: childPath, keys: isObj ? new Set() : null, index: 0 });
          pendingKey = null;
          expect = isObj ? 'key-or-close' : 'value-or-close';
        } else if (t.value === '}' || t.value === ']') {
          if (stack.length > 1) stack.pop();
          expect = 'comma-or-close';
        } else if (t.value === ':') {
          expect = 'value';
        } else if (t.value === ',') {
          if (top.kind === 'array') top.index = (top.index || 0) + 1;
          expect = top.kind === 'object' ? 'key-or-close' : 'value-or-close';
          pendingKey = null;
        }
      } else if (t.type === 'string') {
        if (top.kind === 'object' && expect === 'key-or-close') {
          pendingKey = decodeJSONStringToken(text, t);
          top.keys.add(pendingKey);
          expect = 'colon';
        } else {
          expect = 'comma-or-close';
        }
      } else {
        expect = 'comma-or-close';
      }
    });

    return { tokens, partial, stack, top: stack[stack.length - 1], pendingKey, expect };
  }

  // Property-name / enum-value suggestions for the cursor's current context,
  // or null when nothing sensible applies (e.g. no schema loaded, or the
  // cursor sits somewhere autocomplete doesn't help such as mid-punctuation).
  function jsonAutocompleteSuggestions(text, offset, rootSchema) {
    if (!rootSchema) return null;
    const ctx = computeJSONCursorContext(text, offset, rootSchema);
    const top = ctx.top;
    const partial = ctx.partial;

    // Property-name hints apply right after "{" or "," (key-or-close), but
    // also the moment the user starts typing a fresh '"' right after a
    // value with no separating comma yet (comma-or-close) - they've clearly
    // started a new key, the missing comma is a separate (already-flagged)
    // syntax error, not a reason to withhold the hint.
    const atKeyPosition = top.kind === 'object'
      && (ctx.expect === 'key-or-close' || (ctx.expect === 'comma-or-close' && partial && partial.type === 'string'));
    if (atKeyPosition) {
      const props = (top.effective && top.effective.properties) || {};
      const required = (top.effective && top.effective.required) || [];
      const prefix = partial ? text.slice(partial.start + 1, Math.min(offset, partial.end)) : '';
      const items = Object.keys(props)
        .filter((k) => !top.keys.has(k))
        .filter((k) => k.toLowerCase().startsWith(prefix.toLowerCase()))
        .sort((a, b) => {
          if (required.includes(a) !== required.includes(b)) return required.includes(a) ? -1 : 1;
          return a.localeCompare(b);
        })
        .map((k) => ({
          label: k,
          required: required.includes(k),
          description: props[k].description || resolveSchema(props[k], rootSchema).description || resolveSchema(props[k], rootSchema).title || '',
          insertText: JSON.stringify(k) + ': ',
        }));
      return { range: partial ? [partial.start, offset] : [offset, offset], items };
    }

    if (ctx.expect === 'value') {
      let valueSchema = null;
      if (top.kind === 'object' && ctx.pendingKey) valueSchema = (top.effective && top.effective.properties && top.effective.properties[ctx.pendingKey]) || null;
      else if (top.kind === 'array') valueSchema = (top.effective && top.effective.items) || null;
      else if (top.kind === 'root') valueSchema = top.rawSchema;
      if (!valueSchema) return null;
      const resolved = resolveSchema(valueSchema, rootSchema);
      const isStringPartial = partial && partial.type === 'string';
      const isIdentPartial = partial && partial.type === 'ident';
      if (resolved.enum) {
        const prefix = isStringPartial ? text.slice(partial.start + 1, Math.min(offset, partial.end)) : (isIdentPartial ? text.slice(partial.start, offset) : '');
        const items = resolved.enum
          .filter((v) => String(v).toLowerCase().startsWith(prefix.toLowerCase()))
          .map((v) => ({ label: String(v), description: resolved.description || '', insertText: JSON.stringify(v) }));
        return { range: partial ? [partial.start, offset] : [offset, offset], items };
      }
      if (resolved.type === 'boolean' && (isIdentPartial || !partial)) {
        const prefix = isIdentPartial ? text.slice(partial.start, offset) : '';
        const items = ['true', 'false'].filter((v) => v.startsWith(prefix)).map((v) => ({ label: v, description: '', insertText: v }));
        return { range: partial ? [partial.start, offset] : [offset, offset], items };
      }
      return null;
    }
    return null;
  }

  // Resolves what schema/path applies exactly at the cursor, for the
  // read-only "field info" panel that updates as the caret moves.
  function describeJSONCursorContext(ctx, rootSchema) {
    const top = ctx.top;
    if (ctx.expect === 'value') {
      if (top.kind === 'object' && ctx.pendingKey) {
        const raw = (top.effective && top.effective.properties && top.effective.properties[ctx.pendingKey]) || null;
        return { path: top.path ? top.path + '.' + ctx.pendingKey : ctx.pendingKey, raw, schema: raw ? resolveSchema(raw, rootSchema) : null };
      }
      if (top.kind === 'array') {
        const raw = (top.effective && top.effective.items) || null;
        return { path: top.path + '[' + (top.index || 0) + ']', raw, schema: raw ? resolveSchema(raw, rootSchema) : null };
      }
      if (top.kind === 'root') return { path: '(root)', raw: top.rawSchema, schema: resolveSchema(top.rawSchema, rootSchema) };
    }
    return { path: top.path || '(root)', raw: top.rawSchema, schema: top.effective };
  }

  function jsonLineColAt(text, offset) {
    let line = 1;
    let col = 1;
    for (let i = 0; i < offset && i < text.length; i++) {
      if (text[i] === '\n') { line++; col = 1; } else col++;
    }
    return { line, col };
  }

  function renderJSONHighlightHTML(text, tokens, errorRanges) {
    let html = '';
    let last = 0;
    const overlapsError = (s, e) => errorRanges.some((r) => s < r.end && e > r.start);
    tokens.forEach((t) => {
      if (t.start > last) html += escapeHtml(text.slice(last, t.start));
      let cls;
      if (t.type === 'punct') cls = 'jt-punct';
      else if (t.type === 'string') cls = t.role === 'key' ? 'jt-key' : 'jt-string';
      else if (t.type === 'number') cls = 'jt-number';
      else if (t.type === 'boolean') cls = 'jt-boolean';
      else if (t.type === 'null') cls = 'jt-null';
      else cls = 'jt-plain';
      if (overlapsError(t.start, t.end)) cls += ' jt-error';
      html += '<span class="' + cls + '">' + escapeHtml(text.slice(t.start, t.end)) + '</span>';
      last = t.end;
    });
    if (last < text.length) html += escapeHtml(text.slice(last));
    if (text.length === 0 || text.endsWith('\n')) html += '\n';
    return html;
  }

  // Mirrors the textarea's text-affecting CSS onto a hidden, off-screen div
  // so a marker span inserted at a given character offset reports the pixel
  // position the caret would render at - the standard technique for
  // positioning UI (here, the autocomplete popup) relative to caret in a
  // plain <textarea>, which has no DOM API for this.
  let jsonCaretMirror = null;
  function jsonCaretCoordinates(textarea, offset) {
    if (!jsonCaretMirror) {
      jsonCaretMirror = document.createElement('div');
      jsonCaretMirror.style.position = 'absolute';
      jsonCaretMirror.style.visibility = 'hidden';
      jsonCaretMirror.style.top = '0';
      jsonCaretMirror.style.left = '-9999px';
      jsonCaretMirror.style.whiteSpace = 'pre-wrap';
      jsonCaretMirror.style.overflowWrap = 'break-word';
      document.body.appendChild(jsonCaretMirror);
    }
    const cs = window.getComputedStyle(textarea);
    ['fontFamily', 'fontSize', 'fontWeight', 'lineHeight', 'letterSpacing', 'padding', 'border', 'boxSizing'].forEach((p) => { jsonCaretMirror.style[p] = cs[p]; });
    jsonCaretMirror.style.width = textarea.clientWidth + 'px';
    jsonCaretMirror.textContent = '';
    jsonCaretMirror.appendChild(document.createTextNode(textarea.value.slice(0, offset)));
    const marker = document.createElement('span');
    marker.textContent = '​';
    jsonCaretMirror.appendChild(marker);
    const lineHeight = parseInt(cs.lineHeight, 10) || 18;
    return {
      left: Math.max(0, marker.offsetLeft - textarea.scrollLeft),
      top: Math.max(0, marker.offsetTop - textarea.scrollTop),
      height: lineHeight,
    };
  }

  // The suggestion box lists the property/enum options valid at the cursor
  // (from jsonAutocompleteSuggestions). Clicking an entry inserts it in place
  // of whatever was already typed at that position; Tab and Enter still keep
  // their normal textarea behavior (focus-out / newline) - insertion is
  // mouse-only, unlike a typical code editor's autocomplete.
  let jsonSuggestionsShown = false;
  // The {range, items} last passed to showJSONSuggestions, so a click on one
  // of the <li> elements it rendered knows what text to insert and where.
  let jsonSuggestionsState = null;

  function hideJSONSuggestions() {
    const box = $('#schema-json-suggest');
    if (box) { box.hidden = true; box.innerHTML = ''; }
    jsonSuggestionsShown = false;
    jsonSuggestionsState = null;
  }

  function applyJSONSuggestion(textarea, item) {
    if (!jsonSuggestionsState) return;
    const [start, end] = jsonSuggestionsState.range;
    const value = textarea.value;
    textarea.value = value.slice(0, start) + item.insertText + value.slice(end);
    const caret = start + item.insertText.length;
    textarea.setSelectionRange(caret, caret);
    hideJSONSuggestions();
    textarea.focus();
    refreshJSONEditor();
    // Picking a property name commonly leaves the caret right after ": ",
    // ready for a value - if that value has its own enum/boolean
    // suggestions, show them immediately instead of making the user
    // re-trigger autocomplete.
    maybeShowJSONSuggestions(textarea);
  }

  function showJSONSuggestions(textarea, sugg) {
    const box = $('#schema-json-suggest');
    if (!box || !sugg || !sugg.items.length) { hideJSONSuggestions(); return; }
    jsonSuggestionsShown = true;
    jsonSuggestionsState = sugg;
    box.innerHTML = '';
    sugg.items.forEach((item) => {
      box.appendChild(el('li', { onclick: () => applyJSONSuggestion(textarea, item) }, [
        el('span', { class: 'suggest-name', text: item.label + (item.required ? ' *' : '') }),
        item.description ? el('span', { class: 'suggest-desc', text: item.description }) : null,
      ]));
    });
    const coords = jsonCaretCoordinates(textarea, sugg.range[0]);
    box.style.left = coords.left + 'px';
    box.style.top = (coords.top + coords.height) + 'px';
    box.hidden = false;
  }

  function maybeShowJSONSuggestions(textarea) {
    const sugg = jsonAutocompleteSuggestions(textarea.value, textarea.selectionStart, state.schema);
    if (!sugg || !sugg.items.length) { hideJSONSuggestions(); return; }
    showJSONSuggestions(textarea, sugg);
  }

  function renderJSONInfoPanel(textarea) {
    const info = $('#schema-json-info');
    if (!info) return;
    if (!state.schema) { info.innerHTML = ''; return; }
    const ctx = computeJSONCursorContext(textarea.value, textarea.selectionStart, state.schema);
    const desc = describeJSONCursorContext(ctx, state.schema);
    if (!desc.schema) { info.innerHTML = '<em>No schema information available here.</em>'; return; }
    const s = desc.schema;
    const typeLabel = s.enum ? ('enum: ' + s.enum.map((v) => JSON.stringify(v)).join(' | ')) : (Array.isArray(s.type) ? s.type.join('|') : (s.type || 'object'));
    const parts = ['<strong>' + escapeHtml(desc.path) + '</strong>', '<span class="json-info-type">' + escapeHtml(typeLabel) + '</span>'];
    if (desc.raw && desc.raw.title) parts.push(escapeHtml(desc.raw.title));
    if (s.default !== undefined) parts.push('default: ' + escapeHtml(JSON.stringify(s.default)));
    let html = parts.join(' &middot; ');
    const description = (desc.raw && desc.raw.description) || s.description;
    if (description) html += '<br>' + escapeHtml(description);
    info.innerHTML = html;
  }

  function renderJSONProblems(text, errors, textarea) {
    const list = $('#schema-json-problems');
    if (!list) return;
    list.innerHTML = '';
    errors.slice().sort((a, b) => a.start - b.start).forEach((e) => {
      const { line, col } = jsonLineColAt(text, e.start);
      list.appendChild(el('li', {
        onclick: () => {
          textarea.focus();
          textarea.setSelectionRange(e.start, Math.max(e.end, e.start + 1));
          renderJSONInfoPanel(textarea);
        },
      }, [
        el('span', { class: 'problem-loc', text: 'Ln ' + line + ', Col ' + col }),
        document.createTextNode(e.message),
      ]));
    });
  }

  function syncJSONEditorScroll() {
    const textarea = $('#schema-json-textarea');
    const highlight = $('#schema-json-highlight');
    if (textarea && highlight) { highlight.scrollTop = textarea.scrollTop; highlight.scrollLeft = textarea.scrollLeft; }
  }

  // Re-runs highlighting, validation and the info panel for the JSON
  // editor's current content. Call after any programmatic change to
  // #schema-json-textarea's value (typing itself is handled by the 'input'
  // listener) or to state.schema.
  function refreshJSONEditor() {
    const textarea = $('#schema-json-textarea');
    const highlightCode = $('#schema-json-highlight code');
    if (!textarea || !highlightCode) return;
    const text = textarea.value;
    const { root, errors, tokens } = jsonParseLenient(text);
    const schemaErrors = [];
    if (root && state.schema) validateJSONAgainstSchema(text, root, state.schema, state.schema, '', schemaErrors);
    const allErrors = errors.concat(schemaErrors);
    highlightCode.innerHTML = renderJSONHighlightHTML(text, tokens, allErrors);
    renderJSONProblems(text, allErrors, textarea);
    renderJSONInfoPanel(textarea);
    syncJSONEditorScroll();
  }

  // Full schema validation walks the whole document and resolves ($ref/allOf)
  // schema nodes as it goes, which on a realistic multi-thousand-line
  // bngblaster config is far too slow to run on every keystroke. Typing
  // therefore only re-runs tokenizing and highlighting - cheap, and what the
  // caret needs to stay in sync - and defers validation to a short pause.
  function refreshJSONEditorSyntaxOnly() {
    const textarea = $('#schema-json-textarea');
    const highlightCode = $('#schema-json-highlight code');
    if (!textarea || !highlightCode) return;
    const { tokens } = jsonParseLenient(textarea.value);
    highlightCode.innerHTML = renderJSONHighlightHTML(textarea.value, tokens, []);
    syncJSONEditorScroll();
  }

  const refreshJSONEditorDebounced = debounce(refreshJSONEditor, 180);

  // Setting a textarea's .value moves the caret to the very end of the text
  // (browser default), which is right after the root value's closing
  // brace/bracket - outside the document entirely. From there, typing '"'
  // starts a new (invalid) root value instead of a property, so suggestions
  // never appear. Move the caret to just inside the root container instead,
  // so typing starts a key/item right away. Never called after user
  // keystrokes - only right after we set .value ourselves.
  function placeJSONEditorCaretInsideRoot(textarea) {
    const { root } = jsonParseLenient(textarea.value);
    if (!root || (root.type !== 'object' && root.type !== 'array')) return;
    const pos = Math.max(root.start + 1, root.end - 1);
    textarea.setSelectionRange(pos, pos);
  }

  function initJSONEditorDOM() {
    const textarea = $('#schema-json-textarea');
    if (!textarea) return;
    // Without this, clicking a suggestion first blurs the textarea (the
    // mousedown that precedes the click), which schedules hideJSONSuggestions
    // and can clear the list out from under the click.
    const suggestBox = $('#schema-json-suggest');
    if (suggestBox) suggestBox.addEventListener('mousedown', (ev) => ev.preventDefault());
    textarea.addEventListener('input', () => {
      refreshJSONEditorSyntaxOnly();
      refreshJSONEditorDebounced();
      maybeShowJSONSuggestions(textarea);
    });
    textarea.addEventListener('scroll', syncJSONEditorScroll);
    textarea.addEventListener('click', () => { hideJSONSuggestions(); renderJSONInfoPanel(textarea); });
    textarea.addEventListener('keyup', (ev) => {
      if (['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End'].includes(ev.key)) {
        hideJSONSuggestions();
        renderJSONInfoPanel(textarea);
      }
    });
    textarea.addEventListener('keydown', (ev) => {
      if (jsonSuggestionsShown && ev.key === 'Escape') {
        ev.preventDefault();
        ev.stopPropagation();
        hideJSONSuggestions();
        return;
      }
      if (ev.key === ' ' && (ev.ctrlKey || ev.metaKey)) {
        ev.preventDefault();
        maybeShowJSONSuggestions(textarea);
      }
    });
    textarea.addEventListener('blur', () => { setTimeout(hideJSONSuggestions, 100); });
  }

  // Builds the left-side "jump to section" index for the schema-driven form,
  // one entry per top-level property. Clicking an entry opens that section
  // (if it collapses into a <details>) and scrolls it into view.
  function buildFormSectionNav(fields, properties, schema, requiredTop) {
    const nav = $('#schema-form-nav');
    nav.innerHTML = '';
    if (fields.length < 2) { nav.hidden = true; return; }
    fields.forEach(([k, field]) => {
      const label = labelFor(k, resolveSchema(properties[k], schema));
      nav.appendChild(el('button', {
        type: 'button',
        class: 'schema-form-nav-link',
        text: label + (requiredTop.includes(k) ? ' *' : ''),
        onclick: () => {
          if (field.el.tagName === 'DETAILS') field.el.open = true;
          field.el.scrollIntoView({ behavior: 'smooth', block: 'start' });
        },
      }));
    });
    nav.hidden = false;
  }

  // null while creating a new instance; the instance name while editing an
  // existing (stopped) one's configuration in place.
  let editingInstanceName = null;

  // Opens the New/Edit Instance dialog. Pass an existing (stopped) instance
  // name to edit its current configuration in place (PUT overwrites it,
  // same as creating); omit it to create a new instance.
  async function openInstanceDialog(existingName) {
    editingInstanceName = existingName || null;
    setStatusMessage($('#new-instance-status'), '');
    $('#new-instance-name').value = existingName || '';
    $('#new-instance-name').readOnly = !!existingName;
    $('#dialog-new-instance-title').textContent = existingName ? ('Edit Instance — ' + existingName) : 'New Test Instance';
    $('#btn-create-instance').textContent = existingName ? 'Save' : 'Create';
    $('#btn-create-and-start-instance').textContent = existingName ? 'Save & Start' : 'Create & Start';
    setStatusMessage($('#schema-json-status'), '');
    state.schema = null;
    $('#schema-json-textarea').value = '{\n  \n}';
    placeJSONEditorCaretInsideRoot($('#schema-json-textarea'));
    refreshJSONEditor();
    newInstanceFields = null;
    configMode = 'form';
    $('#config-mode-toggle').hidden = false;
    $('#config-mode-btn-form').setAttribute('aria-pressed', 'true');
    $('#config-mode-btn-json').setAttribute('aria-pressed', 'false');
    $('#schema-json-root').hidden = true;
    const root = $('#schema-form-root');
    root.innerHTML = '<p class="hint">Loading configuration schema…</p>';
    root.hidden = false;
    const nav = $('#schema-form-nav');
    nav.hidden = true;
    nav.innerHTML = '';
    openDialog('dialog-new-instance');

    const existingConfig = existingName ? await API.getConfig(existingName).catch(() => null) : null;

    try {
      const [schema, interfaces] = await Promise.all([API.schema(), API.interfaces().catch(() => [])]);
      state.schema = schema;
      state.interfaces = interfaces || [];
      root.innerHTML = '';
      const effective = resolveSchema(schema, schema);
      if (!effective.properties) {
        root.appendChild(el('p', { class: 'hint' }, [document.createTextNode('Schema has no top-level "properties" — use the JSON editor below.')]));
        forceJSONOnlyMode();
        if (existingConfig) $('#schema-json-textarea').value = JSON.stringify(existingConfig, null, 2);
        refreshJSONEditor();
        return;
      }
      const requiredTop = effective.required || [];
      const fields = Object.entries(effective.properties).map(([k, s]) => [k, buildField(k, s, schema, requiredTop.includes(k))]);
      fields.forEach(([, f]) => root.appendChild(f.el));
      newInstanceFields = fields;
      buildFormSectionNav(fields, effective.properties, schema, requiredTop);

      if (existingConfig) applyJSONToForm(existingConfig);
      if (existingConfig) $('#schema-json-textarea').value = JSON.stringify(existingConfig, null, 2);
      refreshJSONEditor();
    } catch (e) {
      root.innerHTML = '';
      root.appendChild(el('p', { class: 'hint' }, [document.createTextNode('Could not load /api/v1/schema (' + e.message + '). Use the JSON editor below.')]));
      state.schema = null;
      forceJSONOnlyMode();
      if (existingConfig) $('#schema-json-textarea').value = JSON.stringify(existingConfig, null, 2);
      refreshJSONEditor();
    }
  }

  async function submitNewInstance(andStart) {
    const status = $('#new-instance-status');
    setStatusMessage(status, '');
    const name = editingInstanceName || $('#new-instance-name').value.trim();
    if (!name || !/^[A-Za-z0-9_-]+$/.test(name)) {
      setStatusMessage(status, 'Please enter a valid instance name (letters, digits, "-" and "_").', 'error');
      $('#new-instance-name').focus();
      return;
    }
    let config;
    if (configMode === 'json') {
      try {
        const text = $('#schema-json-textarea').value.trim();
        config = text ? JSON.parse(text) : {};
      } catch (e) {
        setStatusMessage($('#schema-json-status'), 'Invalid JSON: ' + e.message, 'error');
        $('#schema-json-textarea').focus();
        return;
      }
    } else {
      config = collectFormJSON();
    }
    try {
      await API.create(name, config);
      announce((editingInstanceName ? 'Saved instance ' : 'Created instance ') + name, 'success');
      closeDialog('dialog-new-instance');
      await loadInstances();
      if (andStart) {
        openStartDialog(name, () => openInstance(name));
      }
    } catch (e) {
      showDetailedError(status, 'Could not save instance', e.message);
    }
  }
  $('#btn-create-instance').addEventListener('click', () => submitNewInstance(false));
  $('#btn-create-and-start-instance').addEventListener('click', () => submitNewInstance(true));
  $('#btn-new-instance').addEventListener('click', () => openInstanceDialog(null));

  //= ========================================================================
  // Instance detail view: tabs
  //= ========================================================================
  const TABS = ['overview', 'sessions', 'streams', 'bgp', 'isis', 'ospf', 'lag', 'commands', 'generate'];
  function isTabActive(name) {
    const tab = $('#tab-' + name);
    return !!tab && tab.getAttribute('aria-selected') === 'true';
  }
  // userInitiated moves focus onto the newly selected tab, which is what a
  // click or arrow-key press should do. Programmatic activation (opening an
  // instance) leaves focus alone instead of stealing it and scrolling the
  // page to the tab strip.
  function isTabHidden(name) {
    const tab = $('#tab-' + name);
    return !!tab && tab.hidden;
  }

  // Which counters have to be non-zero for a tab to have anything to show.
  // Streams covers both flow kinds because the stream table lists them
  // together, exactly as state.stream.total counts them.
  const TAB_AVAILABILITY = {
    sessions: (c) => !!c['sessions'],
    streams: (c) => !!((c['session-traffic-flows'] || 0) + (c['stream-traffic-flows'] || 0)),
  };

  // Hides a tab that can only ever render an empty table - Sessions during a
  // pure stream test, Streams during a run with no traffic flows at all.
  //
  // available: true, false, or null when it is not known yet. A stopped or
  // unreachable instance reports no counters at all, and hiding a tab on
  // missing information would be wrong - the panel's own "not running"
  // message is more useful than a vanished tab.
  function setTabAvailable(name, available) {
    const tab = $('#tab-' + name);
    if (!tab) return;
    const hidden = available === false;
    if (tab.hidden === hidden) return;
    tab.hidden = hidden;
    // Never strand the user on a tab that has just disappeared.
    if (hidden && isTabActive(name)) activateTab('overview');
  }

  function updateTabAvailability(counters) {
    Object.keys(TAB_AVAILABILITY).forEach((name) => {
      setTabAvailable(name, counters ? TAB_AVAILABILITY[name](counters) : null);
    });
  }

  function activateTab(name, userInitiated) {
    if (isTabHidden(name)) return;
    TABS.forEach((t) => {
      const tab = $('#tab-' + t);
      const panel = $('#panel-' + t);
      const active = t === name;
      tab.setAttribute('aria-selected', String(active));
      tab.tabIndex = active ? 0 : -1;
      panel.hidden = !active;
    });
    if (userInitiated) $('#tab-' + name).focus();
    // The overview poll runs for the whole instance view (the header badge
    // needs it on every tab), so selecting the tab only needs to render the
    // sections that were not being updated while it was hidden.
    if (name === 'overview') pollOverview();
    if (name === 'sessions') initSessionView(); else stopSessionPolling();
    if (name === 'streams') initStreamView(); else stopStreamPolling();
    if (name === 'bgp') initBgpView(); else stopBgpPolling();
    if (name === 'isis') initIsisView(); else stopIsisPolling();
    if (name === 'ospf') initOspfView(); else stopOspfPolling();
    if (name === 'lag') initLagView(); else stopLagPolling();
    if (name === 'commands') loadCommandBuilder();
    if (name === 'generate') initGenerateView();
  }
  $all('#view-instance [role="tab"]').forEach((tab, idx, all) => {
    tab.addEventListener('click', () => activateTab(tab.id.replace('tab-', ''), true));
    tab.addEventListener('keydown', (ev) => {
      // Step over any hidden tab rather than stopping on it, in whichever
      // direction the key implies.
      const seek = (from, step) => {
        for (let i = 1; i <= all.length; i++) {
          const candidate = all[(from + step * i + all.length * all.length) % all.length];
          if (!isTabHidden(candidate.id.replace('tab-', ''))) return candidate;
        }
        return null;
      };
      let target = null;
      if (ev.key === 'ArrowRight') target = seek(idx, 1);
      else if (ev.key === 'ArrowLeft') target = seek(idx, -1);
      else if (ev.key === 'Home') target = seek(-1, 1);
      else if (ev.key === 'End') target = seek(all.length, -1);
      if (target) { ev.preventDefault(); activateTab(target.id.replace('tab-', ''), true); }
    });
  });

  // Clears everything in the instance detail view that belongs to a specific
  // instance. Without this the view keeps showing the previous instance's
  // data until (and unless) a response for the new one replaces it: opening
  // an instance with sessions and then one with none left the first
  // instance's session rows on screen, because the "no sessions" path only
  // hides the table it never populated.
  function resetInstanceViews() {
    // Session overview.
    updateTabAvailability(null);
    clearSessionCounters();
    renderInterfaceStats(null);
    renderTestDuration(null);

    // Stream and session tables, including the virtual-scroll bookkeeping
    // that sizes their scrollbars.
    [
      ['stream', '#stream-tbody', '#streams-content', '#streams-empty', '#stream-viewport', '#stream-count-hint', '#stream-filters-form'],
      ['session', '#session-tbody', '#sessions-content', '#sessions-empty', '#session-viewport', '#session-count-hint', '#session-filters-form'],
    ].forEach(([key, tbody, content, empty, viewport, hint, filtersForm]) => {
      $(tbody).innerHTML = '';
      $(content).hidden = true;
      $(empty).hidden = false;
      $(empty).textContent = DEFAULT_EMPTY_TEXT[empty];
      $(viewport).scrollTop = 0;
      $(hint).textContent = '';
      $(filtersForm).reset();
      state[key].instance = null;
      state[key].total = 0;
      state[key].filters = {};
    });

    // BGP tab: hidden again until the new instance's config says otherwise.
    setTabAvailable('bgp', false);
    $('#bgp-tbody').innerHTML = '';
    state.bgp.selected.clear();
    state.bgp.items = [];
    updateBgpSelectionControls();
    $('#bgp-content').hidden = true;
    $('#bgp-empty').hidden = false;
    $('#bgp-empty').textContent = DEFAULT_EMPTY_TEXT['#bgp-empty'];
    $('#bgp-count-hint').textContent = '';

    // ISIS and OSPF tabs: same as BGP; the selects are refilled from the
    // new instance's config when the tab is opened, and the database starts
    // collapsed again.
    ['isis', 'ospf'].forEach((proto) => {
      setTabAvailable(proto, false);
      state[proto].instance = null;
      state[proto].database = [];
      state[proto].dbOffset = 0;
      $('#' + proto + '-db-details').open = false;
      $('#' + proto + '-db-prev').hidden = true;
      $('#' + proto + '-db-next').hidden = true;
      $('#' + proto + '-content').hidden = true;
      $('#' + proto + '-empty').hidden = false;
      $('#' + proto + '-empty').textContent = DEFAULT_EMPTY_TEXT['#' + proto + '-empty'];
      $('#' + proto + '-db-tbody').innerHTML = '';
      $('#' + proto + '-db-hint').textContent = '';
      $('#' + proto + '-db-filter').value = '';
    });
    $('#isis-adjacency-tbody').innerHTML = '';
    $('#ospf-interface-tbody').innerHTML = '';
    $('#ospf-neighbor-tbody').innerHTML = '';
    // Emptied rather than kept: an instance or level picked for the
    // previous instance would otherwise stay selected if the next one has
    // it too, e.g. showing its L2 database instead of starting with L1.
    ['#isis-instance', '#isis-db-level', '#ospf-instance'].forEach((sel) => { $(sel).innerHTML = ''; });

    // LAG tab: same as BGP, driven by the new instance's config.
    setTabAvailable('lag', false);
    state.lag.pending.clear();
    $('#lag-content').innerHTML = '';
    $('#lag-content').hidden = true;
    $('#lag-empty').hidden = false;
    $('#lag-empty').textContent = DEFAULT_EMPTY_TEXT['#lag-empty'];

    // Generate tab: prefilled from the instance's config, so rebuilt for
    // the next instance; whether it is shown at all does not depend on
    // the instance.
    Object.assign(state.generate, { instance: null, tool: null, fields: [], files: [], config: null, running: false, history: [] });
    $('#generate-fields').innerHTML = '';
    $('#generate-result').innerHTML = '';
    $('#generate-result').hidden = true;
    $('#generate-result-empty').hidden = false;
    $('#generate-history').innerHTML = '';
    $('#generate-history-wrap').hidden = true;

    // Commands tab: the command list is per instance (it is discovered from
    // the instance itself), as is any response already shown.
    $('#command-select').innerHTML = '';
    $('#command-args-fieldset').innerHTML = '';
    currentCommandArgFields = [];
    $('#command-output').hidden = true;
    $('#command-output-meta').hidden = true;
    $('#command-output-empty').hidden = false;
    $('#command-output-empty').textContent = DEFAULT_EMPTY_TEXT['#command-output-empty'];
  }

  // Marks the primary navigation entry of the visible view. The instance
  // detail view belongs to the dashboard.
  function setCurrentNav(view) {
    ['dashboard', 'host'].forEach((name) => {
      const button = $('#nav-' + name);
      if (name === view) button.setAttribute('aria-current', 'page');
      else button.removeAttribute('aria-current');
    });
  }

  function leaveInstance() {
    state.currentInstance = null;
    stopInstancePolling();
    stopStreamPolling();
    stopSessionPolling();
    stopBgpPolling();
    stopLagPolling();
  }

  function showDashboard() {
    leaveInstance();
    stopHostPolling();
    $('#view-instance').hidden = true;
    $('#view-host').hidden = true;
    $('#view-dashboard').hidden = false;
    setCurrentNav('dashboard');
    loadInstances();
  }
  $('#btn-back-dashboard').addEventListener('click', showDashboard);
  $('#nav-dashboard').addEventListener('click', showDashboard);

  //= ========================================================================
  // Host view
  //= ========================================================================
  // Pinned CPUs are colored per instance. Colors are assigned in order of
  // first appearance and kept for the session, so an instance keeps its
  // color while others start and stop. White text stays readable on all.
  const PIN_COLORS = ['#0b5fa5', '#8a1c1c', '#146c2e', '#7a3fa0', '#a35200',
    '#00727a', '#55640a', '#a0306b', '#3d4f8f', '#6b4a2b'];
  // A bngblaster not started by this controller has no instance name.
  const FOREIGN_PIN_COLOR = '#4b5563';

  const hostState = {
    timer: null,
    // CPU ids per NUMA node the rendered CPU map was built for.
    layoutKey: null,
    cells: new Map(),
    nodeMeta: new Map(),
    colors: new Map(),
  };

  function pinKey(pin) { return pin.instance || ('pid:' + pin.pid); }
  function pinLabel(pin) { return pin.instance || ('PID ' + pin.pid); }
  function pinColor(pin) {
    if (!pin.instance) return FOREIGN_PIN_COLOR;
    if (!hostState.colors.has(pin.instance)) {
      hostState.colors.set(pin.instance, PIN_COLORS[hostState.colors.size % PIN_COLORS.length]);
    }
    return hostState.colors.get(pin.instance);
  }

  // [0,1,2,3,8,10,11] -> "0-3,8,10-11", the kernel's CPU list notation
  // that is also used to pin CPUs in the bngblaster configuration.
  function formatCpuList(ids) {
    const runs = contiguousRuns(ids);
    return runs.map((run) => (run.length > 1 ? run[0] + '-' + run[run.length - 1] : String(run[0]))).join(',');
  }

  function contiguousRuns(ids) {
    const sorted = ids.slice().sort((a, b) => a - b);
    const runs = [];
    sorted.forEach((id) => {
      const last = runs[runs.length - 1];
      if (last && id === last[last.length - 1] + 1) last.push(id);
      else runs.push([id]);
    });
    return runs;
  }

  function usageMeter(label, percent, count, detail) {
    const pct = Math.max(0, Math.min(100, Number(percent) || 0));
    let cls = 'session-meter usage-meter';
    if (pct >= 90) cls += ' level-crit';
    else if (pct >= 80) cls += ' level-warn';
    const id = nextId('usage');
    return el('div', { class: cls }, [
      el('div', { class: 'meter-label' }, [
        el('span', { id: id }, [
          document.createTextNode(label),
          detail ? el('span', { class: 'detail', text: detail }) : null,
        ]),
        el('span', { class: 'count', text: count }),
      ]),
      el('progress', { value: pct.toFixed(1), max: '100', 'aria-labelledby': id }),
    ]);
  }

  function percentOf(part, whole) {
    return whole > 0 ? (part * 100) / whole : 0;
  }

  function formatPercent(pct) {
    return (Math.round(pct * 10) / 10).toFixed(1) + '%';
  }

  function showHost() {
    leaveInstance();
    $('#view-dashboard').hidden = true;
    $('#view-instance').hidden = true;
    $('#view-host').hidden = false;
    setCurrentNav('host');
    // Link, speed and queues may have changed since the view was last
    // open; they are read once per opening, not polled (ethtool per NIC).
    loadHostInterfaces();
    refreshHost();
    hostState.timer = cancelPoll(hostState.timer);
    hostState.timer = schedulePoll(refreshHost, 2000);
  }
  $('#nav-host').addEventListener('click', showHost);

  function stopHostPolling() {
    hostState.timer = cancelPoll(hostState.timer);
  }

  async function refreshHost() {
    let info;
    try {
      info = await API.host();
    } catch (e) {
      showDetailedError($('#host-status'), 'Could not load host information', e.message);
      return;
    }
    // The view may have been left while the request was in flight.
    if ($('#view-host').hidden) return;
    setStatusMessage($('#host-status'), '');
    renderHost(info);
  }

  function renderHost(info) {
    const cpus = info.cpus || [];
    const memory = info.memory || {};
    $('#host-name').textContent = info.hostname ? '— ' + info.hostname : '';
    $('#btn-host-topology').hidden = !info.topology;

    // Live usage
    const load = (info.load_average || []).map((v) => v.toFixed(2)).join(' ');
    $('#host-usage').replaceChildren(
      usageMeter('CPU', info.cpu.usage, formatPercent(info.cpu.usage || 0),
        info.cpu.threads + ' CPUs' + (load ? ' · load ' + load : '')),
      usageMeter('Memory', percentOf(memory.used, memory.total),
        formatFileSize(memory.used) + ' / ' + formatFileSize(memory.total) +
        ' (' + formatPercent(percentOf(memory.used, memory.total)) + ')',
        formatFileSize(memory.available) + ' available'),
    );
    if (memory.swap_total > 0) {
      const swapUsed = memory.swap_total - memory.swap_free;
      $('#host-usage').appendChild(usageMeter('Swap', percentOf(swapUsed, memory.swap_total),
        formatFileSize(swapUsed) + ' / ' + formatFileSize(memory.swap_total) +
        ' (' + formatPercent(percentOf(swapUsed, memory.swap_total)) + ')'));
    }
    const disks = info.disks || [];
    // Percentages as df: relative to the space usable by non-root users.
    $('#host-disks').replaceChildren(...disks.map((disk) => {
      const usable = disk.used + disk.available;
      return usageMeter(disk.mount, percentOf(disk.used, usable),
        formatFileSize(disk.used) + ' / ' + formatFileSize(usable) +
        ' (' + formatPercent(percentOf(disk.used, usable)) + ')',
        disk.device + ' · ' + disk.fstype + ' · ' + formatFileSize(disk.available) + ' free');
    }));
    $('#host-disks-empty').hidden = disks.length > 0;

    // Facts
    const facts = [
      ['CPU', info.cpu.model || 'unknown'],
      ['Sockets / cores / threads', info.cpu.sockets + ' / ' + info.cpu.cores + ' / ' + info.cpu.threads],
      ['NUMA nodes', String((info.numa_nodes || []).length)],
      ['Memory', formatFileSize(memory.total)],
      ['Huge pages', memory.hugepages_total > 0
        ? memory.hugepages_free + ' free of ' + memory.hugepages_total + ' × ' + formatFileSize(memory.hugepage_size)
        : 'none'],
      ['Kernel', info.kernel || 'unknown'],
      ['Uptime', formatDuration(Math.floor(info.uptime || 0))],
    ];
    $('#host-facts').replaceChildren(...facts.map(([term, value]) => el('div', {}, [
      el('dt', { text: term }),
      el('dd', { text: value }),
    ])));

    // CPU summary tiles
    const online = cpus.filter((c) => c.online);
    const free = online.filter((c) => !(c.pins && c.pins.length));
    const freeIsolated = free.filter((c) => c.isolated);
    $('#host-cpu-tiles').replaceChildren(
      statTile('Online CPUs', online.length),
      statTile('Pinned', info.cpu.pinned),
      statTile('Free', free.length),
      statTile('Isolated', info.cpu.isolated),
      statTile('Free isolated', freeIsolated.length),
    );

    renderCpuMap(info);
    renderPinTable(cpus);
  }

  function renderCpuMap(info) {
    const nodes = info.numa_nodes || [];
    const key = JSON.stringify(nodes.map((n) => [n.id, n.cpus]));
    if (key !== hostState.layoutKey) buildCpuMap(nodes, key);

    const byId = new Map((info.cpus || []).map((c) => [c.id, c]));
    hostState.cells.forEach((refs, id) => {
      const cpu = byId.get(id);
      if (cpu) updateCpuCell(refs, cpu);
    });
    nodes.forEach((node) => {
      const meta = hostState.nodeMeta.get(node.id);
      if (!meta) return;
      const nodeCpus = node.cpus.map((id) => byId.get(id)).filter(Boolean);
      const pinned = nodeCpus.filter((c) => c.pins && c.pins.length).length;
      const onlineCount = nodeCpus.filter((c) => c.online).length;
      meta.textContent = nodeCpus.length + ' CPUs (' + formatCpuList(node.cpus) + ') · ' +
        pinned + ' pinned · ' + (onlineCount - pinned) + ' free · memory ' +
        formatFileSize(node.memory_free) + ' free of ' + formatFileSize(node.memory_total);
    });
  }

  // (Re)builds the cells, only when the CPUs or NUMA nodes change: updating
  // them in place keeps a tooltip open across refreshes.
  function buildCpuMap(nodes, key) {
    hostState.layoutKey = key;
    hostState.cells.clear();
    hostState.nodeMeta.clear();
    const container = $('#host-numa');
    container.replaceChildren(...nodes.map((node) => {
      const meta = el('span', { class: 'numa-node-meta' });
      hostState.nodeMeta.set(node.id, meta);
      const runs = contiguousRuns(node.cpus);
      return el('div', { class: 'numa-node' }, [
        el('div', { class: 'numa-node-header' }, [
          el('h3', { text: 'NUMA node ' + node.id }),
          meta,
        ]),
        ...runs.map((run) => el('div', { class: 'cpu-range' }, [
          runs.length > 1
            ? el('div', { class: 'cpu-range-label', text: 'CPUs ' + run[0] + '–' + run[run.length - 1] })
            : null,
          el('ul', { class: 'cpu-grid', 'aria-label': 'CPUs ' + run[0] + ' to ' + run[run.length - 1] },
            run.map((id) => {
              const loadBar = el('span', { class: 'cpu-load', 'aria-hidden': 'true' });
              const cell = el('li', { class: 'cpu-cell', role: 'img' }, [
                loadBar,
                el('span', { class: 'cpu-id', 'aria-hidden': 'true', text: String(id) }),
              ]);
              hostState.cells.set(id, { cell: cell, load: loadBar });
              return cell;
            })),
        ])),
      ]);
    }));
  }

  function updateCpuCell(refs, cpu) {
    const pins = cpu.pins || [];
    const cell = refs.cell;
    cell.classList.toggle('is-pinned', pins.length > 0);
    cell.classList.toggle('is-shared', pins.length > 1);
    cell.classList.toggle('is-isolated', cpu.isolated);
    cell.classList.toggle('is-offline', !cpu.online);
    if (pins.length) {
      cell.style.setProperty('--pin-color', pinColor(pins[0]));
      cell.style.setProperty('--pin-color-2', pinColor(pins[pins.length > 1 ? 1 : 0]));
    } else {
      cell.style.removeProperty('--pin-color');
      cell.style.removeProperty('--pin-color-2');
    }
    const usage = cpu.online ? (cpu.usage || 0) : 0;
    refs.load.style.height = usage.toFixed(0) + '%';

    let text = 'CPU ' + cpu.id + ' · node ' + cpu.node + ' · socket ' + cpu.socket + ' · core ' + cpu.core;
    text += cpu.online ? ' · load ' + formatPercent(usage) : ' · offline';
    if (cpu.isolated) text += ' · isolated';
    text += pins.length
      ? ' · pinned by ' + pins.map((p) => pinLabel(p) + (p.instance ? ' (PID ' + p.pid + ')' : '')).join(', ')
      : ' · free';
    if (cell.title !== text) {
      cell.title = text;
      cell.setAttribute('aria-label', text);
    }
  }

  function renderPinTable(cpus) {
    const byPin = new Map();
    cpus.forEach((cpu) => {
      (cpu.pins || []).forEach((pin) => {
        const k = pinKey(pin) + '/' + pin.pid;
        if (!byPin.has(k)) byPin.set(k, { pin: pin, cpus: [], nodes: new Set() });
        const entry = byPin.get(k);
        entry.cpus.push(cpu.id);
        entry.nodes.add(cpu.node);
      });
    });
    const entries = Array.from(byPin.values()).sort((a, b) =>
      pinLabel(a.pin).localeCompare(pinLabel(b.pin)) || a.pin.pid - b.pin.pid);
    $('#host-pins-tbody').replaceChildren(...entries.map((entry) => {
      const pin = entry.pin;
      const chip = el('span', { class: 'pin-chip', 'aria-hidden': 'true' });
      chip.style.setProperty('--pin-color', pinColor(pin));
      const name = pin.instance
        ? el('a', { href: '#', text: pin.instance, onclick: (ev) => { ev.preventDefault(); openInstance(pin.instance); } })
        : el('span', { text: 'not started by this controller' });
      return el('tr', {}, [
        el('td', {}, [chip, name]),
        el('td', { text: String(pin.pid) }),
        el('td', { class: 'cpu-list', text: formatCpuList(entry.cpus) }),
        el('td', { text: String(entry.cpus.length) }),
        el('td', { text: Array.from(entry.nodes).sort((a, b) => a - b).join(', ') }),
      ]);
    }));
    $('#host-pins-table').hidden = entries.length === 0;
    $('#host-pins-empty').hidden = entries.length > 0;
  }

  //= ------------------------------------------------------------------------
  // Host tabs: Overview and Interfaces
  //= ------------------------------------------------------------------------
  const HOST_TABS = ['overview', 'interfaces'];
  function activateHostTab(name, focus) {
    HOST_TABS.forEach((t) => {
      const tab = $('#hosttab-' + t);
      const active = t === name;
      tab.setAttribute('aria-selected', String(active));
      tab.tabIndex = active ? 0 : -1;
      $('#hostpanel-' + t).hidden = !active;
    });
    if (focus) $('#hosttab-' + name).focus();
  }
  HOST_TABS.forEach((name, idx) => {
    const tab = $('#hosttab-' + name);
    tab.addEventListener('click', () => activateHostTab(name));
    tab.addEventListener('keydown', (ev) => {
      let next = null;
      if (ev.key === 'ArrowRight') next = HOST_TABS[(idx + 1) % HOST_TABS.length];
      else if (ev.key === 'ArrowLeft') next = HOST_TABS[(idx - 1 + HOST_TABS.length) % HOST_TABS.length];
      else if (ev.key === 'Home') next = HOST_TABS[0];
      else if (ev.key === 'End') next = HOST_TABS[HOST_TABS.length - 1];
      if (next) { ev.preventDefault(); activateHostTab(next, true); }
    });
  });

  //= ------------------------------------------------------------------------
  // Host interfaces
  //= ------------------------------------------------------------------------
  let hostInterfaces = null;

  async function loadHostInterfaces() {
    $('#nics-loading').hidden = hostInterfaces !== null;
    $('#btn-nics-refresh').disabled = true;
    try {
      hostInterfaces = await API.hostInterfaces();
      setStatusMessage($('#nics-status'), '');
      renderHostInterfaces();
    } catch (e) {
      showDetailedError($('#nics-status'), 'Could not load interfaces', e.message);
    } finally {
      $('#nics-loading').hidden = true;
      $('#btn-nics-refresh').disabled = false;
    }
  }
  $('#btn-nics-refresh').addEventListener('click', loadHostInterfaces);
  $('#nics-show-virtual').addEventListener('change', renderHostInterfaces);

  // Mb/s as reported by the kernel; 2500 -> "2.5 Gb/s".
  function formatLinkSpeed(mbps) {
    if (typeof mbps !== 'number') return '';
    return mbps >= 1000 ? (mbps / 1000) + ' Gb/s' : mbps + ' Mb/s';
  }

  // Link state pill: "up", "down" or "no carrier" (administratively up,
  // but no link - e.g. a cable or peer problem).
  function linkPill(nic) {
    // Virtual interfaces without link detection report "unknown".
    if (nic.operstate === 'up' || (nic.operstate === 'unknown' && nic.carrier)) {
      return el('span', { class: 'status-pill started', text: 'up' });
    }
    if (nic.carrier === false) {
      return el('span', { class: 'status-pill link-down', text: 'no carrier' });
    }
    return el('span', { class: 'status-pill stopped', text: nic.operstate || 'unknown' });
  }

  // "Gen3 x8" plus, if the link runs below what the device and slot
  // support, the capability - a common cause of throughput limits.
  function pcieLink(pci) {
    if (!pci || !pci.link_gen) return null;
    const current = 'Gen' + pci.link_gen + ' x' + pci.link_width;
    const degraded = (pci.max_link_gen && pci.link_gen < pci.max_link_gen) ||
      (pci.max_link_width && pci.link_width < pci.max_link_width);
    if (!degraded) return el('span', { text: current });
    return el('span', {}, [
      el('span', { class: 'nic-warn', text: current }),
      document.createTextNode(' (capable of Gen' + pci.max_link_gen + ' x' + pci.max_link_width + ')'),
    ]);
  }

  // Configured and available channels from ethtool -l, e.g.
  // "Combined 4 of 63 · Other 1 of 1", else the queues sysfs lists.
  function nicQueues(nic) {
    const ch = nic.channels;
    if (ch) {
      const parts = [['Combined', 'combined'], ['RX', 'rx'], ['TX', 'tx'], ['Other', 'other']]
        .filter(([, key]) => typeof ch.max[key] === 'number' && ch.max[key] > 0)
        .map(([label, key]) => label + ' ' + (ch.current[key] ?? 0) + ' of ' + ch.max[key]);
      if (parts.length) return parts.join(' · ');
    }
    return 'RX ' + nic.rx_queues + ' · TX ' + nic.tx_queues;
  }

  // Configured and maximum ring sizes from ethtool -g, e.g.
  // "RX 4096 of 8160 · TX 512 of 8160"; null if not available.
  function nicRings(nic) {
    const rings = nic.rings;
    if (!rings) return null;
    const parts = [['RX', 'rx'], ['RX mini', 'rx_mini'], ['RX jumbo', 'rx_jumbo'], ['TX', 'tx']]
      .filter(([, key]) => typeof rings.max[key] === 'number' && rings.max[key] > 0)
      .map(([label, key]) => label + ' ' + (rings.current[key] ?? 0) + ' of ' + rings.max[key]);
    return parts.length ? parts.join(' · ') : null;
  }

  function buildNicCard(nic) {
    const pci = nic.pci;
    const facts = [];
    const add = (term, value) => {
      if (value === null || value === undefined || value === '') return;
      facts.push(el('dt', { text: term }));
      facts.push(typeof value === 'string' ? el('dd', { text: value }) : el('dd', {}, [value]));
    };
    add('Driver', [nic.driver, nic.driver_version].filter(Boolean).join(' · '));
    add('Firmware', nic.firmware);
    add('PCI', pci ? pci.address + ' (' + pci.vendor + ':' + pci.device + ')' : nic.kind);
    add('PCIe link', pcieLink(pci));
    if (pci) add('NUMA node', pci.numa_node >= 0 ? String(pci.numa_node) : 'not reported');
    add('Queues', nicQueues(nic));
    add('Rings', nicRings(nic));
    add('MAC', nic.mac + (nic.permanent_mac && nic.permanent_mac !== nic.mac ? ' (permanent ' + nic.permanent_mac + ')' : ''));
    add('MTU', nic.mtu ? String(nic.mtu) : '');
    add('Port', [nic.port, nic.autoneg ? 'auto-negotiation ' + nic.autoneg : '', nic.duplex ? nic.duplex + ' duplex' : '']
      .filter(Boolean).join(' · '));
    if (pci && pci.sriov_total_vfs) add('SR-IOV', 'up to ' + pci.sriov_total_vfs + ' VFs');

    const model = pci && (pci.device_name || pci.vendor_name)
      ? [pci.vendor_name, pci.device_name].filter(Boolean).join(' · ')
      : '';
    return el('div', { class: 'nic-card' }, [
      el('div', { class: 'nic-card-header' }, [
        el('h3', { text: nic.name }),
        linkPill(nic),
        el('span', { class: 'spacer' }),
        el('span', { class: 'nic-speed', text: formatLinkSpeed(nic.speed) }),
      ]),
      el('p', { class: 'nic-model', text: model || (nic.driver ? nic.driver + ' (' + nic.kind + ')' : nic.kind) }),
      el('dl', { class: 'nic-facts' }, facts),
    ]);
  }

  function renderHostInterfaces() {
    if (!hostInterfaces) return;
    const nics = hostInterfaces.interfaces || [];
    // SR-IOV virtual functions are PCI devices, but not NICs of their own:
    // a host can have dozens of them, so they are listed with the virtual
    // interfaces.
    const isVF = (n) => Boolean(n.pci && n.pci.physical_function);
    const physical = nics.filter((n) => n.physical && !isVF(n));
    const virtual = nics.filter((n) => !n.physical || isVF(n));
    $('#nics-no-ethtool').hidden = hostInterfaces.ethtool;
    $('#nics-physical').replaceChildren(...physical.map(buildNicCard));
    $('#nics-physical-empty').hidden = physical.length > 0;

    const showVirtual = $('#nics-show-virtual').checked;
    $('#nics-virtual-group').hidden = !showVirtual || virtual.length === 0;
    $('#nics-virtual-tbody').replaceChildren(...virtual.map((nic) => el('tr', {}, [
      el('th', { scope: 'row', class: 'cpu-list', text: nic.name }),
      el('td', {
        text: isVF(nic)
          ? 'SR-IOV VF of ' + (nic.pci.physical_function_interface || nic.pci.physical_function)
          : nic.kind,
        title: isVF(nic) ? 'PCI ' + nic.pci.address : null,
      }),
      el('td', { text: nic.driver || '' }),
      el('td', {}, [linkPill(nic)]),
      el('td', { text: formatLinkSpeed(nic.speed) }),
      el('td', { text: nic.mtu ? String(nic.mtu) : '' }),
      el('td', { text: nic.rx_queues + ' / ' + nic.tx_queues }),
      el('td', { class: 'cpu-list', text: nic.mac || '' }),
    ])));
    $('label[for="nics-show-virtual"]').textContent = 'Show virtual interfaces (' + virtual.length + ')';
  }

  // Topology image rendered by lstopo (hwloc), on demand: rendering takes a
  // few seconds on large hosts, so it is not part of the polled data.
  $('#btn-host-topology').addEventListener('click', () => {
    const img = $('#topology-image');
    img.hidden = true;
    $('#topology-loading').hidden = false;
    setStatusMessage($('#topology-status'), '');
    img.onload = () => {
      $('#topology-loading').hidden = true;
      img.hidden = false;
    };
    img.onerror = () => {
      $('#topology-loading').hidden = true;
      setStatusMessage($('#topology-status'), 'Could not render the topology with lstopo.', 'error');
    };
    img.src = '/api/v1/host/topology.png?t=' + Date.now();
    openDialog('dialog-topology');
  });
  $('#btn-topology-fit').addEventListener('click', (ev) => {
    const fit = $('#topology-body').classList.toggle('is-fit');
    ev.currentTarget.textContent = fit ? 'Actual size' : 'Fit to window';
  });

  // The instance whose data is currently rendered in the detail view. Not
  // the same as state.currentInstance, which is cleared on the way back to
  // the dashboard: reopening the same instance can keep what is on screen
  // (it is refreshed within a poll interval anyway), switching to a
  // different one must not.
  let renderedInstance = null;

  async function openInstance(name) {
    if (renderedInstance !== name) {
      resetInstanceViews();
      renderedInstance = name;
    }
    state.currentInstance = name;
    stopHostPolling();
    $('#view-dashboard').hidden = true;
    $('#view-host').hidden = true;
    $('#view-instance').hidden = false;
    setCurrentNav('dashboard');
    $('#instance-name').textContent = name;
    if (!state.log.manualSelect) {
      $('#logdock-instance-select').value = name;
      onLogInstanceChange();
    }
    await refreshInstanceStatus();
    updateGenerateTabAvailability(name);
    activateTab('overview');
    loadCommandBuilder(true /* silent preload for stream controls */);
    startInstancePolling();
  }

  async function refreshInstanceStatus() {
    const name = state.currentInstance;
    if (!name) return;
    try {
      const s = await API.status(name);
      const running = s.status === 'started';
      const pill = $('#instance-status-pill');
      pill.textContent = s.status;
      pill.className = 'status-pill ' + (running ? 'started' : 'stopped');
      $('#btn-instance-start').disabled = running;
      $('#btn-instance-stop').disabled = !running;
      $('#btn-instance-kill').disabled = !running;
    } catch (e) {
      announce('Failed to refresh status for ' + name, 'error');
    }
    updateConfigTabAvailability(name);
    // A start or stop changes what the Generate tab may write.
    if (isTabActive('generate')) initGenerateView();
  }
  $('#btn-instance-refresh').addEventListener('click', refreshInstanceStatus);

  // Read-only view of the stored config.json. The config is re-serialized
  // rather than shown as stored, so it is always consistently indented, and
  // highlighted with the JSON editor's tokenizer to match its colors.
  async function showInstanceConfig() {
    const name = state.currentInstance;
    if (!name) return;
    const code = $('#instance-config-code');
    $('#dialog-instance-config-title').textContent = 'Configuration — ' + name;
    code.textContent = 'Loading…';
    openDialog('dialog-instance-config');
    try {
      const text = JSON.stringify(await API.getConfig(name), null, 2);
      const { tokens } = jsonParseLenient(text);
      code.innerHTML = renderJSONHighlightHTML(text, tokens, []);
    } catch (e) {
      code.textContent = 'Could not load configuration: ' + e.message;
    }
  }
  $('#btn-instance-config').addEventListener('click', showInstanceConfig);
  $('#btn-instance-start').addEventListener('click', () => openStartDialog(state.currentInstance));
  $('#btn-instance-stop').addEventListener('click', () => { doStop(state.currentInstance).then(refreshInstanceStatus); });
  $('#btn-instance-kill').addEventListener('click', () => { doKill(state.currentInstance).then(refreshInstanceStatus); });

  // Compact "1h 02m 03s" rendering of a duration given in seconds (the unit
  // reported by the bngblaster "test-info" socket command).
  function formatDuration(totalSeconds) {
    const s = Math.max(0, Math.floor(totalSeconds) || 0);
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    const sec = s % 60;
    if (h > 0) return h + 'h ' + String(m).padStart(2, '0') + 'm ' + String(sec).padStart(2, '0') + 's';
    if (m > 0) return m + 'm ' + String(sec).padStart(2, '0') + 's';
    return sec + 's';
  }

  // test-info reports the running test's overall state ("active", "stopped",
  // ...) and elapsed duration in seconds. Rendered from the shared overview
  // poll so the header badge stays current across every tab, not just while
  // the Overview tab is visible.
  //
  // Newer bngblaster versions open the control socket at the beginning of
  // the startup and additionally report a status ("starting", "running",
  // "stopping") and the current task. While starting or stopping, the task
  // ("wait-startup-lock", "init-interfaces", "teardown", ...) says more than
  // the state and the duration, which stays 0 until the test has started.
  function renderTestDuration(testInfo) {
    const pill = $('#instance-duration-pill');
    if (!testInfo || typeof testInfo.duration !== 'number') {
      // Instance not running (anymore), or bngblaster predates test-info.
      pill.hidden = true;
      return;
    }
    pill.hidden = false;
    if (testInfo.status === 'starting' || testInfo.status === 'stopping') {
      let text = prettyLabel(testInfo.status);
      if (testInfo.task) text += ' · ' + prettyLabel(testInfo.task);
      if (testInfo['startup-lock-pid']) text += ' (PID ' + testInfo['startup-lock-pid'] + ')';
      if (testInfo.status === 'stopping' && testInfo.duration) text += ' · ' + formatDuration(testInfo.duration);
      pill.textContent = text;
      pill.title = testInfo['startup-lock-pid']
        ? 'Waiting for the startup of another BNG Blaster instance (PID ' + testInfo['startup-lock-pid'] + ')'
        : '';
      return;
    }
    pill.title = '';
    pill.textContent = (testInfo.state ? prettyLabel(testInfo.state) + ' · ' : '') + formatDuration(testInfo.duration);
  }

  //= ========================================================================
  // 3a. Session overview + progress bars
  //= ========================================================================
  function meter(labelText, value, max, variant) {
    // Three decimals, rounded down: with thousands of sessions a plain
    // rounded percentage reads "100%" while some are still missing (e.g.
    // 9999 / 10000), so only a truly complete bar may show 100.000%.
    // Counts are integers, so multiplying before dividing keeps this exact
    // (29 / 100 must not floor to 28.999%).
    const milli = max > 0 ? Math.min(100000, Math.floor((value * 100000) / max)) : 0;
    const pct = (milli / 1000).toFixed(3);
    let cls = 'session-meter';
    if (variant) cls += ' session-meter--' + variant;
    if (max > 0 && value >= max) cls += ' is-complete';
    const wrap = el('div', { class: cls });
    const id = nextId('meter');
    wrap.appendChild(el('div', { class: 'meter-label' }, [
      el('span', { id: id, text: labelText }),
      el('span', { class: 'count', text: value.toLocaleString() + ' / ' + max.toLocaleString() + ' (' + pct + '%)' }),
    ]));
    wrap.appendChild(el('progress', { value: String(value), max: String(Math.max(max, 1)), 'aria-labelledby': id }));
    return wrap;
  }

  function statTile(label, value) {
    return el('div', { class: 'stat-tile' }, [
      el('div', { class: 'stat-value', text: String(value) }),
      el('div', { class: 'stat-label', text: label }),
    ]);
  }

  function clearSessionCounters() {
    $('#overview-empty').hidden = false;
    $('#overview-content').hidden = true;
    $('#overview-stat-tiles').innerHTML = '';
    $('#overview-meters').innerHTML = '';
  }

  function renderSessionCounters(c) {
    // A test does not have to have sessions: a pure stream test reports
    // sessions: 0 alongside a non-zero stream-traffic-flows. Keying the
    // empty state off the session count alone therefore blanked the whole
    // overview for those runs, hiding the stream traffic progress along
    // with everything else.
    const reported = !!c && (c['sessions'] || c['session-traffic-flows'] || c['stream-traffic-flows']);
    if (!reported) {
      // Instance not running (anymore), or nothing reported yet: reset to the
      // empty state instead of leaving stale counters on screen.
      clearSessionCounters();
      return;
    }
    $('#overview-empty').hidden = true;
    $('#overview-content').hidden = false;

    // Stat tiles + progress bars mirror the default view of the bngblaster
    // interactive (ncurses) UI: session counts/breakdown, setup time/rate
    // (current, min, avg, max) and flapped count.
    const tiles = $('#overview-stat-tiles');
    tiles.innerHTML = '';
    [
      ['Sessions', c['sessions']],
      ['PPPoE', c['sessions-pppoe']],
      ['IPoE', c['sessions-ipoe']],
      ['Outstanding', c['sessions-outstanding']],
      ['Flapped', c['sessions-flapped']],
      ['Setup Time (ms)', c['setup-time']],
      ['Setup Rate (CPS)', (c['setup-rate'] || 0).toFixed(2)],
      ['Setup Rate Min', (c['setup-rate-min'] || 0).toFixed(2)],
      ['Setup Rate Avg', (c['setup-rate-avg'] || 0).toFixed(2)],
      ['Setup Rate Max', (c['setup-rate-max'] || 0).toFixed(2)],
    ].forEach(([l, v]) => tiles.appendChild(statTile(l, v)));

    const meters = $('#overview-meters');
    meters.innerHTML = '';
    const total = c['sessions'] || 0;
    // A stream-only test has no sessions at all; "0 / 0 (0%)" bars would be
    // pure noise there, so the session meters appear only once there are
    // sessions to report on.
    if (total) {
      meters.appendChild(meter('Sessions established', c['sessions-established'] || 0, total));
      meters.appendChild(meter('Sessions outstanding', c['sessions-outstanding'] || 0, total, 'outstanding'));
      meters.appendChild(meter('Sessions terminated', c['sessions-terminated'] || 0, total, 'terminated'));
    }
    if (c['dhcp-sessions']) meters.appendChild(meter('DHCPv4', c['dhcp-sessions-established'] || 0, c['dhcp-sessions']));
    if (c['dhcpv6-sessions']) meters.appendChild(meter('DHCPv6', c['dhcpv6-sessions-established'] || 0, c['dhcpv6-sessions']));
    // Session traffic (the per-session flows bngblaster sets up itself) and
    // stream traffic (the configured traffic streams) are independent: a run
    // can have either, both or neither, so each gets its own progress bar
    // whenever that kind of flow is present.
    if (c['session-traffic-flows']) {
      meters.appendChild(meter('Session traffic flows verified',
        c['session-traffic-flows-verified'] || 0, c['session-traffic-flows']));
    }
    if (c['stream-traffic-flows']) {
      meters.appendChild(meter('Stream traffic flows verified',
        c['stream-traffic-flows-verified'] || 0, c['stream-traffic-flows']));
    }
  }

  // Renders the packets/PPS/throughput breakdown for one interface (as
  // reported by the network-interfaces / access-interfaces / a10nsp-interfaces
  // socket commands) as a small card — the web equivalent of the interface
  // sections in the bngblaster interactive (ncurses) UI.
  function fmtPPS(v) { return (v || 0).toLocaleString() + ' pps'; }
  function fmtKbps(kbps) {
    if (!kbps) return '0 kbps';
    if (kbps >= 1000000) return (kbps / 1000000).toFixed(2) + ' Gbps';
    if (kbps >= 1000) return (kbps / 1000).toFixed(2) + ' Mbps';
    return kbps.toLocaleString() + ' kbps';
  }
  // Builds an RX cell with the packet count and, when bngblaster reports a
  // loss counter for that traffic type, the lost packets plus the loss ratio.
  // The ratio is relative to the packets that should have arrived (received
  // + lost), with three decimals so a handful of drops in a long run still
  // shows up instead of rounding to 0%. Any loss is highlighted in red.
  function rxWithLoss(rx, pps, loss) {
    const cell = el('span', { text: (rx || 0).toLocaleString() + ' (' + fmtPPS(pps) + ')' });
    if (loss === undefined) return cell;
    const expected = (rx || 0) + loss;
    const pct = expected ? (loss / expected) * 100 : 0;
    cell.appendChild(document.createTextNode(' · '));
    cell.appendChild(el('span', {
      class: loss > 0 ? 'rx-loss-nonzero' : '',
      text: 'loss ' + loss.toLocaleString() + ' (' + pct.toFixed(3) + '%)',
    }));
    return cell;
  }
  function buildInterfaceStatCard(iface) {
    const rows = [];
    rows.push(['Packets',
      (iface['tx-packets'] || 0).toLocaleString() + ' (' + fmtPPS(iface['tx-pps']) + ')',
      (iface['rx-packets'] || 0).toLocaleString() + ' (' + fmtPPS(iface['rx-pps']) + ')']);
    if (iface['tx-kbps'] !== undefined || iface['rx-kbps'] !== undefined) {
      rows.push(['Throughput', fmtKbps(iface['tx-kbps']), fmtKbps(iface['rx-kbps'])]);
    }
    ['ipv4', 'ipv6', 'ipv6pd'].forEach((fam) => {
      const txKey = 'tx-packets-session-' + fam;
      const rxKey = 'rx-packets-session-' + fam;
      if (iface[txKey] === undefined && iface[rxKey] === undefined) return;
      rows.push(['Session ' + prettyLabel(fam),
        (iface[txKey] || 0).toLocaleString() + ' (' + fmtPPS(iface['tx-pps-session-' + fam]) + ')',
        rxWithLoss(iface[rxKey], iface['rx-pps-session-' + fam], iface['rx-loss-packets-session-' + fam])]);
    });
    if (iface['tx-packets-streams'] !== undefined || iface['rx-packets-streams'] !== undefined) {
      rows.push(['Stream Traffic',
        (iface['tx-packets-streams'] || 0).toLocaleString() + ' (' + fmtPPS(iface['tx-pps-streams']) + ')',
        rxWithLoss(iface['rx-packets-streams'], iface['rx-pps-streams'], iface['rx-loss-packets-streams'])]);
    }
    if (iface['rx-packets-multicast'] !== undefined) {
      rows.push(['Multicast', '—',
        rxWithLoss(iface['rx-packets-multicast'], iface['rx-pps-multicast'], iface['rx-loss-packets-multicast'])]);
    }
    const table = el('table', { class: 'iface-stat-table' }, [
      el('thead', {}, [el('tr', {}, [el('th', { scope: 'col', text: '' }), el('th', { scope: 'col', text: 'TX' }), el('th', { scope: 'col', text: 'RX' })])]),
      el('tbody', {}, rows.map(([label, tx, rx]) => el('tr', {}, [
        el('th', { scope: 'row', text: label }),
        el('td', { text: tx }),
        // RX is either plain text or a node carrying the loss highlight.
        typeof rx === 'string' ? el('td', { text: rx }) : el('td', {}, [rx]),
      ]))),
    ]);
    return el('div', { class: 'iface-card' }, [
      el('h4', { text: iface.name + (iface.type ? ' (' + iface.type + ')' : '') }),
      table,
    ]);
  }

  const INTERFACE_STAT_GROUPS = [
    ['network-interfaces', 'overview-network-interfaces', 'group-network-interfaces'],
    ['access-interfaces', 'overview-access-interfaces', 'group-access-interfaces'],
    ['a10nsp-interfaces', 'overview-a10nsp-interfaces', 'group-a10nsp-interfaces'],
  ];
  function renderInterfaceStats(overview) {
    let any = false;
    INTERFACE_STAT_GROUPS.forEach(([key, containerId, groupId]) => {
      const container = $('#' + containerId);
      container.innerHTML = '';
      const list = overview && overview[key];
      if (!Array.isArray(list) || !list.length) { $('#' + groupId).hidden = true; return; }
      any = true;
      $('#' + groupId).hidden = false;
      list.forEach((iface) => container.appendChild(buildInterfaceStatCard(iface)));
    });
    $('#overview-interfaces').hidden = !any;
  }

  // A single poll drives everything on the instance detail view: the header
  // duration badge (on every tab) and, while it is visible, the Session
  // Overview tab's counters and interface cards. It hits one aggregated,
  // server-cached endpoint rather than the five separate control-socket
  // commands this used to issue every two seconds per open browser tab.
  //
  // The response is also what sizes the stream and session virtual
  // scrollbars, so their totals are refreshed here for free.
  async function pollOverview() {
    const name = state.currentInstance;
    if (!name) return null;
    let overview;
    try {
      overview = await API.overview(name);
    } catch (e) {
      // Not running, or unreachable: drop the counters rather than leaving
      // stale ones on screen, and reset the totals that size the stream and
      // session scrollbars.
      renderTestDuration(null);
      state.stream.total = 0;
      state.session.total = 0;
      updateTabAvailability(null);
      if (isTabActive('overview')) {
        clearSessionCounters();
        renderInterfaceStats(null);
      }
      syncGenerateRunning(false);
      return null;
    }
    syncGenerateRunning(true);
    renderTestDuration(overview['test-info']);
    const counters = overview['session-counters'];
    if (counters) {
      state.stream.total = (counters['session-traffic-flows'] || 0) + (counters['stream-traffic-flows'] || 0);
      state.session.total = counters['sessions'] || 0;
    }
    // Counters absent means "not reported", not "zero".
    updateTabAvailability(counters);
    if (isTabActive('overview')) {
      renderSessionCounters(counters);
      renderInterfaceStats(overview);
    }
    return overview;
  }

  function startInstancePolling() {
    stopInstancePolling();
    pollOverview();
    state.overviewTimer = schedulePoll(pollOverview, 2000);
  }
  function stopInstancePolling() {
    state.overviewTimer = cancelPoll(state.overviewTimer);
  }

  //= ========================================================================
  // 3b. Stream summary — virtual scrolling + controls
  //= ========================================================================
  function initStreamView() {
    const sameInstance = state.stream.instance === state.currentInstance;
    state.stream.instance = state.currentInstance;
    if (sameInstance) fetchStreamWindow();
    else refreshStreamTotal().then(fetchStreamWindow);
    startStreamPolling();
  }

  // Keeps the currently visible stream window live while the Streams tab is
  // open, re-fetching just that window (fetchStreamWindow always recomputes
  // it from the viewport's current scroll position) every 2s. The server
  // itself caches stream-summary responses for 2s per instance/filter
  // combination, so this lines up with that instead of forcing a socket
  // round-trip on every tick.
  function startStreamPolling() {
    stopStreamPolling();
    state.stream.pollTimer = schedulePoll(fetchStreamWindow, 2000);
  }
  function stopStreamPolling() {
    state.stream.pollTimer = cancelPoll(state.stream.pollTimer);
  }

  // Reads the stream-summary filters (mirrors the arguments accepted by the
  // bngblaster "stream-summary" control command) from the Filters panel.
  function collectStreamFilters() {
    const flowId = $('#filter-flow-id').value.trim();
    return {
      'session-id': $('#filter-session-id').value.trim(),
      'session-group-id': $('#filter-session-group-id').value.trim(),
      'flow-id': flowId,
      // An exact flow id identifies a single stream, so the range bounds are
      // dropped rather than sent alongside it - the field hint promises that
      // it "overrides the min/max range below".
      'flow-id-min': flowId ? '' : $('#filter-flow-id-min').value.trim(),
      'flow-id-max': flowId ? '' : $('#filter-flow-id-max').value.trim(),
      name: $('#filter-name').value.trim(),
      interface: $('#filter-interface').value.trim(),
      direction: $('#filter-direction').value,
      state: $('#filter-state').value,
    };
  }

  // True once the user has actually applied a filter from the panel (as
  // opposed to the flow-id-min/max the virtual-scroll window itself adds —
  // see fetchStreamWindow).
  function streamFiltersActive() {
    return Object.values(state.stream.filters || {}).some((v) => v !== '' && v !== undefined && v !== null);
  }

  function applyStreamFilters() {
    state.stream.filters = collectStreamFilters();
    $('#stream-viewport').scrollTop = 0;
    if (streamFiltersActive()) fetchStreamWindow();
    else refreshStreamTotal().then(fetchStreamWindow);
  }
  $('#btn-stream-filters-apply').addEventListener('click', applyStreamFilters);
  $('#btn-stream-filters-clear').addEventListener('click', () => {
    $('#stream-filters-form').reset();
    applyStreamFilters();
  });

  // The grand total stream count, used to size the virtual-scroll spacer.
  // Every configured stream (session-traffic or not) is added to the same
  // flow-id chain that "stream-summary" walks, so session-traffic-flows +
  // stream-traffic-flows from session-counters gives the true total — one
  // cheap call instead of fetching the (potentially huge) full stream list
  // just to count it.
  async function refreshStreamTotal() {
    // pollOverview() already keeps state.stream.total current from the same
    // cached endpoint; this is the explicit refresh for the paths that need
    // the total before the next poll tick (opening the tab, Refresh, or
    // clearing filters).
    await pollOverview();
  }

  function computeVisibleRange() {
    const viewport = $('#stream-viewport');
    const rh = state.stream.rowHeight;
    const scrollTop = viewport.scrollTop;
    const visibleCount = Math.ceil(viewport.clientHeight / rh) || 10;
    const first = Math.floor(scrollTop / rh);
    const start = Math.max(0, first - state.stream.buffer);
    const limit = Math.min(500, visibleCount + state.stream.buffer * 2);
    return { start, limit };
  }

  async function fetchStreamWindow() {
    const name = state.currentInstance;
    if (!name) return;
    const { start, limit } = computeVisibleRange();
    const manualFilters = streamFiltersActive();
    const requestFilters = Object.assign({}, state.stream.filters);
    if (!manualFilters) {
      // No manual filter: narrow the request to exactly the visible
      // window's flow-id range (flow ids are assigned sequentially from 1)
      // so bngblaster only has to look up and serialize the handful of
      // streams we are about to render, instead of the entire stream list,
      // every couple of seconds while this tab is open.
      requestFilters['flow-id-min'] = start + 1;
      requestFilters['flow-id-max'] = start + limit;
      // Tells the server this range came from the scroll position, not from
      // the filter panel. A user-entered range is paginated normally (offset
      // is a row index, total is the filtered count); a window is returned
      // as-is with offset being its absolute first row. Conflating the two
      // made a manual range filter compute its spacer heights and row counts
      // from a flow id instead of a row index.
      requestFilters.window = '1';
    }
    let resp;
    try {
      resp = await API.streams(name, start, limit, requestFilters);
    } catch (e) {
      if (e.status === 412) {
        $('#streams-empty').hidden = false;
        $('#streams-empty').textContent = 'Instance is not running.';
        $('#streams-content').hidden = true;
      }
      return;
    }
    // In windowed (unfiltered) mode resp.total only counts this narrow
    // flow-id slice, not every stream, so the separately tracked grand
    // total is used for the scrollbar instead.
    const offset = manualFilters ? resp.offset : start;
    let total = manualFilters ? resp.total : (state.stream.total || resp.total);
    // The cached grand total (from session-counters, refreshed once per
    // tab-open) can be off by a little from the real highest flow-id. If
    // this window got fewer streams back than it asked for, it has reached
    // the real end of the flow-id chain - lock the total to that exact
    // boundary. Otherwise, at the tail, the assumed and real totals disagree
    // and the bottom spacer's height flips by one row on every 2s poll,
    // which is what made the scrollbar visibly bounce when scrolled to the
    // end.
    if (!manualFilters && resp.items.length < limit) total = offset + resp.items.length;
    state.stream.total = total;
    if (total === 0) {
      $('#streams-empty').hidden = false;
      $('#streams-content').hidden = true;
      return;
    }
    $('#streams-empty').hidden = true;
    $('#streams-content').hidden = false;
    renderStreamWindow(offset, resp.items, total);
    $('#stream-count-hint').textContent = 'Showing rows ' + (offset + 1) + '–' + (offset + resp.items.length) + ' of ' + total + '.';
  }

  function findCommand(instance, matchStart, matchStop) {
    const list = state.instanceCommands[instance] || [];
    const start = list.find((c) => /stream/i.test(c.name) && matchStart.test(c.name));
    const stop = list.find((c) => /stream/i.test(c.name) && matchStop.test(c.name));
    return { start, stop };
  }

  function primaryArgName(cmd, flowId) {
    if (!cmd || !cmd.args || !cmd.args.length) return null;
    const preferred = cmd.args.find((a) => /flow[-_]?id/i.test(a.name)) || cmd.args.find((a) => a.required) || cmd.args[0];
    return preferred ? preferred.name : null;
  }

  async function toggleStream(name, flowId, isStart) {
    const { start, stop } = findCommand(name, /start|enable|resume/i, /stop|disable|pause/i);
    const cmd = isStart ? start : stop;
    if (!cmd) {
      announce('No matching stream ' + (isStart ? 'start' : 'stop') + ' command was discovered for this instance.', 'error');
      return;
    }
    const argName = primaryArgName(cmd, flowId) || 'flow-id';
    try {
      await API.command(name, cmd.name, { [argName]: flowId });
      announce((isStart ? 'Started' : 'Stopped') + ' stream ' + flowId, 'success');
      fetchStreamWindow();
    } catch (e) {
      announce('Command failed: ' + e.message, 'error');
    }
  }

  function renderStreamWindow(offset, items, total) {
    const tbody = $('#stream-tbody');
    tbody.innerHTML = '';
    const rh = state.stream.rowHeight;
    const topH = offset * rh;
    const bottomH = Math.max(0, (total - offset - items.length)) * rh;
    tbody.appendChild(el('tr', {}, [el('td', { colspan: '11', style: 'padding:0;border:0;height:' + topH + 'px' })]));
    items.forEach((item) => {
      const lossClass = item['rx-loss'] > 0 ? 'rx-loss-nonzero' : '';
      const enabled = !!item.enabled;
      const flag = (label, on) => (on ? el('span', { class: 'badge badge-yes', text: label }) : null);
      tbody.appendChild(el('tr', {}, [
        el('td', { text: String(item['flow-id']) }),
        el('td', { text: item.name || '' }),
        el('td', { text: (item.type || '') + (item['sub-type'] ? ' / ' + item['sub-type'] : '') }),
        el('td', { text: item.direction || '' }),
        el('td', {}, [
          el('div', { class: 'stream-flags-cell' }, [
            flag('Active', enabled && !!item.active),
            flag('Verified', !!item.verified),
          ]),
        ]),
        el('td', { text: (item['tx-packets'] || 0).toLocaleString() }),
        el('td', { text: (item['tx-pps'] || 0).toLocaleString() }),
        el('td', { text: (item['rx-packets'] || 0).toLocaleString() }),
        el('td', { text: (item['rx-pps'] || 0).toLocaleString() }),
        el('td', { class: lossClass, text: (item['rx-loss'] || 0).toLocaleString() }),
        el('td', {}, [
          el('div', { class: 'row-actions-cell' }, [
            enabled
              ? el('button', { class: 'btn btn-sm', type: 'button', text: 'Stop', onclick: () => toggleStream(state.currentInstance, item['flow-id'], false) })
              : el('button', { class: 'btn btn-sm', type: 'button', text: 'Start', onclick: () => toggleStream(state.currentInstance, item['flow-id'], true) }),
            el('button', { class: 'btn btn-sm', type: 'button', text: 'Detail', onclick: () => showStreamDetail(item) }),
          ]),
        ]),
      ]));
    });
    tbody.appendChild(el('tr', {}, [el('td', { colspan: '11', style: 'padding:0;border:0;height:' + bottomH + 'px' })]));
  }

  // Shared by the stream and session detail dialogs: both execute a
  // "*-info" command and print every field the response returns as-is.
  // Array/object values (e.g. a session's nested "streams" list) are
  // pretty-printed and shown in a monospace block that spans the full row
  // width, instead of being squashed into a single-line JSON string inside
  // the narrow value column.
  function renderDetailFields(dlId, fields) {
    const dl = $(dlId);
    dl.innerHTML = '';
    Object.entries(fields).forEach(([k, v]) => {
      const isComplex = typeof v === 'object' && v !== null;
      dl.appendChild(el('dt', { text: k }));
      const dd = el('dd', { class: isComplex ? 'detail-value-wide' : null });
      if (isComplex) {
        dd.appendChild(el('pre', { class: 'detail-value-json', text: JSON.stringify(v, null, 2) }));
      } else {
        dd.textContent = String(v);
      }
      dl.appendChild(dd);
    });
  }

  // Renders a *-info / ad-hoc command result via renderDetailFields: a
  // plain object is shown key-by-key, everything else (a bare array,
  // string, number, ...) is wrapped under a single "result" field so it
  // still gets the same pretty-printing instead of being dumped as an
  // opaque blob. Shared by the stream/session detail dialogs and the
  // Commands tab's generic command output.
  function renderResultFields(dlId, result) {
    if (result && typeof result === 'object' && !Array.isArray(result)) {
      renderDetailFields(dlId, result);
    } else {
      renderDetailFields(dlId, { result: result });
    }
  }

  // showLoading is false for the 2s auto-refresh ticks so the field list
  // isn't blanked out to a "Loading…" placeholder every cycle - only the
  // very first load (and a manual Refresh click) show it.
  async function loadStreamDetail(showLoading) {
    const instance = state.currentInstance;
    const flowId = state.stream.detailFlowId;
    if (!instance || flowId == null) return;
    $('#dialog-stream-detail-title').textContent = 'Stream Detail — Flow ' + flowId;
    if (showLoading) renderDetailFields('#stream-detail-list', { Loading: '…' });
    try {
      const resp = await API.command(instance, 'stream-info', { 'flow-id': flowId });
      renderResultFields('#stream-detail-list', splitCommandResponse(resp).result);
    } catch (e) {
      renderDetailFields('#stream-detail-list', { Error: e.message });
    }
  }

  function stopStreamDetailPolling() {
    state.stream.detailTimer = cancelPoll(state.stream.detailTimer);
  }

  function showStreamDetail(item) {
    state.stream.detailFlowId = item['flow-id'];
    openDialog('dialog-stream-detail');
    loadStreamDetail(true);
    stopStreamDetailPolling();
    state.stream.detailTimer = schedulePoll(() => loadStreamDetail(false), 2000);
  }

  // The native <dialog> "close" event also fires for Escape/backdrop
  // dismissal, not just the Close button, so this is the one place that
  // reliably stops the auto-refresh whenever the dialog goes away.
  $('#dialog-stream-detail').addEventListener('close', stopStreamDetailPolling);

  $('#btn-stream-detail-refresh').addEventListener('click', () => loadStreamDetail(true));

  $('#stream-viewport').addEventListener('scroll', () => {
    if (state.stream.timer) clearTimeout(state.stream.timer);
    state.stream.timer = setTimeout(fetchStreamWindow, 100);
  });
  $('#btn-streams-refresh').addEventListener('click', () => {
    if (streamFiltersActive()) fetchStreamWindow();
    else refreshStreamTotal().then(fetchStreamWindow);
  });

  //= ========================================================================
  // 3b2. Session summary — virtual scrolling + controls (mirrors 3b)
  //= ========================================================================
  function initSessionView() {
    const sameInstance = state.session.instance === state.currentInstance;
    state.session.instance = state.currentInstance;
    if (sameInstance) fetchSessionWindow();
    else refreshSessionTotal().then(fetchSessionWindow);
    startSessionPolling();
  }

  function startSessionPolling() {
    stopSessionPolling();
    state.session.pollTimer = schedulePoll(fetchSessionWindow, 2000);
  }
  function stopSessionPolling() {
    state.session.pollTimer = cancelPoll(state.session.pollTimer);
  }

  // Reads the session-summary filters (mirrors the arguments accepted by the
  // bngblaster "session-summary" control command) from the Filters panel.
  function collectSessionFilters() {
    const sessionId = $('#session-filter-session-id').value.trim();
    return {
      'session-id': sessionId,
      // As in collectStreamFilters: an exact session id overrides the range.
      'session-id-min': sessionId ? '' : $('#session-filter-session-id-min').value.trim(),
      'session-id-max': sessionId ? '' : $('#session-filter-session-id-max').value.trim(),
      'session-group-id': $('#session-filter-session-group-id').value.trim(),
    };
  }

  // True once the user has actually applied a filter from the panel (as
  // opposed to the session-id-min/max the virtual-scroll window itself adds
  // - see fetchSessionWindow).
  function sessionFiltersActive() {
    return Object.values(state.session.filters || {}).some((v) => v !== '' && v !== undefined && v !== null);
  }

  function applySessionFilters() {
    state.session.filters = collectSessionFilters();
    $('#session-viewport').scrollTop = 0;
    if (sessionFiltersActive()) fetchSessionWindow();
    else refreshSessionTotal().then(fetchSessionWindow);
  }
  $('#btn-session-filters-apply').addEventListener('click', applySessionFilters);
  $('#btn-session-filters-clear').addEventListener('click', () => {
    $('#session-filters-form').reset();
    applySessionFilters();
  });

  // The grand total session count, used to size the virtual-scroll spacer -
  // one cheap session-counters call instead of fetching the (potentially
  // huge) full session list just to count it.
  async function refreshSessionTotal() {
    // See refreshStreamTotal: same cached endpoint, same reason.
    await pollOverview();
  }

  function computeSessionVisibleRange() {
    const viewport = $('#session-viewport');
    const rh = state.session.rowHeight;
    const scrollTop = viewport.scrollTop;
    const visibleCount = Math.ceil(viewport.clientHeight / rh) || 10;
    const first = Math.floor(scrollTop / rh);
    const start = Math.max(0, first - state.session.buffer);
    const limit = Math.min(500, visibleCount + state.session.buffer * 2);
    return { start, limit };
  }

  async function fetchSessionWindow() {
    const name = state.currentInstance;
    if (!name) return;
    const { start, limit } = computeSessionVisibleRange();
    const manualFilters = sessionFiltersActive();
    const requestFilters = Object.assign({}, state.session.filters);
    if (!manualFilters) {
      // No manual filter: narrow the request to exactly the visible
      // window's session-id range (session ids are assigned sequentially
      // from 1) so bngblaster only has to look up and serialize the
      // handful of sessions we are about to render, instead of the entire
      // session list, every couple of seconds while this tab is open.
      requestFilters['session-id-min'] = start + 1;
      requestFilters['session-id-max'] = start + limit;
      // See fetchStreamWindow: distinguishes a scroller window from a
      // user-entered session-id range.
      requestFilters.window = '1';
    }
    let resp;
    try {
      resp = await API.sessions(name, start, limit, requestFilters);
    } catch (e) {
      if (e.status === 412) {
        $('#sessions-empty').hidden = false;
        $('#sessions-empty').textContent = 'Instance is not running.';
        $('#sessions-content').hidden = true;
      }
      return;
    }
    // In windowed (unfiltered) mode resp.total only counts this narrow
    // session-id slice, not every session, so the separately tracked grand
    // total is used for the scrollbar instead.
    const offset = manualFilters ? resp.offset : start;
    let total = manualFilters ? resp.total : (state.session.total || resp.total);
    // See the equivalent guard in fetchStreamWindow: if this window got
    // fewer sessions back than requested, it has reached the real end of
    // the session-id chain, so lock the total to that exact boundary
    // instead of trusting the cached estimate.
    if (!manualFilters && resp.items.length < limit) total = offset + resp.items.length;
    state.session.total = total;
    if (total === 0) {
      $('#sessions-empty').hidden = false;
      $('#sessions-content').hidden = true;
      return;
    }
    $('#sessions-empty').hidden = true;
    $('#sessions-content').hidden = false;
    renderSessionWindow(offset, resp.items, total);
    $('#session-count-hint').textContent = 'Showing rows ' + (offset + 1) + '–' + (offset + resp.items.length) + ' of ' + total + '.';
  }

  async function toggleSession(name, sessionId, isStart) {
    try {
      await API.command(name, isStart ? 'session-start' : 'session-stop', { 'session-id': sessionId });
      announce((isStart ? 'Started' : 'Stopped') + ' session ' + sessionId, 'success');
      fetchSessionWindow();
    } catch (e) {
      announce('Command failed: ' + e.message, 'error');
    }
  }

  function renderSessionWindow(offset, items, total) {
    const tbody = $('#session-tbody');
    tbody.innerHTML = '';
    const rh = state.session.rowHeight;
    const topH = offset * rh;
    const bottomH = Math.max(0, (total - offset - items.length)) * rh;
    tbody.appendChild(el('tr', {}, [el('td', { colspan: '10', style: 'padding:0;border:0;height:' + topH + 'px' })]));
    items.forEach((item) => {
      const established = item['session-state'] === 'Established';
      const vlan = item['inner-vlan'] ? (item['outer-vlan'] + '.' + item['inner-vlan']) : String(item['outer-vlan'] || 0);
      tbody.appendChild(el('tr', {}, [
        el('td', { text: String(item['session-id']) }),
        el('td', { text: item.type || '' }),
        el('td', { text: item['session-state'] || '' }),
        el('td', { text: item.interface || '' }),
        el('td', { text: vlan }),
        el('td', { text: item.username || '' }),
        el('td', { text: item['ipv4-address'] || '' }),
        el('td', { text: (item['tx-packets'] || 0).toLocaleString() }),
        el('td', { text: (item['rx-packets'] || 0).toLocaleString() }),
        el('td', {}, [
          el('div', { class: 'row-actions-cell' }, [
            established
              ? el('button', { class: 'btn btn-sm', type: 'button', text: 'Stop', onclick: () => toggleSession(state.currentInstance, item['session-id'], false) })
              : el('button', { class: 'btn btn-sm', type: 'button', text: 'Start', onclick: () => toggleSession(state.currentInstance, item['session-id'], true) }),
            el('button', { class: 'btn btn-sm', type: 'button', text: 'Detail', onclick: () => showSessionDetail(item) }),
          ]),
        ]),
      ]));
    });
    tbody.appendChild(el('tr', {}, [el('td', { colspan: '10', style: 'padding:0;border:0;height:' + bottomH + 'px' })]));
  }

  // showLoading is false for the 2s auto-refresh ticks so the field list
  // isn't blanked out to a "Loading…" placeholder every cycle - only the
  // very first load (and a manual Refresh click) show it.
  async function loadSessionDetail(showLoading) {
    const instance = state.currentInstance;
    const sessionId = state.session.detailSessionId;
    if (!instance || sessionId == null) return;
    $('#dialog-session-detail-title').textContent = 'Session Detail — Session ' + sessionId;
    if (showLoading) renderDetailFields('#session-detail-list', { Loading: '…' });
    try {
      const resp = await API.command(instance, 'session-info', { 'session-id': sessionId });
      renderResultFields('#session-detail-list', splitCommandResponse(resp).result);
    } catch (e) {
      renderDetailFields('#session-detail-list', { Error: e.message });
    }
  }

  function stopSessionDetailPolling() {
    state.session.detailTimer = cancelPoll(state.session.detailTimer);
  }

  function showSessionDetail(item) {
    state.session.detailSessionId = item['session-id'];
    openDialog('dialog-session-detail');
    loadSessionDetail(true);
    stopSessionDetailPolling();
    state.session.detailTimer = schedulePoll(() => loadSessionDetail(false), 2000);
  }

  // The native <dialog> "close" event also fires for Escape/backdrop
  // dismissal, not just the Close button, so this is the one place that
  // reliably stops the auto-refresh whenever the dialog goes away.
  $('#dialog-session-detail').addEventListener('close', stopSessionDetailPolling);

  $('#btn-session-detail-refresh').addEventListener('click', () => loadSessionDetail(true));

  $('#session-viewport').addEventListener('scroll', () => {
    if (state.session.timer) clearTimeout(state.session.timer);
    state.session.timer = setTimeout(fetchSessionWindow, 100);
  });
  $('#btn-sessions-refresh').addEventListener('click', () => {
    if (sessionFiltersActive()) fetchSessionWindow();
    else refreshSessionTotal().then(fetchSessionWindow);
  });

  //= ========================================================================
  // 3b3. BGP sessions
  //= ========================================================================
  // Unlike Sessions and Streams, BGP has no counter in session-counters that
  // could drive the tab's visibility, and polling bgp-sessions from the
  // overview for every instance just to find out would cost a socket round
  // trip per tick for the (common) runs without BGP. The instance config
  // already answers the question - BGP sessions exist only if it has a
  // non-empty "bgp" section - and is a plain file read that also works while
  // the instance is stopped. The same read also decides the ISIS, OSPF and
  // LAG tabs, which have nothing to show unless the config defines ISIS or
  // OSPF instances or LAG interfaces.
  async function updateConfigTabAvailability(name) {
    const present = (v) => (Array.isArray(v) ? v.length > 0 : (!!v && typeof v === 'object'));
    let config = null;
    try {
      config = await API.getConfig(name);
    } catch (e) {
      config = null;
    }
    // The user may have moved on to another instance meanwhile.
    if (state.currentInstance === name) {
      setTabAvailable('bgp', present(config && config.bgp));
      setTabAvailable('isis', present(config && config.isis));
      setTabAvailable('ospf', present(config && config.ospf));
      setTabAvailable('lag', present(config && config.interfaces && config.interfaces.lag));
    }
  }

  // bgp-sessions has no session id, so a session is identified by the
  // interface and address pair it runs on.
  function bgpSessionKey(item) {
    return [item.interface, item['local-address'], item['peer-address']].join('|');
  }

  async function fetchBgpSessions() {
    const name = state.currentInstance;
    if (!name) return null;
    const resp = await API.command(name, 'bgp-sessions');
    const result = splitCommandResponse(resp).result;
    return Array.isArray(result) ? result : [];
  }

  // The number of BGP sessions is small (one per configured peer), so the
  // whole list is fetched and rendered on every tick - no virtual scrolling.
  async function loadBgpSessions() {
    const name = state.currentInstance;
    if (!name) return;
    let items;
    try {
      items = await fetchBgpSessions();
    } catch (e) {
      if (state.currentInstance !== name) return;
      $('#bgp-empty').hidden = false;
      $('#bgp-empty').textContent = e.status === 412 ? 'Instance is not running.' : 'Could not load BGP sessions: ' + e.message;
      $('#bgp-content').hidden = true;
      return;
    }
    if (state.currentInstance !== name) return;
    if (!items.length) {
      $('#bgp-empty').hidden = false;
      $('#bgp-empty').textContent = 'No BGP sessions reported by this instance.';
      $('#bgp-content').hidden = true;
      return;
    }
    $('#bgp-empty').hidden = true;
    $('#bgp-content').hidden = false;
    renderBgpSessions(items);
    const established = items.filter((i) => i.state === 'established').length;
    $('#bgp-count-hint').textContent = items.length + ' BGP session' + (items.length === 1 ? '' : 's') + ', ' + established + ' established.';
  }

  function renderBgpSessions(items) {
    state.bgp.items = items;
    // Forget selected sessions that are gone or no longer established.
    const selectable = new Set(items.filter(bgpEstablished).map(bgpSessionKey));
    state.bgp.selected.forEach((key) => { if (!selectable.has(key)) state.bgp.selected.delete(key); });

    const tbody = $('#bgp-tbody');
    tbody.innerHTML = '';
    items.forEach((item) => {
      const established = bgpEstablished(item);
      const key = bgpSessionKey(item);
      const label = (item['local-address'] || '?') + ' → ' + (item['peer-address'] || '?');
      const stats = item.stats || {};
      tbody.appendChild(el('tr', {}, [
        el('td', { class: 'select-col' }, [el('input', {
          type: 'checkbox',
          'aria-label': 'Select BGP session ' + label,
          checked: state.bgp.selected.has(key) ? '' : null,
          disabled: established ? null : '',
          title: established ? null : 'Only established sessions can be updated',
          onchange: (e) => {
            if (e.target.checked) state.bgp.selected.add(key);
            else state.bgp.selected.delete(key);
            updateBgpSelectionControls();
          },
        })]),
        el('td', { text: item.interface || '' }),
        el('td', { text: item['local-address'] || '' }),
        el('td', { text: item['peer-address'] || '' }),
        el('td', {}, [el('span', { class: 'badge ' + (established ? 'status-ok' : 'status-code'), text: item.state || 'unknown' })]),
        el('td', {}, [bgpRawUpdateCell(item)]),
        el('td', { text: (stats['update-tx'] || 0).toLocaleString() + ' / ' + (stats['update-rx'] || 0).toLocaleString() }),
        el('td', {}, [
          el('div', { class: 'row-actions-cell' }, [
            el('button', {
              class: 'btn btn-sm', type: 'button', text: 'Send update',
              disabled: established ? null : '',
              title: established ? null : 'Only established sessions can be updated',
              onclick: () => showBgpUpdate([item]),
            }),
            el('button', { class: 'btn btn-sm', type: 'button', text: 'Detail', onclick: () => showBgpDetail(item) }),
          ]),
        ]),
      ]));
    });
    updateBgpSelectionControls();
  }

  function bgpEstablished(item) {
    return item.state === 'established';
  }

  // The raw update progress of a session: the file, its state and, while
  // sending, the bytes acknowledged by the peer.
  function bgpRawUpdateCell(item) {
    const file = item['raw-update-file'];
    const rawState = item['raw-update-state'];
    if (!file) return el('span', { class: 'hint', text: '—' });
    const total = item['raw-update-bytes'] || 0;
    const acked = rawState === 'done' ? total : (item['raw-update-bytes-acked'] || 0);
    const pct = total > 0 ? Math.min(100, (acked / total) * 100) : 0;
    let detail = rawState || '';
    if (rawState === 'sending') detail += ' ' + pct.toFixed(0) + '% of ' + formatFileSize(total);
    else if (rawState === 'done') detail += ' (' + formatFileSize(total) + (item['raw-update-duration'] ? ', ' + item['raw-update-duration'] + 's' : '') + ')';
    return el('div', { class: 'session-meter bgp-update-meter' + (rawState === 'done' ? ' is-complete' : '') }, [
      el('div', { class: 'meter-label' }, [
        el('span', { text: file }),
        el('span', { class: 'count', text: detail }),
      ]),
      el('progress', { value: pct.toFixed(1), max: '100', 'aria-label': 'Raw update ' + file + ' ' + detail }),
    ]);
  }

  // The header checkbox and the toolbar button follow the selection: the
  // button sends to the selected sessions, or to all established ones if
  // none is selected.
  function updateBgpSelectionControls() {
    const established = state.bgp.items.filter(bgpEstablished);
    const selected = state.bgp.selected.size;
    const all = $('#bgp-select-all');
    all.checked = established.length > 0 && selected === established.length;
    all.indeterminate = selected > 0 && selected < established.length;
    all.disabled = established.length === 0;
    const btn = $('#btn-bgp-send-update');
    btn.disabled = established.length === 0;
    btn.textContent = selected ? 'Send update to ' + selected + ' selected…' : 'Send update to all…';
  }

  $('#bgp-select-all').addEventListener('change', (e) => {
    state.bgp.selected.clear();
    if (e.target.checked) state.bgp.items.filter(bgpEstablished).forEach((i) => state.bgp.selected.add(bgpSessionKey(i)));
    renderBgpSessions(state.bgp.items);
  });
  $('#btn-bgp-send-update').addEventListener('click', () => {
    const established = state.bgp.items.filter(bgpEstablished);
    const targets = state.bgp.selected.size ? established.filter((i) => state.bgp.selected.has(bgpSessionKey(i))) : established;
    if (targets.length) showBgpUpdate(targets);
  });

  const bgpIsV6 = (item) => String(item['local-address'] || '').includes(':');

  // The bgp-raw-update commands that send a file to exactly the target
  // sessions. Updating every session takes one command without filter;
  // otherwise each session is addressed by its local and peer address of
  // its address family. This relies on bngblaster applying the IPv6 filters
  // to IPv6 sessions only; up to 1.0.2 they matched every IPv4 session too.
  function bgpUpdatePlan(targets, items) {
    if (targets.length === items.length) return [{ targets, args: {} }];
    return targets.map((item) => {
      const family = bgpIsV6(item) ? 'ipv6' : 'ipv4';
      return {
        targets: [item],
        args: {
          ['local-' + family + '-address']: item['local-address'],
          ['peer-' + family + '-address']: item['peer-address'],
        },
      };
    });
  }

  function bgpTargetText(targets) {
    if (targets.length === state.bgp.items.length) {
      return 'All ' + targets.length + ' BGP session' + (targets.length === 1 ? '' : 's') + ' of this instance.';
    }
    if (targets.length === 1) {
      const t = targets[0];
      return 'BGP session ' + t['local-address'] + ' → ' + t['peer-address'] + ' (' + t.interface + ').';
    }
    return targets.length + ' selected BGP sessions.';
  }

  function setBgpUpdateStatus(kind, text) {
    const status = $('#bgp-update-status');
    status.hidden = !text;
    status.className = 'form-status ' + (kind || 'info');
    status.textContent = text || '';
  }

  async function showBgpUpdate(targets) {
    const name = state.currentInstance;
    state.bgp.update = { instance: name, targets, skipped: [] };
    $('#bgp-update-target').textContent = bgpTargetText(targets);
    $('#btn-bgp-update-retry').hidden = true;
    setBgpUpdateStatus(null, '');
    const select = $('#bgp-update-file');
    select.innerHTML = '';
    select.appendChild(el('option', { value: '', text: 'Loading…' }));
    $('#btn-bgp-update-send').disabled = true;
    openDialog('dialog-bgp-update');

    await loadBgpUpdateFiles(name);
  }

  // Offers the .bgp files of the instance folder plus every file bngblaster
  // has loaded already (bgp-raw-update-list), whatever its name; loaded
  // files are marked, as sending those again needs no file read.
  async function loadBgpUpdateFiles(name) {
    const select = $('#bgp-update-file');
    let files = [];
    let loaded = [];
    try {
      const [list, resp] = await Promise.all([
        API.files(name),
        API.command(name, 'bgp-raw-update-list').catch(() => null),
      ]);
      files = (list || []).map((f) => f.name);
      const result = resp ? splitCommandResponse(resp).result : null;
      loaded = Array.isArray(result) ? result : [];
    } catch (e) {
      if (state.bgp.update && state.bgp.update.instance === name) setBgpUpdateStatus('error', 'Could not list the instance files: ' + e.message);
      select.innerHTML = '';
      return;
    }
    if (!state.bgp.update || state.bgp.update.instance !== name) return;
    const loadedByName = new Map(loaded.map((u) => [u.file, u]));
    const names = new Set(files.filter((f) => /\.bgp$/i.test(f)));
    loaded.forEach((u) => names.add(u.file));
    select.innerHTML = '';
    const sorted = Array.from(names).sort((a, b) => a.localeCompare(b, undefined, { numeric: true }));
    if (!sorted.length) {
      select.appendChild(el('option', { value: '', text: 'No .bgp files in the instance folder' }));
      return;
    }
    sorted.forEach((file) => {
      const u = loadedByName.get(file);
      const text = u ? file + ' (loaded, ' + (u.updates || 0).toLocaleString() + ' updates, ' + formatFileSize(u.len) + ')' : file;
      select.appendChild(el('option', { value: file, text }));
    });
    // Preselect the most recent file of a numbered series (update-003.bgp).
    select.value = sorted[sorted.length - 1];
    $('#btn-bgp-update-send').disabled = false;
  }

  async function sendBgpUpdate(targets) {
    const update = state.bgp.update;
    const file = $('#bgp-update-file').value;
    if (!update || !file) return;
    const commands = bgpUpdatePlan(targets, state.bgp.items);
    $('#btn-bgp-update-send').disabled = true;
    $('#btn-bgp-update-retry').hidden = true;
    setBgpUpdateStatus('info', 'Sending ' + file + '…');
    // bngblaster skips a session that is still sending an earlier file and
    // only counts them in its reply; which ones those are is read from a
    // fresh bgp-sessions just before sending.
    const sending = new Set();
    try {
      (await fetchBgpSessions() || []).forEach((i) => { if (i['raw-update-state'] === 'sending') sending.add(bgpSessionKey(i)); });
    } catch (e) {
      // Without it, Retry skipped is not offered.
    }
    let started = 0;
    let skipped = 0;
    let error = null;
    for (const cmd of commands) {
      try {
        const resp = await API.command(update.instance, 'bgp-raw-update', Object.assign({ file }, cmd.args));
        const result = splitCommandResponse(resp).result || {};
        started += result.started || 0;
        skipped += result.skipped || 0;
      } catch (e) {
        // Same file for every command, so a failure (e.g. file not
        // loadable) would repeat for the rest.
        error = e.message;
        break;
      }
    }
    if (state.bgp.update !== update) return;
    update.skipped = skipped ? targets.filter((t) => sending.has(bgpSessionKey(t))) : [];
    $('#btn-bgp-update-send').disabled = false;
    $('#btn-bgp-update-retry').hidden = update.skipped.length === 0;
    const sessions = (n) => n + ' session' + (n === 1 ? '' : 's');
    if (error) {
      setBgpUpdateStatus('error', 'Sending ' + file + ' failed' + (started ? ' after starting on ' + sessions(started) : '') + ': ' + error);
    } else if (skipped) {
      setBgpUpdateStatus('error', file + ' started on ' + sessions(started) + ', skipped on ' + sessions(skipped) +
        ' still sending an earlier update. Retry once they are done.');
    } else {
      setBgpUpdateStatus('success', file + ' started on ' + sessions(started) + '. The Raw Update column shows the progress.');
    }
    loadBgpSessions();
  }

  $('#btn-bgp-update-send').addEventListener('click', () => {
    if (state.bgp.update) sendBgpUpdate(state.bgp.update.targets);
  });
  $('#btn-bgp-update-retry').addEventListener('click', () => {
    const update = state.bgp.update;
    if (!update || !update.skipped.length) return;
    // Retry with the current state of the skipped sessions; one that is no
    // longer established is left out.
    const current = new Map(state.bgp.items.map((i) => [bgpSessionKey(i), i]));
    const targets = update.skipped.map((t) => current.get(bgpSessionKey(t))).filter((i) => i && bgpEstablished(i));
    if (targets.length) sendBgpUpdate(targets);
  });
  $('#dialog-bgp-update').addEventListener('close', () => { state.bgp.update = null; });

  function initBgpView() {
    loadBgpSessions();
    stopBgpPolling();
    state.bgp.pollTimer = schedulePoll(loadBgpSessions, 2000);
  }
  function stopBgpPolling() {
    state.bgp.pollTimer = cancelPoll(state.bgp.pollTimer);
  }

  // There is no per-session info command, so the detail dialog re-fetches
  // bgp-sessions and shows the full entry of the selected session, rendered
  // the same way as the session/stream detail dialogs.
  async function loadBgpDetail(showLoading) {
    const key = state.bgp.detailKey;
    if (!state.currentInstance || key == null) return;
    if (showLoading) renderDetailFields('#bgp-detail-list', { Loading: '…' });
    try {
      const items = await fetchBgpSessions();
      const item = (items || []).find((i) => bgpSessionKey(i) === key);
      if (item) renderResultFields('#bgp-detail-list', item);
      else renderDetailFields('#bgp-detail-list', { Error: 'BGP session no longer reported by this instance.' });
    } catch (e) {
      renderDetailFields('#bgp-detail-list', { Error: e.message });
    }
  }

  function stopBgpDetailPolling() {
    state.bgp.detailTimer = cancelPoll(state.bgp.detailTimer);
  }

  function showBgpDetail(item) {
    state.bgp.detailKey = bgpSessionKey(item);
    $('#dialog-bgp-detail-title').textContent = 'BGP Session Detail — ' + (item['local-address'] || '?') + ' → ' + (item['peer-address'] || '?');
    openDialog('dialog-bgp-detail');
    renderResultFields('#bgp-detail-list', item);
    stopBgpDetailPolling();
    state.bgp.detailTimer = schedulePoll(() => loadBgpDetail(false), 2000);
  }

  // See the session detail dialog: "close" also covers Escape/backdrop.
  $('#dialog-bgp-detail').addEventListener('close', stopBgpDetailPolling);
  $('#btn-bgp-detail-refresh').addEventListener('click', () => loadBgpDetail(true));
  $('#btn-bgp-refresh').addEventListener('click', loadBgpSessions);

  //= ========================================================================
  // 3b3a. ISIS and OSPF
  //= ========================================================================
  // Both tabs follow the BGP tab: shown if the config has the protocol,
  // adjacencies/neighbors fetched and rendered as a whole every 2 seconds.
  // Both show one protocol instance at a time, picked at the top of the tab
  // (OSPF commands take the instance; isis-adjacencies returns all of them
  // and is filtered here).
  //
  // The link state database is different: with an lspgen topology it holds
  // thousands of entries. It is collapsed by default, fetched and polled
  // (less often) only while open, and rendered a page of ROUTING_DB_PAGE
  // entries at a time.
  const ROUTING_DB_POLL_MS = 10000;
  const ROUTING_DB_PAGE = 500;

  // The instances of a protocol section of the config ("isis" or "ospf"),
  // which may be one object or a list of them.
  function configInstances(section) {
    if (!section) return [];
    return (Array.isArray(section) ? section : [section]).filter((i) => i && typeof i === 'object');
  }

  function fillInstanceSelect(select, instances) {
    const previous = select.value;
    select.innerHTML = '';
    instances.forEach((inst) => {
      const id = inst['instance-id'] ?? 1;
      select.appendChild(el('option', { value: String(id), text: String(id) + (inst.hostname ? ' (' + inst.hostname + ')' : '') }));
    });
    if ([...select.options].some((o) => o.value === previous)) select.value = previous;
  }

  // The selected protocol instance of a tab, or null before the select is
  // filled. instance-id 0 is valid, hence no plain truthiness check.
  function selectedInstance(proto) {
    const value = $('#' + proto + '-instance').value;
    return value === '' ? null : Number(value);
  }

  // Up/Full are the healthy end states; Down is a failure, anything else a
  // state on the way up.
  function routingStateBadge(s) {
    const cls = (s === 'Up' || s === 'Full') ? 'status-ok' : (s === 'Down' ? 'status-error' : 'status-code');
    return el('span', { class: 'badge ' + cls, text: s || 'unknown' });
  }

  // Shows "not running" (412) or the error in a tab's empty state.
  function showRoutingError(proto, what, e) {
    $('#' + proto + '-empty').hidden = false;
    $('#' + proto + '-empty').textContent = e.status === 412 ? 'Instance is not running.' : 'Could not load ' + what + ': ' + e.message;
    $('#' + proto + '-content').hidden = true;
  }

  // Renders the current page of the database entries matching the filter
  // (case-insensitive, any of fields). The page offset is kept per tab and
  // pulled back if the database shrank or the filter matches fewer entries.
  function renderRoutingDatabase(proto, fields, row, noun) {
    const st = state[proto];
    const needle = $('#' + proto + '-db-filter').value.trim().toLowerCase();
    const entries = st.database;
    const matching = needle
      ? entries.filter((e) => fields.some((f) => String(e[f] ?? '').toLowerCase().includes(needle)))
      : entries;
    if (st.dbOffset >= matching.length) st.dbOffset = Math.max(0, Math.floor((matching.length - 1) / ROUTING_DB_PAGE) * ROUTING_DB_PAGE);
    const page = matching.slice(st.dbOffset, st.dbOffset + ROUTING_DB_PAGE);
    const tbody = $('#' + proto + '-db-tbody');
    tbody.innerHTML = '';
    page.forEach((e) => tbody.appendChild(row(e)));

    const plural = (n) => n + ' ' + noun + (n === 1 ? '' : 's');
    let hint = needle ? matching.length + ' matching (' + plural(entries.length) + ')' : plural(entries.length);
    if (matching.length > ROUTING_DB_PAGE) {
      hint = 'Showing ' + (st.dbOffset + 1) + '–' + (st.dbOffset + page.length) + ' of ' + hint;
    }
    $('#' + proto + '-db-hint').textContent = hint + '.';
    const paged = matching.length > ROUTING_DB_PAGE;
    $('#' + proto + '-db-prev').hidden = !paged;
    $('#' + proto + '-db-next').hidden = !paged;
    $('#' + proto + '-db-prev').disabled = st.dbOffset === 0;
    $('#' + proto + '-db-next').disabled = st.dbOffset + ROUTING_DB_PAGE >= matching.length;
  }

  // Wires the collapsed database section of a tab: fetched and polled only
  // while open, paged with the Previous/Next buttons, and back to the first
  // page whenever the filter changes.
  function bindRoutingDatabase(proto, load, render) {
    const st = state[proto];
    $('#' + proto + '-db-details').addEventListener('toggle', () => {
      st.dbTimer = cancelPoll(st.dbTimer);
      if (!$('#' + proto + '-db-details').open || !isTabActive(proto)) return;
      load();
      st.dbTimer = schedulePoll(load, ROUTING_DB_POLL_MS);
    });
    $('#' + proto + '-db-prev').addEventListener('click', () => { st.dbOffset = Math.max(0, st.dbOffset - ROUTING_DB_PAGE); render(); });
    $('#' + proto + '-db-next').addEventListener('click', () => { st.dbOffset += ROUTING_DB_PAGE; render(); });
    $('#' + proto + '-db-filter').addEventListener('input', debounce(() => { st.dbOffset = 0; render(); }, 150));
  }

  // Starts the polls of a tab; the database only if its section is open.
  function startRoutingPolling(proto, load, loadDatabase) {
    const st = state[proto];
    load();
    st.pollTimer = cancelPoll(st.pollTimer);
    st.pollTimer = schedulePoll(load, 2000);
    st.dbTimer = cancelPoll(st.dbTimer);
    if ($('#' + proto + '-db-details').open) {
      loadDatabase();
      st.dbTimer = schedulePoll(loadDatabase, ROUTING_DB_POLL_MS);
    }
  }

  function stopRoutingPolling(proto) {
    state[proto].pollTimer = cancelPoll(state[proto].pollTimer);
    state[proto].dbTimer = cancelPoll(state[proto].dbTimer);
  }

  // Selects the instance's protocol instances from its config, once per
  // instance (the selects are emptied by resetInstanceViews).
  async function prepareRoutingTab(proto) {
    const name = state.currentInstance;
    if (state[proto].instance === name) return true;
    let section = null;
    try {
      const config = await API.getConfig(name);
      section = config && config[proto];
    } catch (e) {
      section = null;
    }
    if (state.currentInstance !== name) return false;
    state[proto].instance = name;
    state[proto].config = section;
    state[proto].database = [];
    state[proto].dbOffset = 0;
    fillInstanceSelect($('#' + proto + '-instance'), configInstances(section));
    return true;
  }

  // The detail dialog of an ISIS adjacency or OSPF interface. Neither has a
  // per-entry info command, so the dialog shows the entry from the latest
  // poll of its tab, found again by key on every tick.
  function showRoutingDetail(title, kind, key, item) {
    state.routingDetail = { kind, key, item };
    $('#dialog-routing-detail-title').textContent = title;
    renderResultFields('#routing-detail-list', item);
    openDialog('dialog-routing-detail');
  }

  function refreshRoutingDetail(kind, items, keyOf) {
    const detail = state.routingDetail;
    if (!detail || detail.kind !== kind || !$('#dialog-routing-detail').open) return;
    const item = items.find((i) => keyOf(i) === detail.key);
    if (item) renderResultFields('#routing-detail-list', item);
    else renderDetailFields('#routing-detail-list', { Error: 'No longer reported by this instance.' });
  }

  $('#dialog-routing-detail').addEventListener('close', () => { state.routingDetail = null; });
  $('#btn-routing-detail-refresh').addEventListener('click', () => {
    const detail = state.routingDetail;
    if (detail && detail.kind === 'isis') loadIsisAdjacencies();
    if (detail && detail.kind === 'ospf') loadOspf();
  });

  // ---- ISIS ----

  function isisAdjacencyKey(item) {
    return [item.interface, item.level, item['instance-id']].join('|');
  }

  // A P2P adjacency has one peer, a broadcast one a list of them.
  function isisAdjacencyPeers(item) {
    if (item.peer) return item.peer['system-id'] || '';
    return (item.peers || []).map((p) => p['system-id'] + (p.state ? ' (' + p.state + ')' : '')).join(', ');
  }

  async function loadIsisAdjacencies() {
    const name = state.currentInstance;
    const instance = selectedInstance('isis');
    if (!name) return;
    let items;
    try {
      const result = splitCommandResponse(await API.command(name, 'isis-adjacencies')).result;
      items = Array.isArray(result) ? result : [];
    } catch (e) {
      if (state.currentInstance === name) showRoutingError('isis', 'ISIS adjacencies', e);
      return;
    }
    if (state.currentInstance !== name) return;
    if (instance !== null) items = items.filter((i) => i['instance-id'] === instance);
    $('#isis-empty').hidden = true;
    $('#isis-content').hidden = false;
    const tbody = $('#isis-adjacency-tbody');
    tbody.innerHTML = '';
    items.forEach((item) => {
      tbody.appendChild(el('tr', {}, [
        el('td', { text: item.interface || '' }),
        el('td', { text: item.type || '' }),
        el('td', { text: item.level || '' }),
        el('td', { text: isisAdjacencyPeers(item) }),
        el('td', {}, [routingStateBadge(item['adjacency-state'])]),
        el('td', {}, [
          el('div', { class: 'row-actions-cell' }, [
            el('button', {
              class: 'btn btn-sm', type: 'button', text: 'Detail',
              onclick: () => showRoutingDetail('ISIS Adjacency — ' + item.interface + ' ' + item.level, 'isis', isisAdjacencyKey(item), item),
            }),
          ]),
        ]),
      ]));
    });
    const up = items.filter((i) => i['adjacency-state'] === 'Up').length;
    $('#isis-adjacency-hint').textContent = items.length
      ? items.length + ' adjacenc' + (items.length === 1 ? 'y' : 'ies') + ', ' + up + ' up.'
      : 'No ISIS adjacencies reported for this instance.';
    refreshRoutingDetail('isis', items, isisAdjacencyKey);
  }

  async function loadIsisDatabase() {
    const name = state.currentInstance;
    const instance = selectedInstance('isis');
    if (!name || instance === null || $('#isis-db-level').value === '') return;
    const level = Number($('#isis-db-level').value);
    let entries;
    try {
      const result = splitCommandResponse(await API.command(name, 'isis-database', { instance, level })).result;
      entries = Array.isArray(result) ? result : [];
    } catch (e) {
      if (state.currentInstance !== name) return;
      // 404: no database for this level (the instance does not run it).
      state.isis.database = [];
      $('#isis-db-tbody').innerHTML = '';
      $('#isis-db-prev').hidden = true;
      $('#isis-db-next').hidden = true;
      $('#isis-db-hint').textContent = e.status === 404 ? 'No database for this level.' : 'Could not load the database: ' + e.message;
      return;
    }
    if (state.currentInstance !== name) return;
    state.isis.database = entries;
    renderIsisDatabase();
  }

  function renderIsisDatabase() {
    renderRoutingDatabase('isis', ['id', 'source-type', 'source-system-id'], (e) => el('tr', {}, [
      el('td', { text: e.id || '' }),
      el('td', { text: String(e.seq ?? '') }),
      el('td', { text: String(e.lifetime ?? '') }),
      el('td', { text: String(e['lifetime-remaining'] ?? '') }),
      el('td', { text: e['source-type'] || '' }),
      el('td', { text: e['source-system-id'] || '' }),
    ]), 'LSP');
  }

  // The levels the selected instance runs: "level" is 1, 2 or 3 (both,
  // the default).
  function fillIsisLevelSelect() {
    const id = $('#isis-instance').value;
    const instance = configInstances(state.isis.config).find((i) => String(i['instance-id'] ?? 1) === id);
    const level = (instance && instance.level) || 3;
    const select = $('#isis-db-level');
    const previous = select.value;
    select.innerHTML = '';
    [1, 2].filter((l) => level === 3 || level === l).forEach((l) => {
      select.appendChild(el('option', { value: String(l), text: 'L' + l }));
    });
    if ([...select.options].some((o) => o.value === previous)) select.value = previous;
  }

  async function initIsisView() {
    const name = state.currentInstance;
    if (!(await prepareRoutingTab('isis'))) return;
    if (!$('#isis-db-level').options.length) fillIsisLevelSelect();
    if (state.currentInstance === name && isTabActive('isis')) startRoutingPolling('isis', loadIsisAdjacencies, loadIsisDatabase);
  }
  function stopIsisPolling() {
    stopRoutingPolling('isis');
  }

  bindRoutingDatabase('isis', loadIsisDatabase, renderIsisDatabase);
  $('#btn-isis-refresh').addEventListener('click', () => {
    loadIsisAdjacencies();
    if ($('#isis-db-details').open) loadIsisDatabase();
  });
  $('#isis-instance').addEventListener('change', () => {
    fillIsisLevelSelect();
    state.isis.dbOffset = 0;
    loadIsisAdjacencies();
    if ($('#isis-db-details').open) loadIsisDatabase();
  });
  $('#isis-db-level').addEventListener('change', () => { state.isis.dbOffset = 0; loadIsisDatabase(); });

  // ---- OSPF ----

  function ospfInterfaceKey(item) {
    return item.name || '';
  }

  // Interfaces and neighbors are small and polled together.
  async function loadOspf() {
    const name = state.currentInstance;
    const instance = selectedInstance('ospf');
    if (!name || instance === null) return;
    let interfaces;
    let neighbors;
    try {
      const [ifResp, nbResp] = await Promise.all([
        API.command(name, 'ospf-interfaces', { instance }),
        API.command(name, 'ospf-neighbors', { instance }),
      ]);
      interfaces = splitCommandResponse(ifResp).result;
      neighbors = splitCommandResponse(nbResp).result;
    } catch (e) {
      if (state.currentInstance === name) showRoutingError('ospf', 'OSPF data', e);
      return;
    }
    if (state.currentInstance !== name) return;
    interfaces = Array.isArray(interfaces) ? interfaces : [];
    neighbors = Array.isArray(neighbors) ? neighbors : [];
    $('#ospf-empty').hidden = true;
    $('#ospf-content').hidden = false;

    const ifBody = $('#ospf-interface-tbody');
    ifBody.innerHTML = '';
    interfaces.forEach((item) => {
      ifBody.appendChild(el('tr', {}, [
        el('td', { text: item.name || '' }),
        el('td', { text: item.type || '' }),
        el('td', { text: item.state || '' }),
        el('td', { text: item.dr || '' }),
        el('td', { text: item.bdr || '' }),
        el('td', { text: (item['neighbors-full'] ?? 0) + ' of ' + (item.neighbors ?? 0) + ' full' }),
        el('td', {}, [
          el('div', { class: 'row-actions-cell' }, [
            el('button', {
              class: 'btn btn-sm', type: 'button', text: 'Detail',
              onclick: () => showRoutingDetail('OSPF Interface — ' + item.name, 'ospf', ospfInterfaceKey(item), item),
            }),
          ]),
        ]),
      ]));
    });

    const nbBody = $('#ospf-neighbor-tbody');
    nbBody.innerHTML = '';
    neighbors.forEach((item) => {
      nbBody.appendChild(el('tr', {}, [
        el('td', { text: item.interface || '' }),
        el('td', { text: item['router-id'] || '' }),
        el('td', {}, [routingStateBadge(item.state)]),
        el('td', { text: String(item['retry-tree-entries'] ?? '') }),
        el('td', { text: String(item['request-tree-entries'] ?? '') }),
      ]));
    });
    const full = neighbors.filter((n) => n.state === 'Full').length;
    $('#ospf-neighbor-hint').textContent = neighbors.length
      ? neighbors.length + ' neighbor' + (neighbors.length === 1 ? '' : 's') + ', ' + full + ' full.'
      : 'No OSPF neighbors reported for this instance.';
    refreshRoutingDetail('ospf', interfaces, ospfInterfaceKey);
  }

  async function loadOspfDatabase() {
    const name = state.currentInstance;
    const instance = selectedInstance('ospf');
    if (!name || instance === null) return;
    let entries;
    try {
      const result = splitCommandResponse(await API.command(name, 'ospf-database', { instance })).result;
      entries = Array.isArray(result) ? result : [];
    } catch (e) {
      if (state.currentInstance !== name) return;
      state.ospf.database = [];
      $('#ospf-db-tbody').innerHTML = '';
      $('#ospf-db-prev').hidden = true;
      $('#ospf-db-next').hidden = true;
      $('#ospf-db-hint').textContent = e.status === 412 ? '' : 'Could not load the database: ' + e.message;
      return;
    }
    if (state.currentInstance !== name) return;
    state.ospf.database = entries;
    renderOspfDatabase();
  }

  function renderOspfDatabase() {
    renderRoutingDatabase('ospf', ['id', 'router', 'source-type', 'source-router-id'], (e) => el('tr', {}, [
      el('td', { text: String(e.type ?? '') }),
      el('td', { text: e.id || '' }),
      el('td', { text: e.router || '' }),
      el('td', { text: e.seq || '' }),
      el('td', { text: e.checksum || '' }),
      el('td', { text: String(e.age ?? '') }),
      el('td', { text: e['source-type'] || '' }),
      el('td', { text: e['source-router-id'] || '' }),
    ]), 'LSA');
  }

  async function initOspfView() {
    const name = state.currentInstance;
    if (!(await prepareRoutingTab('ospf'))) return;
    if (state.currentInstance === name && isTabActive('ospf')) startRoutingPolling('ospf', loadOspf, loadOspfDatabase);
  }
  function stopOspfPolling() {
    stopRoutingPolling('ospf');
  }

  bindRoutingDatabase('ospf', loadOspfDatabase, renderOspfDatabase);
  $('#btn-ospf-refresh').addEventListener('click', () => {
    loadOspf();
    if ($('#ospf-db-details').open) loadOspfDatabase();
  });
  $('#ospf-instance').addEventListener('change', () => {
    state.ospf.dbOffset = 0;
    loadOspf();
    if ($('#ospf-db-details').open) loadOspfDatabase();
  });

  //= ========================================================================
  // 3b4. LAG
  //= ========================================================================
  // lag-info returns every LAG with its members in one small response, so it
  // is fetched and rendered as a whole on every tick, like bgp-sessions.
  async function loadLagInfo() {
    const name = state.currentInstance;
    if (!name) return;
    let lags;
    try {
      const resp = await API.command(name, 'lag-info');
      const result = splitCommandResponse(resp).result;
      lags = Array.isArray(result) ? result : [];
    } catch (e) {
      if (state.currentInstance !== name) return;
      $('#lag-empty').hidden = false;
      $('#lag-empty').textContent = e.status === 412 ? 'Instance is not running.' : 'Could not load LAG info: ' + e.message;
      $('#lag-content').hidden = true;
      return;
    }
    if (state.currentInstance !== name) return;
    if (!lags.length) {
      $('#lag-empty').hidden = false;
      $('#lag-empty').textContent = 'No LAG interfaces reported by this instance.';
      $('#lag-content').hidden = true;
      return;
    }
    $('#lag-empty').hidden = true;
    $('#lag-content').hidden = false;
    renderLagInfo(lags);
  }

  // Up is the only healthy state; Down is a failure, while Disabled (by the
  // user) and Standby (lacp-max-active-links) are deliberate, so neutral.
  function lagStateBadge(s) {
    const cls = s === 'Up' ? 'status-ok' : (s === 'Down' ? 'status-error' : 'status-code');
    return el('span', { class: 'badge ' + cls, text: s || 'unknown' });
  }

  function renderLagInfo(lags) {
    const content = $('#lag-content');
    content.innerHTML = '';
    lags.forEach((lag) => {
      const members = Array.isArray(lag.members) ? lag.members : [];
      const tbody = el('tbody', {}, members.map((m) => {
        // interface-enable/-disable act on the member interface itself; the
        // LAG interface is rejected by bngblaster, hence no group-level button.
        const disabled = m.state === 'Disabled';
        return el('tr', {}, [
          el('td', { text: m.interface || '' }),
          el('td', {}, [lagStateBadge(m.state)]),
          el('td', { text: m['lacp-state'] || '' }),
          el('td', { text: String(m['packets-rx'] ?? '') }),
          el('td', { text: String(m['packets-tx'] ?? '') }),
          el('td', { text: String(m['stream-count'] ?? '') }),
          el('td', { text: String(m['stream-pps'] ?? '') }),
          el('td', {}, [
            el('div', { class: 'row-actions-cell' }, [
              el('button', {
                class: 'btn btn-sm' + (disabled ? '' : ' btn-danger'),
                type: 'button',
                text: disabled ? 'Enable' : 'Disable',
                disabled: state.lag.pending.has(m.interface) ? '' : undefined,
                'aria-label': (disabled ? 'Enable ' : 'Disable ') + m.interface,
                onclick: () => setLagMemberEnabled(m.interface, disabled),
              }),
            ]),
          ]),
        ]);
      }));
      content.appendChild(el('div', { class: 'iface-group' }, [
        el('h3', {}, [
          el('span', { text: (lag.interface || 'lag') + ' ' }),
          lagStateBadge(lag.state),
        ]),
        el('p', {
          class: 'hint',
          text: (lag['members-active'] ?? 0) + ' of ' + members.length + ' member' + (members.length === 1 ? '' : 's') +
            ' active, ' + (lag['stream-count'] ?? 0) + ' stream' + (lag['stream-count'] === 1 ? '' : 's') + '.',
        }),
        el('div', { class: 'table-scroll' }, [
          el('table', {}, [
            el('caption', { class: 'visually-hidden', text: 'Members of ' + lag.interface }),
            el('thead', {}, [el('tr', {}, ['Interface', 'State', 'LACP State', 'RX Packets', 'TX Packets', 'Streams', 'Stream PPS', 'Actions']
              .map((h) => el('th', { scope: 'col', text: h })))]),
            tbody,
          ]),
        ]),
      ]));
    });
  }

  async function setLagMemberEnabled(iface, enable) {
    const name = state.currentInstance;
    if (!name || !iface) return;
    state.lag.pending.add(iface);
    loadLagInfo();
    try {
      await API.command(name, enable ? 'interface-enable' : 'interface-disable', { interface: iface });
      announce((enable ? 'Enabled' : 'Disabled') + ' interface ' + iface, 'success');
    } catch (e) {
      announce('Command failed: ' + e.message, 'error');
    } finally {
      state.lag.pending.delete(iface);
      loadLagInfo();
    }
  }

  function initLagView() {
    loadLagInfo();
    stopLagPolling();
    state.lag.pollTimer = schedulePoll(loadLagInfo, 2000);
  }
  function stopLagPolling() {
    state.lag.pollTimer = cancelPoll(state.lag.pollTimer);
  }

  $('#btn-lag-refresh').addEventListener('click', loadLagInfo);

  //= ========================================================================
  // 3b5. Generate (lspgen, bgpupdate, ldpupdate)
  //= ========================================================================
  // The generators and their options come from /api/v1/generators, so the
  // forms below follow whatever the controller supports. The endpoint only
  // exists with -generators; without it (or without any usable tool) the
  // tab stays hidden. Probing the tools starts Python, so the list is
  // fetched once per page load.
  let generatorsPromise = null;
  function loadGenerators() {
    if (!generatorsPromise) {
      generatorsPromise = API.generators()
        .then((list) => (Array.isArray(list) ? list : []))
        .catch(() => []);
    }
    return generatorsPromise;
  }

  async function updateGenerateTabAvailability(name) {
    const list = await loadGenerators();
    if (state.currentInstance === name) setTabAvailable('generate', list.some((tool) => tool.available));
  }

  // The kind of file a tool's output option writes. Keyed by tool and
  // output option rather than by file extension, so any file name works.
  const GENERATED_FILE_KINDS = {
    'bgpupdate:file': 'bgp',
    'ldpupdate:file': 'ldp',
    'lspgen:mrt-file': 'mrt',
  };

  // Per kind: the runtime command that loads such a file into a running
  // instance, and where the configuration of a stopped one references it.
  const GENERATED_FILE_USES = {
    bgp: {
      command: (file) => ({ command: 'bgp-raw-update', args: { file } }),
      snippet: (file) => 'bgp: "raw-update-file": "' + file + '" (or "bgp-raw-update-files": ["' + file + '"])',
    },
    ldp: {
      command: (file) => ({ command: 'ldp-raw-update', args: { file } }),
      snippet: (file) => 'ldp: "raw-update-file": "' + file + '" (or "ldp-raw-update-files": ["' + file + '"])',
    },
    mrt: {
      // isis-load-mrt and ospf-load-mrt need the instance the topology is
      // attached to: the first one of the protocol in the config.
      command: (file, request, config) => {
        const ospf = String(request.protocol || 'isis').startsWith('ospf');
        const list = ospf ? config.ospf : config.isis;
        const first = Array.isArray(list) ? list[0] : list;
        const instance = (first && first['instance-id']) || 1;
        return { command: ospf ? 'ospf-load-mrt' : 'isis-load-mrt', args: { file, instance } };
      },
      snippet: (file) => 'isis/ospf: "external": { "mrt-file": "' + file + '" }',
    },
  };

  // Splits "update-007.bgp" into its stem, sequence number and extension,
  // so that the next free name continues the sequence ("update-008.bgp").
  // taken(name) reports whether a name is in use; for an output with
  // suffixes (lspgen's graphviz base name) that is any of its files.
  function nextFreeFileName(name, taken) {
    if (!taken(name)) return name;
    const m = /^(.*?)(?:-(\d+))?(\.[^.]*)?$/.exec(name);
    const stem = m[1] || name;
    const ext = m[3] || '';
    let n = m[2] ? Number(m[2]) + 1 : 1;
    let candidate;
    do {
      candidate = stem + '-' + String(n).padStart(3, '0') + ext;
      n++;
    } while (taken(candidate));
    return candidate;
  }

  // Values taken from the instance configuration for an empty form, so the
  // generated files fit the instance: the ISIS system-id is the lspgen
  // connector, the BGP local address the next hop, the LDP LSR-ID the LSR-ID.
  function generatorPrefill(tool, config) {
    const first = (v) => (Array.isArray(v) ? v[0] : v) || null;
    const values = {};
    if (!config) return values;
    if (tool === 'lspgen') {
      const isis = first(config.isis);
      const ospf = first(config.ospf);
      if (isis) {
        if (isis['system-id']) values.connector = [isis['system-id']];
        if (Array.isArray(isis.area) && isis.area.length) values.area = isis.area;
        if (isis['level1-auth-key']) {
          values['authentication-key'] = isis['level1-auth-key'];
          if (isis['level1-auth-type']) values['authentication-type'] = isis['level1-auth-type'];
        }
        values['mrt-file'] = 'isis.mrt';
      } else if (ospf) {
        values.protocol = ospf.version === 3 ? 'ospf3' : 'ospf2';
        values['mrt-file'] = 'ospf.mrt';
      }
    } else if (tool === 'bgpupdate') {
      const bgp = first(config.bgp);
      if (bgp) {
        if (bgp['local-address']) values['next-hop-base'] = bgp['local-address'];
        if (bgp['local-as'] !== undefined) values.asn = [bgp['local-as']];
      }
    } else if (tool === 'ldpupdate') {
      const ldp = first(config.ldp);
      if (ldp && ldp['lsr-id']) values['lsr-id'] = ldp['lsr-id'];
    }
    return values;
  }

  // Builds one form field for a generator option (see the generatorOption
  // schema in swagger.yaml). getValue() returns undefined for an option the
  // user left empty, so the tool's own default applies.
  function buildGeneratorField(option) {
    const id = nextId('generate-opt');
    const hasDefault = option.default !== undefined && option.default !== null;
    const isList = option.repeat || option.join;
    const hint = [option.help, isList && option.type !== 'enum' ? 'comma-separated' : ''].filter(Boolean).join('; ');

    if (option.type === 'bool') {
      const input = el('input', { type: 'checkbox', id: id });
      const wrap = el('div', { class: 'field checkbox-field' }, [input, el('label', { for: id, text: option.name })]);
      if (hint) wrap.appendChild(el('span', { class: 'hint', text: hint }));
      return {
        option, el: wrap,
        getValue: () => (input.checked ? true : undefined),
        setValue: (v) => { input.checked = !!v; },
      };
    }

    if (option.type === 'enum' && isList) {
      const boxes = option.enum.map((value) => {
        const boxId = nextId('generate-opt');
        const input = el('input', { type: 'checkbox', id: boxId, value: value });
        return { input, el: el('div', { class: 'field checkbox-field' }, [input, el('label', { for: boxId, text: value })]) };
      });
      const wrap = el('fieldset', {}, [el('legend', { text: option.name })]);
      wrap.appendChild(el('div', { class: 'flags-grid' }, boxes.map((b) => b.el)));
      if (hint) wrap.appendChild(el('span', { class: 'hint', text: hint }));
      return {
        option, el: wrap,
        getValue: () => {
          const values = boxes.filter((b) => b.input.checked).map((b) => b.input.value);
          return values.length ? values : undefined;
        },
        setValue: (v) => { boxes.forEach((b) => { b.input.checked = Array.isArray(v) && v.includes(b.input.value); }); },
      };
    }

    if (option.type === 'enum') {
      const select = el('select', { id: id }, [
        el('option', { value: '', text: hasDefault ? 'default (' + option.default + ')' : '—' }),
        ...option.enum.map((value) => el('option', { value: value, text: value })),
      ]);
      return {
        option, el: fieldWrap(id, option.name, option.required, hint, select),
        getValue: () => (select.value === '' ? undefined : select.value),
        setValue: (v) => { select.value = (v === undefined || v === null) ? '' : v; },
      };
    }

    const numeric = option.type === 'int' || option.type === 'number';
    const input = el('input', { type: numeric && !isList ? 'number' : 'text', id: id, required: option.required ? '' : null });
    if (numeric && !isList) {
      if (option.minimum !== undefined) input.setAttribute('min', option.minimum);
      if (option.maximum !== undefined) input.setAttribute('max', option.maximum);
      input.setAttribute('step', option.type === 'int' ? '1' : 'any');
    }
    if (hasDefault) input.setAttribute('placeholder', String(option.default));
    if (option.type === 'input') input.setAttribute('list', 'generate-files-list');
    const parse = (text) => (numeric ? Number(text) : text);
    return {
      option, el: fieldWrap(id, option.name, option.required, hint, input),
      input,
      getValue: () => {
        const text = input.value.trim();
        if (text === '') return undefined;
        if (!isList) return parse(text);
        return text.split(',').map((s) => s.trim()).filter(Boolean).map(parse);
      },
      setValue: (v) => {
        if (v === undefined || v === null) input.value = '';
        else input.value = Array.isArray(v) ? v.join(', ') : String(v);
      },
    };
  }

  // Groups the options as the controller does (topology, output, ...); the
  // first group, the output files and any group with a required option are
  // open, everything else collapsed.
  function renderGeneratorForm(tool) {
    const container = $('#generate-fields');
    container.innerHTML = '';
    state.generate.fields = [];
    const groups = new Map();
    tool.options.forEach((option) => {
      const group = option.group || 'options';
      if (!groups.has(group)) groups.set(group, []);
      groups.get(group).push(option);
    });
    let firstGroup = true;
    groups.forEach((options, group) => {
      const body = el('div', { class: 'form-grid' });
      options.forEach((option) => {
        const field = buildGeneratorField(option);
        state.generate.fields.push(field);
        body.appendChild(field.el);
      });
      const open = firstGroup || group === 'output' || options.some((o) => o.required);
      container.appendChild(el('details', { open: open ? '' : null }, [el('summary', { text: prettyLabel(group) }), body]));
      firstGroup = false;
    });
    // File suggestions for input options (lspgen --read-config-file).
    container.appendChild(el('datalist', { id: 'generate-files-list' },
      state.generate.files.map((f) => el('option', { value: f }))));
  }

  function generatorField(name) {
    return state.generate.fields.find((f) => f.option.name === name);
  }

  // While the instance is running, bngblaster has loaded the existing files
  // already (RAW update files once per name), and the controller refuses to
  // replace them: move every output name on to the next free one.
  function suggestFreeOutputNames() {
    if (!state.generate.running) return;
    const existing = new Set(state.generate.files);
    state.generate.fields.forEach((field) => {
      if (field.option.type !== 'output' || !field.input) return;
      const suffixes = field.option.suffixes || [''];
      const taken = (name) => suffixes.some((suffix) => existing.has(name + suffix));
      const current = field.input.value.trim() || (field.option.default || '');
      if (current && taken(current)) field.setValue(nextFreeFileName(current, taken));
    });
  }

  function selectGenerator(name) {
    const tool = (state.generate.tools || []).find((t) => t.name === name);
    state.generate.tool = tool || null;
    $('#generate-tool-description').textContent = tool ? tool.description : '';
    $('#btn-generate').disabled = !tool || !tool.available;
    if (!tool) { $('#generate-fields').innerHTML = ''; return; }
    if (!tool.available) {
      $('#generate-fields').innerHTML = '';
      $('#generate-fields').appendChild(el('p', { class: 'hint', text: 'Not available on this controller: ' + (tool.error || 'unknown error') }));
      return;
    }
    renderGeneratorForm(tool);
    Object.entries(generatorPrefill(tool.name, state.generate.config)).forEach(([key, value]) => {
      const field = generatorField(key);
      if (field) field.setValue(value);
    });
    suggestFreeOutputNames();
  }

  function collectGeneratorRequest() {
    const request = {};
    state.generate.fields.forEach((field) => {
      const v = field.getValue();
      if (v !== undefined) request[field.option.name] = v;
    });
    return request;
  }

  function renderGenerateHint() {
    $('#generate-hint').textContent = state.generate.running
      ? 'The instance is running: generated files can be loaded right away, but existing files are never ' +
        'replaced, so every further update needs a new file name.'
      : 'Files are written into the instance folder, where the configuration can reference them ' +
        '(e.g. an ISIS/OSPF external mrt-file or a BGP/LDP raw-update-file).';
  }

  // Refreshes what the form depends on: whether the instance is running,
  // its files (for free output names) and its configuration (for prefill).
  async function refreshGenerateContext(name) {
    const [status, files, config] = await Promise.all([
      API.status(name).catch(() => null),
      API.files(name).catch(() => []),
      state.generate.config ? Promise.resolve(state.generate.config) : API.getConfig(name).catch(() => null),
    ]);
    if (state.currentInstance !== name) return false;
    state.generate.running = !!status && status.status === 'started';
    state.generate.files = (files || []).map((f) => f.name);
    state.generate.config = config;
    renderGenerateHint();
    return true;
  }

  // Called by the overview poll: a test that ends (or starts) on its own
  // changes what the Generate tab offers, without anyone touching the tab.
  // The poll's own answer is taken as is rather than asking for the status
  // again: while an instance starts up its status is already "started" but
  // the overview still fails, and refetching on every poll would not settle.
  function syncGenerateRunning(running) {
    if (!isTabActive('generate') || state.generate.instance !== state.currentInstance) return;
    if (running === state.generate.running) return;
    state.generate.running = running;
    renderGenerateHint();
    suggestFreeOutputNames();
  }

  async function initGenerateView() {
    const name = state.currentInstance;
    if (!name) return;
    const tools = await loadGenerators();
    if (!(await refreshGenerateContext(name))) return;
    if (state.generate.instance === name) {
      // Same instance as before: keep what the user has entered, only the
      // running state (and so the free output names) may have changed.
      suggestFreeOutputNames();
      return;
    }
    state.generate.instance = name;
    state.generate.tools = tools;
    const select = $('#generate-tool-select');
    select.innerHTML = '';
    tools.forEach((tool) => {
      select.appendChild(el('option', {
        value: tool.name,
        text: tool.name + (tool.available ? (tool.version ? ' ' + tool.version : '') : ' (not available)'),
      }));
    });
    const firstAvailable = tools.find((t) => t.available) || tools[0];
    if (firstAvailable) select.value = firstAvailable.name;
    selectGenerator(select.value);
  }

  function addGenerateHistory(text, ok) {
    const history = state.generate.history;
    history.unshift({ time: new Date(), text, ok });
    history.length = Math.min(history.length, 20);
    const list = $('#generate-history');
    list.innerHTML = '';
    history.forEach((entry) => {
      list.appendChild(el('li', {}, [
        el('span', { class: 'badge ' + (entry.ok ? 'status-ok' : 'status-error'), text: entry.ok ? 'ok' : 'failed' }),
        el('span', { class: 'hint', text: ' ' + entry.time.toLocaleTimeString() + ' ' }),
        el('span', { text: entry.text }),
      ]));
    });
    $('#generate-history-wrap').hidden = false;
  }

  async function applyGeneratedFile(cmd, button) {
    const name = state.currentInstance;
    if (!name) return;
    button.disabled = true;
    try {
      await API.command(name, cmd.command, cmd.args);
      announce('Loaded ' + cmd.args.file + ' (' + cmd.command + ')', 'success');
      addGenerateHistory(cmd.command + ' ' + cmd.args.file, true);
    } catch (e) {
      announce(cmd.command + ' failed: ' + e.message, 'error');
      addGenerateHistory(cmd.command + ' ' + cmd.args.file + ': ' + e.message, false);
      button.disabled = false;
    }
  }

  function formatMillis(ms) {
    return ms < 1000 ? ms + ' ms' : (ms / 1000).toFixed(1) + ' s';
  }

  function renderGenerateResult(tool, request, result, error) {
    const box = $('#generate-result');
    box.innerHTML = '';
    $('#generate-result-empty').hidden = true;
    box.hidden = false;
    const ok = !error;
    box.appendChild(el('p', {}, [
      el('span', { class: 'badge ' + (ok ? 'status-ok' : 'status-error'), text: ok ? 'ok' : 'failed' }),
      el('span', { text: ' ' + (ok ? 'Generated in ' + formatMillis(result['duration-ms'] || 0) : error) }),
    ]));
    if (result && result.command) box.appendChild(el('pre', { class: 'json-view' }, [el('code', { text: result.command })]));

    // An output left empty is written under its default name (out.bgp).
    const outputKinds = {};
    Object.entries(GENERATED_FILE_KINDS).forEach(([key, kind]) => {
      const [toolName, optionName] = key.split(':');
      if (toolName !== tool.name) return;
      const option = tool.options.find((o) => o.name === optionName);
      const file = request[optionName] || (option && option.default);
      if (typeof file === 'string') outputKinds[file] = kind;
    });
    const files = (result && result.files) || [];
    if (files.length) {
      const rows = files.map((file) => {
        const kind = outputKinds[file];
        const use = kind && GENERATED_FILE_USES[kind];
        const cmd = use && state.generate.running ? use.command(file, request, state.generate.config || {}) : null;
        const actions = [el('a', { class: 'btn btn-sm', href: API.fileDownloadURL(state.currentInstance, file), download: file, text: 'Download' })];
        if (cmd) {
          const button = el('button', { class: 'btn btn-sm btn-primary', type: 'button', text: 'Load now', title: cmd.command });
          button.addEventListener('click', () => applyGeneratedFile(cmd, button));
          actions.push(button);
        }
        const usage = use && !state.generate.running ? use.snippet(file) : (cmd ? cmd.command : '');
        return el('tr', {}, [
          el('td', { text: file }),
          el('td', {}, [el('code', { text: usage })]),
          el('td', {}, [el('div', { class: 'row-actions-cell' }, actions)]),
        ]);
      });
      box.appendChild(el('div', { class: 'table-scroll' }, [
        el('table', {}, [
          el('caption', { class: 'visually-hidden', text: 'Generated files' }),
          el('thead', {}, [el('tr', {}, ['File', 'Use', 'Actions'].map((h) => el('th', { scope: 'col', text: h })))]),
          el('tbody', {}, rows),
        ]),
      ]));
    }
    [['Tool output', result && result.stdout], ['Tool errors', result && result.stderr]].forEach(([label, text]) => {
      if (!text) return;
      box.appendChild(el('details', { open: !ok ? '' : null }, [
        el('summary', { text: label }),
        el('pre', { class: 'json-view' }, [el('code', { text: text })]),
      ]));
    });
  }

  async function submitGenerate(ev) {
    ev.preventDefault();
    const name = state.currentInstance;
    const tool = state.generate.tool;
    if (!name || !tool || state.generate.busy) return;
    const request = collectGeneratorRequest();
    const button = $('#btn-generate');
    state.generate.busy = true;
    button.disabled = true;
    button.textContent = 'Generating…';
    let result = null;
    let error = null;
    try {
      result = await API.generate(name, tool.name, request);
    } catch (e) {
      // A failed run still reports the tool's output.
      result = (e.body && typeof e.body === 'object') ? e.body : null;
      error = e.message;
    } finally {
      state.generate.busy = false;
      button.disabled = false;
      button.textContent = 'Generate';
    }
    if (state.currentInstance !== name) return;
    const files = (result && result.files) || [];
    addGenerateHistory(tool.name + (files.length ? ': ' + files.join(', ') : '') + (error ? ': ' + error : ''), !error);
    if (error) announce(tool.name + ' failed: ' + error, 'error');
    else announce(tool.name + ' wrote ' + (files.join(', ') || 'no files'), 'success');
    await refreshGenerateContext(name);
    renderGenerateResult(tool, request, result || {}, error);
    // Ready for the next update of a running instance - also after a 409
    // for a name that exists already, which only needs the next free one.
    suggestFreeOutputNames();
  }

  $('#generate-tool-select').addEventListener('change', (ev) => selectGenerator(ev.target.value));
  $('#generate-form').addEventListener('submit', submitGenerate);
  $('#btn-generate-reset').addEventListener('click', () => selectGenerator($('#generate-tool-select').value));

  //= ========================================================================
  // 3c. Dynamic command builder
  //= ========================================================================
  function pick(obj, keys) {
    for (const k of keys) if (obj[k] !== undefined) return obj[k];
    return undefined;
  }

  function normalizeCommands(payload) {
    let list = [];
    if (Array.isArray(payload)) list = payload;
    else if (Array.isArray(payload.commands)) list = payload.commands;
    else if (Array.isArray(payload['command-list'])) list = payload['command-list'];
    return list.map((item) => {
      if (typeof item === 'string') return { name: item, description: '', args: [] };
      const name = pick(item, ['command', 'name']) || 'unknown';
      const description = pick(item, ['description', 'help']) || '';
      const rawArgs = pick(item, ['arguments', 'args', 'parameters']) || [];
      const args = rawArgs.map((a) => {
        if (typeof a === 'string') return { name: a, type: 'string', required: false, description: '' };
        return {
          name: pick(a, ['name', 'argument']) || 'value',
          type: (pick(a, ['type']) || 'string').toLowerCase(),
          subType: (pick(a, ['sub-type', 'subType']) || 'string').toLowerCase(),
          required: !!pick(a, ['required', 'mandatory']),
          description: pick(a, ['description', 'help']) || '',
          enum: pick(a, ['enum', 'choices']),
        };
      });
      return { name, description, args };
    });
  }

  async function loadCommandBuilder(silent) {
    const name = state.currentInstance;
    if (!name) return;
    if (!silent) {
      $('#command-select').innerHTML = '<option>Loading…</option>';
    }
    if (state.instanceCommands[name] && silent) return;
    try {
      const resp = await API.command(name, 'commands');
      state.instanceCommands[name] = normalizeCommands(resp);
    } catch (e) {
      state.instanceCommands[name] = [];
      if (!silent) setCommandOutput({ Error: 'Could not load command list: ' + e.message });
    }
    if (!silent) renderCommandSelect();
  }

  function renderCommandSelect() {
    const name = state.currentInstance;
    const list = state.instanceCommands[name] || [];
    const select = $('#command-select');
    select.innerHTML = '';
    list.forEach((c) => select.appendChild(el('option', { value: c.name, text: prettyLabel(c.name) + (c.description ? ' — ' + c.description : '') })));
    if (list.length) renderCommandArgs(list[0]);
  }
  $('#command-select').addEventListener('change', () => {
    const name = state.currentInstance;
    const cmd = (state.instanceCommands[name] || []).find((c) => c.name === $('#command-select').value);
    if (cmd) renderCommandArgs(cmd);
  });

  let currentCommandArgFields = [];
  function renderCommandArgs(cmd) {
    const fieldset = $('#command-args-fieldset');
    fieldset.innerHTML = '';
    fieldset.appendChild(el('legend', { text: 'Arguments — ' + prettyLabel(cmd.name) }));
    if (!cmd.args.length) {
      fieldset.appendChild(el('p', { class: 'hint', text: 'This command takes no arguments.' }));
      currentCommandArgFields = [];
      return;
    }
    currentCommandArgFields = cmd.args.map((arg) => {
      const id = nextId('arg');
      let input;
      let hint = arg.description;
      if (Array.isArray(arg.enum)) {
        input = el('select', { id: id, required: arg.required || null });
        if (!arg.required) input.appendChild(el('option', { value: '', text: '(not set)' }));
        arg.enum.forEach((v) => input.appendChild(el('option', { value: v, text: String(v) })));
      } else if (arg.type === 'boolean') {
        input = el('input', { type: 'checkbox', id: id });
      } else if (arg.type === 'number') {
        input = el('input', { type: 'number', id: id, required: arg.required || null, step: 'any' });
      } else if (arg.type === 'array') {
        input = el('input', { type: 'text', id: id, required: arg.required || null });
        const subTypeHint = 'Comma-separated list of ' + arg.subType + ' values.';
        hint = hint ? hint + ' ' + subTypeHint : subTypeHint;
      } else {
        input = el('input', { type: 'text', id: id, required: arg.required || null });
      }
      let wrap;
      if (input.type === 'checkbox') {
        wrap = el('div', { class: 'field checkbox-field' }, [input, el('label', { for: id, text: prettyLabel(arg.name) })]);
      } else {
        wrap = fieldWrap(id, prettyLabel(arg.name), arg.required, hint, input);
      }
      fieldset.appendChild(wrap);
      return { name: arg.name, input, type: arg.type, subType: arg.subType };
    });
  }

  // Shows fields (via renderDetailFields) in the Commands tab's response
  // area, swapping the "no command executed yet"/empty placeholder for the
  // field list - same field-per-key, pretty-printed-nested-values rendering
  // used by the stream/session detail dialogs, instead of a single dumped
  // JSON blob.
  function setCommandOutput(fields) {
    $('#command-output-empty').hidden = true;
    $('#command-output').hidden = false;
    renderDetailFields('#command-output', fields);
  }

  $('#command-args-form').addEventListener('submit', async (ev) => {
    ev.preventDefault();
    const name = state.currentInstance;
    const cmdName = $('#command-select').value;
    if (!cmdName) return;
    const args = {};
    const convertScalar = (value, type) => (type === 'number' ? Number(value) : value);
    currentCommandArgFields.forEach((f) => {
      if (f.input.type === 'checkbox') { args[f.name] = f.input.checked; return; }
      if (f.input.value === '') return;
      if (f.type === 'array') {
        args[f.name] = f.input.value.split(',').map((v) => v.trim()).filter(Boolean)
          .map((v) => convertScalar(v, f.subType));
        return;
      }
      args[f.name] = convertScalar(f.input.value, f.type);
    });
    $('#command-output-meta').hidden = true;
    setCommandOutput({ Running: '…' });
    try {
      const resp = await API.command(name, cmdName, args);
      renderCommandResponse(resp);
    } catch (e) {
      if (e.body && typeof e.body === 'object') {
        renderCommandResponse(e.body);
      } else {
        setCommandOutput({ Error: e.message });
      }
    }
  });

  // The bngblaster command response is always {status, code, <result>},
  // where <result> is the one key holding the command's actual payload
  // (a list, an object, or absent for commands with no data to return).
  // Status/code are rendered as small badges above the output box so the
  // main output area only ever shows the payload itself.
  function splitCommandResponse(resp) {
    if (!resp || typeof resp !== 'object' || Array.isArray(resp)) {
      return { status: undefined, code: undefined, result: resp };
    }
    const { status, code, ...rest } = resp;
    const keys = Object.keys(rest);
    const result = keys.length === 0 ? null : (keys.length === 1 ? rest[keys[0]] : rest);
    return { status, code, result };
  }

  function renderCommandResponse(resp) {
    const { status, code, result } = splitCommandResponse(resp);
    const meta = $('#command-output-meta');
    meta.innerHTML = '';
    meta.hidden = status === undefined && code === undefined;
    if (status !== undefined) {
      meta.appendChild(el('span', { class: 'badge ' + (status === 'ok' ? 'status-ok' : 'status-error'), text: 'Status: ' + status }));
    }
    if (code !== undefined) {
      meta.appendChild(el('span', { class: 'badge status-code', text: 'Code: ' + code }));
    }
    if (result === null || result === undefined) {
      $('#command-output').hidden = true;
      const empty = $('#command-output-empty');
      empty.hidden = false;
      empty.textContent = 'Command executed — no data returned.';
    } else {
      renderResultFields('#command-output', result);
    }
  }

  //= ========================================================================
  // 3d. Log viewer dock
  //= ========================================================================
  function onLogInstanceChange() {
    const sel = $('#logdock-instance-select');
    const next = sel.value || null;
    if (next === state.log.instance) return;
    // Lines are appended to whatever is already in the dock, so following a
    // different instance has to start from an empty one - otherwise the new
    // instance's log simply continues below the previous instance's.
    state.log.instance = next;
    clearLogDock();
    restartLogPolling();
  }
  $('#logdock-instance-select').addEventListener('change', () => {
    state.log.manualSelect = true;
    onLogInstanceChange();
  });
  $('#btn-logdock-pause').addEventListener('click', (ev) => {
    state.log.paused = !state.log.paused;
    ev.target.textContent = state.log.paused ? 'Resume' : 'Pause';
    ev.target.setAttribute('aria-pressed', String(state.log.paused));
  });
  function clearLogDock() { $('#logdock-body').innerHTML = ''; }
  $('#btn-logdock-clear').addEventListener('click', clearLogDock);
  // The dock has three sizes: 'normal' (a strip at the bottom), 'collapsed'
  // (header only, the default, set in index.html to avoid a flash on load)
  // and 'maximized' (full viewport). Both buttons go through here so the
  // body classes and button labels never disagree.
  function setLogDockMode(mode) {
    const body = $('#logdock-body');
    const wasAtBottom = body.scrollHeight - body.scrollTop - body.clientHeight < 40;
    document.body.classList.toggle('logdock-collapsed', mode === 'collapsed');
    document.body.classList.toggle('logdock-maximized', mode === 'maximized');
    state.log.mode = mode;
    const toggle = $('#btn-logdock-toggle');
    toggle.textContent = mode === 'collapsed' ? 'Expand' : 'Collapse';
    toggle.setAttribute('aria-expanded', String(mode !== 'collapsed'));
    const maximize = $('#btn-logdock-maximize');
    maximize.textContent = mode === 'maximized' ? 'Restore' : 'Maximize';
    maximize.setAttribute('aria-pressed', String(mode === 'maximized'));
    // Resizing changes clientHeight, so a dock that was following the tail
    // would otherwise stop short of the newest lines.
    if (wasAtBottom) body.scrollTop = body.scrollHeight;
  }
  $('#btn-logdock-toggle').addEventListener('click', () => {
    setLogDockMode(state.log.mode === 'collapsed' ? 'normal' : 'collapsed');
  });
  $('#btn-logdock-maximize').addEventListener('click', () => {
    setLogDockMode(state.log.mode === 'maximized' ? 'normal' : 'maximized');
  });
  // Escape leaves full screen, but not while a dialog is open - there it
  // belongs to the dialog.
  document.addEventListener('keydown', (ev) => {
    if (ev.key === 'Escape' && state.log.mode === 'maximized' && !document.querySelector('dialog[open]')) {
      setLogDockMode('normal');
    }
  });

  function restartLogPolling() {
    state.log.timer = cancelPoll(state.log.timer);
    state.log.timer = schedulePoll(pollLogs, 1500);
    pollLogs();
  }

  function levelClassFor(line) {
    if (/\berror\b/i.test(line)) return 'level-error';
    if (/\bwarn/i.test(line)) return 'level-warn';
    return '';
  }

  async function pollLogs() {
    const name = state.log.instance;
    if (!name || state.log.paused) return;
    const offset = state.log.offsets[name] || 0;
    let resp;
    try {
      resp = await API.logs(name, offset);
    } catch (e) {
      return;
    }
    // Starting an instance deletes and recreates run.log, so an offset from
    // the previous run points into a file that no longer exists. Once the new
    // log has grown past that offset the server cannot detect this from the
    // size alone, and everything written before it would be skipped silently.
    // The generation (the file's identity) changes on every recreation, so
    // rewind and re-read from the beginning whenever it does.
    const generation = resp.generation;
    if (state.log.generations[name] !== generation) {
      state.log.generations[name] = generation;
      if (offset !== 0) {
        state.log.offsets[name] = 0;
        clearLogDock();
        // Re-read this instance from the start on the next tick rather than
        // rendering the tail slice that was fetched against the old offset.
        return;
      }
    }
    state.log.offsets[name] = resp.next_offset;
    if (!resp.lines.length) return;
    const body = $('#logdock-body');
    const wasAtBottom = body.scrollHeight - body.scrollTop - body.clientHeight < 40;
    resp.lines.forEach((line) => {
      body.appendChild(el('div', { class: 'log-line ' + levelClassFor(line), text: line }));
    });
    while (body.childElementCount > 2000) body.removeChild(body.firstChild);
    if (wasAtBottom) body.scrollTop = body.scrollHeight;
    $('#logdock-sr-status').textContent = resp.lines.length + ' new log line' + (resp.lines.length === 1 ? '' : 's') + ' received.';
  }

  //= ========================================================================
  // Uploads dialog
  //= ========================================================================
  const dropzone = $('#dropzone');
  const fileInput = $('#file-input');
  function activateFilePicker() { fileInput.click(); }
  dropzone.addEventListener('click', activateFilePicker);
  dropzone.addEventListener('keydown', (ev) => {
    if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); activateFilePicker(); }
  });
  ['dragenter', 'dragover'].forEach((evt) => dropzone.addEventListener(evt, (ev) => {
    ev.preventDefault(); dropzone.classList.add('dragover');
  }));
  ['dragleave', 'drop'].forEach((evt) => dropzone.addEventListener(evt, (ev) => {
    ev.preventDefault(); dropzone.classList.remove('dragover');
  }));
  dropzone.addEventListener('drop', (ev) => {
    const files = ev.dataTransfer.files;
    if (files && files.length) uploadFiles(files);
  });
  fileInput.addEventListener('change', () => {
    if (fileInput.files.length) uploadFiles(fileInput.files);
    fileInput.value = '';
  });

  async function uploadFiles(fileList) {
    const target = state.uploads.instance;
    if (!target) { announce('No target instance selected for upload.', 'error'); return; }
    const list = $('#upload-list');
    for (const file of Array.from(fileList)) {
      const item = el('li', {}, [el('span', { text: file.name }), el('span', { text: 'Uploading…' })]);
      list.appendChild(item);
      try {
        await API.upload(target, file);
        item.lastChild.textContent = 'Done';
        item.lastChild.style.color = 'var(--color-success)';
      } catch (e) {
        item.lastChild.textContent = 'Failed: ' + e.message;
        item.lastChild.style.color = 'var(--color-danger)';
      }
    }
  }

  //= ========================================================================
  // Boot
  //= ========================================================================
  ['#streams-empty', '#sessions-empty', '#bgp-empty', '#isis-empty', '#ospf-empty', '#lag-empty', '#command-output-empty'].forEach((sel) => {
    DEFAULT_EMPTY_TEXT[sel] = $(sel).textContent;
  });
  initJSONEditorDOM();
  refreshVersion();
  loadInstances();
  schedulePoll(() => { if (!$('#view-dashboard').hidden) loadInstances(); }, 5000);
})();
