// Configuration management JavaScript

let apiKeys = [];
let createdKeys = [];

// Switch between config tabs
function switchConfigTab(tab) {
  document.querySelectorAll("#configTabs .tab-item").forEach(btn => {
    btn.classList.toggle("active",
      (tab === 'basic' && btn.textContent.includes('基础')) ||
      (tab === 'auth' && btn.textContent.includes('API Key')) ||
      (tab === 'proxy' && btn.textContent.includes('代理'))
    );
  });
  document.getElementById("basicConfig").style.display = tab === 'basic' ? 'block' : 'none';
  document.getElementById("authConfig").style.display = tab === 'auth' ? 'block' : 'none';
  document.getElementById("proxyConfig").style.display = tab === 'proxy' ? 'block' : 'none';

  if (tab === 'auth') loadApiKeys();
}

// Update switch label
function updateSwitchLabel(el, text) {
  const span = document.getElementById("label_" + el.id);
  if (span) {
    span.textContent = text + (el.checked ? " (已开启)" : " (已关闭)");
  }
}

// Toggle password visibility
function togglePassword(fieldId) {
  const field = document.getElementById(fieldId);
  if (field) {
    field.type = field.type === 'password' ? 'text' : 'password';
  }
}

// Copy field value to clipboard
function copyFieldValue(fieldId) {
  const field = document.getElementById(fieldId);
  if (field && field.value) {
    copyToClipboard(field.value);
  }
}

// Load configuration from API
async function loadConfiguration() {
  try {
    const res = await fetch("/api/config");
    if (res.status === 401) {
      window.location.href = "./login.html";
      return;
    }
    const cfg = await res.json();

    document.getElementById("cfg_admin_pass").value = cfg.admin_pass || "";
    document.getElementById("cfg_admin_token").value = cfg.admin_token || "";
    document.getElementById("cfg_max_retries").value = cfg.max_retries || 3;
    document.getElementById("cfg_retry_delay").value = cfg.retry_delay || 1000;
    document.getElementById("cfg_switch_count").value = cfg.account_switch_count || 5;
    document.getElementById("cfg_request_timeout").value = cfg.request_timeout || 120;
    document.getElementById("cfg_refresh_interval").value = cfg.token_refresh_interval || 30;

    // Proxy Config Loading
    document.getElementById("cfg_proxy_http").value = cfg.proxy_http || "";
    document.getElementById("cfg_proxy_https").value = cfg.proxy_https || "";
    document.getElementById("cfg_proxy_user").value = cfg.proxy_user || "";
    document.getElementById("cfg_proxy_pass").value = cfg.proxy_pass || "";
    document.getElementById("cfg_proxy_bypass").value = (cfg.proxy_bypass || []).join("\n");

    const autoToken = document.getElementById("cfg_auto_refresh_token");
    autoToken.checked = cfg.auto_refresh_token || false;
    updateSwitchLabel(autoToken, "自动刷新Token");

    const autoUsage = document.getElementById("cfg_auto_refresh_usage");
    autoUsage.checked = cfg.auto_refresh_usage || false;
    updateSwitchLabel(autoUsage, "自动刷新用量");

    const outputTokenCount = document.getElementById("cfg_output_token_count");
    outputTokenCount.checked = cfg.output_token_count || false;
    updateSwitchLabel(outputTokenCount, "输出Token计数");

    const cacheTokenCount = document.getElementById("cfg_cache_token_count");
    cacheTokenCount.checked = cfg.cache_token_count || false;
    updateSwitchLabel(cacheTokenCount, "缓存Token计数");
    document.getElementById("cfg_cache_ttl").value = cfg.cache_ttl || 5;
    document.getElementById("cfg_cache_strategy").value = cfg.cache_strategy || "split";

  } catch (err) {
    showToast("加载配置失败", "error");
  }
}

// Save configuration to API
async function saveConfiguration() {
  const data = {
    admin_pass: document.getElementById("cfg_admin_pass").value,
    admin_token: document.getElementById("cfg_admin_token").value,
    max_retries: parseInt(document.getElementById("cfg_max_retries").value),
    retry_delay: parseInt(document.getElementById("cfg_retry_delay").value),
    account_switch_count: parseInt(document.getElementById("cfg_switch_count").value),
    request_timeout: parseInt(document.getElementById("cfg_request_timeout").value),
    token_refresh_interval: parseInt(document.getElementById("cfg_refresh_interval").value),
    auto_refresh_token: document.getElementById("cfg_auto_refresh_token").checked,
    auto_refresh_usage: document.getElementById("cfg_auto_refresh_usage").checked,
    output_token_count: document.getElementById("cfg_output_token_count").checked,
    cache_token_count: document.getElementById("cfg_cache_token_count").checked,
    cache_ttl: parseInt(document.getElementById("cfg_cache_ttl").value),
    cache_strategy: document.getElementById("cfg_cache_strategy").value,

    // Proxy Config Saving
    proxy_http: document.getElementById("cfg_proxy_http").value,
    proxy_https: document.getElementById("cfg_proxy_https").value,
    proxy_user: document.getElementById("cfg_proxy_user").value,
    proxy_pass: document.getElementById("cfg_proxy_pass").value,
    proxy_bypass: document.getElementById("cfg_proxy_bypass").value.split("\n").filter(line => line.trim() !== "")
  };

  try {
    const res = await fetch("/api/config", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data)
    });
    if (!res.ok) throw new Error(await res.text());
    showToast("配置保存成功");
  } catch (err) {
    showToast("保存失败: " + err.message, "error");
  }
}

