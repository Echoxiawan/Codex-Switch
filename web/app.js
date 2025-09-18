let autoRefreshTimer = null;

const state = {
  status: null,
  backups: [],
};

const els = {
  target: document.getElementById('status-target'),
  badge: document.getElementById('status-badge'),
  size: document.getElementById('status-size'),
  mtime: document.getElementById('status-mtime'),
  fingerprint: document.getElementById('status-fp'),
  hash: document.getElementById('status-hash'),
  latestFingerprint: document.getElementById('status-latest-fp'),
  interval: document.getElementById('status-interval'),
  remarkInput: document.getElementById('remark-input'),
  scanBtn: document.getElementById('scan-btn'),
  backupBtn: document.getElementById('backup-btn'),
  codexBtn: document.getElementById('codex-btn'),
  refreshBtn: document.getElementById('refresh-btn'),
  tbody: document.getElementById('backup-tbody'),
  toast: document.getElementById('toast-container'),
  metricCount: document.getElementById('metric-count'),
  metricLatest: document.getElementById('metric-latest'),
  metricAuto: document.getElementById('metric-auto'),
};

async function apiRequest(path, { method = 'GET', body, allowOkFalse = false } = {}) {
  const options = { method, headers: { 'Content-Type': 'application/json' } };
  if (body !== undefined) {
    options.body = JSON.stringify(body);
  }
  const resp = await fetch(path, options);
  let json = {};
  try {
    json = await resp.json();
  } catch (err) {
    throw new Error('服务器返回非 JSON 数据');
  }
  if (!resp.ok) {
    const msg = json.error || resp.statusText;
    const error = new Error(msg);
    error.status = resp.status;
    throw error;
  }
  if (!json.ok && !allowOkFalse) {
    const msg = json.error || '请求失败';
    const error = new Error(msg);
    error.status = resp.status;
    error.data = json.data;
    throw error;
  }
  return json;
}

function showToast(message, type = 'info', timeout = 3600) {
  const toast = document.createElement('div');
  toast.className = `toast ${type === 'error' ? 'error' : type === 'success' ? 'success' : ''}`;
  toast.textContent = message;
  els.toast.appendChild(toast);
  const timer = setTimeout(() => closeToast(toast), timeout);
  toast.addEventListener('click', () => {
    clearTimeout(timer);
    closeToast(toast);
  });
}

function closeToast(toast) {
  toast.style.animation = 'toast-out 0.22s forwards';
  setTimeout(() => toast.remove(), 200);
}

function setButtonLoading(button, loading, loadingText = '处理中…') {
  if (!button) return;
  if (loading) {
    button.dataset.originalText = button.textContent;
    button.textContent = loadingText;
    button.disabled = true;
  } else {
    if (button.dataset.originalText) {
      button.textContent = button.dataset.originalText;
    }
    button.disabled = false;
  }
}

async function loadStatus({ silent = false } = {}) {
  try {
    const res = await apiRequest('/api/status');
    state.status = res.data;
    renderStatus();
    renderMetrics();
  } catch (err) {
    if (!silent) {
      showToast(`读取状态失败：${err.message}`, 'error');
    } else {
      console.warn('读取状态失败：', err);
    }
  }
}

function renderStatus() {
  const status = state.status;
  if (!status) return;
  els.target.textContent = status.target_path || '-';
  els.target.title = status.target_path || '';
  const exists = !!status.exists;
  els.badge.textContent = exists ? '在线' : '离线';
  els.badge.className = `badge ${exists ? 'badge-success' : 'badge-danger'}`;
  els.size.textContent = exists ? formatBytes(status.size) : '-';
  els.size.title = exists ? `${status.size} 字节` : '';
  els.mtime.textContent = exists && status.mod_time ? formatDate(status.mod_time) : '-';
  els.fingerprint.textContent = exists && status.fingerprint ? status.fingerprint : '-';
  els.fingerprint.title = status.fingerprint || '';
  els.latestFingerprint.textContent = status.latest_fingerprint || '-';
  els.latestFingerprint.title = status.latest_fingerprint || '';
  if (exists && status.content_hash) {
    els.hash.textContent = truncate(status.content_hash, 18);
    els.hash.title = status.content_hash;
  } else {
    els.hash.textContent = '-';
    els.hash.title = '';
  }
  if (status.scan_interval_seconds && status.scan_interval_seconds > 0) {
    els.interval.textContent = `${status.scan_interval_seconds} 秒`;
    els.interval.title = `${status.scan_interval_seconds} 秒`;
  } else {
    els.interval.textContent = '已关闭';
    els.interval.title = '自动刷新已禁用';
  }
}

