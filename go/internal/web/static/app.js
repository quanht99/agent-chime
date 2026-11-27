// AgentChime settings page. Plain ES2017, no framework, no inline code (CSP).
(function () {
  'use strict';

  var token = document.querySelector('meta[name="agentchime-token"]').content;
  var S = null; // last /api/state body
  var pickers = {};
  var CFG_KEY = { done: 'soundDone', attention: 'soundAttention' };
  var VOL_KEY = { done: 'volumeDone', attention: 'volumeAttention' };
  var HOVER_MS = 1000;

  function $(id) { return document.getElementById(id); }
  function T(key) { return (S && S.texts && S.texts[key]) || ''; }
  function show(el, on) { el.hidden = !on; }
  function setErr(id, msg) { var el = $(id); el.textContent = msg || ''; show(el, !!msg); }

  // ---- network ----

  async function api(method, path, body, raw) {
    var headers = { 'X-AgentChime-Token': token };
    var opts = { method: method, headers: headers };
    if (raw) {
      Object.keys(raw.headers).forEach(function (k) { headers[k] = raw.headers[k]; });
      opts.body = raw.body;
    } else if (method === 'POST') {
      headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body || {});
    }
    var res;
    try {
      res = await fetch(path, opts);
    } catch (e) {
      return { ok: false, status: 0, data: { error: 'network' } };
    }
    var data = {};
    try { data = await res.json(); } catch (e) { /* empty body */ }
    if (res.status === 401) {
      var b = $('banner-reload');
      b.textContent = 'This page has expired. Reload it from the AgentChime menu.';
      show(b, true);
    }
    return { ok: res.ok, status: res.status, data: data };
  }

  function apply(r) {
    if (r.ok && r.data && r.data.texts) { S = r.data; render(); return true; }
    return false;
  }

  // act sends a mutating call; on failure the message goes to errId.
  async function act(path, body, errId) {
    var r = await api('POST', path, body);
    if (apply(r)) { if (errId) setErr(errId, ''); return r; }
    if (errId) setErr(errId, r.data.message || ('Error: ' + (r.data.error || r.status)));
    return r;
  }

  async function refresh() {
    var r = await api('GET', '/api/state');
    apply(r);
  }

  // ---- helpers ----

  function baseName(v) {
    var n = String(v).split(/[\\/]/).pop();
    var i = n.lastIndexOf('.');
    return i > 0 ? n.slice(0, i) : n;
  }
  function sameChoice(a, b) { return a && b && a.source === b.source && a.value === b.value; }
  function currentChoice(kind) { return S.config[CFG_KEY[kind]]; }
  function displayName(choice) {
    for (var g = 0; g < S.groups.length; g++) {
      var items = S.groups[g].items || [];
      for (var i = 0; i < items.length; i++) {
        if (sameChoice(items[i].choice, choice)) return items[i].name;
      }
    }
    return baseName(choice.value);
  }
  var GROUP_KEY = { system: 'groupSystem', builtin: 'groupBuiltIn', file: 'groupFile' };

  // ---- sound picker (custom listbox: a native select has no hover events) ----

  function makePicker(kind) {
    var btn = $('pick-' + kind), list = $('list-' + kind), file = $('file-' + kind);
    var items = []; // {el, choice, name, chooseFile}
    var active = -1, timer = null, isOpen = false;
    var self = { isOpen: function () { return isOpen; }, close: close, relabel: relabel };

    function cancelTimer() { if (timer) { clearTimeout(timer); timer = null; } }
    function startTimer(it) {
      cancelTimer();
      if (!it || !it.choice) return;
      timer = setTimeout(async function () {
        timer = null;
        var r = await api('POST', '/api/preview', { kind: kind, choice: it.choice });
        if (!apply(r)) setErr('sound-error', r.data.message || ('Error: ' + (r.data.error || r.status)));
        else setErr('sound-error', '');
      }, HOVER_MS);
    }

    function relabel() { btn.textContent = displayName(currentChoice(kind)); }

    function build() {
      list.textContent = '';
      items = [];
      S.groups.forEach(function (g) {
        var h = document.createElement('li');
        h.className = 'group-header';
        h.setAttribute('role', 'presentation');
        h.textContent = T(GROUP_KEY[g.id]);
        list.appendChild(h);
        (g.items || []).forEach(function (it) {
          add({ choice: it.choice, name: it.name });
        });
        if (g.id === 'file') {
          add({ choice: null, name: T('chooseFile'), chooseFile: true });
        }
      });
    }

    function add(it) {
      var li = document.createElement('li');
      li.id = 'opt-' + kind + '-' + items.length;
      li.className = 'option';
      li.setAttribute('role', 'option');
      li.setAttribute('aria-selected', it.choice && sameChoice(it.choice, currentChoice(kind)) ? 'true' : 'false');
      li.textContent = it.name;
      var idx = items.length;
      li.addEventListener('mouseover', function () { setActive(idx, false); startTimer(it); });
      li.addEventListener('mouseleave', cancelTimer);
      li.addEventListener('click', function (e) { e.stopPropagation(); choose(it); });
      it.el = li;
      items.push(it);
      list.appendChild(li);
    }

    function setActive(i, fromKey) {
      if (i < 0 || i >= items.length) return;
      if (active >= 0 && items[active]) items[active].el.classList.remove('active');
      active = i;
      items[i].el.classList.add('active');
      list.setAttribute('aria-activedescendant', items[i].el.id);
      if (fromKey) {
        items[i].el.scrollIntoView({ block: 'nearest' });
        startTimer(items[i]);
      }
    }

    function open() {
      Object.keys(pickers).forEach(function (k) { if (k !== kind) pickers[k].close(false); });
      build();
      show(list, true);
      btn.setAttribute('aria-expanded', 'true');
      isOpen = true;
      var sel = items.findIndex(function (it) { return it.choice && sameChoice(it.choice, currentChoice(kind)); });
      active = -1;
      list.focus();
      setActive(sel >= 0 ? sel : 0, false);
      if (sel >= 0) items[sel].el.scrollIntoView({ block: 'nearest' });
    }

    function close(returnFocus) {
      if (!isOpen) return;
      isOpen = false;
      cancelTimer();
      show(list, false);
      btn.setAttribute('aria-expanded', 'false');
      list.removeAttribute('aria-activedescendant');
      api('POST', '/api/preview/stop', {});
      if (returnFocus) btn.focus();
    }

    async function choose(it) {
      close(true);
      if (it.chooseFile) {
        file.click();
        return;
      }
      var body = {};
      body[CFG_KEY[kind]] = it.choice;
      var r = await api('POST', '/api/config', body);
      if (apply(r)) { setErr('sound-error', ''); return; }
      setErr('sound-error', r.data.message || ('Error: ' + (r.data.error || r.status)));
    }

    btn.addEventListener('click', function () { if (isOpen) close(true); else open(); });
    btn.addEventListener('keydown', function (e) {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); if (!isOpen) open(); }
    });
    list.addEventListener('keydown', function (e) {
      if (e.key === 'ArrowDown') { e.preventDefault(); setActive(Math.min(items.length - 1, active + 1), true); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); setActive(Math.max(0, active - 1), true); }
      else if (e.key === 'Home') { e.preventDefault(); setActive(0, true); }
      else if (e.key === 'End') { e.preventDefault(); setActive(items.length - 1, true); }
      else if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); if (items[active]) choose(items[active]); }
      else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); close(true); }
      else if (e.key === 'Tab') { close(false); }
    });
    list.addEventListener('mouseleave', cancelTimer);
    document.addEventListener('click', function (e) {
      if (isOpen && !btn.parentNode.contains(e.target)) close(false);
    });

    file.addEventListener('change', async function () {
      var f = file.files && file.files[0];
      file.value = '';
      if (!f) return;
      var r = await api('POST', '/api/upload?kind=' + kind, null, {
        headers: { 'X-File-Name': encodeURIComponent(f.name) },
        body: f
      });
      if (apply(r)) { setErr('sound-error', ''); return; }
      setErr('sound-error', r.data.message || ('Could not use that file (' + (r.data.error || r.status) + ')'));
    });

    $('preview-' + kind).addEventListener('click', async function () {
      var r = await api('POST', '/api/preview', { kind: kind, choice: currentChoice(kind) });
      if (apply(r)) setErr('sound-error', '');
      else setErr('sound-error', r.data.message || ('Error: ' + (r.data.error || r.status)));
    });

    $('vol-' + kind).addEventListener('input', function () { $('vol-out-' + kind).textContent = this.value + '%'; });
    $('vol-' + kind).addEventListener('change', function () {
      var body = {};
      body[VOL_KEY[kind]] = Number(this.value) / 100;
      act('/api/config', body, 'sound-error');
    });

    pickers[kind] = self;
    return self;
  }

  // ---- render ----

  function focused(el) { return document.activeElement === el; }

  function renderHook(id, info, agent) {
    var row = $(id);
    row.querySelector('.hook-text').textContent = info.text;
    var inst = row.querySelector('.install'), rem = row.querySelector('.remove');
    inst.textContent = T('install');
    rem.textContent = T('remove');
    show(inst, info.kind === 'notInstalled' || info.kind === 'partial');
    show(rem, info.kind === 'installed' || info.kind === 'partial');
    inst.onclick = function () { act('/api/hooks/' + agent + '/install', {}, 'hook-error'); };
    rem.onclick = function () { act('/api/hooks/' + agent + '/uninstall', {}, 'hook-error'); };
  }

  function render() {
    document.documentElement.lang = S.lang;
    $('title').textContent = T('settingsTitle');
    document.title = T('settingsTitle');
    $('banner-notray').textContent = T('noTray');
    show($('banner-notray'), S.noTray);
    $('banner-noplayer').textContent = T('noPlayer');
    show($('banner-noplayer'), S.noPlayer);

    ['done', 'attention'].forEach(function (kind) {
      $('label-' + kind).textContent = T(CFG_KEY[kind] === 'soundDone' ? 'soundDone' : 'soundAttention');
      pickers[kind].relabel();
      $('preview-' + kind).textContent = T('preview');
      $('vol-label-' + kind).textContent = T('volume');
      var v = Math.round(S.config[VOL_KEY[kind]] * 100);
      if (!focused($('vol-' + kind))) $('vol-' + kind).value = v;
      $('vol-out-' + kind).textContent = $('vol-' + kind).value + '%';
    });

    $('threshold-prefix').textContent = T('thresholdPrefix');
    $('threshold-suffix').textContent = T('thresholdSuffix');
    if (!focused($('threshold-input'))) $('threshold-input').value = Math.round(S.config.doneThresholdSeconds / 60);

    renderHook('hook-claude', S.hooks.claude, 'claude');
    renderHook('hook-codex', S.hooks.codex, 'codex');
    var retrust = $('hook-codex').querySelector('.hook-retrust');
    retrust.textContent = T('codexRetrust');
    show(retrust, S.hooks.codex.needsRetrust);
    var warn = $('hook-codex').querySelector('.hook-warn');
    warn.textContent = T('codexPathWarning');
    show(warn, S.hooks.codex.warn);

    $('opt-headless-text').textContent = T('alertHeadless');
    $('opt-headless').checked = S.config.alertHeadless;
    $('opt-autostart-text').textContent = T('openAtLogin');
    $('opt-autostart').checked = S.autostart.enabled;
    show($('opt-autostart-row'), S.autostart.supported);
    $('opt-language-text').textContent = T('language');
    $('opt-language').value = S.lang;

    $('mute-title').textContent = T('menuMute');
    $('mute-text').textContent = S.muted.text;
    $('mute-30').textContent = T('mute30');
    $('mute-60').textContent = T('mute60');
    $('mute-forever').textContent = T('muteForever');
    $('unmute').textContent = T('menuUnmute');
    ['mute-30', 'mute-60', 'mute-forever'].forEach(function (id) { show($(id), !S.muted.on); });
    show($('unmute'), S.muted.on);

    $('status-title').textContent = T('statusTitle');
    $('status-last-alert').textContent = S.status.lastAlert;
    $('status-last-skip').textContent = S.status.lastSkip;
    show($('status-last-skip'), !!S.status.lastSkip);
    $('status-warning').textContent = S.status.soundWarning;
    show($('status-warning'), !!S.status.soundWarning);
    $('open-log').textContent = T('openLog');
    $('quit').textContent = T('menuQuit');
    $('version').textContent = S.version;
  }

  // ---- wiring ----

  function init() {
    makePicker('done');
    makePicker('attention');

    $('threshold-input').addEventListener('change', function () {
      var m = Math.min(120, Math.max(0, Math.round(Number(this.value) || 0)));
      this.value = m;
      act('/api/config', { doneThresholdSeconds: m * 60 }, 'sound-error');
    });
    $('opt-headless').addEventListener('change', function () {
      act('/api/config', { alertHeadless: this.checked }, null);
    });
    $('opt-autostart').addEventListener('change', async function () {
      var r = await act('/api/autostart', { enabled: this.checked }, 'autostart-error');
      if (!r.ok) this.checked = S.autostart.enabled;
    });
    $('opt-language').addEventListener('change', function () {
      act('/api/config', { language: this.value }, null);
    });

    $('mute-30').addEventListener('click', function () { act('/api/mute', { minutes: 30 }, null); });
    $('mute-60').addEventListener('click', function () { act('/api/mute', { minutes: 60 }, null); });
    $('mute-forever').addEventListener('click', function () { act('/api/mute', { minutes: null }, null); });
    $('unmute').addEventListener('click', function () { act('/api/unmute', {}, null); });
    $('open-log').addEventListener('click', function () { act('/api/open-log', {}, null); });
    $('quit').addEventListener('click', function () { act('/api/quit', {}, null); });

    setInterval(function () {
      if (document.visibilityState !== 'visible') return;
      if (Object.keys(pickers).some(function (k) { return pickers[k].isOpen(); })) return;
      var a = document.activeElement;
      if (a && (a.tagName === 'INPUT' || a.tagName === 'TEXTAREA' || a.tagName === 'SELECT')) return;
      refresh();
    }, 5000);

    refresh();
  }

  init();
})();
