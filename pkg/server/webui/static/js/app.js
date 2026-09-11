// BNG Blaster Controller — Web UI application logic.
// Vanilla JS, no build step, no framework: this file is served as-is by the
// embedded controller binary.
(function () {
  'use strict';

  //= ========================================================================
  // API client
  //= ========================================================================
  const API = {
    async fetchJSON(url, opts) {
      const res = await fetch(url, opts);
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
      // used elsewhere and can't go through fetchJSON as-is.
      return fetch('/api/v1/instances/' + encodeURIComponent(name) + '/config.json').then((res) => {
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
    upload(name, file) {
      const fd = new FormData();
      fd.append('file', file);
      return fetch('/api/v1/instances/' + encodeURIComponent(name) + '/_upload', { method: 'POST', body: fd })
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
    // the Session Overview sections while that tab is visible.
    overviewTimer: null,
    stream: { instance: null, total: 0, rowHeight: 34, buffer: 8, pending: false, timer: null, pollTimer: null, filters: {}, detailFlowId: null, detailTimer: null },
    session: { instance: null, total: 0, rowHeight: 34, buffer: 8, pending: false, timer: null, pollTimer: null, filters: {}, detailSessionId: null, detailTimer: null },
    // generations maps instance -> the identity of the run.log it last read,
    // so a restarted instance (which recreates the file) is detected.
    log: { instance: null, offsets: {}, generations: {}, paused: false, manualSelect: false, timer: null },
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

  function renderInstancesTable() {
    const tbody = $('#instances-tbody');
    tbody.innerHTML = '';
    $('#instances-empty').hidden = state.instances.length > 0;
    state.instances.forEach((inst) => {
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
        !running ? el('button', { class: 'btn btn-sm', type: 'button', text: 'Download', onclick: () => showDownloads(inst.name) }) : null,
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
    const units = ['B', 'KB', 'MB', 'GB'];
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

  function showDownloads(name) {
    state.downloads.instance = name;
    $('#dialog-downloads-title').textContent = 'Downloads — ' + name;
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
      report_flags: $('#start-opt-report').checked ? ['sessions', 'streams'] : [],
      logging: $('#start-opt-logging').checked,
      logging_flags: [],
      pcap_capture: $('#start-opt-pcap').checked,
      session_count: sessionCount,
      stream_config: $('#start-opt-stream-config').value.trim(),
      metric_flags: ['session_counters', 'interfaces', 'streams'],
    };
  }
  $('#btn-start-instance-confirm').addEventListener('click', async () => {
    const cfg = collectRunningConfig();
    try {
      await API.start(startDialogTarget, cfg);
      announce('Started ' + startDialogTarget, 'success');
      closeDialog('dialog-start-instance');
      if (startDialogThen) startDialogThen();
      loadInstances();
      if (state.currentInstance === startDialogTarget) refreshInstanceStatus();
    } catch (e) {
      showDetailedError($('#start-instance-status'), 'Could not start instance', e.message);
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
    return /(^|[-_])interface(name)?$/i.test(key);
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
  function buildField(key, rawSchema, root, required) {
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
    if (isInterfaceField(key)) {
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
    const body = el('div', { class: 'form-grid' });
    const children = Object.entries(schema.properties).map(([childKey, childSchema]) => {
      const field = buildField(childKey, childSchema, root, requiredChildren.includes(childKey));
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

  function updateSchemaPreview() {
    const preview = $('#schema-json-preview');
    if (preview) preview.textContent = JSON.stringify(collectFormJSON(), null, 2);
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
      updateSchemaPreview();
      setStatusMessage(jsonStatus, '');
    }
    configMode = mode;
    $('#config-mode-btn-form').setAttribute('aria-pressed', String(mode === 'form'));
    $('#config-mode-btn-json').setAttribute('aria-pressed', String(mode === 'json'));
    $('#schema-form-root').hidden = mode !== 'form';
    $('#schema-json-root').hidden = mode !== 'json';
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
          .map((v) => ({ label: String(v), description: resolved.description || '' }));
        return { range: partial ? [partial.start, offset] : [offset, offset], items };
      }
      if (resolved.type === 'boolean' && (isIdentPartial || !partial)) {
        const prefix = isIdentPartial ? text.slice(partial.start, offset) : '';
        const items = ['true', 'false'].filter((v) => v.startsWith(prefix)).map((v) => ({ label: v, description: '' }));
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

  // The suggestion box is a read-only hint - it lists the property/enum
  // options valid at the cursor (from jsonAutocompleteSuggestions) purely as
  // information, like a tooltip. It never inserts anything: Tab and Enter
  // keep their normal textarea behavior (focus-out / newline).
  let jsonSuggestionsShown = false;

  function hideJSONSuggestions() {
    const box = $('#schema-json-suggest');
    if (box) { box.hidden = true; box.innerHTML = ''; }
    jsonSuggestionsShown = false;
  }

  function showJSONSuggestions(textarea, sugg) {
    const box = $('#schema-json-suggest');
    if (!box || !sugg || !sugg.items.length) { hideJSONSuggestions(); return; }
    jsonSuggestionsShown = true;
    box.innerHTML = '';
    sugg.items.forEach((item) => {
      box.appendChild(el('li', {}, [
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

      const preview = el('details', {}, [el('summary', { text: 'View generated JSON' }), el('pre', { class: 'command-output', id: 'schema-json-preview' })]);
      root.appendChild(preview);
      root.addEventListener('input', debounce(updateSchemaPreview, 200));
      root.addEventListener('change', updateSchemaPreview);
      if (existingConfig) applyJSONToForm(existingConfig);
      updateSchemaPreview();
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
  const TABS = ['overview', 'sessions', 'streams', 'commands'];
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
    if (name === 'commands') loadCommandBuilder();
  }
  $all('[role="tab"]').forEach((tab, idx, all) => {
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

  function showDashboard() {
    state.currentInstance = null;
    stopInstancePolling();
    stopStreamPolling();
    stopSessionPolling();
    $('#view-instance').hidden = true;
    $('#view-dashboard').hidden = false;
    loadInstances();
  }
  $('#btn-back-dashboard').addEventListener('click', showDashboard);
  $('#nav-dashboard').addEventListener('click', showDashboard);

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
    $('#view-dashboard').hidden = true;
    $('#view-instance').hidden = false;
    $('#instance-name').textContent = name;
    if (!state.log.manualSelect) {
      $('#logdock-instance-select').value = name;
      onLogInstanceChange();
    }
    await refreshInstanceStatus();
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
  }
  $('#btn-instance-refresh').addEventListener('click', refreshInstanceStatus);
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
  // the Session Overview tab is visible.
  function renderTestDuration(testInfo) {
    const pill = $('#instance-duration-pill');
    if (!testInfo || typeof testInfo.duration !== 'number') {
      // Instance not running (anymore), or bngblaster predates test-info.
      pill.hidden = true;
      return;
    }
    pill.hidden = false;
    pill.textContent = (testInfo.state ? prettyLabel(testInfo.state) + ' · ' : '') + formatDuration(testInfo.duration);
  }

  //= ========================================================================
  // 3a. Session overview + progress bars
  //= ========================================================================
  function meter(labelText, value, max, variant) {
    const pct = max > 0 ? Math.min(100, Math.round((value / max) * 100)) : 0;
    let cls = 'session-meter';
    if (variant) cls += ' session-meter--' + variant;
    if (pct >= 100) cls += ' is-complete';
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
      const loss = iface['rx-loss-packets-session-' + fam];
      rows.push(['Session ' + prettyLabel(fam),
        (iface[txKey] || 0).toLocaleString() + ' (' + fmtPPS(iface['tx-pps-session-' + fam]) + ')',
        (iface[rxKey] || 0).toLocaleString() + ' (' + fmtPPS(iface['rx-pps-session-' + fam]) + ')' +
          (loss ? ' · loss ' + loss.toLocaleString() : '')]);
    });
    if (iface['tx-packets-streams'] !== undefined || iface['rx-packets-streams'] !== undefined) {
      const loss = iface['rx-loss-packets-streams'];
      rows.push(['Stream Traffic',
        (iface['tx-packets-streams'] || 0).toLocaleString() + ' (' + fmtPPS(iface['tx-pps-streams']) + ')',
        (iface['rx-packets-streams'] || 0).toLocaleString() + ' (' + fmtPPS(iface['rx-pps-streams']) + ')' +
          (loss ? ' · loss ' + loss.toLocaleString() : '')]);
    }
    if (iface['rx-packets-multicast'] !== undefined) {
      const loss = iface['rx-loss-packets-multicast'];
      rows.push(['Multicast', '—',
        (iface['rx-packets-multicast'] || 0).toLocaleString() + ' (' + fmtPPS(iface['rx-pps-multicast']) + ')' +
          (loss ? ' · loss ' + loss.toLocaleString() : '')]);
    }
    const table = el('table', { class: 'iface-stat-table' }, [
      el('thead', {}, [el('tr', {}, [el('th', { scope: 'col', text: '' }), el('th', { scope: 'col', text: 'TX' }), el('th', { scope: 'col', text: 'RX' })])]),
      el('tbody', {}, rows.map(([label, tx, rx]) => el('tr', {}, [
        el('th', { scope: 'row', text: label }),
        el('td', { text: tx }),
        el('td', { text: rx }),
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
      return null;
    }
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
  $('#btn-logdock-toggle').addEventListener('click', (ev) => {
    const collapsed = document.body.classList.toggle('logdock-collapsed');
    ev.target.textContent = collapsed ? 'Expand' : 'Collapse';
    ev.target.setAttribute('aria-expanded', String(!collapsed));
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
  ['#streams-empty', '#sessions-empty', '#command-output-empty'].forEach((sel) => {
    DEFAULT_EMPTY_TEXT[sel] = $(sel).textContent;
  });
  initJSONEditorDOM();
  refreshVersion();
  loadInstances();
  schedulePoll(() => { if (!state.currentInstance) loadInstances(); }, 5000);
})();