async function loadBackups({ silent = false } = {}) {
  try {
    const res = await apiRequest('/api/backups');
    state.backups = res.data || [];
    renderBackups();
    renderMetrics();
  } catch (err) {
    if (!silent) {
      showToast(`读取备份失败：${err.message}`, 'error');
    } else {
      console.warn('读取备份失败：', err);
    }
  }
}

async function refreshAll({ silent = false } = {}) {
  await Promise.all([loadStatus({ silent }), loadBackups({ silent })]);
  scheduleAutoRefresh();
}

function scheduleAutoRefresh() {
  const intervalSec = state.status?.scan_interval_seconds;
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer);
    autoRefreshTimer = null;
  }
  if (!intervalSec || intervalSec <= 0) {
    return;
  }
  const intervalMs = Math.max(1000, intervalSec * 1000);
  autoRefreshTimer = setInterval(() => {
    if (document.hidden) return;
    refreshAll({ silent: true }).catch((err) => console.warn('自动刷新失败', err));
  }, intervalMs);
}

function renderBackups() {
  const tbody = els.tbody;
  tbody.innerHTML = '';
  if (!state.backups || state.backups.length === 0) {
    const tr = document.createElement('tr');
    const td = document.createElement('td');
    td.colSpan = 6;
    td.className = 'empty';
    td.textContent = '暂无备份';
    tr.appendChild(td);
    tbody.appendChild(tr);
    return;
  }
  state.backups.forEach((item) => {
    const tr = document.createElement('tr');

    const created = document.createElement('td');
    created.textContent = formatDate(item.created_at);
    tr.appendChild(created);

    const remark = document.createElement('td');
    remark.textContent = item.remark || '（未填写）';
    remark.title = item.remark || '';
    tr.appendChild(remark);

    const hash = document.createElement('td');
    hash.className = 'mono';
    hash.textContent = truncate(item.content_hash, 18);
    hash.title = item.content_hash;
    tr.appendChild(hash);

    const size = document.createElement('td');
    size.textContent = formatBytes(item.size);
    tr.appendChild(size);

    const auto = document.createElement('td');
    auto.innerHTML = item.is_auto ? '<span class="tag">自动</span>' : '-';
    tr.appendChild(auto);

    const actions = document.createElement('td');
    actions.className = 'actions';
    actions.appendChild(createActionButton('编辑备注', 'edit', item.id, item.remark));
    actions.appendChild(createActionButton('还原', 'restore', item.id));
    actions.appendChild(createActionButton('删除', 'delete', item.id));
    tr.appendChild(actions);

    tbody.appendChild(tr);
  });
}

function renderMetrics() {
  const count = state.backups ? state.backups.length : 0;
  els.metricCount.textContent = String(count);
  els.metricCount.title = `共 ${count} 个备份`;
  if (count > 0) {
    const latest = state.backups[0];
    const latestText = formatDate(latest.created_at);
    els.metricLatest.textContent = latestText;
    els.metricLatest.title = latestText;
  } else {
    els.metricLatest.textContent = '-';
    els.metricLatest.title = '暂无备份';
  }
  const interval = state.status?.scan_interval_seconds || 0;
  els.metricAuto.textContent = interval > 0 ? `每 ${interval} 秒` : '未启用';
  els.metricAuto.title = interval > 0 ? `自动刷新：每 ${interval} 秒` : '未启用';
}

function createActionButton(text, action, id, remark = '') {
  const btn = document.createElement('button');
  btn.className = 'btn ghost';
  btn.textContent = text;
  btn.dataset.action = action;
  btn.dataset.id = id;
  if (remark) {
    btn.dataset.remark = remark;
  }
  return btn;
}

async function handleScan() {
  setButtonLoading(els.scanBtn, true, '检测中…');
  try {
    const res = await apiRequest('/api/scan', { method: 'POST', body: {} });
    const data = res.data || {};
    const message = data.created ? '检测完成，已生成备份' : `检测完成：${data.reason || '无需备份'}`;
    showToast(message, data.created ? 'success' : 'info');
    await refreshAll();
  } catch (err) {
    showToast(`检测失败：${err.message}`, 'error');
  } finally {
    setButtonLoading(els.scanBtn, false);
  }
}

async function handleBackup() {
  const remarkValue = els.remarkInput.value.trim();
  setButtonLoading(els.backupBtn, true, '备份中…');
  try {
    const body = {};
    if (remarkValue) {
      body.remark = remarkValue;
    }
    const res = await apiRequest('/api/backups', { method: 'POST', body });
    const data = res.data || {};
    if (data.created) {
      showToast('备份成功', 'success');
      els.remarkInput.value = '';
    } else {
      showToast(data.reason || '已存在相同内容备份', 'info');
    }
    await refreshAll();
  } catch (err) {
    if (err.status === 409) {
      showToast('备注已存在，请更换', 'error');
    } else {
      showToast(`备份失败：${err.message}`, 'error');
    }
  } finally {
    setButtonLoading(els.backupBtn, false);
  }
}

