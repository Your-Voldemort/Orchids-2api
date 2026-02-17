(() => {
  const imagineState = {
    running: false,
    mode: "auto",
    effectiveMode: "auto",
    taskIDs: [],
    wsSockets: [],
    sseStreams: [],
    imageCount: 0,
    latencySum: 0,
    latencyCount: 0,
    fallbackTimer: null,
  };

  const cacheOnlineState = {
    selectedTokens: new Set(),
    accounts: [],
    details: [],
    online: {},
    onlineScope: "none",
    accountMap: new Map(),
    detailMap: new Map(),
  };

  const cacheBatchState = {
    running: false,
    action: "",
    taskID: "",
    total: 0,
    processed: 0,
    statusText: "空闲",
    eventSource: null,
  };

  function handleUnauthorized(res) {
    if (res && res.status === 401) {
      window.location.href = "./login.html";
      return true;
    }
    return false;
  }

  function detectImageMime(b64) {
    const raw = String(b64 || "");
    if (raw.startsWith("iVBOR")) return "image/png";
    if (raw.startsWith("/9j/")) return "image/jpeg";
    if (raw.startsWith("R0lGOD")) return "image/gif";
    return "image/jpeg";
  }

  function formatBytes(bytes) {
    const num = Number(bytes || 0);
    if (!Number.isFinite(num) || num <= 0) return "0 B";
    const units = ["B", "KB", "MB", "GB", "TB"];
    let value = num;
    let idx = 0;
    while (value >= 1024 && idx < units.length - 1) {
      value /= 1024;
      idx++;
    }
    return `${value.toFixed(value >= 10 ? 1 : 2)} ${units[idx]}`;
  }

  function formatTimeMS(ms) {
    const num = Number(ms || 0);
    if (!Number.isFinite(num) || num <= 0) return "-";
    return `${Math.round(num)} ms`;
  }

  function formatDateTime(ms) {
    const num = Number(ms || 0);
    if (!Number.isFinite(num) || num <= 0) return "-";
    return new Date(num).toLocaleString();
  }

  function resolveOnlineStatusText(status) {
    const raw = String(status || "").trim();
    if (raw === "ok") return "连接正常";
    if (raw === "not_loaded") return "未加载";
    if (raw === "no_token") return "无可用 Token";
    if (!raw) return "未知";
    return raw;
  }

  function normalizeOnlineToken(raw) {
    const token = String(raw || "").trim();
    if (!token) return "";
    if (!token.includes("sso=")) return token;
    const idx = token.indexOf("sso=");
    const tail = token.slice(idx + 4);
    const semi = tail.indexOf(";");
    return (semi >= 0 ? tail.slice(0, semi) : tail).trim();
  }

  function formatTokenMask(token) {
    const raw = String(token || "").trim();
    if (!raw) return "";
    if (raw.length <= 24) return raw;
    return `${raw.slice(0, 8)}...${raw.slice(-16)}`;
  }

  function toNumberOrZero(value) {
    const n = Number(value || 0);
    return Number.isFinite(n) ? n : 0;
  }

  function closeCacheBatchStream() {
    const es = cacheBatchState.eventSource;
    cacheBatchState.eventSource = null;
    if (!es) return;
    try {
      es.close();
    } catch (err) {
      // ignore
    }
  }

  function updateCacheBatchUI() {
    const statusEl = document.getElementById("cacheOnlineBatchStatus");
    const progressEl = document.getElementById("cacheOnlineBatchProgress");
    const barEl = document.getElementById("cacheOnlineBatchBar");
    const cancelBtn = document.getElementById("cacheOnlineBatchCancelBtn");
    const loadSelectedBtn = document.getElementById("cacheOnlineLoadSelectedBtn");
    const loadAllBtn = document.getElementById("cacheOnlineLoadAllBtn");
    const clearSelectedBtn = document.getElementById("cacheOnlineClearSelectedBtn");

    const total = Math.max(0, Math.floor(toNumberOrZero(cacheBatchState.total)));
    const processed = Math.max(0, Math.floor(toNumberOrZero(cacheBatchState.processed)));
    const safeTotal = total > 0 ? total : 0;
    const safeProcessed = total > 0 ? Math.min(processed, total) : 0;
    const percent = safeTotal > 0 ? Math.floor((safeProcessed / safeTotal) * 100) : 0;

    if (statusEl) {
      const text = String(cacheBatchState.statusText || "").trim();
      statusEl.textContent = text || (cacheBatchState.running ? "运行中" : "空闲");
    }
    if (progressEl) progressEl.textContent = `${safeProcessed}/${safeTotal}`;
    if (barEl) barEl.value = percent;
    if (cancelBtn) {
      cancelBtn.style.display = cacheBatchState.running ? "inline-flex" : "none";
      cancelBtn.disabled = !cacheBatchState.running;
    }
    if (loadSelectedBtn) loadSelectedBtn.disabled = cacheBatchState.running;
    if (loadAllBtn) loadAllBtn.disabled = cacheBatchState.running;
    if (clearSelectedBtn) clearSelectedBtn.disabled = cacheBatchState.running;
  }

  function applyCacheBatchProgress(msg) {
    if (!msg || typeof msg !== "object") return;
    if (typeof msg.total === "number" && Number.isFinite(msg.total)) {
      cacheBatchState.total = Math.max(0, Math.floor(msg.total));
    }
    if (typeof msg.processed === "number" && Number.isFinite(msg.processed)) {
      cacheBatchState.processed = Math.max(0, Math.floor(msg.processed));
    } else if (typeof msg.done === "number" && Number.isFinite(msg.done)) {
      cacheBatchState.processed = Math.max(0, Math.floor(msg.done));
    }
    if (cacheBatchState.total > 0 && cacheBatchState.processed > cacheBatchState.total) {
      cacheBatchState.total = cacheBatchState.processed;
    }
    updateCacheBatchUI();
  }

  function beginCacheBatch(action, taskID, total, statusText) {
    closeCacheBatchStream();
    cacheBatchState.running = true;
    cacheBatchState.action = String(action || "").trim();
    cacheBatchState.taskID = String(taskID || "").trim();
    cacheBatchState.total = Math.max(0, Math.floor(toNumberOrZero(total)));
    cacheBatchState.processed = 0;
    cacheBatchState.statusText = String(statusText || "运行中");
    updateCacheBatchUI();
  }

  function finishCacheBatch(statusText) {
    cacheBatchState.running = false;
    cacheBatchState.action = "";
    cacheBatchState.taskID = "";
    cacheBatchState.statusText = String(statusText || "空闲");
    closeCacheBatchStream();
    updateCacheBatchUI();
  }

  function openCacheBatchStream(taskID, handlers = {}) {
    const cleanTaskID = String(taskID || "").trim();
    if (!cleanTaskID) throw new Error("empty task_id");

    const url = `/api/v1/admin/batch/${encodeURIComponent(cleanTaskID)}/stream?t=${Date.now()}`;
    const es = new EventSource(url);
    cacheBatchState.eventSource = es;
    let ended = false;

    const doneOnce = (fn) => {
      if (ended) return;
      ended = true;
      closeCacheBatchStream();
      if (typeof fn === "function") {
        Promise.resolve()
          .then(() => fn())
          .catch((err) => {
            showToast(err?.message || String(err || "批量任务处理失败"), "error");
          });
      }
    };

    es.onmessage = (event) => {
      let msg = null;
      try {
        msg = JSON.parse(event.data);
      } catch (err) {
        return;
      }
      if (!msg || typeof msg !== "object") return;
      const msgTaskID = String(msg.task_id || "").trim();
      if (msgTaskID && msgTaskID !== cleanTaskID) return;

      applyCacheBatchProgress(msg);
      const type = String(msg.type || "").trim().toLowerCase();
      if (type === "snapshot" || type === "progress") {
        return;
      }
      if (type === "done") {
        doneOnce(() => {
          if (typeof handlers.onDone === "function") {
            handlers.onDone(msg);
          }
        });
        return;
      }
      if (type === "cancelled") {
        doneOnce(() => {
          if (typeof handlers.onCancelled === "function") {
            handlers.onCancelled(msg);
          }
        });
        return;
      }
      if (type === "error") {
        doneOnce(() => {
          if (typeof handlers.onError === "function") {
            handlers.onError(String(msg.error || "unknown error"), msg);
          }
        });
      }
    };

    es.onerror = () => {
      doneOnce(() => {
        if (typeof handlers.onError === "function") {
          handlers.onError("连接中断", null);
        }
      });
    };
  }

  function setImagineStatus(text) {
    const el = document.getElementById("imagineStatus");
    if (el) el.textContent = String(text || "");
  }

  function setImagineButtons(running) {
    const startBtn = document.getElementById("imagineStartBtn");
    const stopBtn = document.getElementById("imagineStopBtn");
    if (startBtn) startBtn.disabled = !!running;
    if (stopBtn) stopBtn.disabled = !running;
  }

  function updateImagineActiveCount() {
    const el = document.getElementById("imagineActive");
    if (!el) return;
    if (imagineState.effectiveMode === "sse") {
      const count = imagineState.sseStreams.filter((s) => s && s.readyState !== 2).length;
      el.textContent = String(count);
      return;
    }
    const count = imagineState.wsSockets.filter((w) => w && w.readyState === 1).length;
    el.textContent = String(count);
  }

  function resetImagineMetrics() {
    imagineState.imageCount = 0;
    imagineState.latencySum = 0;
    imagineState.latencyCount = 0;
    const count = document.getElementById("imagineCount");
    const latency = document.getElementById("imagineLatency");
    if (count) count.textContent = "0";
    if (latency) latency.textContent = "-";
  }

  function appendImagineImage(b64, seq, elapsedMS, fileURL) {
    const grid = document.getElementById("imagineGrid");
    const empty = document.getElementById("imagineEmpty");
    if (!grid) return;
    if (empty) empty.style.display = "none";

    const card = document.createElement("div");
    card.className = "imagine-card";

    const img = document.createElement("img");
    let src = "";
    if (fileURL) {
      src = fileURL;
    } else if (b64) {
      const mime = detectImageMime(b64);
      src = `data:${mime};base64,${b64}`;
    }
    if (!src) return;
    img.src = src;
    img.alt = `imagine-${seq || 0}`;
    img.loading = "lazy";
    const openURL = fileURL || src;
    img.addEventListener("click", () => window.open(openURL, "_blank", "noopener"));

    const meta = document.createElement("div");
    meta.className = "imagine-meta";
    const left = document.createElement("span");
    left.textContent = `#${seq || 0}`;
    const right = document.createElement("span");
    right.textContent = formatTimeMS(elapsedMS);
    meta.appendChild(left);
    meta.appendChild(right);

    card.appendChild(img);
    card.appendChild(meta);
    grid.prepend(card);
  }

  function handleImagineMessage(payload) {
    if (!payload || typeof payload !== "object") return;
    if (payload.type === "image") {
      const b64 = String(payload.b64_json || "");
      const fileURL = String(payload.file_url || payload.url || "");
      if (!b64 && !fileURL) return;
      imagineState.imageCount += 1;
      const count = document.getElementById("imagineCount");
      if (count) count.textContent = String(imagineState.imageCount);

      const elapsed = Number(payload.elapsed_ms || 0);
      if (elapsed > 0) {
        imagineState.latencySum += elapsed;
        imagineState.latencyCount += 1;
        const avg = Math.round(imagineState.latencySum / imagineState.latencyCount);
        const latency = document.getElementById("imagineLatency");
        if (latency) latency.textContent = `${avg} ms`;
      }
      appendImagineImage(b64, payload.sequence, payload.elapsed_ms, fileURL);
      return;
    }

    if (payload.type === "status") {
      if (payload.status === "running") {
        setImagineStatus(`运行中 (${imagineState.effectiveMode.toUpperCase()})`);
      } else if (payload.status === "stopped") {
        if (imagineState.running) {
          setImagineStatus("已停止");
        }
      }
      return;
    }

    if (payload.type === "error") {
      const msg = String(payload.message || "Imagine 运行出错");
      showToast(msg, "error");
      setImagineStatus("错误");
    }
  }

  function closeImagineConnections(sendStop) {
    if (imagineState.fallbackTimer) {
      clearTimeout(imagineState.fallbackTimer);
      imagineState.fallbackTimer = null;
    }

    imagineState.wsSockets.forEach((ws) => {
      if (!ws) return;
      if (sendStop && ws.readyState === 1) {
        try {
          ws.send(JSON.stringify({ type: "stop" }));
        } catch (err) {
          // ignore
        }
      }
      try {
        ws.close(1000, "stop");
      } catch (err) {
        // ignore
      }
    });
    imagineState.wsSockets = [];

    imagineState.sseStreams.forEach((es) => {
      if (!es) return;
      try {
        es.close();
      } catch (err) {
        // ignore
      }
    });
    imagineState.sseStreams = [];
    updateImagineActiveCount();
  }

  async function createImagineTask(prompt, aspectRatio) {
    const res = await fetch("/api/v1/admin/imagine/start", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        prompt,
        aspect_ratio: aspectRatio,
      }),
    });
    if (handleUnauthorized(res)) {
      throw new Error("unauthorized");
    }
    if (!res.ok) {
      throw new Error(await res.text());
    }
    const data = await res.json();
    return String((data && data.task_id) || "").trim();
  }

  async function stopImagineTasks(taskIDs) {
    if (!Array.isArray(taskIDs) || taskIDs.length === 0) return;
    const res = await fetch("/api/v1/admin/imagine/stop", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ task_ids: taskIDs }),
    });
    if (handleUnauthorized(res)) return;
  }

  function startImagineSSE(taskIDs) {
    imagineState.effectiveMode = "sse";
    setImagineStatus("连接中 (SSE)");
    closeImagineConnections(false);

    taskIDs.forEach((taskID, idx) => {
      const url = `/api/v1/admin/imagine/sse?task_id=${encodeURIComponent(taskID)}&conn=${idx}&t=${Date.now()}`;
      const es = new EventSource(url);
      es.onopen = () => {
        setImagineStatus("运行中 (SSE)");
        updateImagineActiveCount();
      };
      es.onmessage = (event) => {
        try {
          handleImagineMessage(JSON.parse(event.data));
        } catch (err) {
          // ignore bad payload
        }
      };
      es.onerror = () => {
        updateImagineActiveCount();
        const alive = imagineState.sseStreams.filter((s) => s && s.readyState !== 2).length;
        if (alive === 0 && imagineState.running) {
          setImagineStatus("连接异常");
        }
      };
      imagineState.sseStreams.push(es);
    });
  }

  function startImagineWS(taskIDs, prompt, aspectRatio, allowFallback) {
    imagineState.effectiveMode = "ws";
    setImagineStatus("连接中 (WS)");
    closeImagineConnections(false);

    let opened = 0;
    let switched = false;

    if (allowFallback) {
      imagineState.fallbackTimer = setTimeout(() => {
        if (!imagineState.running || opened > 0 || switched) return;
        switched = true;
        showToast("WS 建连失败，自动切换 SSE", "info");
        startImagineSSE(taskIDs);
      }, 1500);
    }

    taskIDs.forEach((taskID) => {
      const protocol = window.location.protocol === "https:" ? "wss" : "ws";
      const url = `${protocol}://${window.location.host}/api/v1/admin/imagine/ws?task_id=${encodeURIComponent(taskID)}`;
      const ws = new WebSocket(url);

      ws.onopen = () => {
        opened += 1;
        updateImagineActiveCount();
        setImagineStatus("运行中 (WS)");
        try {
          ws.send(JSON.stringify({
            type: "start",
            prompt,
            aspect_ratio: aspectRatio,
          }));
        } catch (err) {
          // ignore
        }
      };

      ws.onmessage = (event) => {
        try {
          handleImagineMessage(JSON.parse(event.data));
        } catch (err) {
          // ignore bad payload
        }
      };

      ws.onerror = () => {
        if (allowFallback && opened === 0 && !switched) {
          switched = true;
          startImagineSSE(taskIDs);
          return;
        }
        updateImagineActiveCount();
      };

      ws.onclose = () => {
        updateImagineActiveCount();
      };

      imagineState.wsSockets.push(ws);
    });
  }

  async function startImagine() {
    if (imagineState.running) {
      showToast("Imagine 已在运行中", "info");
      return;
    }
    const prompt = String(document.getElementById("imaginePrompt")?.value || "").trim();
    if (!prompt) {
      showToast("请输入 Prompt", "error");
      return;
    }
    const ratio = String(document.getElementById("imagineRatio")?.value || "2:3");
    const concurrent = Math.max(1, Math.min(3, Number(document.getElementById("imagineConcurrent")?.value || 1)));
    const mode = String(document.getElementById("imagineMode")?.value || "auto").toLowerCase();

    imagineState.running = true;
    imagineState.mode = mode;
    setImagineButtons(true);
    setImagineStatus("创建任务中");

    const taskIDs = [];
    try {
      for (let i = 0; i < concurrent; i++) {
        const taskID = await createImagineTask(prompt, ratio);
        if (!taskID) {
          throw new Error("创建任务失败：空 task_id");
        }
        taskIDs.push(taskID);
      }

      imagineState.taskIDs = taskIDs;
      if (mode === "sse") {
        startImagineSSE(taskIDs);
      } else if (mode === "ws") {
        startImagineWS(taskIDs, prompt, ratio, false);
      } else {
        startImagineWS(taskIDs, prompt, ratio, true);
      }
      showToast(`Imagine 已启动 (${taskIDs.length} 并发)`, "success");
    } catch (err) {
      imagineState.running = false;
      setImagineButtons(false);
      setImagineStatus("启动失败");
      await stopImagineTasks(taskIDs);
      imagineState.taskIDs = [];
      showToast(`启动失败: ${err.message || err}`, "error");
    }
  }

  async function stopImagine() {
    const taskIDs = imagineState.taskIDs.slice();
    imagineState.running = false;
    setImagineButtons(false);
    setImagineStatus("停止中");
    closeImagineConnections(true);
    imagineState.taskIDs = [];
    try {
      await stopImagineTasks(taskIDs);
    } catch (err) {
      // ignore
    }
    updateImagineActiveCount();
    setImagineStatus("已停止");
  }

  function clearImagineGrid() {
    const grid = document.getElementById("imagineGrid");
    const empty = document.getElementById("imagineEmpty");
    if (grid) grid.innerHTML = "";
    if (empty) empty.style.display = "block";
    resetImagineMetrics();
  }

  async function fetchVoiceToken() {
    const voice = String(document.getElementById("voiceName")?.value || "ara").trim() || "ara";
    const personality = String(document.getElementById("voicePersonality")?.value || "assistant").trim() || "assistant";
    const speed = Number(document.getElementById("voiceSpeed")?.value || 1);
    const url = `/api/v1/admin/voice/token?voice=${encodeURIComponent(voice)}&personality=${encodeURIComponent(personality)}&speed=${encodeURIComponent(speed > 0 ? speed : 1)}`;

    const res = await fetch(url);
    if (handleUnauthorized(res)) return;
    if (!res.ok) {
      throw new Error(await res.text());
    }
    const data = await res.json();
    const tokenOutput = document.getElementById("voiceTokenOutput");
    const urlOutput = document.getElementById("voiceUrlOutput");
    if (tokenOutput) tokenOutput.value = String(data.token || "");
    if (urlOutput) urlOutput.value = String(data.url || "");
    showToast("Voice Token 获取成功", "success");
  }

  function normalizeOnlineAccounts(rawAccounts) {
    const list = Array.isArray(rawAccounts) ? rawAccounts : [];
    const out = [];
    for (const item of list) {
      const token = normalizeOnlineToken(item?.token);
      if (!token) continue;
      out.push({
        ...item,
        token,
        token_masked: String(item?.token_masked || formatTokenMask(token)),
      });
    }
    return out;
  }

  function normalizeOnlineDetails(rawDetails) {
    const list = Array.isArray(rawDetails) ? rawDetails : [];
    const out = [];
    for (const item of list) {
      const token = normalizeOnlineToken(item?.token);
      if (!token) continue;
      out.push({
        ...item,
        token,
        token_masked: String(item?.token_masked || formatTokenMask(token)),
      });
    }
    return out;
  }

  function currentOnlineRows() {
    const rows = [];
    const online = cacheOnlineState.online || {};
    const detailsMap = cacheOnlineState.detailMap;
    if (cacheOnlineState.accounts.length > 0) {
      for (const acc of cacheOnlineState.accounts) {
        const token = normalizeOnlineToken(acc.token);
        if (!token) continue;
        const detail = detailsMap.get(token);
        const isOnlineToken = normalizeOnlineToken(online.token) === token;
        const count = detail ? toNumberOrZero(detail.count) : (isOnlineToken ? toNumberOrZero(online.count) : null);
        const status = String(detail?.status || (isOnlineToken ? online.status : "not_loaded") || "not_loaded");
        const lastClear = detail?.last_asset_clear_at ?? (isOnlineToken ? online.last_asset_clear_at : acc.last_asset_clear_at);
        rows.push({
          token,
          token_masked: String(acc.token_masked || detail?.token_masked || formatTokenMask(token)),
          pool: String(acc.pool || "-"),
          count,
          status,
          last_asset_clear_at: lastClear,
        });
      }
      return rows;
    }
    for (const detail of cacheOnlineState.details) {
      rows.push({
        token: detail.token,
        token_masked: String(detail.token_masked || formatTokenMask(detail.token)),
        pool: "-",
        count: toNumberOrZero(detail.count),
        status: String(detail.status || "not_loaded"),
        last_asset_clear_at: detail.last_asset_clear_at,
      });
    }
    return rows;
  }

  function syncCacheOnlineSelectAll() {
    const selectAll = document.getElementById("cacheOnlineSelectAll");
    const body = document.getElementById("cacheOnlineBody");
    if (!selectAll || !body) return;
    const checkboxes = Array.from(body.querySelectorAll("input.cache-online-check"));
    if (checkboxes.length === 0) {
      selectAll.checked = false;
      selectAll.indeterminate = false;
      return;
    }
    const selected = checkboxes.filter((item) => item.checked).length;
    selectAll.checked = selected > 0 && selected === checkboxes.length;
    selectAll.indeterminate = selected > 0 && selected < checkboxes.length;
  }

  function renderCacheOnlineTable() {
    const body = document.getElementById("cacheOnlineBody");
    if (!body) return;
    const rows = currentOnlineRows();
    if (rows.length === 0) {
      body.innerHTML = `<tr><td colspan="7" style="text-align:center;color:var(--text-secondary);padding:24px;">暂无在线账号</td></tr>`;
      syncCacheOnlineSelectAll();
      return;
    }

    body.innerHTML = rows.map((row) => {
      const checked = cacheOnlineState.selectedTokens.has(row.token) ? "checked" : "";
      const countText = row.count === null ? "-" : String(row.count);
      const statusText = resolveOnlineStatusText(row.status);
      const lastClear = formatDateTime(row.last_asset_clear_at);
      return `
        <tr>
          <td style="text-align:center;">
            <input type="checkbox" class="cache-online-check" data-token="${encodeURIComponent(row.token)}" ${checked} />
          </td>
          <td><code>${row.token_masked || formatTokenMask(row.token)}</code></td>
          <td><span class="tag">${row.pool || "-"}</span></td>
          <td>${countText}</td>
          <td>${statusText}</td>
          <td>${lastClear}</td>
          <td>
            <button class="btn btn-danger-outline cache-online-clear-btn" data-token="${encodeURIComponent(row.token)}" style="padding:4px 8px;">清理</button>
          </td>
        </tr>
      `;
    }).join("");
    syncCacheOnlineSelectAll();
  }

  function applyCacheOnlineData(data) {
    const online = (data && typeof data === "object") ? (data.online || {}) : {};
    const onlineScope = String(data?.online_scope || "none");
    const accounts = normalizeOnlineAccounts(data?.online_accounts);
    const details = normalizeOnlineDetails(data?.online_details);

    cacheOnlineState.accounts = accounts;
    cacheOnlineState.details = details;
    cacheOnlineState.online = online;
    cacheOnlineState.onlineScope = onlineScope;
    cacheOnlineState.accountMap = new Map();
    cacheOnlineState.detailMap = new Map();
    accounts.forEach((item) => cacheOnlineState.accountMap.set(item.token, item));
    details.forEach((item) => cacheOnlineState.detailMap.set(item.token, item));

    const available = new Set();
    accounts.forEach((item) => available.add(item.token));
    details.forEach((item) => available.add(item.token));
    Array.from(cacheOnlineState.selectedTokens).forEach((token) => {
      if (!available.has(token)) {
        cacheOnlineState.selectedTokens.delete(token);
      }
    });

    const onlineCountEl = document.getElementById("cacheOnlineCount");
    const onlineStatusEl = document.getElementById("cacheOnlineStatus");
    const onlineScopeEl = document.getElementById("cacheOnlineScope");
    const onlineLastClearEl = document.getElementById("cacheOnlineLastClear");
    if (onlineCountEl) onlineCountEl.textContent = String(toNumberOrZero(online.count));
    if (onlineStatusEl) onlineStatusEl.textContent = resolveOnlineStatusText(online.status);
    if (onlineScopeEl) onlineScopeEl.textContent = onlineScope;
    if (onlineLastClearEl) onlineLastClearEl.textContent = formatDateTime(online.last_asset_clear_at);

    renderCacheOnlineTable();
  }

  async function loadCacheSummary(options = {}) {
    const params = new URLSearchParams();
    const tokens = Array.isArray(options.tokens) ? options.tokens.map(normalizeOnlineToken).filter(Boolean) : [];
    const scope = String(options.scope || "").trim().toLowerCase();
    const token = normalizeOnlineToken(options.token);
    if (tokens.length > 0) {
      params.set("tokens", tokens.join(","));
    } else if (scope === "all") {
      params.set("scope", "all");
    } else if (token) {
      params.set("token", token);
    }

    const url = params.toString() ? `/api/v1/admin/cache?${params.toString()}` : "/api/v1/admin/cache";
    const res = await fetch(url);
    if (handleUnauthorized(res)) return;
    if (!res.ok) {
      throw new Error(await res.text());
    }
    const data = await res.json();
    const imageText = `${data?.image?.count || 0} / ${formatBytes(data?.image?.bytes || 0)}`;
    const videoText = `${data?.video?.count || 0} / ${formatBytes(data?.video?.bytes || 0)}`;
    const totalText = `${data?.total?.count || 0} / ${formatBytes(data?.total?.bytes || 0)}`;

    const imageEl = document.getElementById("cacheImageSummary");
    const videoEl = document.getElementById("cacheVideoSummary");
    const totalEl = document.getElementById("cacheTotalSummary");
    const baseEl = document.getElementById("cacheBaseDir");
    if (imageEl) imageEl.textContent = imageText;
    if (videoEl) videoEl.textContent = videoText;
    if (totalEl) totalEl.textContent = totalText;
    if (baseEl) baseEl.textContent = String(data.base_dir || "-");

    applyCacheOnlineData(data);
    return data;
  }

  function renderCacheList(items) {
    const body = document.getElementById("cacheListBody");
    if (!body) return;
    const list = Array.isArray(items) ? items : [];
    if (list.length === 0) {
      body.innerHTML = `<tr><td colspan="5" style="text-align:center;color:var(--text-secondary);padding:24px;">暂无缓存数据</td></tr>`;
      return;
    }

    body.innerHTML = list.map((item) => {
      const mediaType = String(item.media_type || "");
      const name = String(item.name || "");
      const url = String(item.view_url || item.url || "");
      const size = formatBytes(item.size_bytes || item.size || 0);
      const updatedAt = formatDateTime(item.mtime_ms || item.updated_at || 0);
      return `
        <tr>
          <td><span class="tag">${mediaType}</span></td>
          <td><a href="${url}" target="_blank" rel="noopener"><code>${name}</code></a></td>
          <td>${size}</td>
          <td>${updatedAt}</td>
          <td>
            <button class="btn btn-danger-outline cache-delete-btn" data-media-type="${encodeURIComponent(mediaType)}" data-name="${encodeURIComponent(name)}" style="padding:4px 8px;">删除</button>
          </td>
        </tr>
      `;
    }).join("");
  }

  function selectedOnlineTokens() {
    return Array.from(cacheOnlineState.selectedTokens);
  }

  async function cancelCacheBatchTask() {
    const taskID = String(cacheBatchState.taskID || "").trim();
    if (!cacheBatchState.running || !taskID) return;
    const res = await fetch(`/api/v1/admin/batch/${encodeURIComponent(taskID)}/cancel`, {
      method: "POST",
    });
    if (handleUnauthorized(res)) return;
    if (!res.ok) {
      throw new Error(await res.text());
    }
    showToast("已发送取消请求", "info");
  }

  async function startOnlineLoadBatch(payload, label) {
    if (cacheBatchState.running) {
      showToast("有任务正在运行，请稍候", "info");
      return;
    }
    const res = await fetch("/api/v1/admin/cache/online/load/async", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload || {}),
    });
    if (handleUnauthorized(res)) return;
    let data = {};
    try {
      data = await res.json();
    } catch (err) {
      // ignore
    }
    if (!res.ok || String(data.status || "") !== "success") {
      throw new Error(data.detail || data.error || (await res.text()) || "请求失败");
    }

    const taskID = String(data.task_id || "").trim();
    if (!taskID) {
      throw new Error("创建任务失败：空 task_id");
    }
    const total = toNumberOrZero(data.total);
    beginCacheBatch("load", taskID, total, "在线统计加载中");
    showToast(`开始加载 ${label || "账号"} (${total})`, "info");

    openCacheBatchStream(taskID, {
      onDone: async (msg) => {
        try {
          const result = (msg && typeof msg.result === "object") ? msg.result : null;
          if (result) {
            applyCacheOnlineData(result);
          } else {
            await loadCacheSummary(payload || {});
          }

          let ok = 0;
          let fail = 0;
          const details = Array.isArray(result?.online_details) ? result.online_details : [];
          if (details.length > 0) {
            details.forEach((item) => {
              const status = String(item?.status || "").trim().toLowerCase();
              if (status === "ok") ok++;
              else fail++;
            });
          } else {
            const totalDone = Math.max(toNumberOrZero(msg?.total), toNumberOrZero(data.total));
            ok = totalDone;
          }

          cacheBatchState.processed = Math.max(toNumberOrZero(msg?.total), toNumberOrZero(data.total));
          cacheBatchState.total = cacheBatchState.processed;
          finishCacheBatch("空闲");
          showToast(`在线统计加载完成：成功 ${ok}，失败 ${fail}`, fail > 0 ? "info" : "success");
        } catch (err) {
          finishCacheBatch("失败");
          throw err;
        }
      },
      onCancelled: () => {
        finishCacheBatch("已取消");
        showToast("已终止加载", "info");
      },
      onError: (message) => {
        finishCacheBatch("失败");
        showToast(`加载失败: ${message || "未知错误"}`, "error");
      },
    });
  }

  async function startOnlineClearBatch(tokens) {
    const cleanTokens = Array.isArray(tokens) ? tokens.map(normalizeOnlineToken).filter(Boolean) : [];
    if (cleanTokens.length === 0) {
      showToast("请先选择在线账号", "info");
      return;
    }
    if (cacheBatchState.running) {
      showToast("有任务正在运行，请稍候", "info");
      return;
    }

    const res = await fetch("/api/v1/admin/cache/online/clear/async", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ tokens: cleanTokens }),
    });
    if (handleUnauthorized(res)) return;
    let data = {};
    try {
      data = await res.json();
    } catch (err) {
      // ignore
    }
    if (!res.ok || String(data.status || "") !== "success") {
      throw new Error(data.detail || data.error || (await res.text()) || "请求失败");
    }

    const taskID = String(data.task_id || "").trim();
    if (!taskID) {
      throw new Error("创建任务失败：空 task_id");
    }
    const total = toNumberOrZero(data.total);
    beginCacheBatch("clear", taskID, total, "在线资产清理中");
    showToast(`开始清理 ${cleanTokens.length} 个账号`, "info");

    openCacheBatchStream(taskID, {
      onDone: async (msg) => {
        try {
          const result = (msg && typeof msg.result === "object") ? msg.result : {};
          const summary = (result && typeof result.summary === "object") ? result.summary : {};
          const ok = toNumberOrZero(summary.ok);
          const fail = toNumberOrZero(summary.fail);
          const doneTotal = Math.max(toNumberOrZero(summary.total), toNumberOrZero(msg?.total), cleanTokens.length);
          cacheBatchState.processed = doneTotal;
          cacheBatchState.total = doneTotal;
          finishCacheBatch("空闲");
          showToast(`在线清理完成：成功 ${ok}，失败 ${fail}`, fail > 0 ? "info" : "success");
          await loadCacheSummary({ tokens: cleanTokens });
        } catch (err) {
          finishCacheBatch("失败");
          throw err;
        }
      },
      onCancelled: () => {
        finishCacheBatch("已取消");
        showToast("已终止清理", "info");
      },
      onError: (message) => {
        finishCacheBatch("失败");
        showToast(`清理失败: ${message || "未知错误"}`, "error");
      },
    });
  }

  async function clearOnlineAssets(tokens) {
    const cleanTokens = Array.isArray(tokens) ? tokens.map(normalizeOnlineToken).filter(Boolean) : [];
    if (cleanTokens.length === 0) {
      throw new Error("no tokens selected");
    }
    const body = cleanTokens.length === 1 ? { token: cleanTokens[0] } : { tokens: cleanTokens };
    const res = await fetch("/api/v1/admin/cache/online/clear", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (handleUnauthorized(res)) return null;
    if (!res.ok) {
      throw new Error(await res.text());
    }
    return res.json();
  }

  function summarizeOnlineClear(data) {
    if (!data || typeof data !== "object") {
      return { total: 0, success: 0, failed: 0 };
    }
    const result = data.result || {};
    let total = toNumberOrZero(result.total);
    let success = toNumberOrZero(result.success);
    let failed = toNumberOrZero(result.failed);
    if (total > 0 || success > 0 || failed > 0) {
      return { total, success, failed };
    }
    const results = data.results || {};
    Object.values(results).forEach((item) => {
      const sub = item?.result || {};
      total += toNumberOrZero(sub.total);
      success += toNumberOrZero(sub.success);
      failed += toNumberOrZero(sub.failed);
    });
    return { total, success, failed };
  }

  async function loadCacheList() {
    const filter = String(document.getElementById("cacheTypeFilter")?.value || "").trim();
    const fetchByType = async (mediaType) => {
      const url = `/api/v1/admin/cache/list?media_type=${encodeURIComponent(mediaType)}`;
      const res = await fetch(url);
      if (handleUnauthorized(res)) return null;
      if (!res.ok) {
        throw new Error(await res.text());
      }
      const data = await res.json();
      return Array.isArray(data?.items) ? data.items : [];
    };

    if (filter) {
      const items = await fetchByType(filter);
      if (items === null) return;
      renderCacheList(items);
      return;
    }

    const [imageItems, videoItems] = await Promise.all([fetchByType("image"), fetchByType("video")]);
    if (imageItems === null || videoItems === null) return;
    const merged = imageItems.concat(videoItems);
    merged.sort((a, b) => {
      const left = toNumberOrZero(a?.mtime_ms ?? a?.updated_at);
      const right = toNumberOrZero(b?.mtime_ms ?? b?.updated_at);
      return right - left;
    });
    renderCacheList(merged);
  }

  async function refreshCacheView(options = {}) {
    await loadCacheSummary(options);
    await loadCacheList();
  }

  async function loadSelectedOnlineStats() {
    const tokens = selectedOnlineTokens();
    if (tokens.length === 0) {
      showToast("请先选择在线账号", "info");
      return;
    }
    await startOnlineLoadBatch({ tokens }, "选中账号");
  }

  async function loadAllOnlineStats() {
    const allTokens = cacheOnlineState.accounts.map((item) => normalizeOnlineToken(item.token)).filter(Boolean);
    if (allTokens.length === 0) {
      showToast("暂无在线账号", "info");
      return;
    }
    await startOnlineLoadBatch({ scope: "all" }, "全部账号");
  }

  async function clearSelectedOnlineAssets() {
    const tokens = selectedOnlineTokens();
    if (tokens.length === 0) {
      showToast("请先选择在线账号", "info");
      return;
    }
    if (cacheBatchState.running) {
      showToast("有任务正在运行，请稍候", "info");
      return;
    }
    if (!window.confirm(`确认清理选中的 ${tokens.length} 个账号在线资产？`)) return;
    await startOnlineClearBatch(tokens);
  }

  async function clearSingleOnlineAssets(token) {
    if (cacheBatchState.running) {
      showToast("有任务正在运行，请稍候", "info");
      return;
    }
    const cleanToken = normalizeOnlineToken(token);
    if (!cleanToken) return;
    const display = cacheOnlineState.accountMap.get(cleanToken)?.token_masked || formatTokenMask(cleanToken);
    if (!window.confirm(`确认清理账号 ${display} 的在线资产？`)) return;
    const data = await clearOnlineAssets([cleanToken]);
    if (!data) return;
    const summary = summarizeOnlineClear(data);
    showToast(`在线清理完成：成功 ${summary.success}，失败 ${summary.failed}`, "success");
    await loadCacheSummary({ token: cleanToken });
  }

  async function clearCache() {
    const filter = String(document.getElementById("cacheTypeFilter")?.value || "").trim();
    const confirmText = filter ? `确认清空 ${filter} 缓存？` : "确认清空全部缓存？";
    if (!window.confirm(confirmText)) return;

    const requestClear = async (mediaType) => {
      const res = await fetch("/api/v1/admin/cache/clear", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ media_type: mediaType }),
      });
      if (handleUnauthorized(res)) return null;
      if (!res.ok) {
        throw new Error(await res.text());
      }
      return res.json();
    };

    if (filter) {
      const single = await requestClear(filter);
      if (single === null) return;
    } else {
      const result = await Promise.all([requestClear("image"), requestClear("video")]);
      if (result[0] === null || result[1] === null) return;
    }
    showToast("缓存已清空", "success");
    await refreshCacheView();
  }

  async function deleteCacheItem(mediaType, name) {
    const res = await fetch("/api/v1/admin/cache/item/delete", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        media_type: mediaType,
        name,
      }),
    });
    if (handleUnauthorized(res)) return;
    if (!res.ok) {
      throw new Error(await res.text());
    }
    await refreshCacheView();
  }

  function switchGrokToolTab(tab) {
    const sections = {
      imagine: document.getElementById("grokImagineSection"),
      voice: document.getElementById("grokVoiceSection"),
      cache: document.getElementById("grokCacheSection"),
    };
    Object.keys(sections).forEach((key) => {
      const section = sections[key];
      if (!section) return;
      section.style.display = key === tab ? "grid" : "none";
    });

    const tabs = document.querySelectorAll("#grokToolsTabs .tab-item");
    tabs.forEach((btn) => {
      const active = String(btn.dataset.tab || "").toLowerCase() === String(tab || "").toLowerCase();
      btn.classList.toggle("active", active);
    });
  }

  window.switchGrokToolTab = switchGrokToolTab;

  function bindEvents() {
    const startBtn = document.getElementById("imagineStartBtn");
    const stopBtn = document.getElementById("imagineStopBtn");
    const clearBtn = document.getElementById("imagineClearBtn");
    if (startBtn) startBtn.addEventListener("click", () => startImagine());
    if (stopBtn) stopBtn.addEventListener("click", () => stopImagine());
    if (clearBtn) clearBtn.addEventListener("click", () => clearImagineGrid());

    const voiceFetchBtn = document.getElementById("voiceFetchBtn");
    if (voiceFetchBtn) {
      voiceFetchBtn.addEventListener("click", async () => {
        try {
          await fetchVoiceToken();
        } catch (err) {
          showToast(`获取失败: ${err.message || err}`, "error");
        }
      });
    }
    const voiceCopyBtn = document.getElementById("voiceCopyBtn");
    if (voiceCopyBtn) {
      voiceCopyBtn.addEventListener("click", () => {
        const token = String(document.getElementById("voiceTokenOutput")?.value || "");
        if (!token) {
          showToast("暂无可复制 Token", "info");
          return;
        }
        copyToClipboard(token);
      });
    }

    const cacheRefreshBtn = document.getElementById("cacheRefreshBtn");
    if (cacheRefreshBtn) {
      cacheRefreshBtn.addEventListener("click", async () => {
        try {
          await refreshCacheView();
          showToast("缓存已刷新", "success");
        } catch (err) {
          showToast(`刷新失败: ${err.message || err}`, "error");
        }
      });
    }
    const cacheClearBtn = document.getElementById("cacheClearBtn");
    if (cacheClearBtn) {
      cacheClearBtn.addEventListener("click", async () => {
        try {
          await clearCache();
        } catch (err) {
          showToast(`清空失败: ${err.message || err}`, "error");
        }
      });
    }
    const cacheFilter = document.getElementById("cacheTypeFilter");
    if (cacheFilter) {
      cacheFilter.addEventListener("change", async () => {
        try {
          await loadCacheList();
        } catch (err) {
          showToast(`加载失败: ${err.message || err}`, "error");
        }
      });
    }

    const cacheListBody = document.getElementById("cacheListBody");
    if (cacheListBody) {
      cacheListBody.addEventListener("click", async (event) => {
        const btn = event.target.closest(".cache-delete-btn");
        if (!btn || !cacheListBody.contains(btn)) return;
        const mediaType = decodeURIComponent(btn.dataset.mediaType || "");
        const name = decodeURIComponent(btn.dataset.name || "");
        if (!mediaType || !name) return;
        if (!window.confirm(`确认删除 ${mediaType}/${name} ?`)) return;
        try {
          await deleteCacheItem(mediaType, name);
          showToast("删除成功", "success");
        } catch (err) {
          showToast(`删除失败: ${err.message || err}`, "error");
        }
      });
    }

    const cacheOnlineBody = document.getElementById("cacheOnlineBody");
    if (cacheOnlineBody) {
      cacheOnlineBody.addEventListener("change", (event) => {
        const input = event.target.closest(".cache-online-check");
        if (!input || !cacheOnlineBody.contains(input)) return;
        const token = normalizeOnlineToken(decodeURIComponent(input.dataset.token || ""));
        if (!token) return;
        if (input.checked) {
          cacheOnlineState.selectedTokens.add(token);
        } else {
          cacheOnlineState.selectedTokens.delete(token);
        }
        syncCacheOnlineSelectAll();
      });
      cacheOnlineBody.addEventListener("click", async (event) => {
        const btn = event.target.closest(".cache-online-clear-btn");
        if (!btn || !cacheOnlineBody.contains(btn)) return;
        const token = normalizeOnlineToken(decodeURIComponent(btn.dataset.token || ""));
        if (!token) return;
        try {
          await clearSingleOnlineAssets(token);
        } catch (err) {
          showToast(`在线清理失败: ${err.message || err}`, "error");
        }
      });
    }

    const cacheOnlineSelectAll = document.getElementById("cacheOnlineSelectAll");
    if (cacheOnlineSelectAll) {
      cacheOnlineSelectAll.addEventListener("change", () => {
        const body = document.getElementById("cacheOnlineBody");
        if (!body) return;
        const checked = !!cacheOnlineSelectAll.checked;
        const checkboxes = Array.from(body.querySelectorAll("input.cache-online-check"));
        checkboxes.forEach((item) => {
          const token = normalizeOnlineToken(decodeURIComponent(item.dataset.token || ""));
          if (!token) return;
          item.checked = checked;
          if (checked) {
            cacheOnlineState.selectedTokens.add(token);
          } else {
            cacheOnlineState.selectedTokens.delete(token);
          }
        });
        syncCacheOnlineSelectAll();
      });
    }

    const cacheOnlineLoadSelectedBtn = document.getElementById("cacheOnlineLoadSelectedBtn");
    if (cacheOnlineLoadSelectedBtn) {
      cacheOnlineLoadSelectedBtn.addEventListener("click", async () => {
        try {
          await loadSelectedOnlineStats();
        } catch (err) {
          showToast(`加载失败: ${err.message || err}`, "error");
        }
      });
    }
    const cacheOnlineLoadAllBtn = document.getElementById("cacheOnlineLoadAllBtn");
    if (cacheOnlineLoadAllBtn) {
      cacheOnlineLoadAllBtn.addEventListener("click", async () => {
        try {
          await loadAllOnlineStats();
        } catch (err) {
          showToast(`加载失败: ${err.message || err}`, "error");
        }
      });
    }
    const cacheOnlineClearSelectedBtn = document.getElementById("cacheOnlineClearSelectedBtn");
    if (cacheOnlineClearSelectedBtn) {
      cacheOnlineClearSelectedBtn.addEventListener("click", async () => {
        try {
          await clearSelectedOnlineAssets();
        } catch (err) {
          showToast(`在线清理失败: ${err.message || err}`, "error");
        }
      });
    }
    const cacheOnlineBatchCancelBtn = document.getElementById("cacheOnlineBatchCancelBtn");
    if (cacheOnlineBatchCancelBtn) {
      cacheOnlineBatchCancelBtn.addEventListener("click", async () => {
        try {
          await cancelCacheBatchTask();
        } catch (err) {
          showToast(`取消失败: ${err.message || err}`, "error");
        }
      });
    }
  }

  async function init() {
    bindEvents();
    window.addEventListener("beforeunload", () => {
      if (Array.isArray(imagineState.taskIDs) && imagineState.taskIDs.length > 0) {
        closeImagineConnections(true);
        try {
          const payload = JSON.stringify({ task_ids: imagineState.taskIDs });
          navigator.sendBeacon(
            "/api/v1/admin/imagine/stop",
            new Blob([payload], { type: "application/json" }),
          );
        } catch (err) {
          // ignore unload errors
        }
      }
      closeCacheBatchStream();
    });
    switchGrokToolTab("imagine");
    resetImagineMetrics();
    updateImagineActiveCount();
    updateCacheBatchUI();
    try {
      await refreshCacheView();
    } catch (err) {
      showToast(`缓存加载失败: ${err.message || err}`, "error");
    }
  }

  document.addEventListener("DOMContentLoaded", init);
})();
