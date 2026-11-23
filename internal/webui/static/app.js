(function () {
  'use strict';

  var STORAGE_KEY = 'nekospeed_webui_prefs_v3';
  var LEGACY_STORAGE_KEY = 'nekospeed_webui_prefs_v2';
  var RECENT_SUBS_KEY = 'nekospeed_recent_subs_v1';
  var THEME_KEY = 'nekospeed_ui_color_mode';
  var TOPO_SPLIT_KEY = 'nekospeed_topo_split_v1';
  var DEFAULT_TOPO_SPLIT = 42.5;
  var MAX_LOG_LINES = 1500;

  var el = function (id) { return document.getElementById(id); };

  var runBtn = el('runBtn');
  var runBtnLabel = el('runBtnLabel');
  var cancelBtn = el('cancelBtn');
  var resetPrefsBtn = el('resetPrefsBtn');
  var previewNodesBtn = el('previewNodesBtn');
  var previewBtnLabel = el('previewBtnLabel');

  var statusPill = el('statusPill');
  var statusText = el('statusText');
  var envGeoWrap = el('envGeoWrap');
  var envGeoBtn = el('envGeoBtn');
  var envGeoDot = el('envGeoDot');
  var envGeoSummary = el('envGeoSummary');
  var envGeoPopover = el('envGeoPopover');
  var netEnvTag = el('netEnvTag');
  var netEnvText = el('netEnvText');
  var themeToggleBtn = el('themeToggleBtn');
  var themeIconUse = el('themeIconUse');
  var toggleLogCardBtn = el('toggleLogCardBtn');
  var updateGeoBtn = el('updateGeoBtn');
  var geoBtnLabel = el('geoBtnLabel');
  var geoModeBadge = el('geoModeBadge');
  var geoModeText = el('geoModeText');
  var geoProgressWrap = el('geoProgressWrap');
  var geoProgressBar = el('geoProgressBar');
  var geoFilesList = el('geoFilesList');
  var geoAttribution = el('geoAttribution');
  var toggleAdvBtn = el('toggleAdvBtn');
  var advDrawer = el('advDrawer');
  var advSummaryChip = el('advSummaryChip');
  var customModulesBtn = el('customModulesBtn');
  var testsRow = el('tests');

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

  var historyPickerWrap = el('historyPickerWrap');
  var historyPickerBtn = el('historyPickerBtn');
  var historyPickerLabel = el('historyPickerLabel');
  var historyCountBadge = el('historyCountBadge');
  var historyPopover = el('historyPopover');
  var historyListEl = el('historyList');
  var refreshHistoryBtn = el('refreshHistoryBtn');

  var emptyStateCard = el('emptyStateCard');
  var resultCard = el('resultCard');
  var resultBarRight = el('resultBarRight');
  var resultBadge = el('resultBadge');
  var summaryEl = el('summary');
  var statGrid = el('statGrid');
  var thumbs = el('thumbs');
  var jsonLink = el('jsonLink');
  var tabNodesLabel = el('tabNodesLabel');

  var galleryTypePills = el('galleryTypePills');
  var galleryCopyBtn = el('galleryCopyBtn');
  var galleryDownloadBtn = el('galleryDownloadBtn');
  var galleryFullscreenBtn = el('galleryFullscreenBtn');
  var toggleRerenderBtn = el('toggleRerenderBtn');
  var rerenderDrawer = el('rerenderDrawer');
  var themeSwatches = el('themeSwatches');
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
  var lbZoomToggleBtn = el('lbZoomToggleBtn');
  var lbDownloadBtn = el('lbDownloadBtn');
  var lbCopyBtn = el('lbCopyBtn');
  var lbCloseBtn = el('lbCloseBtn');
  var lbPrevBtn = el('lbPrevBtn');
  var lbNextBtn = el('lbNextBtn');
  var lbZoomInBtn = el('lbZoomInBtn');
  var lbZoomOutBtn = el('lbZoomOutBtn');
  var lbStage = el('lbStage');

  var state = {
    sourceMode: 'url',
    uploadedFilename: '',
    uploadedFileContent: '',
    subNameBoundSource: '',
    lastAutoSubName: '',
    historyItems: [],
    historyMap: {},
    historyCacheBust: {},
    historyPopoverOpen: false,
    armedDeleteId: '',
    armedDeleteTimer: null,
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
    nodeFilter: 'all',
    previewNodesData: [],
    selectedIndicesSet: new Set(),
    nodePickerActive: false,
    galleryList: [],
    activeGalleryIdx: 0,
    activeGalleryCategory: '',
    lbIndex: 0,
    lbMode: 'fit',
    lbScale: 100,
    lastFocusedEl: null,
    rerenderDirty: false,
    rerenderBusy: false,
    authToken: '',
    eventSource: null,
    tableRenderTimer: null,
    tableRenderRaf: null,
    geoUpdating: false,
    geoPollTimer: null,
    lastGeoState: null,
    lastNetEnv: null,
    envPopoverPinned: false,
    envHoverTimer: null,
    availableThemes: null,
    customModulesManuallyOpen: false
  };

  var PRESET_MAP = {
    'default': ['speed', 'media', 'ai', 'topo'],
    'speed': ['speed'],
    'nodl': ['media', 'ai', 'topo'],
    'all': ['speed', 'media', 'ai', 'topo', 'nat']
  };

  var PLATFORM_SHORT = {
    youtube: 'YT',
    netflix: 'NF',
    disney_plus: 'D+',
    bilibili: 'B站',
    tiktok: 'TT',
    dazn: 'DAZN',
    abema: 'Abema',
    bahamut: '巴哈',
    steam: 'Steam',
    spotify: 'Spotify',
    openai: 'GPT',
    copilot: 'Copilot',
    gemini: 'Gemini',
    llama: 'Meta',
    claude: 'Claude',
    perplexity: 'PPLX',
    sora: 'Sora',
    grok: 'Grok'
  };

  var COUNTRY_ZH = {
    HK: '中国香港', TW: '中国台湾', MO: '中国澳门', CN: '中国',
    JP: '日本', SG: '新加坡', US: '美国', KR: '韩国',
    GB: '英国', UK: '英国', DE: '德国', FR: '法国',
    NL: '荷兰', CA: '加拿大', AU: '澳大利亚', RU: '俄罗斯',
    IN: '印度', IL: '以色列', MY: '马来西亚', TH: '泰国',
    VN: '越南', PH: '菲律宾', ID: '印尼', TR: '土耳其',
    BR: '巴西', AR: '阿根廷', AE: '阿联酋', CH: '瑞士',
    SE: '瑞典', IT: '意大利', ES: '西班牙', NG: '尼日利亚',
    PL: '波兰', FI: '芬兰', UA: '乌克兰', RO: '罗马尼亚',
    CZ: '捷克', AT: '奥地利', IE: '爱尔兰', NO: '挪威',
    DK: '丹麦', BE: '比利时', PT: '葡萄牙', GR: '希腊',
    HU: '匈牙利', BG: '保加利亚', LU: '卢森堡', IS: '冰岛',
    EE: '爱沙尼亚', LV: '拉脱维亚', LT: '立陶宛', MX: '墨西哥',
    CL: '智利', CO: '哥伦比亚', PE: '秘鲁', ZA: '南非',
    EG: '埃及', KZ: '哈萨克斯坦', PK: '巴基斯坦', BD: '孟加拉国',
    NZ: '新西兰', SA: '沙特阿拉伯', QA: '卡塔尔', KH: '柬埔寨',
    CY: '塞浦路斯', RS: '塞尔维亚', HR: '克罗地亚', SK: '斯洛伐克',
    SI: '斯洛文尼亚', MM: '缅甸', LA: '老挝', NP: '尼泊尔',
    LK: '斯里兰卡', BH: '巴林', OM: '阿曼', KW: '科威特',
    JO: '约旦', GE: '格鲁吉亚', AM: '亚美尼亚', AZ: '阿塞拜疆',
    BY: '白俄罗斯', MD: '摩尔多瓦', AL: '阿尔巴尼亚', MK: '北马其顿',
    BA: '波黑', ME: '黑山', MT: '马耳他', MC: '摩纳哥',
    LI: '列支敦士登', AD: '安道尔', SM: '圣马力诺'
  };

  var THEME_META = {
    'default': { label: '经典粉', color: '#EE7299' },
    'sky_light': { label: '晴空蓝', color: '#1C88E5' },
    'rose_light': { label: '玫瑰红', color: '#F43F5E' },
    'peach_light': { label: '蜜桃橙', color: '#FB923C' },
    'amber_light': { label: '琥珀金', color: '#F59E0B' },
    'yellow_light': { label: '柠檬黄', color: '#EAB308' },
    'lime_light': { label: '青柠绿', color: '#84CC16' },
    'green_light': { label: '草木绿', color: '#22C55E' },
    'mint_light': { label: '薄荷绿', color: '#10B981' },
    'teal_light': { label: '水鸭青', color: '#14B8A6' },
    'cyan_light': { label: '冰川青', color: '#06B6D4' },
    'azure_light': { label: '蔚蓝', color: '#0EA5E9' },
    'blue_light': { label: '宝石蓝', color: '#3B82F6' },
    'indigo_light': { label: '靛青', color: '#6366F1' },
    'violet_light': { label: '紫罗兰', color: '#8B5CF6' },
    'purple_light': { label: '丁香紫', color: '#A855F7' },
    'pink_light': { label: '樱花粉', color: '#EC4899' },
    'nord_dark': { label: '极夜蓝', color: '#1E3A5F' },
    'tokyo_dark': { label: '东京暗', color: '#273654' },
    'forest_dark': { label: '暗森林', color: '#1E4620' },
    'sunset_dark': { label: '暮色棕', color: '#6D3B1A' },
    '': { label: '随机配色', color: '#94A3B8' }
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
      item.classList.add('is-leaving');
      setTimeout(function () { item.remove(); }, 200);
    }, 2800);
  }

  function copyText(text, successMsg) {
    var value = String(text || '');
    function fallbackCopy() {
      try {
        var ta = document.createElement('textarea');
        ta.value = value;
        ta.setAttribute('readonly', '');
        ta.className = 'sr-only';
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

  // 统一请求封装与 401 令牌恢复
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
      if (resp.status === 401) throw new Error('访问令牌无效，请重试');
      if (!resp.ok) throw new Error('服务端返回异常状态 ' + resp.status);
      state.authToken = tok;
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
    if (disableTransitions) root.classList.add('theme-switching');
    if (mode === 'dark') {
      root.setAttribute('data-theme', 'dark');
      themeToggleBtn.setAttribute('aria-label', '切换为浅色界面');
      themeToggleBtn.title = '切换为浅色界面';
      if (themeIconUse) {
        var useEl = themeIconUse.querySelector('use');
        if (useEl) useEl.setAttribute('href', '#i-sun');
      }
    } else {
      root.removeAttribute('data-theme');
      themeToggleBtn.setAttribute('aria-label', '切换为深色界面');
      themeToggleBtn.title = '切换为深色界面';
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

  // 2. 顶栏环境与 GeoIP 合并弹出卡片
  function setEnvPopoverOpen(open, pinned) {
    if (typeof pinned === 'boolean') state.envPopoverPinned = pinned;
    envGeoPopover.classList.toggle('hidden', !open);
    envGeoBtn.setAttribute('aria-expanded', open ? 'true' : 'false');
  }

  envGeoBtn.addEventListener('click', function (e) {
    e.stopPropagation();
    var isOpen = !envGeoPopover.classList.contains('hidden');
    if (isOpen && state.envPopoverPinned) {
      setEnvPopoverOpen(false, false);
    } else {
      setEnvPopoverOpen(true, true);
    }
  });

  envGeoWrap.addEventListener('mouseenter', function () {
    if (state.envHoverTimer) { clearTimeout(state.envHoverTimer); state.envHoverTimer = null; }
    setEnvPopoverOpen(true);
  });

  envGeoWrap.addEventListener('mouseleave', function () {
    if (state.envPopoverPinned || state.geoUpdating) return;
    state.envHoverTimer = setTimeout(function () {
      if (!state.envPopoverPinned && !state.geoUpdating) setEnvPopoverOpen(false, false);
    }, 180);
  });

  function updateEnvGeoSummaryPill() {
    var net = state.lastNetEnv;
    var geo = state.lastGeoState;
    var netPart = '直连就绪';
    if (net && net.physicalInterface) {
      netPart = net.tunDetected ? 'TUN绕过' : '物理直连';
    }
    var geoPart = 'IP库检测中';
    envGeoDot.classList.remove('is-offline', 'is-hybrid', 'is-online', 'is-updating');
    if (geo) {
      if (geo.updating) {
        geoPart = '更新中 ' + (geo.done || 0) + '/' + (geo.total || 3);
        envGeoDot.classList.add('is-updating');
      } else if (geo.mode === 'offline') {
        geoPart = geo.stale ? '离线IP(待更新)' : '离线IP';
        envGeoDot.classList.add(geo.stale ? 'is-online' : 'is-offline');
      } else if (geo.mode === 'hybrid') {
        geoPart = '混合IP';
        envGeoDot.classList.add('is-hybrid');
      } else {
        geoPart = '纯在线IP';
        envGeoDot.classList.add('is-online');
      }
    }
    envGeoSummary.textContent = netPart + ' · ' + geoPart;
  }

  // 3. 日志与高级参数抽屉
  toggleLogCardBtn.addEventListener('click', function () {
    var willShow = runMonitor.classList.contains('hidden');
    runMonitor.classList.toggle('hidden', !willShow);
    toggleLogCardBtn.setAttribute('aria-expanded', willShow ? 'true' : 'false');
    if (willShow) logDrawer.open = true;
  });

  toggleAdvBtn.addEventListener('click', function () {
    var willShow = advDrawer.classList.contains('hidden');
    advDrawer.classList.toggle('hidden', !willShow);
    toggleAdvBtn.setAttribute('aria-expanded', willShow ? 'true' : 'false');
  });

  customModulesBtn.addEventListener('click', function () {
    var willShow = testsRow.classList.contains('hidden');
    state.customModulesManuallyOpen = willShow;
    testsRow.classList.toggle('hidden', !willShow);
    customModulesBtn.setAttribute('aria-expanded', willShow ? 'true' : 'false');
    customModulesBtn.classList.toggle('active', willShow || !matchesAnyPreset(getSelectedTests()));
  });

  function updateAdvSummaryChip() {
    if (!advSummaryChip) return;
    var threads = parseInt(el('threads').value, 10) || 8;
    var dur = parseInt(el('duration').value, 10) || 10;
    var conc = parseInt(el('concurrency').value, 10) || 6;
    var parts = [threads + '线程', dur + 's', conc + '并发'];
    var maxMB = parseInt(el('maxMB').value, 10) || 0;
    if (maxMB > 0) parts.push('限流' + maxMB + 'MB');
    var name = subNameInput.value.trim();
    if (name) parts.push(name);
    advSummaryChip.textContent = parts.join(' · ');
  }

  ['threads', 'duration', 'concurrency', 'maxMB', 'subName', 'pingCount'].forEach(function (id) {
    var field = el(id);
    if (field) field.addEventListener('input', updateAdvSummaryChip);
  });

  // 4. 来源模式切换与智能粘贴检测
  function setSourceMode(mode) {
    state.sourceMode = mode;
    document.querySelectorAll('.source-tab[data-source-mode]').forEach(function (btn) {
      var active = btn.getAttribute('data-source-mode') === mode;
      btn.classList.toggle('active', active);
      btn.setAttribute('aria-selected', active ? 'true' : 'false');
    });
    el('paneUrl').classList.toggle('hidden', mode !== 'url');
    el('paneFile').classList.toggle('hidden', mode !== 'file');
    el('panePaste').classList.toggle('hidden', mode !== 'paste');

    if (mode === 'url') {
      uploadStateHint.textContent = '支持拖放文件到启动台任意区域';
    } else if (mode === 'file') {
      uploadStateHint.textContent = state.uploadedFilename
        ? ('当前文件：' + state.uploadedFilename)
        : '选择或拖拽本地配置文件';
    } else {
      uploadStateHint.textContent = '支持多行节点链接或 Clash YAML';
    }
  }

  document.querySelectorAll('.source-tab[data-source-mode]').forEach(function (btn) {
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

  function isPastedNodePayload(text) {
    var trimmed = (text || '').trim();
    if (!trimmed) return false;
    var lines = trimmed.split(/\r?\n/).filter(function (l) { return l.trim() !== ''; });
    if (lines.length === 1 && /^https?:\/\/\S+$/i.test(lines[0])) {
      return false;
    }
    if (lines.length > 1) return true;
    if (/(?:^|\r?\n)\s*proxies\s*:/i.test(text)) return true;
    if (/(?:^|\s)(vless|vmess|ss|ssr|trojan|hysteria2|hy2|tuic|anytls):\/\//i.test(trimmed)) return true;
    return false;
  }

  sourceInput.addEventListener('paste', function (e) {
    var cd = e.clipboardData || window.clipboardData;
    if (!cd) return;
    var text = cd.getData('text') || '';
    if (!isPastedNodePayload(text)) return;
    e.preventDefault();
    pastedContent.value = text;
    setSourceMode('paste');
    updatePasteLineCount();
    maybeResetBoundSubName(getSourceBindingKey());
    resetNodePickerSelection();
    pastedContent.focus();
    showToast('检测到多行节点或分享链接，已自动切换至「粘贴文本」模式', 'ok');
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
      updateAdvSummaryChip();
    }
  }

  subNameInput.addEventListener('input', function () {
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
      updateAdvSummaryChip();
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

  document.querySelectorAll('[data-quickstart]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var mode = btn.getAttribute('data-quickstart');
      setSourceMode(mode);
      if (mode === 'url') {
        sourceInput.focus();
      } else if (mode === 'paste') {
        pastedContent.focus();
      } else if (mode === 'file') {
        fileUploadInput.click();
      }
      launchCard.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    });
  });

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
    if (!src || /(?:^|[\\/])out[\\/]uploads[\\/]/.test(src)) return;
    try {
      var list = JSON.parse(localStorage.getItem(RECENT_SUBS_KEY) || '[]');
      if (!Array.isArray(list)) list = [];
      list = [src].concat(list.filter(function (x) { return x !== src; })).slice(0, 8);
      localStorage.setItem(RECENT_SUBS_KEY, JSON.stringify(list));
      loadRecentSubs();
    } catch (e) {}
  }

  // 5. 测试模块勾选同步与场景预设
  function getSelectedTests() {
    var arr = [];
    document.querySelectorAll('#tests input[type="checkbox"]:checked').forEach(function (cb) {
      arr.push(cb.value);
    });
    return arr;
  }

  function matchesAnyPreset(checked) {
    var sortedCurrent = checked.slice().sort().join(',');
    return Object.keys(PRESET_MAP).some(function (k) {
      return PRESET_MAP[k].slice().sort().join(',') === sortedCurrent;
    });
  }

  function syncModuleVisuals() {
    var checked = getSelectedTests();
    document.querySelectorAll('#tests label.pill-check').forEach(function (lbl) {
      var cb = lbl.querySelector('input[type="checkbox"]');
      lbl.classList.toggle('checked', Boolean(cb && cb.checked));
    });

    var sortedCurrent = checked.slice().sort().join(',');
    var matchedAny = false;
    document.querySelectorAll('.chip-btn[data-preset]').forEach(function (btn) {
      var preset = PRESET_MAP[btn.getAttribute('data-preset')] || [];
      var match = preset.slice().sort().join(',') === sortedCurrent;
      if (match) matchedAny = true;
      btn.classList.toggle('active', match);
      btn.setAttribute('aria-pressed', match ? 'true' : 'false');
    });

    if (!matchedAny) {
      testsRow.classList.remove('hidden');
      customModulesBtn.setAttribute('aria-expanded', 'true');
      customModulesBtn.classList.add('active');
    } else if (!state.customModulesManuallyOpen) {
      testsRow.classList.add('hidden');
      customModulesBtn.setAttribute('aria-expanded', 'false');
      customModulesBtn.classList.remove('active');
    } else {
      customModulesBtn.classList.add('active');
    }
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

  // 6. 挑节点预览与勾选器
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
        updateAdvSummaryChip();
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

  // 7. 参数持久化与恢复
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
    syncThemeSwatches();
    updateAdvSummaryChip();
    savePrefs();
    showToast('已恢复默认测速参数', 'ok');
  });

  // 8. 实时运行状态、ETA 与日志
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
    if (state.lastGeoState) renderGeoStatus(state.lastGeoState);
    if (state.historyPopoverOpen) renderHistoryList();
  }

  function appendLog(line) {
    state.logLineCount++;
    logCountBadge.textContent = String(state.logLineCount);

    var div = document.createElement('div');
    div.className = 'log-line';
    if (/^预检：|^输出目录：|^共 \d+ 个节点|^网络环境：|^GeoIP：/.test(line)) div.className += ' info';
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

  logToolbar.addEventListener('click', function (e) { e.stopPropagation(); });

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

  // 9. GeoIP 格式化与节点解码渲染
  function localizeCountry(code) {
    var c = String(code || '').trim().toUpperCase();
    return COUNTRY_ZH[c] || String(code || '').trim();
  }

  function cleanGeoStr(s) {
    var v = String(s || '').trim();
    if (!v || v === '未知' || v.toLowerCase() === 'unknown') return '';
    v = v.replace(/未知$/g, '').trim();
    return v;
  }

  function formatInboundRegion(g) {
    if (!g) return '';
    var cCode = String(g.country || '').trim().toUpperCase();
    var cName = localizeCountry(cCode);
    var reg = cleanGeoStr(g.region);
    var city = cleanGeoStr(g.city);
    var loc = '';
    if (cCode === 'HK' || cCode === 'TW' || cCode === 'MO') {
      if (city && city !== '香港' && city !== '台湾' && city !== '澳门' && city !== cName) loc = city;
      else if (reg && reg !== '香港' && reg !== '台湾' && reg !== '澳门' && reg !== cName) loc = reg;
    } else if (reg && city) {
      if (city === reg || city.indexOf(reg) === 0) loc = city;
      else if (cCode === 'CN') loc = reg + ' ' + city;
      else loc = reg + ' · ' + city;
    } else {
      loc = city || reg;
    }
    var parts = [];
    if (cName) parts.push(cName);
    if (loc && loc !== cName) parts.push(loc);
    return parts.join(' · ');
  }

  function formatInboundSummary(g) {
    if (!g) return '';
    var region = formatInboundRegion(g);
    var isp = cleanGeoStr(g.isp) || cleanGeoStr(g.org) || cleanGeoStr(g.asn);
    var parts = [];
    if (region) parts.push(region);
    if (isp) parts.push(isp);
    return parts.join(' · ') || '-';
  }

  function formatOutboundRegion(g) {
    if (!g) return '';
    var cCode = String(g.country || '').trim().toUpperCase();
    var cName = localizeCountry(cCode);
    var city = cleanGeoStr(g.city) || cleanGeoStr(g.region);
    if ((cCode === 'HK' || cCode === 'TW' || cCode === 'MO') && (city === '香港' || city === '台湾' || city === '澳门' || city === cName)) {
      city = '';
    }
    var parts = [];
    if (cName) parts.push(cName);
    if (city && city !== cName) parts.push(city);
    return parts.join(' · ');
  }

  function formatOutboundOrg(g) {
    if (!g) return '';
    var org = cleanGeoStr(g.org);
    var isp = cleanGeoStr(g.isp);
    if (org && org.toLowerCase() !== 'private customer') return org;
    return isp || org || cleanGeoStr(g.asn);
  }

  function formatOutboundSummary(g) {
    if (!g) return '';
    var region = formatOutboundRegion(g);
    var org = formatOutboundOrg(g);
    var parts = [];
    if (region) parts.push(region);
    if (org) parts.push(org);
    return parts.join(' · ') || '-';
  }

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

  function shortPlatformName(p) {
    return PLATFORM_SHORT[p] || p || '';
  }

  function formatUnlockText(u, st, shortName) {
    var reg = String(u.region || '').trim();
    var info = String(u.info || '').trim();
    if (st === 'unlock' || st === 'unlocked') {
      if (reg) {
        if (reg === '港澳台地区') reg = '港澳台';
        return shortName + ':' + reg;
      }
      if (info) {
        return shortName + ':' + info.replace(/^货币[:：]\s*/, '');
      }
      return shortName + '✓';
    }
    if (st === 'pending') {
      if (u.platform === 'netflix') return shortName + ':仅自制';
      if (info) return shortName + ':' + info;
      if (reg) return shortName + ':' + reg + '(待)';
      return shortName + ':待解锁';
    }
    if (st === 'senttocn') {
      if (reg && reg !== 'CN') return shortName + ':送中(' + reg + ')';
      return shortName + ':送中';
    }
    if (st === 'failed') {
      return shortName + (reg ? ':' + reg + '✗' : '✗');
    }
    if (st === 'reqfailed') {
      return shortName + '!';
    }
    return shortName;
  }

  function formatUnlockBadges(list) {
    if (!list || !list.length) return '<span class="cell-muted">-</span>';
    var highlighted = [];
    var collapsed = [];

    list.forEach(function (u) {
      var st = (u.status || '').toLowerCase();
      var shortName = shortPlatformName(u.platform);
      var txt = formatUnlockText(u, st, shortName);
      var tip = (u.platform || shortName) + ': ' + st + (u.region ? ' (' + u.region + ')' : '') + (u.info ? ' - ' + u.info : '');

      if (st === 'unlock' || st === 'unlocked') {
        highlighted.push('<span class="pill pill-ok" title="' + escapeHtml(tip) + '">' + escapeHtml(txt) + '</span>');
      } else if (st === 'pending') {
        highlighted.push('<span class="pill pill-warn" title="' + escapeHtml(tip) + '">' + escapeHtml(txt) + '</span>');
      } else if (st === 'senttocn') {
        highlighted.push('<span class="pill pill-sentcn" title="' + escapeHtml(tip) + '">' + escapeHtml(txt) + '</span>');
      } else {
        var failCls = (st === 'failed' || st === 'reqfailed') ? 'pill-fail' : 'pill-muted';
        collapsed.push({
          html: '<span class="pill ' + failCls + '" title="' + escapeHtml(tip) + '">' + escapeHtml(txt) + '</span>',
          tip: shortName + (u.region ? '(' + u.region + ')' : '') + (st === 'reqfailed' ? ' 请求失败' : st === 'na' ? ' N/A' : ' 未解锁')
        });
      }
    });

    var out = '<div class="unlock-cell-list">' + highlighted.join('');
    if (collapsed.length > 0) {
      var summaryTitle = collapsed.map(function (c) { return c.tip; }).join(' / ');
      var extraHtml = collapsed.map(function (c) { return c.html; }).join('');
      out +=
        '<button type="button" class="pill pill-fail-summary" data-expand-unlock="true" data-count="' + collapsed.length + '" aria-expanded="false" title="' + escapeHtml(summaryTitle) + '">+' + collapsed.length + ' 未解锁</button>' +
        '<span class="unlock-extra-list hidden">' + extraHtml + '</span>';
    }
    out += '</div>';
    return out;
  }

  function formatLatencyCell(ms) {
    if (!ms || ms <= 0) return '<span class="cell-muted">-</span>';
    var cls = ms < 80 ? 'lat-fast' : (ms <= 200 ? 'lat-mid' : 'lat-slow');
    return '<span class="' + cls + '">' + ms + ' ms</span>';
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

  function hasPlatformUnlock(n, platform) {
    return [].concat(n.media || [], n.ai || []).some(function (u) {
      return u.platform === platform && isUnlockOK(u);
    });
  }

  function getFilteredAndSortedNodes() {
    if (!state.currentReport || !Array.isArray(state.currentReport.nodes)) return [];
    var q = (el('nodeSearch').value || '').trim().toLowerCase();
    var stFilter = state.nodeFilter || 'all';

    var rows = state.currentReport.nodes.filter(function (n) {
      var ok = isNodeOK(n);
      if (stFilter === 'ok' && !ok) return false;
      if (stFilter === 'fail' && ok) return false;
      if (stFilter.indexOf('unlock:') === 0) {
        var targetPlat = stFilter.slice(7);
        if (!hasPlatformUnlock(n, targetPlat)) return false;
      }
      if (!q) return true;
      var unlockText = []
        .concat(n.media || [], n.ai || [])
        .map(function (u) { return (u.platform || '') + ' ' + shortPlatformName(u.platform) + ' ' + (u.region || '') + ' ' + (u.status || ''); })
        .join(' ');
      var hay = [
        n.name, n.type, n.server,
        n.inbound && n.inbound.ip, n.inbound && n.inbound.country, n.inbound && localizeCountry(n.inbound.country),
        n.inbound && n.inbound.region, n.inbound && n.inbound.city, n.inbound && n.inbound.isp, n.inbound && n.inbound.org, n.inbound && n.inbound.asn,
        n.outbound && n.outbound.ip, n.outbound && n.outbound.country, n.outbound && localizeCountry(n.outbound.country),
        n.outbound && n.outbound.region, n.outbound && n.outbound.city, n.outbound && n.outbound.isp, n.outbound && n.outbound.org, n.outbound && n.outbound.asn,
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
    var allNodes = (state.currentReport && state.currentReport.nodes) || [];
    var rows = getFilteredAndSortedNodes();

    if (rows.length === 0) {
      var trEmpty = document.createElement('tr');
      if (allNodes.length > 0) {
        trEmpty.innerHTML =
          '<td colspan="12" class="table-empty-cell">' +
            '<div class="table-empty-box">' +
              '<span>没有匹配当前搜索或筛选条件的节点（共 ' + allNodes.length + ' 个节点）</span>' +
              '<button type="button" class="btn btn-secondary btn-xs" id="resetNodeFilterBtn">重置筛选条件</button>' +
            '</div>' +
          '</td>';
      } else {
        trEmpty.innerHTML =
          '<td colspan="12" class="table-empty-cell">' +
            '<div class="table-empty-box"><span>正在探测节点，结果将在完成后实时呈现在此处…</span></div>' +
          '</td>';
      }
      tbody.appendChild(trEmpty);
      return;
    }

    var maxAvgSpeed = 0;
    allNodes.forEach(function (item) {
      if (item.speed && item.speed.avg > maxAvgSpeed) maxAvgSpeed = item.speed.avg;
    });

    var frag = document.createDocumentFragment();
    rows.forEach(function (n) {
      var tr = document.createElement('tr');
      var tlsHtml = formatLatencyCell(n.latency && n.latency.avgTlsRTT);
      var httpsHtml = formatLatencyCell(n.latency && n.latency.avgHTTPS);
      var loss = (n.latency && n.latency.packetLoss != null) ? Math.round(n.latency.packetLoss * 100) + '%' : '-';

      var avgSpeedHtml = '<span class="cell-muted">-</span>';
      if (n.speed && n.speed.avg > 0) {
        var pct = maxAvgSpeed > 0 ? Math.max(4, Math.min(100, Math.round((n.speed.avg / maxAvgSpeed) * 100))) : 0;
        avgSpeedHtml =
          '<div class="speed-cell-wrap">' +
            '<span class="speed-val">' + escapeHtml(formatSpeedBytes(n.speed.avg)) + '</span>' +
            '<progress class="speed-meter" max="100" value="' + pct + '"></progress>' +
          '</div>';
      }
      var maxSpd = (n.speed && n.speed.max > 0) ? escapeHtml(formatSpeedBytes(n.speed.max)) : '<span class="cell-muted">-</span>';

      var inStr = n.inbound && n.inbound.ip
        ? '<button type="button" class="ip-copy" data-ip="' + escapeHtml(n.inbound.ip) + '" title="点击复制 IP">' + escapeHtml(n.inbound.ip) + '</button>' +
          '<small class="geo-meta">' + escapeHtml(formatInboundSummary(n.inbound)) + '</small>'
        : '<span class="cell-muted">-</span>';

      var outStr = n.outbound && n.outbound.ip
        ? '<button type="button" class="ip-copy" data-ip="' + escapeHtml(n.outbound.ip) + '" title="点击复制 IP">' + escapeHtml(n.outbound.ip) + '</button>' +
          '<small class="geo-meta">' + escapeHtml(formatOutboundSummary(n.outbound)) + '</small>'
        : (n.error ? '<span class="pill pill-fail">' + escapeHtml(n.error) + '</span>' : '<span class="cell-muted">-</span>');

      tr.innerHTML =
        '<td>' + (n.index || '-') + '</td>' +
        '<td class="cell-name" title="' + escapeHtml(n.name || '') + '">' + escapeHtml(n.name || '-') + '</td>' +
        '<td><span class="pill pill-muted">' + escapeHtml((n.type || '-').toUpperCase()) + '</span></td>' +
        '<td>' + tlsHtml + '</td>' +
        '<td>' + httpsHtml + '</td>' +
        '<td>' + loss + '</td>' +
        '<td class="cell-speed">' + avgSpeedHtml + '</td>' +
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
    resultBarRight.classList.remove('hidden');
    resultBadge.classList.remove('hidden');
    resultBadge.textContent = '实时更新 (' + state.completedNodes + '/' + state.totalNodes + ')';
    tabNodesLabel.textContent = '节点明细 (' + state.currentReport.nodes.length + ')';
    renderStatCards(state.currentReport);
    scheduleNodeTableRender();
  }

  // 10. 全宽结果画廊、主题色板点、免重测重绘、CSV 与拓扑
  function renderThemeSwatches() {
    if (!themeSwatches) return;
    var selectEl = el('themeSelect');
    var list = Array.isArray(state.availableThemes) && state.availableThemes.length
      ? state.availableThemes.slice()
      : Object.keys(THEME_META).filter(Boolean);

    if (list.indexOf('default') > 0) {
      list.splice(list.indexOf('default'), 1);
      list.unshift('default');
    }
    if (list.indexOf('') < 0) list.push('');

    var curVal = selectEl.value;
    selectEl.innerHTML = '';
    themeSwatches.innerHTML = '';

    list.forEach(function (name) {
      var meta = THEME_META[name] || { label: name || '随机配色', color: '#64748B' };
      var opt = document.createElement('option');
      opt.value = name;
      opt.textContent = meta.label + (name ? (' (' + name + ')') : '');
      selectEl.appendChild(opt);

      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'theme-swatch' + (name === curVal ? ' active' : '');
      btn.setAttribute('role', 'radio');
      btn.setAttribute('aria-checked', name === curVal ? 'true' : 'false');
      btn.setAttribute('data-theme-name', name);
      btn.title = name ? (meta.label + ' (' + name + ')') : '随机浅色配色';

      var dot = document.createElement('span');
      dot.className = 'theme-swatch-dot';
      dot.style.backgroundColor = meta.color;

      var txt = document.createElement('span');
      txt.textContent = meta.label;

      btn.appendChild(dot);
      btn.appendChild(txt);
      btn.addEventListener('click', function () {
        selectEl.value = name;
        syncThemeSwatches();
        savePrefs();
        if (state.currentRunId && !state.isRunning) {
          triggerRerender();
        } else if (state.currentRunId) {
          setRerenderDirty(true);
        }
      });
      themeSwatches.appendChild(btn);
    });
    selectEl.value = curVal;
    syncThemeSwatches();
  }

  function syncThemeSwatches() {
    if (!themeSwatches) return;
    var val = el('themeSelect').value;
    themeSwatches.querySelectorAll('.theme-swatch').forEach(function (btn) {
      var active = btn.getAttribute('data-theme-name') === val;
      btn.classList.toggle('active', active);
      btn.setAttribute('aria-checked', active ? 'true' : 'false');
    });
  }

  function setRerenderDirty(dirty) {
    state.rerenderDirty = dirty;
    rerenderDirtyDot.classList.toggle('hidden', !dirty);
    rerenderBtnLabel.textContent = dirty ? '应用重绘' : '重新生成';
  }

  toggleRerenderBtn.addEventListener('click', function () {
    var willShow = rerenderDrawer.classList.contains('hidden');
    rerenderDrawer.classList.toggle('hidden', !willShow);
    toggleRerenderBtn.setAttribute('aria-expanded', willShow ? 'true' : 'false');
  });

  el('sortBy').addEventListener('change', function () {
    savePrefs();
    if (state.currentRunId && !state.isRunning) triggerRerender();
    else if (state.currentRunId) setRerenderDirty(true);
  });

  ['watermark', 'bandwidth'].forEach(function (id) {
    el(id).addEventListener('input', function () {
      if (state.currentRunId) setRerenderDirty(true);
    });
    el(id).addEventListener('keydown', function (e) {
      if (e.key === 'Enter' && state.currentRunId && !state.isRunning) {
        e.preventDefault();
        triggerRerender();
      }
    });
  });

  function copyImageToClipboard(url, triggerBtn) {
    var oldHtml = triggerBtn ? triggerBtn.innerHTML : '';
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
        if (triggerBtn) { triggerBtn.disabled = false; triggerBtn.innerHTML = oldHtml; }
      }, 1500);
    }).catch(function (err) {
      if (err && err.status !== 401) {
        showToast('复制图片失败：' + err.message, 'err');
      }
      if (triggerBtn) { triggerBtn.disabled = false; triggerBtn.innerHTML = oldHtml; }
    });
  }

  function imageCategoryMeta(name) {
    if (/_full\.png$/.test(name)) return { key: 'full', order: 1, shortLabel: '综合全维', label: '综合全维结果图 (Full)' };
    if (/_speed\.png$/.test(name)) return { key: 'speed', order: 2, shortLabel: '测速延迟', label: '节点速度与延迟图 (Speed)' };
    if (/_media\.png$/.test(name)) return { key: 'media', order: 3, shortLabel: '流媒体', label: '流媒体解锁检测图 (Media)' };
    if (/_ai\.png$/.test(name)) return { key: 'ai', order: 4, shortLabel: 'AI 解锁', label: 'AI 平台可用性图 (AI)' };
    if (/_topology\.png$/.test(name)) return { key: 'topology', order: 5, shortLabel: '线路拓扑', label: '出入口线路拓扑图 (Topology)' };
    return { key: 'other', order: 9, shortLabel: '结果图', label: '结果图' };
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
      tabNodesLabel.textContent = '节点明细 (' + ((data.nodes && data.nodes.length) || 0) + ')';
      renderStatCards(data);
      renderNodeTable();
      renderTopologyTables(data.topology);
    }).catch(function () {
      statGrid.classList.add('hidden');
    });
  }

  function renderActiveGalleryItem(idx) {
    if (!state.galleryList.length) {
      thumbs.innerHTML = '<div class="gallery-placeholder">暂无可展示的 PNG 结果图</div>';
      galleryCopyBtn.disabled = true;
      galleryFullscreenBtn.disabled = true;
      galleryDownloadBtn.classList.add('hidden');
      return;
    }
    state.activeGalleryIdx = Math.max(0, Math.min(idx, state.galleryList.length - 1));
    var item = state.galleryList[state.activeGalleryIdx];
    state.activeGalleryCategory = item.category;

    galleryTypePills.querySelectorAll('.chip-btn').forEach(function (pill, i) {
      var active = i === state.activeGalleryIdx;
      pill.classList.toggle('active', active);
      pill.setAttribute('aria-selected', active ? 'true' : 'false');
    });

    galleryCopyBtn.disabled = false;
    galleryFullscreenBtn.disabled = false;
    galleryDownloadBtn.href = item.url;
    galleryDownloadBtn.setAttribute('download', item.name);
    galleryDownloadBtn.classList.remove('hidden');

    thumbs.innerHTML = '';
    var img = document.createElement('img');
    img.src = item.url;
    img.alt = item.label;
    img.className = 'gallery-stage-img';
    img.title = '点击全屏高清查看 · ' + item.label;
    img.addEventListener('click', function () {
      openLightbox(state.activeGalleryIdx);
    });
    thumbs.appendChild(img);
  }

  function renderFiles(files, runId, cacheBust, skipJSONFetch) {
    state.currentRunId = runId || state.currentRunId;
    thumbs.innerHTML = '';
    galleryTypePills.innerHTML = '';
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

    var restoreIdx = 0;
    pngFiles.forEach(function (name, idx) {
      var url = prefix + encodeURIComponent(name) + bust;
      var meta = imageCategoryMeta(name);
      if (state.activeGalleryCategory && meta.key === state.activeGalleryCategory) {
        restoreIdx = idx;
      }
      state.galleryList.push({ url: url, name: name, label: meta.label, shortLabel: meta.shortLabel, category: meta.key });

      var pill = document.createElement('button');
      pill.type = 'button';
      pill.className = 'chip-btn';
      pill.setAttribute('role', 'tab');
      pill.textContent = meta.shortLabel;
      pill.title = meta.label;
      pill.addEventListener('click', function () {
        renderActiveGalleryItem(idx);
      });
      galleryTypePills.appendChild(pill);
    });

    renderActiveGalleryItem(restoreIdx);

    if (jsonFileURL && !skipJSONFetch) {
      fetchAndRenderJSON(jsonFileURL);
    }
  }

  galleryCopyBtn.addEventListener('click', function () {
    var item = state.galleryList[state.activeGalleryIdx];
    if (item) copyImageToClipboard(item.url, galleryCopyBtn);
  });

  galleryFullscreenBtn.addEventListener('click', function () {
    if (state.galleryList.length) openLightbox(state.activeGalleryIdx);
  });

  function triggerRerender() {
    if (state.rerenderBusy || !state.currentRunId) return;
    savePrefs();
    state.rerenderBusy = true;
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
      state.rerenderBusy = false;
      rerenderBtn.disabled = false;
      if (!state.rerenderDirty) rerenderBtnLabel.textContent = '重新生成';
    });
  }

  rerenderBtn.addEventListener('click', triggerRerender);

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
    var headers = [
      '序号', '节点名称', '协议', 'TLS延迟(ms)', 'HTTPS延迟(ms)', '丢包率(%)',
      '平均速度(B/s)', '最高速度(B/s)',
      '入口IP', '入口地区', '入口城市', '入口ASN/组织',
      '出口IP', '出口地区', '出口城市', '出口ASN/组织',
      '流媒体解锁', 'AI解锁', '错误'
    ];
    var lines = [headers.map(csvCell).join(',')];
    rows.forEach(function (n) {
      var mediaStr = (n.media || []).map(function (u) { return u.platform + ':' + (u.region || u.status); }).join('|');
      var aiStr = (n.ai || []).map(function (u) { return u.platform + ':' + (u.region || u.status); }).join('|');
      var inAsnOrg = n.inbound ? [cleanGeoStr(n.inbound.asn), cleanGeoStr(n.inbound.isp) || cleanGeoStr(n.inbound.org)].filter(Boolean).join(' ') : '';
      var outAsnOrg = n.outbound ? [cleanGeoStr(n.outbound.asn), formatOutboundOrg(n.outbound)].filter(Boolean).join(' ') : '';
      lines.push([
        n.index, n.name || '', n.type || '',
        (n.latency && n.latency.avgTlsRTT) || 0,
        (n.latency && n.latency.avgHTTPS) || 0,
        (n.latency && n.latency.packetLoss != null) ? Math.round(n.latency.packetLoss * 100) : '',
        (n.speed && Math.round(n.speed.avg)) || 0,
        (n.speed && Math.round(n.speed.max)) || 0,
        (n.inbound && n.inbound.ip) || '',
        (n.inbound && formatInboundRegion(n.inbound)) || '',
        (n.inbound && cleanGeoStr(n.inbound.city)) || '',
        inAsnOrg,
        (n.outbound && n.outbound.ip) || '',
        (n.outbound && formatOutboundRegion(n.outbound)) || '',
        (n.outbound && cleanGeoStr(n.outbound.city)) || '',
        outAsnOrg,
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
      var region = formatInboundRegion(g) || '-';
      var ispOrg = [cleanGeoStr(g.asn), cleanGeoStr(g.isp), cleanGeoStr(g.org)].filter(function (v, idx, arr) {
        return v && arr.indexOf(v) === idx;
      }).join(' · ') || '-';
      tr.innerHTML =
        '<td>' + e.index + '</td>' +
        '<td><button type="button" class="ip-copy" data-ip="' + escapeHtml(g.ip || '') + '">' + escapeHtml(g.ip || '-') + '</button></td>' +
        '<td class="topo-wrap">' + escapeHtml(region) + '</td>' +
        '<td class="topo-wrap">' + escapeHtml(ispOrg) + '</td>' +
        '<td><span class="pill pill-muted">IPv' + escapeHtml(g.stack || '4') + '</span></td>' +
        '<td><b>' + (e.outCount || 0) + '</b></td>';
      inBody.appendChild(tr);
    });

    (topo.outbounds || []).forEach(function (e, idx) {
      var g = e.geo || {};
      var tr = document.createElement('tr');
      var region = formatOutboundRegion(g) || '-';
      var asnOrg = [cleanGeoStr(g.asn), formatOutboundOrg(g)].filter(Boolean).join(' ') || '-';
      tr.innerHTML =
        '<td>' + (idx + 1) + '</td>' +
        '<td>入口 #' + e.inboundIndex + '</td>' +
        '<td><button type="button" class="ip-copy" data-ip="' + escapeHtml(g.ip || '') + '">' + escapeHtml(g.ip || '-') + '</button></td>' +
        '<td class="topo-wrap">' + escapeHtml(region) + '</td>' +
        '<td class="topo-wrap">' + escapeHtml(asnOrg) + '</td>' +
        '<td class="topo-wrap cell-name">' + escapeHtml(e.nodeName || '-') + '</td>';
      outBody.appendChild(tr);
    });
  }

  // 节点表搜索、快捷筛选与展开未解锁交互
  el('nodeSearch').addEventListener('input', renderNodeTable);

  document.querySelectorAll('#nodeQuickFilters .chip-btn[data-node-filter]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      state.nodeFilter = btn.getAttribute('data-node-filter') || 'all';
      document.querySelectorAll('#nodeQuickFilters .chip-btn[data-node-filter]').forEach(function (other) {
        var active = other === btn;
        other.classList.toggle('active', active);
        other.setAttribute('aria-pressed', active ? 'true' : 'false');
      });
      renderNodeTable();
    });
  });

  el('nodeTableBody').addEventListener('click', function (e) {
    var expandBtn = e.target.closest('button[data-expand-unlock]');
    if (expandBtn) {
      var extra = expandBtn.nextElementSibling;
      if (extra && extra.classList.contains('unlock-extra-list')) {
        var expanded = !extra.classList.contains('hidden');
        extra.classList.toggle('hidden', expanded);
        expandBtn.setAttribute('aria-expanded', expanded ? 'false' : 'true');
        var cnt = expandBtn.getAttribute('data-count') || '';
        expandBtn.textContent = expanded ? ('+' + cnt + ' 未解锁') : ('收起 (-' + cnt + ')');
      }
      return;
    }

    var resetBtn = e.target.closest('#resetNodeFilterBtn');
    if (resetBtn) {
      el('nodeSearch').value = '';
      state.nodeFilter = 'all';
      document.querySelectorAll('#nodeQuickFilters .chip-btn[data-node-filter]').forEach(function (btn) {
        var active = btn.getAttribute('data-node-filter') === 'all';
        btn.classList.toggle('active', active);
        btn.setAttribute('aria-pressed', active ? 'true' : 'false');
      });
      renderNodeTable();
    }
  });

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
    if (btn) {
      var ip = btn.getAttribute('data-ip');
      if (ip) copyText(ip, '已复制 IP：' + ip);
      return;
    }
    if (state.historyPopoverOpen && historyPickerWrap && !historyPickerWrap.contains(e.target)) {
      closeHistoryPopover();
    }
    if (envGeoWrap && !envGeoWrap.contains(e.target)) {
      setEnvPopoverOpen(false, false);
    }
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

  // 11. 无障碍全屏 Lightbox（支持滚轮缩放与双击 1:1 / 适应切换）
  function applyLbZoom() {
    if (state.lbMode === 'fit') {
      lightboxImg.classList.add('is-fit');
      lightboxImg.classList.remove('is-zoomed');
      lightboxImg.style.width = '';
      lbZoomLabel.textContent = '适应';
      return;
    }
    var natW = lightboxImg.naturalWidth || lbStage.clientWidth || 1000;
    lightboxImg.classList.remove('is-fit');
    lightboxImg.classList.add('is-zoomed');
    if (state.lbMode === '1:1') {
      state.lbScale = 100;
      lightboxImg.style.width = natW + 'px';
      lbZoomLabel.textContent = '1:1';
    } else {
      lightboxImg.style.width = Math.round(natW * state.lbScale / 100) + 'px';
      lbZoomLabel.textContent = state.lbScale + '%';
    }
  }

  function stepLbZoom(delta) {
    if (state.lbMode === 'fit') {
      var natW = lightboxImg.naturalWidth || lightboxImg.clientWidth || 1000;
      var fitPct = Math.round(((lightboxImg.clientWidth || natW) / natW) * 100);
      state.lbScale = Math.max(25, Math.min(300, fitPct + delta));
    } else {
      state.lbScale = Math.max(25, Math.min(300, state.lbScale + delta));
    }
    state.lbMode = 'zoom';
    applyLbZoom();
  }

  function toggleLbFitNative() {
    state.lbMode = (state.lbMode === 'fit') ? '1:1' : 'fit';
    applyLbZoom();
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
    state.lbMode = 'fit';
    state.lbScale = 100;
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
  lightboxImg.addEventListener('dblclick', function (e) {
    e.stopPropagation();
    toggleLbFitNative();
  });
  lbZoomToggleBtn.addEventListener('click', toggleLbFitNative);
  lbStage.addEventListener('wheel', function (e) {
    if (!lightbox.classList.contains('open')) return;
    e.preventDefault();
    stepLbZoom(e.deltaY < 0 ? 15 : -15);
  }, { passive: false });
  lbPrevBtn.addEventListener('click', function () { openLightbox(state.lbIndex - 1); });
  lbNextBtn.addEventListener('click', function () { openLightbox(state.lbIndex + 1); });
  lbZoomInBtn.addEventListener('click', function () { stepLbZoom(20); });
  lbZoomOutBtn.addEventListener('click', function () { stepLbZoom(-20); });
  lbCopyBtn.addEventListener('click', function () {
    if (state.galleryList[state.lbIndex]) copyImageToClipboard(state.galleryList[state.lbIndex].url, lbCopyBtn);
  });

  document.addEventListener('keydown', function (e) {
    if (lightbox.classList.contains('open')) {
      if (e.key === 'Escape') closeLightbox();
      else if (e.key === 'ArrowLeft') openLightbox(state.lbIndex - 1);
      else if (e.key === 'ArrowRight') openLightbox(state.lbIndex + 1);
      else if (e.key === '+' || e.key === '=') stepLbZoom(20);
      else if (e.key === '-') stepLbZoom(-20);
      else if (e.key === '0') { state.lbMode = 'fit'; applyLbZoom(); }
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

    if (e.key === 'Escape') {
      if (state.historyPopoverOpen) closeHistoryPopover();
      setEnvPopoverOpen(false, false);
      return;
    }

    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && !runBtn.disabled && authModal.classList.contains('hidden')) {
      e.preventDefault();
      runBtn.click();
    }
  });

  // 12. 历史记录下拉列表（按日期分组 + 两步内联删除）
  function disarmHistoryDelete() {
    state.armedDeleteId = '';
    if (state.armedDeleteTimer) {
      clearTimeout(state.armedDeleteTimer);
      state.armedDeleteTimer = null;
    }
  }

  function openHistoryPopover() {
    state.historyPopoverOpen = true;
    historyPopover.classList.remove('hidden');
    historyPickerBtn.setAttribute('aria-expanded', 'true');
    renderHistoryList();
  }

  function closeHistoryPopover() {
    state.historyPopoverOpen = false;
    disarmHistoryDelete();
    historyPopover.classList.add('hidden');
    historyPickerBtn.setAttribute('aria-expanded', 'false');
  }

  historyPickerBtn.addEventListener('click', function (e) {
    e.stopPropagation();
    if (state.historyPopoverOpen) closeHistoryPopover();
    else openHistoryPopover();
  });

  function formatRunDateGroup(runId) {
    var m = String(runId || '').match(/^(\d{4})(\d{2})(\d{2})_(\d{2})(\d{2})(\d{2})/);
    if (!m) return { group: '其他记录', time: runId };
    var d = new Date(parseInt(m[1], 10), parseInt(m[2], 10) - 1, parseInt(m[3], 10));
    var now = new Date();
    var today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    var diffDays = Math.round((today - d) / 86400000);
    var group = m[1] + '-' + m[2] + '-' + m[3];
    if (diffDays === 0) group = '今天';
    else if (diffDays === 1) group = '昨天';
    return { group: group, time: m[4] + ':' + m[5] + ':' + m[6] };
  }

  function updateHistoryTriggerLabel() {
    var count = state.historyItems.length;
    historyCountBadge.textContent = String(count);
    var item = state.currentRunId && state.historyMap[state.currentRunId];
    if (item) {
      var name = item.subName || item.id;
      var meta = item.total > 0 ? (' · ' + item.succeeded + '/' + item.total) : '';
      var dur = item.elapsed ? (' · ' + item.elapsed) : '';
      historyPickerLabel.textContent = name + meta + dur;
    } else if (state.isRunning && state.currentRunId) {
      historyPickerLabel.textContent = '当前任务 · ' + state.currentRunId;
    } else if (count > 0) {
      historyPickerLabel.textContent = '选择历史测速记录';
    } else {
      historyPickerLabel.textContent = '暂无历史记录';
    }
  }

  function renderHistoryList() {
    historyListEl.innerHTML = '';
    if (!state.historyItems.length) {
      historyListEl.innerHTML = '<div class="history-empty">暂无历史测速归档</div>';
      return;
    }
    var frag = document.createDocumentFragment();
    var lastGroup = '';

    state.historyItems.forEach(function (item) {
      var dt = formatRunDateGroup(item.id);
      if (dt.group !== lastGroup) {
        lastGroup = dt.group;
        var grp = document.createElement('div');
        grp.className = 'history-group-label';
        grp.textContent = dt.group;
        frag.appendChild(grp);
      }

      var row = document.createElement('div');
      row.className = 'history-item' + (item.id === state.currentRunId ? ' active' : '');
      row.setAttribute('role', 'option');
      row.setAttribute('aria-selected', item.id === state.currentRunId ? 'true' : 'false');

      var mainBtn = document.createElement('button');
      mainBtn.type = 'button';
      mainBtn.className = 'history-item-main';
      mainBtn.setAttribute('data-run-id', item.id);

      var titleEl = document.createElement('span');
      titleEl.className = 'history-item-title';
      titleEl.textContent = item.subName || '未命名订阅 (' + item.id + ')';

      var metaParts = [dt.time];
      if (item.total > 0) {
        var rate = Math.round((item.succeeded / item.total) * 100);
        metaParts.push('可用 ' + item.succeeded + '/' + item.total + ' (' + rate + '%)');
      }
      if (item.elapsed) metaParts.push(item.elapsed);
      var metaEl = document.createElement('span');
      metaEl.className = 'history-item-meta';
      metaEl.textContent = metaParts.join(' · ');

      mainBtn.appendChild(titleEl);
      mainBtn.appendChild(metaEl);

      var delBtn = document.createElement('button');
      delBtn.type = 'button';
      var isArmed = state.armedDeleteId === item.id;
      delBtn.className = 'history-del-btn' + (isArmed ? ' armed' : '');
      delBtn.setAttribute('data-del-id', item.id);
      if (state.isRunning && item.id === state.currentRunId) {
        delBtn.disabled = true;
        delBtn.title = '任务运行中，暂不可删除';
        delBtn.textContent = '运行中';
      } else if (isArmed) {
        delBtn.textContent = '确认删除?';
      } else {
        delBtn.innerHTML = '<svg class="ico"><use href="#i-trash"/></svg><span>删除</span>';
      }

      row.appendChild(mainBtn);
      row.appendChild(delBtn);
      frag.appendChild(row);
    });
    historyListEl.appendChild(frag);
  }

  historyListEl.addEventListener('click', function (e) {
    e.stopPropagation();
    var delBtn = e.target.closest('button[data-del-id]');
    if (delBtn) {
      if (delBtn.disabled) return;
      var id = delBtn.getAttribute('data-del-id');
      if (state.armedDeleteId !== id) {
        disarmHistoryDelete();
        state.armedDeleteId = id;
        renderHistoryList();
        state.armedDeleteTimer = setTimeout(function () {
          disarmHistoryDelete();
          if (state.historyPopoverOpen) renderHistoryList();
        }, 3500);
        return;
      }
      disarmHistoryDelete();
      deleteHistoryRun(id);
      return;
    }

    var mainBtn = e.target.closest('button[data-run-id]');
    if (mainBtn) {
      var runId = mainBtn.getAttribute('data-run-id');
      closeHistoryPopover();
      selectHistoryItem(runId, false);
    }
  });

  function deleteHistoryRun(id) {
    if (!id || !state.historyMap[id]) return;
    apiFetch('/api/history/' + encodeURIComponent(id), { method: 'DELETE' }).then(function (resp) {
      if (!resp.ok) return resp.text().then(function (t) { throw new Error(t); });
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
  }

  function selectHistoryItem(id, isAutoInit) {
    if (!id || !state.historyMap[id]) return;
    var item = state.historyMap[id];
    state.currentRunId = item.id;
    emptyStateCard.classList.add('hidden');
    resultCard.classList.remove('hidden');
    resultBarRight.classList.remove('hidden');
    resultBadge.classList.remove('hidden');
    resultBadge.textContent = isAutoInit ? '最近报告' : '历史记录';
    summaryEl.textContent = '';
    updateHistoryTriggerLabel();
    setRerenderDirty(false);
    renderFiles(item.files, item.id, state.historyCacheBust[item.id] || 0);
  }

  function loadHistory(preferId) {
    apiFetch('/api/history').then(function (r) {
      if (!r.ok) throw new Error('HTTP ' + r.status);
      return r.json();
    }).then(function (items) {
      var list = Array.isArray(items) ? items : [];
      state.historyItems = list;
      state.historyMap = {};
      list.forEach(function (item) {
        state.historyMap[item.id] = item;
      });

      var cur = preferId || state.currentRunId;
      if (cur && state.historyMap[cur]) {
        if (preferId && preferId !== state.currentRunId) {
          selectHistoryItem(cur, false);
        }
      } else if (!state.initialHistoryLoaded && list.length > 0 && !state.isRunning) {
        selectHistoryItem(list[0].id, true);
      } else if (list.length === 0 && !state.isRunning && !state.currentReport) {
        resultCard.classList.add('hidden');
        resultBarRight.classList.add('hidden');
        resultBadge.classList.add('hidden');
        summaryEl.textContent = '';
        emptyStateCard.classList.remove('hidden');
      }

      updateHistoryTriggerLabel();
      if (state.historyPopoverOpen) renderHistoryList();
      state.initialHistoryLoaded = true;
    }).catch(function () {});
  }

  refreshHistoryBtn.addEventListener('click', function () {
    loadHistory();
    showToast('已刷新历史记录列表', 'ok');
  });

  // 13. 状态与 SSE 实时连接
  function onDone(evt) {
    setRunningState(false);
    emptyStateCard.classList.add('hidden');
    resultCard.classList.remove('hidden');
    resultBarRight.classList.remove('hidden');
    resultBadge.classList.remove('hidden');
    resultBadge.textContent = '本次结果';
    if (!evt.ok) {
      summaryEl.innerHTML = '<span class="text-danger">任务中断：' + escapeHtml(evt.error || '未知错误') + '</span>';
      showToast('任务中断：' + (evt.error || '未知错误'), 'err');
      return;
    }
    progressStage.textContent = '测速任务全部完成';
    progressFill.style.width = '100%';
    progressBar.setAttribute('aria-valuenow', '100');
    summaryEl.innerHTML = '<span class="text-success">成功 ' + evt.succeeded + ' / ' + evt.total +
      ' 个节点</span> · 耗时 ' + escapeHtml(evt.elapsed || '');
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

  function formatGeoBytes(bytes) {
    if (!bytes || bytes <= 0) return '0 B';
    if (bytes >= 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
    return Math.max(1, Math.round(bytes / 1024)) + ' KB';
  }

  function renderGeoStatus(g) {
    if (!g) return;
    state.lastGeoState = g;
    state.geoUpdating = Boolean(g.updating);
    var busy = state.geoUpdating || state.isRunning;

    if (updateGeoBtn) {
      updateGeoBtn.disabled = busy;
      if (state.geoUpdating) {
        geoBtnLabel.textContent = '更新中 ' + (g.done || 0) + '/' + (g.total || 3);
      } else if (!g.ready) {
        geoBtnLabel.textContent = '下载 GeoIP';
      } else if (g.stale) {
        geoBtnLabel.textContent = '更新 GeoIP (待更新)';
      } else {
        geoBtnLabel.textContent = '更新 GeoIP';
      }
    }

    if (geoProgressWrap && geoProgressBar) {
      geoProgressWrap.classList.toggle('hidden', !state.geoUpdating);
      if (state.geoUpdating) {
        var total = g.total || 3;
        var pct = Math.max(8, Math.min(100, Math.round(((g.done || 0) / total) * 100)));
        geoProgressBar.style.width = pct + '%';
      }
    }

    if (geoModeBadge) {
      geoModeBadge.classList.remove('is-offline', 'is-hybrid', 'is-online');
      if (g.mode === 'offline') {
        geoModeBadge.classList.add('is-offline');
        geoModeBadge.textContent = '离线加速';
      } else if (g.mode === 'hybrid') {
        geoModeBadge.classList.add('is-hybrid');
        geoModeBadge.textContent = '混合模式';
      } else {
        geoModeBadge.classList.add('is-online');
        geoModeBadge.textContent = '纯在线';
      }
    }

    if (geoModeText) {
      geoModeText.textContent = state.geoUpdating ? (g.stage || '正在更新离线 IP 库…') : (g.modeText || '');
    }

    if (geoAttribution && g.attribution) {
      geoAttribution.textContent = g.attribution;
    }

    if (geoFilesList) {
      geoFilesList.innerHTML = '';
      var files = Array.isArray(g.files) ? g.files : [];
      var hasIPinfo = files.some(function (f) { return f.name === 'ipinfo_lite.mmdb' && f.present; });
      var hasMaxMindASN = files.some(function (f) { return f.name === 'GeoLite2-ASN.mmdb' && f.present; });
      files.forEach(function (f) {
        if (f.name === 'GeoLite2-ASN.mmdb' && hasIPinfo && !f.present) return;
        if (f.name === 'ipinfo_lite.mmdb' && !f.present && hasMaxMindASN) return;
        var chip = document.createElement('span');
        chip.className = 'geo-file-chip' + (f.present ? ' is-present' : '') + (f.stale ? ' is-stale' : '');
        var shortName = f.name.replace(/\.mmdb$/i, '');
        if (f.present) {
          chip.textContent = shortName + ' · ' + formatGeoBytes(f.size);
          chip.title = f.label + '（更新于 ' + (f.updatedAt || '未知') + '）';
        } else {
          chip.textContent = shortName + ' · 未下载';
          chip.title = f.label + '（尚未下载）';
        }
        geoFilesList.appendChild(chip);
      });
    }

    updateEnvGeoSummaryPill();
  }

  function stopGeoPolling() {
    if (state.geoPollTimer) {
      clearInterval(state.geoPollTimer);
      state.geoPollTimer = null;
    }
  }

  function startGeoPolling() {
    if (state.geoPollTimer) return;
    state.geoPollTimer = setInterval(function () {
      apiFetch('/api/geoip/status').then(function (r) {
        if (!r.ok) return null;
        return r.json();
      }).then(function (g) {
        if (!g) return;
        var wasUpdating = state.geoUpdating;
        renderGeoStatus(g);
        if (!g.updating) {
          stopGeoPolling();
          if (wasUpdating) {
            if (g.lastError) {
              showToast('IP 库更新失败：' + g.lastError, 'err');
            } else {
              showToast('离线 IP 库已更新：' + (g.modeText || '完成'), 'ok');
            }
          }
        }
      }).catch(function () {});
    }, 1000);
  }

  function triggerGeoUpdate() {
    if (state.geoUpdating || state.isRunning) return;
    var proxyVal = el('proxy') ? el('proxy').value.trim() : '';
    if (updateGeoBtn) updateGeoBtn.disabled = true;
    apiFetch('/api/geoip/update', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ proxy: proxyVal })
    }).then(function (resp) {
      if (!resp.ok) {
        return resp.text().then(function (t) { throw new Error(t || ('HTTP ' + resp.status)); });
      }
      return resp.json();
    }).then(function (g) {
      renderGeoStatus(g);
      startGeoPolling();
      showToast('已开始后台更新离线 IP 库…', 'ok');
    }).catch(function (err) {
      if (state.lastGeoState) renderGeoStatus(state.lastGeoState);
      else if (updateGeoBtn) updateGeoBtn.disabled = false;
      if (err && err.status !== 401) {
        showToast(err.message || '启动更新失败', 'warn');
      }
    });
  }

  if (updateGeoBtn) updateGeoBtn.addEventListener('click', triggerGeoUpdate);

  function loadStatus() {
    return apiFetch('/api/status').then(function (r) {
      if (!r.ok) throw new Error('HTTP ' + r.status);
      return r.json();
    }).then(function (st) {
      if (Array.isArray(st.themes) && st.themes.length) {
        state.availableThemes = st.themes;
        renderThemeSwatches();
      }
      if (st.netEnv) {
        state.lastNetEnv = st.netEnv;
        if (st.netEnv.physicalInterface) {
          if (st.netEnv.tunDetected) {
            netEnvTag.classList.add('tun-warn');
            netEnvTag.textContent = '已绕过 TUN';
            netEnvText.textContent = '检测到虚拟网卡 (' + (st.netEnv.tunInterfaces || []).join(', ') + ')，已强制绑定物理网卡 ' +
              st.netEnv.physicalInterface + (st.netEnv.physicalIP ? ' (' + st.netEnv.physicalIP + ')' : '') + ' 直连出站。';
          } else {
            netEnvTag.classList.remove('tun-warn');
            netEnvTag.textContent = '物理网卡直连';
            netEnvText.textContent = '测速套接字直接绑定物理网卡 ' + st.netEnv.physicalInterface +
              (st.netEnv.physicalIP ? ' (' + st.netEnv.physicalIP + ')' : '') + '。';
          }
        } else {
          netEnvTag.classList.remove('tun-warn');
          netEnvTag.textContent = '系统默认路由';
          netEnvText.textContent = '未检测到特定 TUN 干扰，当前沿用系统默认出站路由。';
        }
      }
      if (st.geoip) {
        renderGeoStatus(st.geoip);
        if (st.geoip.updating) startGeoPolling();
      } else {
        updateEnvGeoSummaryPill();
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

  // 14. 启动与停止任务
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
      galleryTypePills.innerHTML = '';
      thumbs.innerHTML = '<div class="gallery-placeholder">测速完成后将在此生成多维结果图…</div>';
      jsonLink.classList.add('hidden');
      galleryDownloadBtn.classList.add('hidden');
      statGrid.classList.add('hidden');
      el('inboundTableBody').innerHTML = '';
      el('outboundTableBody').innerHTML = '';
      setRerenderDirty(false);
      setRunningState(true, Date.now());
      updateProgressUI('正在拉取订阅并初始化节点池…');
      updateHistoryTriggerLabel();
      emptyStateCard.classList.add('hidden');
      resultCard.classList.remove('hidden');
      resultBarRight.classList.remove('hidden');
      resultBadge.classList.remove('hidden');
      resultBadge.textContent = '初始化中…';
      summaryEl.innerHTML = '<span class="text-primary">测试启动中，节点完成后将实时刷新…</span>';
      tabNodesLabel.textContent = '节点明细';
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

  function initTopoSplitter() {
    var topoGrid = el('topoGrid');
    var splitter = el('topoSplitter');
    if (!topoGrid || !splitter) return;

    var minPct = 22;
    var maxPct = 78;
    var currentPct = DEFAULT_TOPO_SPLIT;

    function clampPct(v) {
      return Math.min(maxPct, Math.max(minPct, v));
    }

    function applyPct(pct, persist) {
      currentPct = clampPct(pct);
      topoGrid.style.setProperty('--topo-left', currentPct.toFixed(1) + '%');
      splitter.setAttribute('aria-valuenow', String(Math.round(currentPct)));
      if (persist) {
        try { localStorage.setItem(TOPO_SPLIT_KEY, currentPct.toFixed(1)); } catch (e) {}
      }
    }

    function resetPct() {
      applyPct(DEFAULT_TOPO_SPLIT, false);
      try { localStorage.removeItem(TOPO_SPLIT_KEY); } catch (e) {}
      showToast('已恢复左右默认宽度比例', 'ok');
    }

    try {
      var saved = parseFloat(localStorage.getItem(TOPO_SPLIT_KEY));
      if (!isNaN(saved)) applyPct(saved, false);
      else applyPct(DEFAULT_TOPO_SPLIT, false);
    } catch (e) {
      applyPct(DEFAULT_TOPO_SPLIT, false);
    }

    var dragging = false;
    var activePointerId = null;

    function updateFromClientX(clientX) {
      var rect = topoGrid.getBoundingClientRect();
      var splitterW = splitter.getBoundingClientRect().width || 14;
      var availW = rect.width - splitterW;
      if (availW <= 0) return;
      var pct = ((clientX - rect.left - splitterW / 2) / availW) * 100;
      applyPct(pct, false);
    }

    function stopDrag(persist) {
      if (!dragging) return;
      dragging = false;
      splitter.classList.remove('dragging');
      document.body.classList.remove('topo-resizing');
      if (activePointerId != null) {
        try {
          if (splitter.hasPointerCapture && splitter.hasPointerCapture(activePointerId)) {
            splitter.releasePointerCapture(activePointerId);
          }
        } catch (e) {}
        activePointerId = null;
      }
      if (persist) {
        applyPct(currentPct, true);
      }
    }

    splitter.addEventListener('pointerdown', function (e) {
      if (e.button != null && e.button !== 0) return;
      e.preventDefault();
      dragging = true;
      activePointerId = e.pointerId;
      splitter.classList.add('dragging');
      document.body.classList.add('topo-resizing');
      if (splitter.setPointerCapture && e.pointerId != null) {
        try { splitter.setPointerCapture(e.pointerId); } catch (err) {}
      }
      updateFromClientX(e.clientX);
    });

    splitter.addEventListener('pointermove', function (e) {
      if (!dragging) return;
      updateFromClientX(e.clientX);
    });

    splitter.addEventListener('pointerup', function () { stopDrag(true); });
    splitter.addEventListener('pointercancel', function () { stopDrag(false); });
    window.addEventListener('blur', function () { stopDrag(true); });

    splitter.addEventListener('dblclick', function (e) {
      e.preventDefault();
      resetPct();
    });

    splitter.addEventListener('keydown', function (e) {
      switch (e.key) {
        case 'ArrowLeft':
          e.preventDefault();
          applyPct(currentPct - 3, true);
          break;
        case 'ArrowRight':
          e.preventDefault();
          applyPct(currentPct + 3, true);
          break;
        case 'Home':
          e.preventDefault();
          applyPct(minPct, true);
          break;
        case 'End':
          e.preventDefault();
          applyPct(maxPct, true);
          break;
        case 'Enter':
        case '0':
          e.preventDefault();
          resetPct();
          break;
      }
    });
  }

  // 初始化
  loadRecentSubs();
  loadPrefs();
  renderThemeSwatches();
  syncModuleVisuals();
  updateAdvSummaryChip();
  initTopoSplitter();
  bootstrapRuntime();
})();