// Load API Keys
async function loadApiKeys() {
  try {
    const res = await fetch("/api/keys");
    if (res.status === 401) {
      window.location.href = "./login.html";
      return;
    }
    apiKeys = (await res.json()) || [];
    renderApiKeys();
  } catch (err) {
    showToast("加载 API Keys 失败", "error");
  }
}

// Render API Keys table
function renderApiKeys() {
  const container = document.getElementById("keysList");
  if (apiKeys.length === 0) {
    container.innerHTML = '<div class="empty-state"><p>暂无 API Key，点击上方按钮创建</p></div>';
    return;
  }

  const rows = apiKeys.map((k, idx) => {
    const keyDisplay = k.key_full || k.key_prefix + '****' + k.key_suffix;
    return `
      <tr>
        <td>
          <div style="display: flex; align-items: center; gap: 8px;">
            <span style="cursor: pointer;" onclick="toggleKeyVisibility(${idx})">👁️</span>
            <span id="key-display-${idx}" style="font-family: monospace; color: var(--text-secondary); cursor: pointer;" onclick="copyToClipboard('${keyDisplay}')">
              ${k.key_prefix}****...${k.key_suffix}
            </span>
          </div>
        </td>
        <td>
          <label class="toggle" style="transform: scale(0.8);">
            <input type="checkbox" ${k.enabled ? "checked" : ""} onchange="toggleKeyStatus(${k.id}, this.checked)">
            <span class="toggle-slider"></span>
          </label>
        </td>
        <td style="color: var(--text-secondary); font-size: 0.8rem;">${k.last_used_at ? formatTime(k.last_used_at) : "从未使用"}</td>
        <td>
          <button class="btn btn-danger-outline" style="padding: 4px 8px;" onclick="openDeleteKeyModal(${k.id}, '${escapeHtml(k.key_prefix)}...${escapeHtml(k.key_suffix)}')">删除</button>
        </td>
      </tr>
    `;
  }).join("");

  container.innerHTML = `
    <table>
      <thead>
        <tr>
          <th>Token</th>
          <th>状态</th>
          <th>最后使用</th>
          <th>操作</th>
        </tr>
      </thead>
      <tbody>
        ${rows}
      </tbody>
    </table>
    <div style="margin-top: 24px; padding: 16px; background: rgba(56, 189, 248, 0.1); border: 1px solid var(--accent-blue); border-radius: 8px; color: var(--text-primary);">
      <div style="display: flex; gap: 8px; align-items: start;">
        <span style="font-size: 1.2rem;">💡</span>
        <div style="flex: 1;">
          <div style="font-weight: 600; margin-bottom: 4px;">提示</div>
          <div style="font-size: 0.9rem; line-height: 1.6;">
            • API Key 用于访问接口的身份认证<br>
            • 禁用的 Key 将无法访问 API<br>
            • 请妥善保管您的 API Key，不要泄露给他人
          </div>
        </div>
      </div>
    </div>
  `;
}

// Toggle key visibility
function toggleKeyVisibility(idx) {
  const span = document.getElementById(`key-display-${idx}`);
  const k = apiKeys[idx];
  if (span.textContent.includes('****')) {
    span.textContent = k.key_full || (k.key_prefix + '****' + k.key_suffix);
  } else {
    span.textContent = `${k.key_prefix}****...${k.key_suffix}`;
  }
}

