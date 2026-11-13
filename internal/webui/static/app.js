(function () {
  'use strict';

  var STORAGE_KEY = 'nekospeed_webui_prefs_v3';
  var LEGACY_STORAGE_KEY = 'nekospeed_webui_prefs_v2';
  var RECENT_SUBS_KEY = 'nekospeed_recent_subs_v1';
  var THEME_KEY = 'nekospeed_ui_color_mode';
  var MAX_LOG_LINES = 1500;

  // 核心 DOM 引用（均在脚本执行时已解析完毕）
  var el = function (id) { return document.getElementById(id); };

  var runBtn = el('runBtn');
  var runBtnLabel = el('runBtnLabel');
  var cancelBtn = el('cancelBtn');
  var resetPrefsBtn = el('resetPrefsBtn');
  var previewNodesBtn = el('previewNodesBtn');
  var previewBtnLabel = el('previewBtnLabel');

  var statusPill = el('statusPill');
  var statusText = el('statusText');
  var netEnvBadge = el('netEnvBadge');
  var netEnvText = el('netEnvText');
  var themeToggleBtn = el('themeToggleBtn');
  var themeToggleLabel = el('themeToggleLabel');
  var themeIconUse = el('themeIconUse');
  var toggleLogCardBtn = el('toggleLogCardBtn');
  var toggleAdvBtn = el('toggleAdvBtn');
  var advDrawer = el('advDrawer');

  var launchCard = el('launchCard');
  var sourceInput = el('source');
  var clearSourceBtn = el('clearSourceBtn');
  var subNameInput = el('subName');
  var uploadStateHint = el('uploadStateHint');
  var fileUploadInput = el('fileUploadInput');
  var fileDropzone = el('fileDropzone');
  var dropzoneEmpty = el('dropzoneEmpty');
  var dropzoneLoaded = el('dropzoneLoaded');
  var loadedFileMeta = el('loadedFileMeta');
  var pastedContent = el('pastedContent');
  var pasteLineHint = el('pasteLineHint');

  var nodePickerDrawer = el('nodePickerDrawer');
  var pickerGrid = el('pickerGrid');
  var pickerCountBadge = el('pickerCountBadge');
  var pickerSearch = el('pickerSearch');
  var pickerSummaryTitle = el('pickerSummaryTitle');

  var runMonitor = el('runMonitor');
  var progressStage = el('progressStage');
  var progressRight = el('progressRight');
  var progressFill = el('progressFill');
  var progressBar = el('progressBar');
  var logDrawer = el('logDrawer');
  var logBox = el('log');
  var logCountBadge = el('logCountBadge');
  var logTrimNotice = el('logTrimNotice');
  var autoScroll = el('autoScroll');
  var copyLogBtn = el('copyLogBtn');
  var clearLogBtn = el('clearLogBtn');
  var logToolbar = el('logToolbar');

  var historySelect = el('historySelect');
  var refreshHistoryBtn = el('refreshHistoryBtn');
  var deleteHistoryBtn = el('deleteHistoryBtn');

  var emptyStateCard = el('emptyStateCard');
  var resultCard = el('resultCard');
  var resultBadge = el('resultBadge');
  var summaryEl = el('summary');
  var statGrid = el('statGrid');
  var thumbs = el('thumbs');
  var jsonLink = el('jsonLink');
  var tabNodesLabel = el('tabNodesLabel');

  var rerenderBtn = el('rerenderBtn');
  var rerenderBtnLabel = el('rerenderBtnLabel');
  var rerenderDirtyDot = el('rerenderDirtyDot');

  var authModal = el('authModal');
  var authForm = el('authForm');
  var authTokenInput = el('authTokenInput');
  var authErrorText = el('authErrorText');
  var authSubmitBtn = el('authSubmitBtn');

  var lightbox = el('lightbox');
  var lightboxImg = el('lightboxImg');
  var lbTitle = el('lbTitle');
  var lbZoomLabel = el('lbZoomLabel');
  var lbDownloadBtn = el('lbDownloadBtn');
  var lbCopyBtn = el('lbCopyBtn');
  var lbCloseBtn = el('lbCloseBtn');
  var lbPrevBtn = el('lbPrevBtn');
  var lbNextBtn = el('lbNextBtn');
  var lbZoomInBtn = el('lbZoomInBtn');
  var lbZoomOutBtn = el('lbZoomOutBtn');
  var lbStage = el('lbStage');

  // 应用状态
  var state = {
    sourceMode: 'url', // 'url' | 'file' | 'paste'
    uploadedFilename: '',
    uploadedFileContent: '',
    subNameBoundSource: '',
    lastAutoSubName: '',
    historyMap: {},
    historyCacheBust: {},
    currentRunId: '',
    initialHistoryLoaded: false,
    isRunning: false,
    logLineCount: 0,
    logTrimmed: false,
    totalNodes: 0,
    completedNodes: 0,
    liveTrafficBytes: 0,
    taskStartMs: 0,
    elapsedTimer: null,
    currentReport: null,
    currentSortKey: 'index',
    currentSortAsc: true,
    previewNodesData: [],
    selectedIndicesSet: new Set(),
    nodePickerActive: false,
    galleryList: [],
    lbIndex: 0,
    lbZoom: 100,
    lastFocusedEl: null,
    rerenderDirty: false,
    authToken: '',
    eventSource: null,
    tableRenderTimer: null,
    tableRenderRaf: null
  };

  var PRESET_MAP = {
    'default': ['speed', 'media', 'ai', 'topo'],
    'all': ['speed', 'media', 'ai', 'topo', 'nat'],
    'nodl': ['media', 'ai', 'topo'],
    'speed': ['speed']
  };

  function escapeHtml(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  function showToast(msg, type) {
    var box = el('toastContainer');
    if (!box) return;
    var item = document.createElement('div');
    item.className = 'toast ' + (type || 'ok');
    item.textContent = msg;
    box.appendChild(item);
    setTimeout(function () {
      item.style.opacity = '0';
      item.style.transition = 'opacity .22s ease';
      setTimeout(function () { item.remove(); }, 240);
    }, 2800);
  }

  // 安全剪贴板复制（兼容非 HTTPS 内网 IP）
  function copyText(text, successMsg) {
    var value = String(text || '');
    function fallbackCopy() {
      try {
        var ta = document.createElement('textarea');
        ta.value = value;
        ta.setAttribute('readonly', '');
        ta.style.position = 'fixed';
        ta.style.left = '-9999px';
        document.body.appendChild(ta);
        ta.select();
        var ok = document.execCommand('copy');
        document.body.removeChild(ta);
        if (ok) {
          showToast(successMsg || '已复制到剪贴板', 'ok');
          return;
        }
      } catch (e) {}
      showToast('复制失败：浏览器限制剪贴板权限', 'err');
    }

    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(value).then(function () {
        showToast(successMsg || '已复制到剪贴板', 'ok');
      }).catch(fallbackCopy);
    } else {
      fallbackCopy();
    }
  }

  // 统一请求封装与 401 令牌弹窗恢复
  function openAuthModal(errMsg) {
    if (!authModal) return;
    if (state.eventSource) {
      try { state.eventSource.close(); } catch (e) {}
      state.eventSource = null;
    }
    if (errMsg) {
      authErrorText.textContent = errMsg;
      authErrorText.classList.remove('hidden');
    } else {
      authErrorText.classList.add('hidden');
    }
    if (authModal.classList.contains('hidden')) {
      state.lastFocusedEl = document.activeElement;
      authModal.classList.remove('hidden');
      setTimeout(function () { authTokenInput.focus(); }, 20);
    }
  }

  function closeAuthModal() {
    if (!authModal) return;
    authModal.classList.add('hidden');
    authErrorText.classList.add('hidden');
    if (state.lastFocusedEl && state.lastFocusedEl.focus) {
      state.lastFocusedEl.focus();
      state.lastFocusedEl = null;
    }
  }

  function apiFetch(url, options) {
    var opts = Object.assign({}, options || {});
    var headers = new Headers(opts.headers || {});
    if (state.authToken) {
      headers.set('X-NekoSpeed-Token', state.authToken);
    }
    opts.headers = headers;
    return fetch(url, opts).then(function (resp) {
      if (resp.status === 401) {
        openAuthModal('身份校验未通过或会话已过期，请输入访问令牌');
        var err = new Error('需要访问令牌（401 未授权）');
        err.status = 401;
        throw err;
      }
      return resp;
    });
  }

  authForm.addEventListener('submit', function (e) {
    e.preventDefault();
    var tok = (authTokenInput.value || '').trim();
    if (!tok) {
      authErrorText.textContent = '请输入访问令牌';
      authErrorText.classList.remove('hidden');
      return;
    }
    authSubmitBtn.disabled = true;
    authSubmitBtn.textContent = '验证中…';
    fetch('/api/status', {
      headers: { 'X-NekoSpeed-Token': tok }
    }).then(function (resp) {
      if (resp.status === 401) {
        throw new Error('访问令牌无效，请重试');
      }
      if (!resp.ok) {
        throw new Error('服务端返回异常状态 ' + resp.status);
      }
      state.authToken = tok;
      // 同步触发一次 GET /?token=... 使服务端写入 HttpOnly Cookie，供 SSE 与图片加载复用
      return fetch('/?token=' + encodeURIComponent(tok), { credentials: 'same-origin' });
    }).then(function () {
      closeAuthModal();
      showToast('令牌验证成功，已恢复连接', 'ok');
      bootstrapRuntime();
    }).catch(function (err) {
      authErrorText.textContent = err.message || '验证失败';
      authErrorText.classList.remove('hidden');
    }).finally(function () {
      authSubmitBtn.disabled = false;
      authSubmitBtn.textContent = '验证并连接';
    });
  });

  // 1. 深浅色主题切换
  function applyUITheme(mode, disableTransitions) {
    var root = document.documentElement;
    if (!root) return;
    if (disableTransitions) {
      root.classList.add('theme-switching');
    }
    if (mode === 'dark') {
      root.setAttribute('data-theme', 'dark');
      if (themeToggleLabel) themeToggleLabel.textContent = '浅色';
      if (themeIconUse) {
        var useEl = themeIconUse.querySelector('use');
        if (useEl) useEl.setAttribute('href', '#i-sun');
      }
    } else {
      root.removeAttribute('data-theme');
      if (themeToggleLabel) themeToggleLabel.textContent = '深色';
      if (themeIconUse) {
        var useEl2 = themeIconUse.querySelector('use');
        if (useEl2) useEl2.setAttribute('href', '#i-moon');
      }
    }
    if (disableTransitions) {
      void root.offsetWidth;
      requestAnimationFrame(function () {
        root.classList.remove('theme-switching');
      });
    }
  }

  var initialTheme = localStorage.getItem(THEME_KEY) ||
    (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  applyUITheme(initialTheme, false);

  themeToggleBtn.addEventListener('click', function () {
    var next = document.documentElement.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
    localStorage.setItem(THEME_KEY, next);
    applyUITheme(next, true);
  });

  // 2. 抽屉控制与 ARIA 同步
  toggleLogCardBtn.addEventListener('click', function () {
    var willShow = runMonitor.classList.contains('hidden');
    runMonitor.classList.toggle('hidden', !willShow);
    toggleLogCardBtn.setAttribute('aria-expanded', willShow ? 'true' : 'false');
    if (willShow) {
      logDrawer.open = true;
    }
  });

  toggleAdvBtn.addEventListener('click', function () {
    var willShow = advDrawer.classList.contains('hidden');
    advDrawer.classList.toggle('hidden', !willShow);
    toggleAdvBtn.setAttribute('aria-expanded', willShow ? 'true' : 'false');
  });

  // 3. 来源模式切换与本地文件拖拽/粘贴
  function setSourceMode(mode) {
    state.sourceMode = mode;
    document.querySelectorAll('.seg-btn[data-source-mode]').forEach(function (btn) {
      var active = btn.getAttribute('data-source-mode') === mode;
      btn.classList.toggle('active', active);
      btn.setAttribute('aria-selected', active ? 'true' : 'false');
    });
    el('paneUrl').classList.toggle('hidden', mode !== 'url');
    el('paneFile').classList.toggle('hidden', mode !== 'file');
    el('panePaste').classList.toggle('hidden', mode !== 'paste');

    if (mode === 'url') {
      uploadStateHint.textContent = '支持拖放文件到卡片任意区域';
    } else if (mode === 'file') {
      uploadStateHint.textContent = state.uploadedFilename
        ? ('当前文件：' + state.uploadedFilename)
        : '选择或拖拽本地配置文件';
    } else {
      uploadStateHint.textContent = '支持多行节点链接或 Clash YAML';
    }
  }

  document.querySelectorAll('.seg-btn[data-source-mode]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var mode = btn.getAttribute('data-source-mode');
      setSourceMode(mode);
      var nextKey = getSourceBindingKey();
      if (nextKey) maybeResetBoundSubName(nextKey);
      if (mode === 'url') sourceInput.focus();
      else if (mode === 'paste') pastedContent.focus();
      resetNodePickerSelection();
    });
  });

  function getSourceBindingKey() {
    if (state.sourceMode === 'file') {
      return state.uploadedFilename ? ('file:' + state.uploadedFilename) : '';
    }
    if (state.sourceMode === 'paste') {
      var txt = pastedContent.value.trim();
      return txt ? ('paste:' + txt.length + ':' + txt.slice(0, 48)) : '';
    }
    var src = sourceInput.value.trim();
    return src ? ('url:' + src) : '';
  }

  function maybeResetBoundSubName(nextKey) {
    if (!state.subNameBoundSource) return;
    if (!nextKey || nextKey !== state.subNameBoundSource) {
      subNameInput.value = '';
      state.lastAutoSubName = '';
      state.subNameBoundSource = '';
    }
  }

  subNameInput.addEventListener('input', function () {
    // 用户手动编辑新名称时解除旧的自动填充标记；仅在启动测速/预览时再绑定到当前来源
    state.lastAutoSubName = '';
    state.subNameBoundSource = '';
  });

  function syncClearSourceBtn() {
    clearSourceBtn.classList.toggle('hidden', !sourceInput.value.trim());
  }

  clearSourceBtn.addEventListener('click', function () {
    sourceInput.value = '';
    syncClearSourceBtn();
    maybeResetBoundSubName('');
    resetNodePickerSelection();
    sourceInput.focus();
  });

  sourceInput.addEventListener('input', function () {
    syncClearSourceBtn();
    var currentVal = sourceInput.value.trim();
    maybeResetBoundSubName(currentVal ? ('url:' + currentVal) : '');
    resetNodePickerSelection();
  });

  function updatePasteLineCount() {
    var txt = pastedContent.value.trim();
    if (!txt) {
      pasteLineHint.textContent = '支持 Base64 订阅原文、节点 URI 列表与完整 YAML';
      return;
    }
    var lines = txt.split(/\r?\n/).filter(function (l) { return l.trim() !== ''; }).length;
    pasteLineHint.textContent = '已输入 ' + lines + ' 行 · ' + txt.length + ' 字符';
  }

  pastedContent.addEventListener('input', function () {
    updatePasteLineCount();
    maybeResetBoundSubName(getSourceBindingKey());
    resetNodePickerSelection();
  });

  el('clearPasteBtn').addEventListener('click', function () {
    pastedContent.value = '';
    updatePasteLineCount();
    maybeResetBoundSubName('');
    resetNodePickerSelection();
    pastedContent.focus();
  });

  function setLoadedFile(name, size, content) {
    var nextKey = 'file:' + name;
    maybeResetBoundSubName(nextKey);
    state.uploadedFilename = name;
    state.uploadedFileContent = content;
    var kb = Math.max(1, Math.round((size || content.length) / 1024));
    loadedFileMeta.textContent = name + ' (' + kb + ' KB)';
    dropzoneEmpty.classList.add('hidden');
    dropzoneLoaded.classList.remove('hidden');

    var baseName = name.replace(/\.[^.]+$/, '');
    if (!subNameInput.value.trim() || subNameInput.value.trim() === state.lastAutoSubName) {
      subNameInput.value = baseName;
      state.lastAutoSubName = baseName;
      state.subNameBoundSource = nextKey;
    }
    setSourceMode('file');
    resetNodePickerSelection();
    showToast('已载入配置文件：' + name, 'ok');
  }

  function clearLoadedFile() {
    state.uploadedFilename = '';
    state.uploadedFileContent = '';
    fileUploadInput.value = '';
    dropzoneLoaded.classList.add('hidden');
    dropzoneEmpty.classList.remove('hidden');
    if (state.sourceMode === 'file') {
      uploadStateHint.textContent = '选择或拖拽本地配置文件';
    }
    maybeResetBoundSubName('');
    resetNodePickerSelection();
  }

  function handleLocalFile(file) {
    if (!file) return;
    var reader = new FileReader();
    reader.onload = function (ev) {
      setLoadedFile(file.name, file.size, String(ev.target.result || ''));
    };
    reader.onerror = function () {
      showToast('读取本地文件失败', 'err');
    };
    reader.readAsText(file);
  }

  fileDropzone.addEventListener('click', function (e) {
    if (e.target.closest('button')) return;
    fileUploadInput.click();
  });
  fileDropzone.addEventListener('keydown', function (e) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      fileUploadInput.click();
    }
  });
  el('replaceFileBtn').addEventListener('click', function () { fileUploadInput.click(); });
  el('clearFileBtn').addEventListener('click', clearLoadedFile);

  fileUploadInput.addEventListener('change', function (e) {
    if (e.target.files && e.target.files[0]) {
      handleLocalFile(e.target.files[0]);
    }
  });

  launchCard.addEventListener('dragover', function (e) {
    e.preventDefault();
    launchCard.classList.add('drag-active');
  });
  launchCard.addEventListener('dragleave', function (e) {
    if (!launchCard.contains(e.relatedTarget)) {
      launchCard.classList.remove('drag-active');
    }
  });
  launchCard.addEventListener('drop', function (e) {
    e.preventDefault();
    launchCard.classList.remove('drag-active');
    if (e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files[0]) {
      handleLocalFile(e.dataTransfer.files[0]);
    }
  });

  // 按当前来源模式获取请求载荷
  function getActiveSourcePayload() {
    if (state.sourceMode === 'file') {
      return {
        hasInput: Boolean(state.uploadedFileContent.trim()),
        source: '',
        content: state.uploadedFileContent,
        filename: state.uploadedFilename
      };
    }
    if (state.sourceMode === 'paste') {
      var inlineText = pastedContent.value.trim();
      return {
        hasInput: Boolean(inlineText),
        source: '',
        content: inlineText,
        filename: ''
      };
    }
    var src = sourceInput.value.trim();
    return {
      hasInput: Boolean(src),
      source: src,
      content: '',
      filename: ''
    };
  }

  function focusMissingSource() {
    if (state.sourceMode === 'url') sourceInput.focus();
    else if (state.sourceMode === 'paste') pastedContent.focus();
    else fileDropzone.focus();
  }

  // 最近订阅记忆（完全兼容旧 key）
  function loadRecentSubs() {
    try {
      var list = JSON.parse(localStorage.getItem(RECENT_SUBS_KEY) || '[]');
      var dl = el('recentSubList');
      dl.innerHTML = '';
      (Array.isArray(list) ? list : []).forEach(function (item) {
        var opt = document.createElement('option');
        opt.value = item;
        dl.appendChild(opt);
      });
    } catch (e) {}
  }

  function rememberSubSource(src) {
    if (!src || /^out[\\/]uploads[\\/]/.test(src)) return;
    try {
      var list = JSON.parse(localStorage.getItem(RECENT_SUBS_KEY) || '[]');
      if (!Array.isArray(list)) list = [];
      list = [src].concat(list.filter(function (x) { return x !== src; })).slice(0, 8);
      localStorage.setItem(RECENT_SUBS_KEY, JSON.stringify(list));
      loadRecentSubs();
    } catch (e) {}
  }

  // 4. 测试模块勾选同步与预设高亮
  function getSelectedTests() {
    var arr = [];
    document.querySelectorAll('#tests input[type="checkbox"]:checked').forEach(function (cb) {
      arr.push(cb.value);
    });
    return arr;
  }

  function syncModuleVisuals() {
    var checked = getSelectedTests();
    document.querySelectorAll('#tests label.pill-check').forEach(function (lbl) {
      var cb = lbl.querySelector('input[type="checkbox"]');
      lbl.classList.toggle('checked', Boolean(cb && cb.checked));
    });

    var sortedCurrent = checked.slice().sort().join(',');
    document.querySelectorAll('.chip-btn[data-preset]').forEach(function (btn) {
      var preset = PRESET_MAP[btn.getAttribute('data-preset')] || [];
      var match = preset.slice().sort().join(',') === sortedCurrent;
      btn.classList.toggle('active', match);
      btn.setAttribute('aria-pressed', match ? 'true' : 'false');
    });
  }

  document.querySelectorAll('#tests input[type="checkbox"]').forEach(function (cb) {
    cb.addEventListener('change', syncModuleVisuals);
  });

  document.querySelectorAll('.chip-btn[data-preset]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var map = PRESET_MAP[btn.getAttribute('data-preset')] || [];
      document.querySelectorAll('#tests input[type="checkbox"]').forEach(function (cb) {
        cb.checked = map.indexOf(cb.value) >= 0;
      });
      syncModuleVisuals();
    });
  });

  // 5. 挑节点预览与勾选器
  function resetNodePickerSelection() {
    state.nodePickerActive = false;
    state.selectedIndicesSet.clear();
    previewBtnLabel.textContent = '挑节点';
  }

  ['filter', 'exclude', 'proxy', 'skipInfo'].forEach(function (id) {
    var field = el(id);
    if (!field) return;
    field.addEventListener(id === 'skipInfo' ? 'change' : 'input', function () {
      if (state.nodePickerActive || !nodePickerDrawer.classList.contains('hidden')) {
        resetNodePickerSelection();
        nodePickerDrawer.classList.add('hidden');
        previewNodesBtn.setAttribute('aria-expanded', 'false');
      }
    });
  });

  function updatePickerSummary() {
    var total = state.previewNodesData.length;
    var selCount = state.selectedIndicesSet.size;
    pickerCountBadge.textContent = selCount + ' / ' + total + ' 已选';
    if (state.nodePickerActive && total > 0 && selCount < total) {
      previewBtnLabel.textContent = '已挑 ' + selCount + '/' + total;
    } else {
      previewBtnLabel.textContent = '挑节点';
    }
  }

  function renderPickerGrid() {
    pickerGrid.innerHTML = '';
    var q = (pickerSearch.value || '').trim().toLowerCase();
    var frag = document.createDocumentFragment();
    state.previewNodesData.forEach(function (n) {
      if (q) {
        var hay = (n.name + ' ' + n.type + ' ' + n.flag + ' ' + n.server).toLowerCase();
        if (hay.indexOf(q) < 0) return;
      }
      var checked = state.selectedIndicesSet.has(n.index);
      var lbl = document.createElement('label');
      lbl.className = 'picker-item' + (checked ? ' checked' : '');
      lbl.title = n.name + ' (' + n.type + ' · ' + n.server + ')';
      lbl.innerHTML =
        '<input type="checkbox" data-idx="' + n.index + '"' + (checked ? ' checked' : '') + '>' +
        '<span class="picker-flag">' + escapeHtml(n.flag || 'UN') + '</span>' +
        '<span class="picker-name">' + escapeHtml(n.cleanName || n.name) + '</span>' +
        (n.isInfo ? '<span class="pill pill-warn">提示</span>' : '<span class="picker-proto">' + escapeHtml(n.type) + '</span>');
      var cb = lbl.querySelector('input');
      cb.addEventListener('change', function () {
        state.nodePickerActive = true;
        if (cb.checked) state.selectedIndicesSet.add(n.index);
        else state.selectedIndicesSet.delete(n.index);
        lbl.classList.toggle('checked', cb.checked);
        updatePickerSummary();
      });
      frag.appendChild(lbl);
    });
    pickerGrid.appendChild(frag);
    updatePickerSummary();
  }

  previewNodesBtn.addEventListener('click', function () {
    var payload = getActiveSourcePayload();
    if (!payload.hasInput) {
      showToast('请先填写订阅链接、选择文件或粘贴节点配置', 'warn');
      focusMissingSource();
      return;
    }

    previewNodesBtn.disabled = true;
    previewBtnLabel.textContent = '解析中…';

    apiFetch('/api/preview', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        source: payload.source,
        content: payload.content,
        filename: payload.filename,
        filter: el('filter').value.trim(),
        exclude: el('exclude').value.trim(),
        skipInfo: el('skipInfo').checked,
        proxy: el('proxy').value.trim()
      })
    }).then(function (resp) {
      if (!resp.ok) return resp.text().then(function (t) { throw new Error(t); });
      return resp.json();
    }).then(function (data) {
      state.previewNodesData = data.nodes || [];
      state.selectedIndicesSet.clear();
      state.previewNodesData.forEach(function (n) {
        if (n.matched) state.selectedIndicesSet.add(n.index);
      });
      state.nodePickerActive = true;
      if (data.subName && (!subNameInput.value.trim() || subNameInput.value.trim() === state.lastAutoSubName)) {
        subNameInput.value = data.subName;
        state.lastAutoSubName = data.subName;
      }
      if (subNameInput.value.trim()) {
        state.subNameBoundSource = getSourceBindingKey();
      }
      pickerSummaryTitle.textContent = '节点预览 · ' + (data.subName || '未命名订阅');
      nodePickerDrawer.classList.remove('hidden');
      previewNodesBtn.setAttribute('aria-expanded', 'true');
      renderPickerGrid();
      showToast('成功解析 ' + state.previewNodesData.length + ' 个节点', 'ok');
    }).catch(function (err) {
      if (err && err.status !== 401) {
        showToast('解析订阅失败：' + err.message, 'err');
      }
    }).finally(function () {
      previewNodesBtn.disabled = false;
      updatePickerSummary();
    });
  });

  pickerSearch.addEventListener('input', renderPickerGrid);
  el('pickerSelectMatchedBtn').addEventListener('click', function () {
    state.selectedIndicesSet.clear();
    state.previewNodesData.forEach(function (n) { if (n.matched) state.selectedIndicesSet.add(n.index); });
    state.nodePickerActive = true;
    renderPickerGrid();
  });
  el('pickerSelectAllBtn').addEventListener('click', function () {
    var q = (pickerSearch.value || '').trim().toLowerCase();
    state.previewNodesData.forEach(function (n) {
      if (!q || (n.name + ' ' + n.type + ' ' + n.flag + ' ' + n.server).toLowerCase().indexOf(q) >= 0) {
        state.selectedIndicesSet.add(n.index);
      }
    });
    state.nodePickerActive = true;
    renderPickerGrid();
  });
  el('pickerInvertBtn').addEventListener('click', function () {
    state.previewNodesData.forEach(function (n) {
      if (state.selectedIndicesSet.has(n.index)) state.selectedIndicesSet.delete(n.index);
      else state.selectedIndicesSet.add(n.index);
    });
    state.nodePickerActive = true;
    renderPickerGrid();
  });
  el('pickerClearBtn').addEventListener('click', function () {
    state.selectedIndicesSet.clear();
    state.nodePickerActive = true;
    renderPickerGrid();
  });
  el('pickerCloseBtn').addEventListener('click', function () {
    nodePickerDrawer.classList.add('hidden');
    previewNodesBtn.setAttribute('aria-expanded', 'false');
  });

  // 6. 参数持久化与恢复（完全兼容旧版 localStorage 结构，同时修正 subName 串台）
  function savePrefs() {
    try {
      var boundKey = getSourceBindingKey() || state.subNameBoundSource;
      if (subNameInput.value.trim() && boundKey) {
        state.subNameBoundSource = boundKey;
      }
      var prefs = {
        source: sourceInput.value,
        subName: subNameInput.value,
        subNameSource: boundKey,
        tests: getSelectedTests(),
        threads: el('threads').value,
        duration: el('duration').value,
        concurrency: el('concurrency').value,
        pingCount: el('pingCount').value,
        maxMB: el('maxMB').value,
        sortBy: el('sortBy').value,
        skipInfo: el('skipInfo').checked,
        preflight: el('preflight').checked,
        filter: el('filter').value,
        exclude: el('exclude').value,
        proxy: el('proxy').value,
        theme: el('themeSelect').value,
        bandwidth: el('bandwidth').value,
        watermark: el('watermark').value,
        speedURL: el('speedURL').value,
        pingURL: el('pingURL').value
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs));
    } catch (e) {}
  }

  function loadPrefs() {
    try {
      var raw = localStorage.getItem(STORAGE_KEY) || localStorage.getItem(LEGACY_STORAGE_KEY);
      if (!raw) return;
      var prefs = JSON.parse(raw);
      if (prefs.source && String(prefs.source).indexOf('[已加载本地文件]') !== 0) {
        sourceInput.value = prefs.source;
        syncClearSourceBtn();
      }
      if (prefs.subName != null) {
        subNameInput.value = prefs.subName;
        var savedBound = prefs.subNameSource != null ? String(prefs.subNameSource).trim() : '';
        if (savedBound && !/^(url|file|paste):/.test(savedBound)) {
          savedBound = 'url:' + savedBound;
        }
        state.subNameBoundSource = savedBound || (prefs.source ? ('url:' + String(prefs.source).trim()) : '');
        state.lastAutoSubName = prefs.subName;
      }
      if (Array.isArray(prefs.tests)) {
        document.querySelectorAll('#tests input[type="checkbox"]').forEach(function (cb) {
          cb.checked = prefs.tests.indexOf(cb.value) >= 0;
        });
      }
      if (prefs.threads != null) el('threads').value = prefs.threads;
      if (prefs.duration != null) el('duration').value = prefs.duration;
      if (prefs.concurrency != null) el('concurrency').value = prefs.concurrency;
      if (prefs.pingCount != null) el('pingCount').value = prefs.pingCount;
      if (prefs.maxMB != null) el('maxMB').value = prefs.maxMB;
      if (prefs.sortBy != null) el('sortBy').value = prefs.sortBy;
      if (typeof prefs.skipInfo === 'boolean') el('skipInfo').checked = prefs.skipInfo;
      if (typeof prefs.preflight === 'boolean') el('preflight').checked = prefs.preflight;
      if (prefs.filter != null) el('filter').value = prefs.filter;
      if (prefs.exclude != null) el('exclude').value = prefs.exclude;
      if (prefs.proxy != null) el('proxy').value = prefs.proxy;
      if (prefs.theme != null) el('themeSelect').value = prefs.theme;
      if (prefs.bandwidth != null) el('bandwidth').value = prefs.bandwidth;
      if (prefs.watermark != null) el('watermark').value = prefs.watermark;
      if (prefs.speedURL != null) el('speedURL').value = prefs.speedURL;
      if (prefs.pingURL != null) el('pingURL').value = prefs.pingURL;
    } catch (e) {}
  }

  resetPrefsBtn.addEventListener('click', function () {
    el('threads').value = 8;
    el('duration').value = 10;
    el('concurrency').value = 6;
    el('pingCount').value = 4;
    el('maxMB').value = 0;
    el('sortBy').value = '';
    el('skipInfo').checked = true;
    el('preflight').checked = true;
    el('filter').value = '';
    el('exclude').value = '';
    el('themeSelect').value = 'default';
    el('bandwidth').value = '';
    el('watermark').value = '';
    el('speedURL').value = '';
    el('pingURL').value = '';
    savePrefs();
    showToast('已恢复默认测速参数', 'ok');
  });

  // 7. 实时运行状态、ETA 与带行数上限的终端日志
  function formatMMSS(sec) {
    sec = Math.max(0, Math.floor(sec));
    var m = Math.floor(sec / 60);
    var s = sec % 60;
    return (m < 10 ? '0' + m : m) + ':' + (s < 10 ? '0' + s : s);
  }

  function formatTrafficBytes(bytes) {
    if (!bytes || bytes <= 0) return '0 MB';
    var mb = bytes / (1024 * 1024);
    if (mb >= 1024) return (mb / 1024).toFixed(2) + ' GB';
    return mb.toFixed(1) + ' MB';
  }

  function formatSpeedBytes(bps) {
    if (!bps || bps < 1) return '0.00 B/s';
    var units = ['B/s', 'KB/s', 'MB/s', 'GB/s'];
    var i = 0;
    var v = bps;
    while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
    return v.toFixed(2) + ' ' + units[i];
  }

  function updateProgressUI(stageText) {
    if (stageText) progressStage.textContent = stageText;
    var pct = state.totalNodes > 0 ? Math.min(100, Math.round((state.completedNodes / state.totalNodes) * 100)) : 5;
    progressFill.style.width = pct + '%';
    progressBar.setAttribute('aria-valuenow', String(pct));
    var elapsedSec = state.taskStartMs > 0 ? (Date.now() - state.taskStartMs) / 1000 : 0;
    var countStr = state.totalNodes > 0 ? (state.completedNodes + ' / ' + state.totalNodes + ' (' + pct + '%)') : '准备中';
    var trafficStr = state.liveTrafficBytes > 0 ? (' · 流量 ' + formatTrafficBytes(state.liveTrafficBytes)) : '';
    var etaStr = '';
    if (state.completedNodes > 0 && state.totalNodes > state.completedNodes && elapsedSec > 2) {
      var remainSec = ((state.totalNodes - state.completedNodes) / state.completedNodes) * elapsedSec;
      etaStr = ' · 预计剩余 ~' + formatMMSS(remainSec);
    }
    progressRight.textContent = countStr + trafficStr + ' · 已用时 ' + formatMMSS(elapsedSec) + etaStr;
    statusText.textContent = '测试中 ' + formatMMSS(elapsedSec);
  }

  function setRunningState(running, startMs) {
    state.isRunning = running;
    if (state.elapsedTimer) { clearInterval(state.elapsedTimer); state.elapsedTimer = null; }
    if (running) {
      state.taskStartMs = startMs || Date.now();
      runBtn.disabled = true;
      runBtnLabel.textContent = '运行中…';
      cancelBtn.disabled = false;
      cancelBtn.classList.remove('hidden');
      statusPill.classList.add('running');
      runMonitor.classList.remove('hidden');
      toggleLogCardBtn.setAttribute('aria-expanded', 'true');
      emptyStateCard.classList.add('hidden');
      updateProgressUI();
      state.elapsedTimer = setInterval(function () { updateProgressUI(); }, 1000);
    } else {
      runBtn.disabled = false;
      runBtnLabel.textContent = '开始测试';
      cancelBtn.classList.add('hidden');
      cancelBtn.disabled = false;
      statusPill.classList.remove('running');
      statusText.textContent = '就绪';
    }
  }

  function appendLog(line) {
    state.logLineCount++;
    logCountBadge.textContent = String(state.logLineCount);

    var div = document.createElement('div');
    div.className = 'log-line';
    if (/^预检：|^输出目录：|^共 \d+ 个节点|^网络环境：/.test(line)) div.className += ' info';
    else if (/跳过/.test(line)) div.className += ' warn';
    else if (/失败|错误|取消/.test(line)) div.className += ' err';
    else if (/^完成：/.test(line)) div.className += ' ok';
    else if (/^\[\d+\/\d+\]/.test(line)) div.className += ' node';
    div.textContent = line;
    logBox.appendChild(div);

    while (logBox.childElementCount > MAX_LOG_LINES) {
      logBox.removeChild(logBox.firstElementChild);
      if (!state.logTrimmed) {
        state.logTrimmed = true;
        logTrimNotice.classList.remove('hidden');
      }
    }

    if (autoScroll.checked) {
      logBox.scrollTop = logBox.scrollHeight;
    }

    var mTotal = line.match(/^共 (\d+) 个节点/);
    if (mTotal) {
      state.totalNodes = parseInt(mTotal[1], 10) || 0;
      state.completedNodes = 0;
      updateProgressUI('正在并发探测节点与预检路由…');
    } else if (/^预检：/.test(line)) {
      updateProgressUI(line);
    } else if (/^完成：/.test(line)) {
      state.completedNodes = state.totalNodes;
      updateProgressUI(line);
    }
  }

  logToolbar.addEventListener('click', function (e) {
    e.stopPropagation();
  });

  clearLogBtn.addEventListener('click', function () {
    logBox.innerHTML = '';
    state.logLineCount = 0;
    state.logTrimmed = false;
    logCountBadge.textContent = '0';
    logTrimNotice.classList.add('hidden');
  });

  copyLogBtn.addEventListener('click', function () {
    copyText(logBox.innerText || '', '已复制运行日志到剪贴板');
  });

  // 8. 节点判定与流式节流渲染
  function isUnlockOK(u) {
    return u && (u.status === 'unlock' || u.status === 'unlocked');
  }

  function isNodeOK(n) {
    if (!n || n.error) return false;
    var hasSpeed = n.speed && n.speed.avg > 0;
    var hasLatency = n.latency && n.latency.avgHTTPS > 0;
    var hasOutbound = n.outbound && n.outbound.ip;
    var hasUnlock = (n.media || []).some(isUnlockOK) || (n.ai || []).some(isUnlockOK);
    return Boolean(hasSpeed || hasLatency || hasOutbound || hasUnlock);
  }

  function formatUnlockBadges(list) {
    if (!list || !list.length) return '<span style="color:var(--muted)">-</span>';
    return list.map(function (u) {
      var st = (u.status || '').toLowerCase();
      var cls = 'pill-muted';
      if (st === 'unlock' || st === 'unlocked') cls = 'pill-ok';
      else if (st === 'restricted') cls = 'pill-warn';
      else if (st === 'failed' || st === 'reqfailed') cls = 'pill-fail';
      var txt = u.platform || '';
      if (u.region) txt += ':' + u.region;
      else if (st === 'unlock') txt += '✓';
      else if (st === 'failed') txt += '✗';
      else if (st === 'reqfailed') txt += '!';
      return '<span class="pill ' + cls + '" title="' + escapeHtml(u.platform + ': ' + st + (u.info ? ' - ' + u.info : '')) + '">' + escapeHtml(txt) + '</span>';
    }).join('');
  }

  function renderStatCards(data) {
    var nodes = data.nodes || [];
    var total = data.total || nodes.length;
    var okCount = data.succeeded || 0;
    var maxPeak = 0, speedSum = 0, speedCount = 0, totalBytes = 0;
    var minLat = 0, latSum = 0, latCount = 0;
    var inSet = new Set(), outSet = new Set();

    nodes.forEach(function (n) {
      if (n.speed && n.speed.max > maxPeak) maxPeak = n.speed.max;
      if (n.speed && n.speed.avg > 0) { speedSum += n.speed.avg; speedCount++; }
      if (n.speed && Array.isArray(n.speed.perSecond)) {
        n.speed.perSecond.forEach(function (b) { totalBytes += (b || 0); });
      }
      if (n.latency && n.latency.avgHTTPS > 0) {
        var l = n.latency.avgHTTPS;
        if (minLat === 0 || l < minLat) minLat = l;
        latSum += l;
        latCount++;
      }
      if (n.inbound && n.inbound.ip) inSet.add(n.inbound.ip);
      if (n.outbound && n.outbound.ip) outSet.add(n.outbound.ip);
    });

    var rate = total > 0 ? Math.round((okCount / total) * 100) : 0;
    el('statNodes').textContent = okCount + ' / ' + total;
    el('statNodesSub').textContent = '连通率 ' + rate + '% · 耗时 ' + (data.elapsed || '进行中');

    var peakMbps = maxPeak > 0 ? Math.round((maxPeak * 8) / 1e6) + ' Mbps' : '-';
    var avgSpd = speedCount > 0 ? formatSpeedBytes(speedSum / speedCount) : '-';
    el('statSpeed').textContent = maxPeak > 0 ? formatSpeedBytes(maxPeak) : '-';
    el('statSpeedSub').textContent = '折合带宽 ' + peakMbps + ' · 均速 ' + avgSpd;

    var avgLat = latCount > 0 ? Math.round(latSum / latCount) + ' ms' : '-';
    el('statLatency').textContent = minLat > 0 ? (minLat + ' ms') : '-';
    el('statLatencySub').textContent = '全场平均 HTTPS 延迟 ' + avgLat;

    var inCount = (data.topology && data.topology.inbounds && data.topology.inbounds.length) || inSet.size;
    var outCount = (data.topology && data.topology.outbounds && data.topology.outbounds.length) || outSet.size;
    el('statTopo').textContent = inCount + ' 入口 → ' + outCount + ' 出口';
    el('statTopoSub').textContent = '累计测速流量 ' + formatTrafficBytes(totalBytes);
    statGrid.classList.remove('hidden');
  }

  function getFilteredAndSortedNodes() {
    if (!state.currentReport || !Array.isArray(state.currentReport.nodes)) return [];
    var q = (el('nodeSearch').value || '').trim().toLowerCase();
    var stFilter = el('nodeStatusFilter').value;

    var rows = state.currentReport.nodes.filter(function (n) {
      var ok = isNodeOK(n);
      if (stFilter === 'ok' && !ok) return false;
      if (stFilter === 'fail' && ok) return false;
      if (!q) return true;
      var unlockText = []
        .concat(n.media || [], n.ai || [])
        .map(function (u) { return (u.platform || '') + ' ' + (u.region || '') + ' ' + (u.status || ''); })
        .join(' ');
      var hay = [
        n.name, n.type, n.server,
        n.inbound && n.inbound.ip, n.inbound && n.inbound.country, n.inbound && n.inbound.org,
        n.outbound && n.outbound.ip, n.outbound && n.outbound.country, n.outbound && n.outbound.org,
        unlockText
      ].join(' ').toLowerCase();
      return hay.indexOf(q) >= 0;
    });

    rows.sort(function (a, b) {
      var va, vb;
      switch (state.currentSortKey) {
        case 'name': va = a.name || ''; vb = b.name || ''; break;
        case 'type': va = a.type || ''; vb = b.type || ''; break;
        case 'tls': va = (a.latency && a.latency.avgTlsRTT) || 999999; vb = (b.latency && b.latency.avgTlsRTT) || 999999; break;
        case 'https': va = (a.latency && a.latency.avgHTTPS) || 999999; vb = (b.latency && b.latency.avgHTTPS) || 999999; break;
        case 'loss': va = (a.latency && a.latency.packetLoss != null) ? a.latency.packetLoss : 1; vb = (b.latency && b.latency.packetLoss != null) ? b.latency.packetLoss : 1; break;
        case 'avgSpeed': va = (a.speed && a.speed.avg) || 0; vb = (b.speed && b.speed.avg) || 0; break;
        case 'maxSpeed': va = (a.speed && a.speed.max) || 0; vb = (b.speed && b.speed.max) || 0; break;
        case 'inbound': va = (a.inbound && a.inbound.ip) || ''; vb = (b.inbound && b.inbound.ip) || ''; break;
        case 'outbound': va = (a.outbound && a.outbound.ip) || ''; vb = (b.outbound && b.outbound.ip) || ''; break;
        default: va = a.index || 0; vb = b.index || 0;
      }
      if (va < vb) return state.currentSortAsc ? -1 : 1;
      if (va > vb) return state.currentSortAsc ? 1 : -1;
      return 0;
    });
    return rows;
  }

  function syncSortHeaders() {
    document.querySelectorAll('#nodeTable th[data-sort]').forEach(function (th) {
      var key = th.getAttribute('data-sort');
      var mark = th.querySelector('.sort-mark');
      if (key === state.currentSortKey) {
        th.setAttribute('aria-sort', state.currentSortAsc ? 'ascending' : 'descending');
        if (mark) mark.textContent = state.currentSortAsc ? '↑' : '↓';
      } else {
        th.setAttribute('aria-sort', 'none');
        if (mark) mark.textContent = '↕';
      }
    });
  }

  function renderNodeTable() {
    var tbody = el('nodeTableBody');
    tbody.innerHTML = '';
    syncSortHeaders();
    var rows = getFilteredAndSortedNodes();
    var frag = document.createDocumentFragment();

    rows.forEach(function (n) {
      var tr = document.createElement('tr');
      var tls = (n.latency && n.latency.avgTlsRTT > 0) ? (n.latency.avgTlsRTT + ' ms') : '-';
      var https = (n.latency && n.latency.avgHTTPS > 0) ? (n.latency.avgHTTPS + ' ms') : '-';
      var loss = (n.latency && n.latency.packetLoss != null) ? Math.round(n.latency.packetLoss * 100) + '%' : '-';
      var avgSpd = (n.speed && n.speed.avg > 0) ? formatSpeedBytes(n.speed.avg) : '-';
      var maxSpd = (n.speed && n.speed.max > 0) ? formatSpeedBytes(n.speed.max) : '-';

      var inStr = n.inbound && n.inbound.ip
        ? '<button type="button" class="ip-copy" data-ip="' + escapeHtml(n.inbound.ip) + '" title="点击复制 IP">' + escapeHtml(n.inbound.ip) + '</button> <small style="color:var(--muted)">(' + escapeHtml(n.inbound.country || '-') + ')</small>'
        : '-';
      var outStr = n.outbound && n.outbound.ip
        ? '<button type="button" class="ip-copy" data-ip="' + escapeHtml(n.outbound.ip) + '" title="点击复制 IP">' + escapeHtml(n.outbound.ip) + '</button> <small style="color:var(--muted)">(' + escapeHtml(n.outbound.country || '-') + ' · ' + escapeHtml(n.outbound.org || n.outbound.asn || '') + ')</small>'
        : (n.error ? '<span class="pill pill-fail">' + escapeHtml(n.error) + '</span>' : '-');

      tr.innerHTML =
        '<td>' + (n.index || '-') + '</td>' +
        '<td style="font-weight:600;">' + escapeHtml(n.name || '-') + '</td>' +
        '<td><span class="pill pill-muted">' + escapeHtml((n.type || '-').toUpperCase()) + '</span></td>' +
        '<td>' + tls + '</td>' +
        '<td><b>' + https + '</b></td>' +
        '<td>' + loss + '</td>' +
        '<td style="color:var(--success-fg); font-weight:700;">' + avgSpd + '</td>' +
        '<td>' + maxSpd + '</td>' +
        '<td>' + inStr + '</td>' +
        '<td>' + outStr + '</td>' +
        '<td class="wrap-cell">' + formatUnlockBadges(n.media) + '</td>' +
        '<td class="wrap-cell">' + formatUnlockBadges(n.ai) + '</td>';
      frag.appendChild(tr);
    });
    tbody.appendChild(frag);
  }

  function scheduleNodeTableRender() {
    if (state.tableRenderTimer) return;
    state.tableRenderTimer = setTimeout(function () {
      state.tableRenderTimer = null;
      if (state.tableRenderRaf) cancelAnimationFrame(state.tableRenderRaf);
      state.tableRenderRaf = requestAnimationFrame(function () {
        state.tableRenderRaf = null;
        renderNodeTable();
      });
    }, 120);
  }

  function onStreamNode(evt) {
    if (!evt || !evt.node) return;
    state.totalNodes = evt.total || state.totalNodes;
    state.completedNodes = evt.done || (state.completedNodes + 1);

    if (!state.currentReport) {
      state.currentReport = { total: state.totalNodes, succeeded: 0, nodes: [], topology: { inbounds: [], outbounds: [] } };
    }
    state.currentReport.total = state.totalNodes;

    var idx = state.currentReport.nodes.findIndex(function (x) { return x.index === evt.node.index; });
    if (idx >= 0) state.currentReport.nodes[idx] = evt.node;
    else state.currentReport.nodes.push(evt.node);

    var okCount = 0;
    var sumBytes = 0;
    state.currentReport.nodes.forEach(function (n) {
      if (isNodeOK(n)) okCount++;
      if (n.speed && Array.isArray(n.speed.perSecond)) {
        n.speed.perSecond.forEach(function (b) { sumBytes += (b || 0); });
      }
    });
    state.currentReport.succeeded = okCount;
    state.liveTrafficBytes = sumBytes;

    updateProgressUI('已完成 [' + state.completedNodes + '/' + state.totalNodes + '] ' + (evt.node.name || ''));

    emptyStateCard.classList.add('hidden');
    resultCard.classList.remove('hidden');
    resultBadge.classList.remove('hidden');
    resultBadge.textContent = '实时更新 (' + state.completedNodes + '/' + state.totalNodes + ')';
    tabNodesLabel.textContent = '节点明细表 (' + state.currentReport.nodes.length + ')';
    renderStatCards(state.currentReport);
    scheduleNodeTableRender();
  }

  // 9. 结果画廊、重绘、CSV 与拓扑渲染
  function setRerenderDirty(dirty) {
    state.rerenderDirty = dirty;
    rerenderDirtyDot.classList.toggle('hidden', !dirty);
    rerenderBtnLabel.textContent = dirty ? '应用并重绘' : '免重测立即重绘';
  }

  ['sortBy', 'themeSelect', 'watermark', 'bandwidth'].forEach(function (id) {
    el(id).addEventListener(id === 'watermark' || id === 'bandwidth' ? 'input' : 'change', function () {
      if (state.currentRunId) setRerenderDirty(true);
    });
  });

  function copyImageToClipboard(url, triggerBtn) {
    var oldLabel = triggerBtn ? triggerBtn.innerHTML : '';
    if (triggerBtn) {
      triggerBtn.disabled = true;
      triggerBtn.textContent = '复制中…';
    }
    apiFetch(url).then(function (r) {
      if (!r.ok) throw new Error('HTTP ' + r.status);
      return r.blob();
    }).then(function (blob) {
      if (!navigator.clipboard || !window.ClipboardItem) {
        throw new Error('当前浏览器不支持图片剪贴板 API（需 HTTPS 或 localhost）');
      }
      var item = new ClipboardItem({ 'image/png': blob });
      return navigator.clipboard.write([item]);
    }).then(function () {
      showToast('图片已复制到剪贴板', 'ok');
      if (triggerBtn) triggerBtn.textContent = '已复制';
      setTimeout(function () {
        if (triggerBtn) { triggerBtn.disabled = false; triggerBtn.innerHTML = oldLabel; }
      }, 1500);
    }).catch(function (err) {
      if (err && err.status !== 401) {
        showToast('复制图片失败：' + err.message, 'err');
      }
      if (triggerBtn) { triggerBtn.disabled = false; triggerBtn.innerHTML = oldLabel; }
    });
  }

  function imageCategoryMeta(name) {
    if (/_full\.png$/.test(name)) return { order: 1, label: '综合全维结果图 (Full)' };
    if (/_speed\.png$/.test(name)) return { order: 2, label: '节点速度与延迟图 (Speed)' };
    if (/_media\.png$/.test(name)) return { order: 3, label: '流媒体解锁检测图 (Media)' };
    if (/_ai\.png$/.test(name)) return { order: 4, label: 'AI 平台可用性图 (AI)' };
    if (/_topology\.png$/.test(name)) return { order: 5, label: '出入口线路拓扑图 (Topology)' };
    return { order: 9, label: '结果图' };
  }

  function switchTab(tabName) {
    document.querySelectorAll('.tab-btn[data-tab]').forEach(function (btn) {
      var active = btn.getAttribute('data-tab') === tabName;
      btn.classList.toggle('active', active);
      btn.setAttribute('aria-selected', active ? 'true' : 'false');
    });
    el('tabGallery').classList.toggle('hidden', tabName !== 'gallery');
    el('tabNodes').classList.toggle('hidden', tabName !== 'nodes');
    el('tabTopo').classList.toggle('hidden', tabName !== 'topo');
  }

  function fetchAndRenderJSON(url) {
    apiFetch(url).then(function (r) {
      if (!r.ok) throw new Error('HTTP ' + r.status);
      return r.json();
    }).then(function (data) {
      state.currentReport = data;
      tabNodesLabel.textContent = '节点明细表 (' + ((data.nodes && data.nodes.length) || 0) + ')';
      renderStatCards(data);
      renderNodeTable();
      renderTopologyTables(data.topology);
    }).catch(function () {
      statGrid.classList.add('hidden');
    });
  }

  function renderFiles(files, runId, cacheBust, skipJSONFetch) {
    state.currentRunId = runId || state.currentRunId;
    thumbs.innerHTML = '';
    jsonLink.classList.add('hidden');
    state.galleryList = [];
    var prefix = state.currentRunId ? ('/files/' + encodeURIComponent(state.currentRunId) + '/') : '/files/';
    var bust = cacheBust ? ('?t=' + cacheBust) : '';

    var pngFiles = [];
    var jsonFileURL = '';
    (files || []).forEach(function (name) {
      if (/_result\.json$/.test(name)) {
        jsonFileURL = prefix + encodeURIComponent(name);
        jsonLink.href = jsonFileURL;
        jsonLink.classList.remove('hidden');
      } else if (/\.png$/.test(name)) {
        pngFiles.push(name);
      }
    });

    pngFiles.sort(function (a, b) {
      return imageCategoryMeta(a).order - imageCategoryMeta(b).order;
    });

    pngFiles.forEach(function (name, idx) {
      var url = prefix + encodeURIComponent(name) + bust;
      var meta = imageCategoryMeta(name);
      state.galleryList.push({ url: url, name: name, label: meta.label });

      var card = document.createElement('div');
      card.className = 'img-card';
      card.innerHTML =
        '<div class="img-card-head">' +
          '<span>' + escapeHtml(meta.label) + '</span>' +
          '<span class="img-card-hint">点击放大</span>' +
        '</div>' +
        '<div class="img-card-body" data-idx="' + idx + '" tabindex="0" role="button" aria-label="放大查看 ' + escapeHtml(meta.label) + '">' +
          '<img src="' + url + '" alt="' + escapeHtml(name) + '" loading="lazy">' +
        '</div>' +
        '<div class="img-card-actions">' +
          '<button type="button" class="btn btn-secondary btn-xs copy-img-btn" data-url="' + url + '">复制图片</button>' +
          '<a class="btn btn-secondary btn-xs" href="' + url + '" download="' + escapeHtml(name) + '">下载 PNG</a>' +
        '</div>';
      thumbs.appendChild(card);
    });

    thumbs.querySelectorAll('.img-card-body[data-idx]').forEach(function (box) {
      var openHandler = function () {
        openLightbox(parseInt(box.getAttribute('data-idx'), 10) || 0);
      };
      box.addEventListener('click', openHandler);
      box.addEventListener('keydown', function (e) {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          openHandler();
        }
      });
    });
    thumbs.querySelectorAll('.copy-img-btn').forEach(function (btn) {
      btn.addEventListener('click', function () {
        copyImageToClipboard(btn.getAttribute('data-url'), btn);
      });
    });

    if (jsonFileURL && !skipJSONFetch) {
      fetchAndRenderJSON(jsonFileURL);
    }
  }

  rerenderBtn.addEventListener('click', function () {
    savePrefs();
    rerenderBtn.disabled = true;
    rerenderBtnLabel.textContent = '重绘中…';
    var t0 = Date.now();

    apiFetch('/api/rerender', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        runId: state.currentRunId,
        sortBy: el('sortBy').value,
        theme: el('themeSelect').value,
        watermark: el('watermark').value.trim(),
        bandwidth: el('bandwidth').value.trim()
      })
    }).then(function (resp) {
      if (!resp.ok) return resp.text().then(function (t) { throw new Error(t); });
      return resp.json();
    }).then(function (res) {
      var cost = ((Date.now() - t0) / 1000).toFixed(2);
      var targetId = res.runId || state.currentRunId;
      var stamp = res.updatedAt || Date.now();
      if (targetId && state.historyMap[targetId] && Array.isArray(res.files)) {
        state.historyMap[targetId].files = res.files;
        state.historyCacheBust[targetId] = stamp;
      }
      renderFiles(res.files || [], targetId, stamp, true);
      setRerenderDirty(false);
      showToast('已按新样式重绘所有结果图（耗时 ' + cost + 's）', 'ok');
    }).catch(function (err) {
      if (err && err.status !== 401) {
        showToast('重绘失败：' + err.message, 'err');
      }
    }).finally(function () {
      rerenderBtn.disabled = false;
      if (!state.rerenderDirty) rerenderBtnLabel.textContent = '免重测立即重绘';
    });
  });

  function csvCell(v) {
    var s = String(v == null ? '' : v);
    if (/^[=+\-@\t\r]/.test(s)) s = "'" + s;
    return '"' + s.replace(/"/g, '""') + '"';
  }

  el('exportCsvBtn').addEventListener('click', function () {
    var rows = getFilteredAndSortedNodes();
    if (!rows.length) {
      showToast('当前没有可导出的节点数据', 'warn');
      return;
    }
    var headers = ['序号', '节点名称', '协议', 'TLS延迟(ms)', 'HTTPS延迟(ms)', '丢包率(%)', '平均速度(B/s)', '最高速度(B/s)', '入口IP', '入口地区', '出口IP', '出口地区', '出口ASN/组织', '流媒体解锁', 'AI解锁', '错误'];
    var lines = [headers.map(csvCell).join(',')];
    rows.forEach(function (n) {
      var mediaStr = (n.media || []).map(function (u) { return u.platform + ':' + (u.region || u.status); }).join('|');
      var aiStr = (n.ai || []).map(function (u) { return u.platform + ':' + (u.region || u.status); }).join('|');
      lines.push([
        n.index, n.name || '', n.type || '',
        (n.latency && n.latency.avgTlsRTT) || 0,
        (n.latency && n.latency.avgHTTPS) || 0,
        (n.latency && n.latency.packetLoss != null) ? Math.round(n.latency.packetLoss * 100) : '',
        (n.speed && Math.round(n.speed.avg)) || 0,
        (n.speed && Math.round(n.speed.max)) || 0,
        (n.inbound && n.inbound.ip) || '',
        (n.inbound && n.inbound.country) || '',
        (n.outbound && n.outbound.ip) || '',
        (n.outbound && n.outbound.country) || '',
        (n.outbound && (n.outbound.org || n.outbound.asn)) || '',
        mediaStr, aiStr, n.error || ''
      ].map(csvCell).join(','));
    });
    var blob = new Blob(['\uFEFF' + lines.join('\r\n')], { type: 'text/csv;charset=utf-8;' });
    var a = document.createElement('a');
    var url = URL.createObjectURL(blob);
    a.href = url;
    a.download = 'nekospeed_' + (state.currentRunId || 'nodes') + '.csv';
    a.click();
    setTimeout(function () { URL.revokeObjectURL(url); }, 1000);
    showToast('已导出 ' + rows.length + ' 条节点数据到 CSV', 'ok');
  });

  function renderTopologyTables(topo) {
    var inBody = el('inboundTableBody');
    var outBody = el('outboundTableBody');
    inBody.innerHTML = '';
    outBody.innerHTML = '';
    if (!topo) return;

    (topo.inbounds || []).forEach(function (e) {
      var g = e.geo || {};
      var tr = document.createElement('tr');
      tr.innerHTML =
        '<td>' + e.index + '</td>' +
        '<td><button type="button" class="ip-copy" data-ip="' + escapeHtml(g.ip || '') + '">' + escapeHtml(g.ip || '-') + '</button></td>' +
        '<td>' + escapeHtml((g.country || '') + (g.city ? ' · ' + g.city : '')) + '</td>' +
        '<td class="topo-wrap">' + escapeHtml((g.asn || '') + ' ' + (g.org || g.isp || '')) + '</td>' +
        '<td><span class="pill pill-muted">IPv' + escapeHtml(g.stack || '4') + '</span></td>' +
        '<td><b>' + (e.outCount || 0) + '</b></td>';
      inBody.appendChild(tr);
    });

    (topo.outbounds || []).forEach(function (e, idx) {
      var g = e.geo || {};
      var tr = document.createElement('tr');
      tr.innerHTML =
        '<td>' + (idx + 1) + '</td>' +
        '<td>入口 #' + e.inboundIndex + '</td>' +
        '<td><button type="button" class="ip-copy" data-ip="' + escapeHtml(g.ip || '') + '">' + escapeHtml(g.ip || '-') + '</button></td>' +
        '<td>' + escapeHtml(g.country || '-') + '</td>' +
        '<td class="topo-wrap">' + escapeHtml((g.asn || '') + ' ' + (g.org || '')) + '</td>' +
        '<td class="topo-wrap" style="font-weight:600;">' + escapeHtml(e.nodeName || '-') + '</td>';
      outBody.appendChild(tr);
    });
  }

  el('nodeSearch').addEventListener('input', renderNodeTable);
  el('nodeStatusFilter').addEventListener('change', renderNodeTable);

  document.querySelectorAll('#nodeTable th[data-sort]').forEach(function (th) {
    var applySort = function () {
      var key = th.getAttribute('data-sort');
      if (state.currentSortKey === key) state.currentSortAsc = !state.currentSortAsc;
      else {
        state.currentSortKey = key;
        state.currentSortAsc = (key !== 'avgSpeed' && key !== 'maxSpeed');
      }
      renderNodeTable();
    };
    th.addEventListener('click', applySort);
    th.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        applySort();
      }
    });
  });

  document.addEventListener('click', function (e) {
    var btn = e.target.closest('.ip-copy');
    if (!btn) return;
    var ip = btn.getAttribute('data-ip');
    if (ip) copyText(ip, '已复制 IP：' + ip);
  });

  var tabOrder = ['gallery', 'nodes', 'topo'];
  document.querySelectorAll('.tab-btn[data-tab]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      switchTab(btn.getAttribute('data-tab'));
    });
    btn.addEventListener('keydown', function (e) {
      var cur = tabOrder.indexOf(btn.getAttribute('data-tab'));
      if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') {
        e.preventDefault();
        var next = (cur + (e.key === 'ArrowRight' ? 1 : -1) + tabOrder.length) % tabOrder.length;
        var nextBtn = document.querySelector('.tab-btn[data-tab="' + tabOrder[next] + '"]');
        if (nextBtn) {
          nextBtn.focus();
          switchTab(tabOrder[next]);
        }
      }
    });
  });

  // 10. 无障碍全屏 Lightbox
  function applyLbZoom() {
    lightboxImg.style.width = state.lbZoom + '%';
    lightboxImg.style.maxWidth = state.lbZoom > 100 ? 'none' : '96%';
    lbZoomLabel.textContent = state.lbZoom + '%';
  }

  function openLightbox(idx) {
    if (!state.galleryList.length) return;
    if (!lightbox.classList.contains('open')) {
      state.lastFocusedEl = document.activeElement;
    }
    state.lbIndex = (idx + state.galleryList.length) % state.galleryList.length;
    var item = state.galleryList[state.lbIndex];
    lightboxImg.src = item.url;
    lightboxImg.alt = item.label;
    lbTitle.textContent = item.label + ' — ' + item.name + ' (' + (state.lbIndex + 1) + '/' + state.galleryList.length + ')';
    lbDownloadBtn.href = item.url;
    lbDownloadBtn.setAttribute('download', item.name);
    state.lbZoom = 100;
    applyLbZoom();
    lightbox.classList.add('open');
    lightbox.setAttribute('aria-hidden', 'false');
    setTimeout(function () { lbCloseBtn.focus(); }, 10);
  }

  function closeLightbox() {
    lightbox.classList.remove('open');
    lightbox.setAttribute('aria-hidden', 'true');
    if (state.lastFocusedEl && state.lastFocusedEl.focus) {
      state.lastFocusedEl.focus();
      state.lastFocusedEl = null;
    }
  }

  lbCloseBtn.addEventListener('click', closeLightbox);
  lbStage.addEventListener('click', function (e) {
    if (e.target === lbStage) closeLightbox();
  });
  lbPrevBtn.addEventListener('click', function () { openLightbox(state.lbIndex - 1); });
  lbNextBtn.addEventListener('click', function () { openLightbox(state.lbIndex + 1); });
  lbZoomInBtn.addEventListener('click', function () { state.lbZoom = Math.min(250, state.lbZoom + 25); applyLbZoom(); });
  lbZoomOutBtn.addEventListener('click', function () { state.lbZoom = Math.max(50, state.lbZoom - 25); applyLbZoom(); });
  lbCopyBtn.addEventListener('click', function () {
    if (state.galleryList[state.lbIndex]) copyImageToClipboard(state.galleryList[state.lbIndex].url, lbCopyBtn);
  });

  document.addEventListener('keydown', function (e) {
    if (lightbox.classList.contains('open')) {
      if (e.key === 'Escape') closeLightbox();
      else if (e.key === 'ArrowLeft') openLightbox(state.lbIndex - 1);
      else if (e.key === 'ArrowRight') openLightbox(state.lbIndex + 1);
      else if (e.key === '+' || e.key === '=') { state.lbZoom = Math.min(250, state.lbZoom + 25); applyLbZoom(); }
      else if (e.key === '-') { state.lbZoom = Math.max(50, state.lbZoom - 25); applyLbZoom(); }
      else if (e.key === 'Tab') {
        var focusables = lightbox.querySelectorAll('button, a[href]');
        if (focusables.length) {
          var first = focusables[0];
          var last = focusables[focusables.length - 1];
          if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
          else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
        }
      }
      return;
    }

    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && !runBtn.disabled && authModal.classList.contains('hidden')) {
      e.preventDefault();
      runBtn.click();
    }
  });

  // 11. 历史记录管理（独立工作区工具栏 + 空状态切换）
  function selectHistoryItem(id, isAutoInit) {
    if (!id || !state.historyMap[id]) {
      deleteHistoryBtn.classList.add('hidden');
      return;
    }
    historySelect.value = id;
    deleteHistoryBtn.classList.remove('hidden');
    var item = state.historyMap[id];
    state.currentRunId = item.id;
    emptyStateCard.classList.add('hidden');
    resultCard.classList.remove('hidden');
    resultBadge.classList.remove('hidden');
    resultBadge.textContent = isAutoInit ? '最近报告' : '历史记录';
    summaryEl.textContent = item.title;
    setRerenderDirty(false);
    renderFiles(item.files, item.id, state.historyCacheBust[item.id] || 0);
  }

  function loadHistory(preferId) {
    apiFetch('/api/history').then(function (r) {
      if (!r.ok) throw new Error('HTTP ' + r.status);
      return r.json();
    }).then(function (items) {
      var list = Array.isArray(items) ? items : [];
      var cur = preferId || historySelect.value;
      historySelect.innerHTML = '<option value="">历史测速记录（共 ' + list.length + ' 条）…</option>';
      state.historyMap = {};
      list.forEach(function (item) {
        state.historyMap[item.id] = item;
        var opt = document.createElement('option');
        opt.value = item.id;
        opt.textContent = item.title;
        historySelect.appendChild(opt);
      });

      if (cur && state.historyMap[cur]) {
        historySelect.value = cur;
        deleteHistoryBtn.classList.remove('hidden');
        if (preferId && preferId !== state.currentRunId) selectHistoryItem(cur, false);
      } else if (!state.initialHistoryLoaded && list.length > 0 && !state.isRunning) {
        selectHistoryItem(list[0].id, true);
      } else {
        deleteHistoryBtn.classList.add('hidden');
        if (list.length === 0 && !state.isRunning && !state.currentReport) {
          resultCard.classList.add('hidden');
          resultBadge.classList.add('hidden');
          summaryEl.textContent = '';
          emptyStateCard.classList.remove('hidden');
        }
      }
      state.initialHistoryLoaded = true;
    }).catch(function () {});
  }

  historySelect.addEventListener('change', function () {
    selectHistoryItem(historySelect.value, false);
  });

  refreshHistoryBtn.addEventListener('click', function () {
    loadHistory();
    showToast('已刷新历史记录列表', 'ok');
  });

  deleteHistoryBtn.addEventListener('click', function () {
    var id = historySelect.value;
    if (!id || !state.historyMap[id]) return;
    var title = state.historyMap[id].title;
    if (!confirm('确定要永久删除历史记录「' + title + '」吗？')) return;
    apiFetch('/api/history/' + encodeURIComponent(id), { method: 'DELETE' }).then(function (resp) {
      if (!resp.ok) return resp.text().then(function (t) { throw new Error(t); });
      historySelect.value = '';
      deleteHistoryBtn.classList.add('hidden');
      if (state.currentRunId === id) {
        state.currentRunId = '';
        state.currentReport = null;
      }
      state.initialHistoryLoaded = false;
      loadHistory();
      showToast('已删除历史记录：' + id, 'ok');
    }).catch(function (err) {
      if (err && err.status !== 401) {
        showToast('删除失败：' + err.message, 'err');
      }
    });
  });

  // 12. 状态与 SSE 实时连接
  function onDone(evt) {
    setRunningState(false);
    emptyStateCard.classList.add('hidden');
    resultCard.classList.remove('hidden');
    resultBadge.classList.remove('hidden');
    resultBadge.textContent = '本次结果';
    if (!evt.ok) {
      summaryEl.innerHTML = '<span style="color:var(--danger-fg)">任务中断：' + escapeHtml(evt.error || '未知错误') + '</span>';
      showToast('任务中断：' + (evt.error || '未知错误'), 'err');
      return;
    }
    progressStage.textContent = '测速任务全部完成';
    progressFill.style.width = '100%';
    progressBar.setAttribute('aria-valuenow', '100');
    summaryEl.innerHTML = '<span style="color:var(--success-fg)">成功 ' + evt.succeeded + ' / ' + evt.total +
      ' 个节点</span> · 总耗时 ' + escapeHtml(evt.elapsed || '');
    switchTab('gallery');
    setRerenderDirty(false);
    renderFiles(evt.files || [], evt.runId || '', Date.now(), false);
    loadHistory(evt.runId || '');
    showToast('测速全部完成！成功 ' + evt.succeeded + '/' + evt.total + ' 个节点', 'ok');
  }

  function connectSSE() {
    if (state.eventSource) {
      try { state.eventSource.close(); } catch (e) {}
    }
    var url = '/api/events' + (state.authToken ? ('?token=' + encodeURIComponent(state.authToken)) : '');
    var es = new EventSource(url);
    state.eventSource = es;
    es.onmessage = function (e) { appendLog(e.data); };
    es.addEventListener('node', function (e) {
      try { onStreamNode(JSON.parse(e.data)); } catch (err) {}
    });
    es.addEventListener('done', function (e) {
      try { onDone(JSON.parse(e.data)); } catch (err) { appendLog('解析结果失败: ' + err); }
    });
    es.onerror = function () {};
  }

  function loadStatus() {
    return apiFetch('/api/status').then(function (r) {
      if (!r.ok) throw new Error('HTTP ' + r.status);
      return r.json();
    }).then(function (st) {
      if (st.netEnv && st.netEnv.physicalInterface) {
        netEnvBadge.classList.remove('hidden');
        if (st.netEnv.tunDetected) {
          netEnvBadge.classList.add('tun-warn');
          netEnvText.textContent = '已绕过本地 TUN · 物理直连: ' + st.netEnv.physicalInterface;
          netEnvBadge.title = '检测到本地虚拟网卡 (' + (st.netEnv.tunInterfaces || []).join(', ') + ')，已强制绑定物理网卡 ' + st.netEnv.physicalInterface + ' (' + (st.netEnv.physicalIP || '') + ') 直连，杜绝代理套代理';
        } else {
          netEnvBadge.classList.remove('tun-warn');
          netEnvText.textContent = '物理直连: ' + st.netEnv.physicalInterface;
          netEnvBadge.title = '测速套接字已绑定物理网卡 ' + st.netEnv.physicalInterface + ' (' + (st.netEnv.physicalIP || '') + ')';
        }
      }
      if (st.running) {
        state.currentRunId = st.runId || '';
        setRunningState(true, st.startedAt);
      }
      return st;
    });
  }

  function bootstrapRuntime() {
    loadStatus().then(function () {
      loadHistory();
      connectSSE();
    }).catch(function () {});
  }

  // 13. 启动与停止任务
  cancelBtn.addEventListener('click', function () {
    cancelBtn.disabled = true;
    apiFetch('/api/cancel', { method: 'POST' }).then(function () {
      showToast('正在停止当前测速任务…', 'warn');
    }).catch(function () {});
  });

  runBtn.addEventListener('click', function () {
    var payload = getActiveSourcePayload();
    if (!payload.hasInput) {
      showToast('请先填写订阅链接、上传文件或粘贴节点配置', 'warn');
      focusMissingSource();
      return;
    }
    var tests = getSelectedTests();
    if (!tests.length) {
      showToast('请至少勾选一项测试模块', 'warn');
      return;
    }
    if (state.nodePickerActive && state.previewNodesData.length > 0 && state.selectedIndicesSet.size === 0) {
      showToast('您在「挑节点」中清空了所有节点，请至少勾选 1 个节点', 'warn');
      return;
    }

    savePrefs();
    if (payload.source) rememberSubSource(payload.source);

    var threadsVal = parseInt(el('threads').value, 10) || 8;
    var selectedIndices = [];
    if (state.nodePickerActive && state.previewNodesData.length > 0 && state.selectedIndicesSet.size < state.previewNodesData.length) {
      selectedIndices = Array.from(state.selectedIndicesSet);
    }

    var body = {
      source: payload.source,
      content: payload.content,
      filename: payload.filename,
      name: subNameInput.value.trim(),
      tests: tests.join(','),
      threads: threadsVal,
      duration: parseInt(el('duration').value, 10) || 10,
      concurrency: parseInt(el('concurrency').value, 10) || 6,
      pingCount: parseInt(el('pingCount').value, 10) || 4,
      maxMB: parseInt(el('maxMB').value, 10) || 0,
      sortBy: el('sortBy').value,
      single: threadsVal === 1,
      skipInfo: el('skipInfo').checked,
      preflight: el('preflight').checked,
      filter: el('filter').value.trim(),
      exclude: el('exclude').value.trim(),
      proxy: el('proxy').value.trim(),
      theme: el('themeSelect').value,
      bandwidth: el('bandwidth').value.trim(),
      watermark: el('watermark').value.trim(),
      speedURL: el('speedURL').value.trim(),
      pingURL: el('pingURL').value.trim(),
      selectedIndices: selectedIndices
    };

    logBox.innerHTML = '';
    state.logLineCount = 0;
    state.logTrimmed = false;
    logCountBadge.textContent = '0';
    logTrimNotice.classList.add('hidden');
    state.totalNodes = 0;
    state.completedNodes = 0;
    state.liveTrafficBytes = 0;
    state.currentReport = { total: 0, succeeded: 0, nodes: [], topology: { inbounds: [], outbounds: [] } };
    nodePickerDrawer.classList.add('hidden');
    previewNodesBtn.setAttribute('aria-expanded', 'false');

    runBtn.disabled = true;
    runBtnLabel.textContent = '启动中…';

    apiFetch('/api/run', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    }).then(function (resp) {
      if (resp.status === 409) {
        showToast('已有任务在运行中，请等待完成或点击停止', 'warn');
        return null;
      }
      if (!resp.ok) return resp.text().then(function (t) { throw new Error(t); });
      return resp.json();
    }).then(function (res) {
      if (!res) {
        runBtn.disabled = false;
        runBtnLabel.textContent = '开始测试';
        return;
      }
      state.currentRunId = res.runId || '';
      state.galleryList = [];
      thumbs.innerHTML = '<div style="grid-column:1/-1; color:var(--muted); font-size:13px; padding:12px 2px;">测速完成后将在此生成多维结果图…</div>';
      jsonLink.classList.add('hidden');
      statGrid.classList.add('hidden');
      el('inboundTableBody').innerHTML = '';
      el('outboundTableBody').innerHTML = '';
      setRerenderDirty(false);
      setRunningState(true, Date.now());
      updateProgressUI('正在拉取订阅并初始化节点池…');
      emptyStateCard.classList.add('hidden');
      resultCard.classList.remove('hidden');
      resultBadge.classList.remove('hidden');
      resultBadge.textContent = '初始化中…';
      summaryEl.innerHTML = '<span style="color:var(--primary-fg)">已启动测试，节点完成后将实时流式刷新…</span>';
      tabNodesLabel.textContent = '节点明细表';
      switchTab('nodes');
      renderNodeTable();
    }).catch(function (err) {
      runBtn.disabled = false;
      runBtnLabel.textContent = '开始测试';
      if (err && err.status !== 401) {
        appendLog('启动失败: ' + err.message);
        showToast('启动失败：' + err.message, 'err');
      }
    });
  });

  // 初始化
  loadRecentSubs();
  loadPrefs();
  syncModuleVisuals();
  bootstrapRuntime();
})();