async function handleCodexLogin() {
  setButtonLoading(els.codexBtn, true, '执行中…');
  let result = null;
  let apiError = null;
  try {
    result = await apiRequest('/api/codex/login', { method: 'POST', allowOkFalse: true });
  } catch (err) {
    apiError = err;
  }
  try {
    await refreshAll();
  } catch (refreshErr) {
    console.warn('刷新登录后状态失败', refreshErr);
  }
  if (apiError) {
    showToast(`codex login 执行异常：${apiError.message}`, 'error');
  } else if (result) {
    const output = result.data || {};
    const exitCode = output.exit_code ?? '-';
    if (result.ok) {
      showToast(`codex login 执行成功（退出码 ${exitCode}）`, 'success');
    } else {
      showToast(`codex login 失败（退出码 ${exitCode}）：${result.error || '未知原因'}`, 'error');
    }
    if (output.stdout) console.info('[codex stdout]\n' + output.stdout);
    if (output.stderr) console.warn('[codex stderr]\n' + output.stderr);
  }
  setButtonLoading(els.codexBtn, false);
}



els.tbody.addEventListener('click', async (event) => {
  const btn = event.target.closest('button[data-action]');
  if (!btn) return;
  const { action, id } = btn.dataset;
  if (!id) return;
  try {
    switch (action) {
      case 'edit':
        await handleEditRemark(btn);
        break;
      case 'restore':
        await handleRestore(btn);
        break;
      case 'delete':
        await handleDelete(btn);
        break;
    }
  } catch (err) {
    showToast(err.message, 'error');
  }
});

async function handleEditRemark(btn) {
  const id = btn.dataset.id;
  const current = btn.dataset.remark || '';
  const next = window.prompt('请输入新的备注（留空表示清除备注）', current);
  if (next === null) return;
  const trimmed = next.trim();
  btn.disabled = true;
  try {
    await apiRequest(`/api/backups/${id}/remark`, { method: 'PATCH', body: { remark: trimmed } });
    showToast('备注已更新', 'success');
    await refreshAll();
  } catch (err) {
    if (err.status === 409) {
      showToast('备注已存在，请更换', 'error');
    } else {
      throw err;
    }
  } finally {
    btn.disabled = false;
  }
}

async function handleRestore(btn) {
  const id = btn.dataset.id;
  if (!confirm('确定用此备份覆盖当前 auth.json 吗？（不会创建 .bak）')) {
    return;
  }
  btn.disabled = true;
  try {
    await apiRequest(`/api/backups/${id}/restore`, { method: 'POST' });
    showToast('还原成功', 'success');
    await refreshAll();
  } catch (err) {
    throw new Error(`还原失败：${err.message}`);
  } finally {
    btn.disabled = false;
  }
}

async function handleDelete(btn) {
  const id = btn.dataset.id;
  if (!confirm('确定删除该备份吗？此操作不可恢复。')) {
    return;
  }
  btn.disabled = true;
  try {
    await apiRequest(`/api/backups/${id}`, { method: 'DELETE' });
    showToast('备份已删除', 'success');
    await refreshAll();
  } catch (err) {
    throw new Error(`删除失败：${err.message}`);
  } finally {
    btn.disabled = false;
  }
}

function formatBytes(bytes) {
  if (typeof bytes !== 'number' || Number.isNaN(bytes)) return '-';
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const idx = Math.floor(Math.log(bytes) / Math.log(1024));
  const value = bytes / Math.pow(1024, idx);
  return `${value.toFixed(idx === 0 ? 0 : 2)} ${units[idx]}`;
}

function formatDate(isoString) {
  if (!isoString) return '-';
  const d = new Date(isoString);
  if (Number.isNaN(d.getTime())) return isoString;
  return d.toLocaleString();
}

function truncate(str, length) {
  if (!str) return '';
  return str.length > length ? `${str.slice(0, length)}…` : str;
}

async function init() {
  els.scanBtn.addEventListener('click', handleScan);
  els.backupBtn.addEventListener('click', handleBackup);
  els.codexBtn.addEventListener('click', handleCodexLogin);
  els.refreshBtn.addEventListener('click', async () => {
    await refreshAll();
    showToast('状态已刷新', 'success');
  });
  await refreshAll();
}

document.addEventListener('visibilitychange', () => {
  if (!document.hidden) {
    refreshAll({ silent: true }).catch((err) => console.warn('前台刷新失败', err));
  }
});

init().catch((err) => {
  console.error(err);
  showToast(`初始化失败：${err.message}`, 'error');
});