// Toggle key status
async function toggleKeyStatus(id, enabled) {
  try {
    await fetch(`/api/keys/${id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ enabled }),
    });
    showToast(enabled ? "已启用" : "已禁用");
  } catch (err) {
    showToast("操作失败", "error");
  }
}

// Open create key modal
function openCreateKeyModal() {
  document.getElementById("keyName").value = "";
  document.getElementById("createKeyModal").classList.add("active");
  document.getElementById("createKeyModal").style.display = "flex";
}

// Close create key modal
function closeCreateKeyModal() {
  document.getElementById("createKeyModal").classList.remove("active");
  document.getElementById("createKeyModal").style.display = "none";
}

// Create API key
async function createApiKey(e) {
  e.preventDefault();
  const names = document.getElementById("keyName").value.split("\n").filter(n => n.trim());
  if (names.length === 0) return;

  createdKeys = [];
  for (const name of names) {
    try {
      const res = await fetch("/api/keys", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      });
      const data = await res.json();
      createdKeys.push({ name, key: data.key });
    } catch (err) {
      createdKeys.push({ name, error: err.message });
    }
  }
  closeCreateKeyModal();
  renderCreatedKeys();
  document.getElementById("showKeyModal").classList.add("active");
  document.getElementById("showKeyModal").style.display = "flex";
  loadApiKeys();
}

// Render created keys
function renderCreatedKeys() {
  const container = document.getElementById("fullKeyDisplay");
  const keyDisplays = createdKeys.map(k => `
    <div class="key-display" style="margin-bottom: 8px; padding: 12px; background: var(--card-soft); border: 1px dashed var(--border-color); border-radius: 8px;">
      <div style="font-size: 0.8rem; color: var(--text-secondary);">${escapeHtml(k.name)}</div>
      <div style="font-weight: bold; margin-top: 4px; word-break: break-all; color: var(--accent-green);">${escapeHtml(k.key || k.error)}</div>
    </div>
  `).join("");
  container.innerHTML = keyDisplays;
}

// Copy all keys
function copyAllKeys() {
  const text = createdKeys.map(k => `${k.name}: ${k.key || k.error}`).join("\n");
  copyToClipboard(text);
}

// Close show key modal
function closeShowKeyModal() {
  document.getElementById("showKeyModal").classList.remove("active");
  document.getElementById("showKeyModal").style.display = "none";
}

// Open delete key modal
function openDeleteKeyModal(id, name) {
  document.getElementById("deleteKeyId").value = id;
  document.getElementById("deleteKeyName").textContent = name;
  const modal = document.getElementById("deleteKeyModal");
  modal.classList.add("active");
  modal.style.display = "flex";
}

// Close delete key modal
function closeDeleteKeyModal() {
  const modal = document.getElementById("deleteKeyModal");
  modal.classList.remove("active");
  modal.style.display = "none";
}

// Confirm delete key
async function confirmDeleteKey() {
  const id = document.getElementById("deleteKeyId").value;
  try {
    await fetch(`/api/keys/${id}`, { method: "DELETE" });
    closeDeleteKeyModal();
    showToast("删除成功");
    loadApiKeys();
  } catch (err) {
    showToast("删除失败", "error");
  }
}

// Format time
function formatTime(iso) {
  const d = new Date(iso);
  const now = new Date();
  const diff = (now - d) / 1000;
  if (diff < 60) return "刚刚";
  if (diff < 3600) return Math.floor(diff / 60) + " 分钟前";
  if (diff < 86400) return Math.floor(diff / 3600) + " 小时前";
  return d.toLocaleDateString();
}

// Escape HTML to prevent XSS
function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

// Toggle cache config details
function toggleCacheConfig(checked) {
  const details = document.getElementById("cacheConfigDetails");
  if (details) {
    details.style.display = checked ? "block" : "none";
  }
}

// Update memory estimation
function updateMemoryEstimation() {
  const ttlMin = parseInt(document.getElementById("cfg_cache_ttl").value) || 5;
  const strategy = document.getElementById("cfg_cache_strategy").value;
  const mult = strategy === "split" ? 2 : 1;
  const ttlSec = ttlMin * 60;

  document.getElementById("estTTLSeconds").textContent = ttlSec;
  document.getElementById("estStrategyMult").textContent = mult === 2 ? "× 2" : "× 1";
  document.getElementById("memoryEstTitle").textContent = `内存估算 (当前: TTL=${ttlMin}分钟, ${strategy === "split" ? "分离缓存×2" : "混合缓存×1"})`;

  const calc = (qps) => {
    const kb = qps * ttlSec * 0.5 * mult;
    if (kb > 1024) return (kb / 1024).toFixed(1) + "MB";
    return kb.toFixed(1) + "KB";
  };

  document.getElementById("estLow").textContent = calc(10);
  document.getElementById("estMid").textContent = calc(50);
  document.getElementById("estHigh").textContent = calc(100);
}

// Load cache stats
async function loadCacheStats() {
  try {
    const res = await fetch("/api/config/cache/stats");
    if (!res.ok) return;
    const data = await res.json();

    if (data.status === "disabled") {
      document.getElementById("cacheStatsText").textContent = "缓存未启用";
      return;
    }

    const sizeStr = data.size_bytes > 1024 * 1024
      ? (data.size_bytes / (1024 * 1024)).toFixed(2) + " MB"
      : (data.size_bytes / 1024).toFixed(2) + " KB";

    document.getElementById("cacheStatsText").textContent = `缓存条目: ${data.count} 条，占用内存: ${sizeStr}`; // Note: size is approximate
  } catch (err) {
    console.error("Failed to load cache stats", err);
  }
}

// Clear cache
async function clearCache() {
  if (!confirm("确定要清空所有缓存吗？")) return;
  try {
    const res = await fetch("/api/config/cache/clear", { method: "POST" });
    if (!res.ok) throw new Error(await res.text());
    showToast("缓存已清空");
    loadCacheStats();
  } catch (err) {
    showToast("清空失败: " + err.message, "error");
  }
}

// Load configuration on page load
document.addEventListener('DOMContentLoaded', () => {
  loadConfiguration().then(() => {
    // Initialize UI states after config load
    const cacheEnabled = document.getElementById("cfg_cache_token_count").checked;
    toggleCacheConfig(cacheEnabled);
    updateMemoryEstimation();
    if (cacheEnabled) {
      loadCacheStats();
    }
    // Load API keys as they are now part of the basic configuration tab
    loadApiKeys();
  });
});
