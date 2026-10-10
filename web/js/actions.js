// --- Send Message & Actions ---

let isSendingMessage = false;

async function sendMessage() {
  if ((!activeCascadeId && !activeDraftSession) || isSendingMessage) return;

  const inputEl = document.getElementById("chat-input");
  const text = inputEl.value.trim();
  const hasImages = pendingImages.length > 0;
  const slashSelected = getSelectedSlashCommand();
  if (!text && !hasImages && !slashSelected) return;

  if (!activeCascadeId && activeDraftSession) {
    isSendingMessage = true;
    const sessionToCreate = activeDraftSession;
    activeDraftSession = null;

    const imagesToSend = [...pendingImages];
    pendingImages = [];
    renderImagePreviews();

    inputEl.value = "";
    clearTimeout(draftDebounceTimer);
    inputEl.style.height = "auto";

    const streamEl = document.getElementById("messages-stream");
    if (streamEl) {
      streamEl.innerHTML = `
        <div class="loading-state">
          <div class="ios-spinner"></div>
          <p>正在创建会话并启动 Agent...</p>
        </div>
      `;
    }

    try {
      const isPure = sessionToCreate.isPure;
      const res = await fetch("/gateway/cascade/new", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          workspaceUri: isPure ? "" : sessionToCreate.uri,
          projectId: isPure ? "outside-of-project" : (sessionToCreate.rawId || undefined),
          prompt: text,
          model: activeModel || undefined
        })
      });

      const data = await res.json();
      if (!res.ok || data.status === "error" || !data.cascadeId) {
        throw new Error(data.error || "创建会话失败");
      }

      const newCascadeId = data.cascadeId;
      currentTrajectories[newCascadeId] = {
        id: newCascadeId,
        annotations: { title: isPure ? "新对话" : sessionToCreate.name },
        status: "CASCADE_RUN_STATUS_RUNNING",
        stepCount: 1,
        workspaceUris: isPure ? [] : [sessionToCreate.uri],
        lastModifiedTime: new Date().toISOString()
      };

      navigateTo("#c=" + newCascadeId);
      loadConversations();
    } catch (err) {
      alert("创建会话失败: " + err.message);
      if (streamEl) {
        streamEl.innerHTML = `
          <div class="chat-empty-state">
            <div class="chat-empty-icon" style="background: rgba(255, 59, 48, 0.12); color: var(--ios-red);">
              <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10"></circle>
                <line x1="12" y1="8" x2="12" y2="12"></line>
                <line x1="12" y1="16" x2="12.01" y2="16"></line>
              </svg>
            </div>
            <div class="chat-empty-title">创建会话失败</div>
            <div class="chat-empty-desc">${escapeHtml(err.message)}</div>
          </div>
        `;
      }
    } finally {
      isSendingMessage = false;
    }
    return;
  }

  if (!activeCascadeId) {
    alert("请先选择或新建一个会话");
    return;
  }

  isSendingMessage = true;
  const isRunning = currentTrajectories[activeCascadeId]?.status === "CASCADE_RUN_STATUS_RUNNING";

  const imagesToSend = [...pendingImages];
  pendingImages = [];
  renderImagePreviews();

  inputEl.value = "";
  inputEl.style.height = "auto";
  clearTimeout(draftDebounceTimer);
  DraftManager.clear(activeCascadeId);
  currentCanProceed = false;
  updateProceedButton(false);
  updateContinueButton(false);

  const items = buildSendItems(text);
  const displayText = slashDisplayText(text);
  const sentSlash = slashSelected;
  clearSelectedSlash();
  // Only media[] is stored by language_server (and read by the model); the legacy images[] copy
  // would just double the upload.
  const mediaPayload = imagesToSend.map(img => ({
    inlineData: img.base64Data,
    mimeType: img.mimeType || "image/jpeg"
  }));

  if (isRunning) {
    // Enqueue message while agent is running
    LocalQueueManager.enqueue(displayText || (imagesToSend.length ? `[${imagesToSend.length} 张图片]` : ""));
    updateChatControls(true, null, false);
    try {
      const payload = {
        cascadeId: activeCascadeId,
        model: activeModel,
        items: items,
        deliveryStrategy: 2 // WHEN_IDLE
      };
      if (mediaPayload.length > 0) {
        payload.media = mediaPayload;
      }
      await rpc("SendUserCascadeMessage", payload);
    } catch (err) {
      console.warn("[Queue] SendUserCascadeMessage with WHEN_IDLE notification:", err);
    } finally {
      isSendingMessage = false;
    }
    return;
  }

  const streamEl = document.getElementById("messages-stream");
  const tempId = `temp-user-${Date.now()}`;
  let imgHtml = "";
  if (imagesToSend.length > 0) {
    imgHtml = `<div class="user-message-images">` +
      imagesToSend.map(img => {
        // Attached images carry a blob preview URL plus the base64 payload (there is no dataUrl).
        const src = img.previewUrl || `data:${img.mimeType || "image/jpeg"};base64,${img.base64Data}`;
        return `<img src="${src}" class="bubble-image" data-action="open-image" alt="上传图片" />`;
      }).join("") +
      `</div>`;
  }
  const textHtml = displayText ? `<div>${escapeHtml(displayText)}</div>` : "";
  streamEl.insertAdjacentHTML("beforeend", `
    <div id="${tempId}" class="message-row user">
      <div class="bubble">${imgHtml}${textHtml}</div>
    </div>
  `);
  userIsNearBottom = true;
  streamEl.scrollTop = streamEl.scrollHeight;

  try {
    if (currentTrajectories[activeCascadeId]) {
      currentTrajectories[activeCascadeId].status = "CASCADE_RUN_STATUS_RUNNING";
      currentTrajectories[activeCascadeId].needsInput = false;
    }
    updateChatControls(true, null, false);

    const payload = {
      cascadeId: activeCascadeId,
      model: activeModel,
      items: items
    };
    if (mediaPayload.length > 0) {
      payload.media = mediaPayload;
    }

    await rpc("SendUserCascadeMessage", payload);

    // Ensure WebSocket stream is actively connected
    if (!activeWs || activeWs.readyState !== WebSocket.OPEN) {
      connectStreamWs(activeCascadeId);
    }
  } catch (err) {
    alert("发送失败: " + err.message);
    const tempEl = document.getElementById(tempId);
    if (tempEl) tempEl.remove();
    if (sentSlash) {
      slashState.selected = sentSlash;
      slashState.cascadeId = activeCascadeId;
      renderSlashChip();
    }
    inputEl.value = text;
    pendingImages = imagesToSend;
    renderImagePreviews();
  } finally {
    isSendingMessage = false;
  }
}

async function handleProceed() {
  if (!currentCanProceed || !currentProceedArtifactUri || !activeCascadeId) return;
  const artifactUri = currentProceedArtifactUri;
  updateProceedButton(false);
  currentCanProceed = false;

  const streamEl = document.getElementById("messages-stream");
  let thinkingIndicator = document.getElementById("agent-thinking-indicator");
  if (!thinkingIndicator) {
    thinkingIndicator = document.createElement("div");
    thinkingIndicator.id = "agent-thinking-indicator";
    thinkingIndicator.className = "agent-thinking-card message-entering";
    thinkingIndicator.innerHTML = `
      <div class="agent-avatar">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
          <path d="M12 2v4m0 12v4M4.93 4.93l2.83 2.83m8.48 8.48l2.83 2.83M2 12h4m12 0h4M4.93 19.07l2.83-2.83m8.48-8.48l2.83-2.83"></path>
        </svg>
      </div>
      <div class="agent-thinking-body">
        <div class="thinking-title-row">
          <span>Agent 正在思考与执行</span>
          <div class="activity-dots">
            <span class="dot"></span>
            <span class="dot"></span>
            <span class="dot"></span>
          </div>
        </div>
      </div>
    `;
    streamEl.appendChild(thinkingIndicator);
  }
  userIsNearBottom = true;
  streamEl.scrollTop = streamEl.scrollHeight;

  try {
    if (currentTrajectories[activeCascadeId]) {
      currentTrajectories[activeCascadeId].status = "CASCADE_RUN_STATUS_RUNNING";
      currentTrajectories[activeCascadeId].needsInput = false;
    }
    updateChatControls(true, null, false);

    await rpc("SendUserCascadeMessage", {
      cascadeId: activeCascadeId,
      model: activeModel,
      items: [],
      artifactComments: [
        {
          artifactUri: artifactUri,
          scope: { case: "fullFile", value: {} },
          approvalStatus: 1,
          comment: ""
        }
      ]
    });

    if (!activeWs || activeWs.readyState !== WebSocket.OPEN) {
      connectStreamWs(activeCascadeId);
    }
  } catch (err) {
    alert("确认方案失败: " + err.message);
    if (thinkingIndicator) thinkingIndicator.remove();
    updateProceedButton(true);
    currentCanProceed = true;
    updateChatControls(false, null, false);
  }
}

async function handleContinue() {
  if (!activeCascadeId || isSendingMessage) return;
  const inputEl = document.getElementById("chat-input");
  const text = inputEl ? inputEl.value.trim() : "";
  const textToSend = text ? `${text}\nContinue` : "Continue";
  if (inputEl) {
    inputEl.value = textToSend;
  }
  updateContinueButton(false);
  await sendMessage();
}

async function cancelCurrentTask() {
  if (!activeCascadeId) return;
  if (!confirm("确定要终止当前 Agent 任务吗？")) return;
  currentCanProceed = false;
  updateProceedButton(false);
  updatePendingInteraction(null, false);

  const tasksToStop = RunningTasksManager.tasks ? [...RunningTasksManager.tasks] : [];
  for (const t of tasksToStop) {
    RunningTasksManager.stopTask(t.stepIndex, t.id, true);
  }

  try {
    await rpc("CancelCascadeInvocation", { cascadeId: activeCascadeId });
    if (currentTrajectories[activeCascadeId]) {
      currentTrajectories[activeCascadeId].status = "CASCADE_RUN_STATUS_IDLE";
    }
    updateChatControls(false);
  } catch (err) {
    alert("取消任务失败: " + err.message);
  }
}

// --- Markdown File Viewer Sheet ---
let currentViewerData = null;
let currentViewerUri = null;
const mdContentCache = new Map();

async function fetchFileContent(uri, cascadeId) {
  const params = new URLSearchParams();
  if (uri) params.set("uri", uri);
  if (cascadeId) params.set("cascade_id", cascadeId);
  const resp = await fetch(`/api/v1/files/content?${params.toString()}`);
  if (!resp.ok) {
    let msg = `HTTP ${resp.status}`;
    try {
      const err = await resp.json();
      if (err.error) msg = err.error;
    } catch (_) {}
    throw new Error(msg);
  }
  return resp.json();
}

async function openMarkdownViewer(uri, title) {
  const sheet = document.getElementById("sheet-markdown-viewer");
  if (!sheet) return;

  currentViewerUri = uri || "implementation_plan.md";
  currentViewerData = null;

  const titleEl = document.getElementById("md-viewer-title");
  const subtitleEl = document.getElementById("md-viewer-subtitle");
  const loadingEl = document.getElementById("md-viewer-loading");
  const errorEl = document.getElementById("md-viewer-error");
  const contentEl = document.getElementById("md-viewer-content");
  const proceedBar = document.getElementById("md-viewer-proceed-bar");

  // Determine display title & subtitle
  let rawFilename = (uri || "").split("/").pop().split("?")[0] || uri;
  let filename = rawFilename;
  try {
    filename = decodeURIComponent(rawFilename);
  } catch (_) {}

  let displayTitle = title;
  try {
    if (displayTitle) displayTitle = decodeURIComponent(displayTitle);
  } catch (_) {}

  if (!displayTitle || displayTitle === "Markdown 文档") {
    if (filename.includes("walkthrough")) {
      displayTitle = "Walkthrough";
    } else if (filename.includes("implementation_plan")) {
      displayTitle = "Implementation Plan";
    } else {
      displayTitle = filename || "Markdown 文档";
    }
  }
  let displaySubtitle = filename || uri || "implementation_plan.md";

  if (titleEl) titleEl.textContent = displayTitle;
  if (subtitleEl) subtitleEl.textContent = displaySubtitle;

  const isPlan = (filename.includes("implementation_plan") || (title && title.includes("实施方案"))) && !filename.includes("walkthrough");

  // Fast-path: Check memory cache first
  const cacheKey = `${activeCascadeId || ""}_${currentViewerUri}`;
  const cached = mdContentCache.get(cacheKey);
  let hasCache = false;

  if (cached && cached.content) {
    hasCache = true;
    currentViewerData = cached;
    if (loadingEl) loadingEl.classList.add("hidden");
    if (errorEl) errorEl.classList.add("hidden");
    if (contentEl) {
      contentEl.innerHTML = renderMarkdown(cached.content || "");
      renderAllMermaidDiagrams(contentEl);
    }
    if (cached.filename) {
      if (subtitleEl) subtitleEl.textContent = cached.filename;
      if (titleEl && (!displayTitle || displayTitle.includes("%") || displayTitle === "Markdown 文档")) {
        titleEl.textContent = cached.filename;
      }
    }
    if (proceedBar) {
      const canProceedThis = currentCanProceed && isPlan && (cached.request_feedback || isPlan);
      if (canProceedThis) {
        proceedBar.classList.remove("hidden");
      } else {
        proceedBar.classList.add("hidden");
      }
    }
  } else {
    // Reset state when not cached
    if (contentEl) contentEl.innerHTML = "";
    if (errorEl) errorEl.classList.add("hidden");
    if (loadingEl) loadingEl.classList.remove("hidden");
    if (proceedBar) {
      if (currentCanProceed && isPlan) {
        proceedBar.classList.remove("hidden");
      } else {
        proceedBar.classList.add("hidden");
      }
    }
  }

  sheet.classList.remove("hidden");
  triggerHaptic("selection");

  try {
    const data = await fetchFileContent(currentViewerUri, activeCascadeId);
    currentViewerData = data;
    mdContentCache.set(cacheKey, data);

    if (loadingEl) loadingEl.classList.add("hidden");

    if (data.filename) {
      if (subtitleEl) subtitleEl.textContent = data.filename;
      if (titleEl && (!displayTitle || displayTitle.includes("%") || displayTitle === "Markdown 文档")) {
        titleEl.textContent = data.filename;
      }
    }

    // Render markdown content using chat's rich markdown parser
    if (contentEl) {
      contentEl.innerHTML = renderMarkdown(data.content || "");
      renderAllMermaidDiagrams(contentEl);
    }

    // Check proceed capability
    if (proceedBar) {
      const canProceedThis = currentCanProceed && isPlan && (data.request_feedback || isPlan);
      if (canProceedThis) {
        proceedBar.classList.remove("hidden");
      } else {
        proceedBar.classList.add("hidden");
      }
    }
  } catch (err) {
    if (loadingEl) loadingEl.classList.add("hidden");
    if (!hasCache && errorEl) {
      errorEl.classList.remove("hidden");
      const errText = document.getElementById("md-viewer-error-text");
      if (errText) errText.textContent = `加载失败: ${err.message}`;
    }
  }
}

function closeMarkdownViewer() {
  const sheet = document.getElementById("sheet-markdown-viewer");
  if (sheet) {
    sheet.classList.add("hidden");
  }
  currentViewerData = null;
}

window.openMarkdownViewer = openMarkdownViewer;
window.closeMarkdownViewer = closeMarkdownViewer;

function enableSheetPullToDismiss(sheetEl, closeCallback) {
  if (!sheetEl) return;
  const cardEl = sheetEl.querySelector(".ios-sheet-card");
  const grabberEl = sheetEl.querySelector(".sheet-grabber");
  const headerEl = sheetEl.querySelector(".sheet-header");
  const bodyEl = sheetEl.querySelector(".sheet-body");

  let startY = 0;
  let startX = 0;
  let currentY = 0;
  let isDragging = false;
  let dragAllowed = false;

  function onStart(clientY, clientX, target) {
    // If clicked on an interactive button or input, do not start drag
    if (target.closest("button") || target.closest("a") || target.closest("input")) {
      return;
    }
    startY = clientY;
    startX = clientX;
    currentY = startY;
    isDragging = false;
    dragAllowed = false;

    if (grabberEl?.contains(target) || headerEl?.contains(target)) {
      dragAllowed = true;
    } else if (bodyEl?.contains(target) && bodyEl.scrollTop <= 0) {
      dragAllowed = true;
    }
  }

  function onMove(clientY, clientX, e) {
    if (!dragAllowed) return;
    const dy = clientY - startY;
    const dx = clientX - startX;

    if (!isDragging) {
      if (dy > 6 && Math.abs(dy) > Math.abs(dx)) {
        if (bodyEl?.contains(e.target) && bodyEl.scrollTop > 0) {
          dragAllowed = false;
          return;
        }
        isDragging = true;
        if (cardEl) cardEl.style.transition = "none";
      }
    }

    if (isDragging && dy > 0 && cardEl) {
      if (e.cancelable) e.preventDefault();
      const dampedDy = dy > 180 ? 180 + (dy - 180) * 0.35 : dy;
      cardEl.style.transform = `translateY(${dampedDy}px)`;
      currentY = clientY;
    }
  }

  function onEnd() {
    if (!isDragging) {
      dragAllowed = false;
      return;
    }
    isDragging = false;
    dragAllowed = false;
    const dy = currentY - startY;
    if (cardEl) {
      cardEl.style.transition = "transform 0.28s cubic-bezier(0.16, 1, 0.3, 1)";
      if (dy > 70) {
        closeCallback();
        setTimeout(() => {
          cardEl.style.transform = "";
          cardEl.style.transition = "";
        }, 300);
      } else {
        cardEl.style.transform = "translateY(0)";
        setTimeout(() => {
          cardEl.style.transform = "";
          cardEl.style.transition = "";
        }, 280);
      }
    }
  }

  // Touch handlers
  cardEl?.addEventListener("touchstart", (e) => {
    if (e.touches.length === 1) {
      onStart(e.touches[0].clientY, e.touches[0].clientX, e.target);
    }
  }, { passive: true });

  cardEl?.addEventListener("touchmove", (e) => {
    if (e.touches.length === 1) {
      onMove(e.touches[0].clientY, e.touches[0].clientX, e);
    }
  }, { passive: false });

  cardEl?.addEventListener("touchend", onEnd, { passive: true });
  cardEl?.addEventListener("touchcancel", onEnd, { passive: true });
}

function initMarkdownViewer() {
  const sheet = document.getElementById("sheet-markdown-viewer");
  const retryBtn = document.getElementById("btn-md-viewer-retry");
  const proceedBtn = document.getElementById("btn-md-viewer-proceed");

  enableSheetPullToDismiss(sheet, closeMarkdownViewer);

  sheet?.addEventListener("click", (e) => {
    if (e.target === sheet) {
      closeMarkdownViewer();
    }
  });

  retryBtn?.addEventListener("click", () => {
    if (currentViewerUri) {
      openMarkdownViewer(currentViewerUri);
    }
  });

  proceedBtn?.addEventListener("click", () => {
    closeMarkdownViewer();
    handleProceed();
  });

  // Delegated click on document for any markdown file links
  document.addEventListener("click", (e) => {
    const card = e.target.closest(".artifact-preview-card");
    if (card) {
      e.preventDefault();
      e.stopPropagation();
      const targetUrl = card.getAttribute("data-uri");
      const targetTitle = card.getAttribute("data-title") || "文档详情";
      if (targetUrl) {
        openMarkdownViewer(targetUrl, targetTitle);
      }
      return;
    }

    const link = e.target.closest("a");
    if (!link) return;

    const dataMdUrl = link.getAttribute("data-md-url");
    const href = link.getAttribute("href") || "";

    let isLocalMd = !!dataMdUrl || link.classList.contains("markdown-file-link");
    if (!isLocalMd) {
      const lower = href.toLowerCase();
      const isHttp = lower.startsWith("http://") || lower.startsWith("https://");
      const isExternal = isHttp && !lower.includes(window.location.host);
      if (!isExternal) {
        if (lower.endsWith(".md") || lower.endsWith(".markdown") || lower.includes("/brain/") || lower.includes("/static/artifacts/")) {
          isLocalMd = true;
        }
      }
    }

    if (isLocalMd) {
      e.preventDefault();
      e.stopPropagation();
      const targetUrl = dataMdUrl || href;
      const targetTitle = link.getAttribute("data-md-title") || link.textContent.trim() || "Markdown 文档";
      openMarkdownViewer(targetUrl, targetTitle);
    }
  });
}

// --- iOS Bottom Sheets (New Conversation & Settings) ---

let discoveredProjects = [];
let activeDraftSession = null;

async function openNewSheet() {
  const sheet = document.getElementById("sheet-new");
  if (!sheet) return;
  sheet.classList.remove("hidden");

  // Render existing cached projects or chat card immediately
  renderNewProjectsList(discoveredProjects);

  // Fetch discovered upstream projects from gateway
  try {
    const res = await fetch("/gateway/projects");
    if (res.ok) {
      discoveredProjects = await res.json();
      renderNewProjectsList(discoveredProjects);
    }
  } catch (err) {
    console.warn("Failed to fetch projects for new conversation sheet:", err);
  }
}

function renderNewProjectsList(projects) {
  const listEl = document.getElementById("new-projects-list");
  const countEl = document.getElementById("new-projects-count");
  if (!listEl) return;

  if (countEl) {
    countEl.textContent = projects && projects.length > 0 ? `${projects.length} 个工作区` : "选择模式";
  }

  // 1. Chat card (Pure Chat / no workspace)
  let html = `
    <div class="project-select-card chat-card" data-mode="chat">
      <div class="project-card-icon indigo">
        <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor">
          <path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm-2 12H6v-2h12v2zm0-3H6V9h12v2zm0-3H6V6h12v2z"/>
        </svg>
      </div>
      <div class="project-card-info">
        <div class="project-card-title-row">
          <span class="project-card-title">Chat</span>
          <span class="project-card-badge indigo">新对话</span>
        </div>
        <span class="project-card-subtitle">新对话 · 不关联任何工作区</span>
      </div>
      <svg class="project-card-chevron" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
        <polyline points="9 18 15 12 9 6"></polyline>
      </svg>
    </div>
  `;

  // 2. Discovered project cards
  if (projects && projects.length > 0) {
    html += projects.map((p, idx) => {
      const isWs = !!p.isWorkspace;
      const countBadge = p.sessionCount > 0 ? `<span class="project-card-badge gray">${p.sessionCount} 会话</span>` : "";
      return `
        <div class="project-select-card" data-index="${idx}">
          <div class="project-card-icon blue">
            ${isWs ? `
              <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
                <path d="M20 6h-4V4c0-1.11-.89-2-2-2h-4c-1.11 0-2 .89-2 2v2H4c-1.11 0-1.99.89-1.99 2L2 19c0 1.11.89 2 2 2h16c1.11 0 2-.89 2-2V8c0-1.11-.89-2-2-2zm-6 0h-4V4h4v2z"/>
              </svg>
            ` : `
              <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
                <path d="M10 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z"/>
              </svg>
            `}
          </div>
          <div class="project-card-info">
            <div class="project-card-title-row">
              <span class="project-card-title">${escapeHtml(p.name)}</span>
              ${countBadge}
            </div>
            <span class="project-card-subtitle monospaced">${escapeHtml(p.path)}</span>
          </div>
          <svg class="project-card-chevron" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="9 18 15 12 9 6"></polyline>
          </svg>
        </div>
      `;
    }).join("");
  }

  // 3. Custom NAS directory card
  html += `
    <div class="project-select-card custom-path-card" data-mode="custom">
      <div class="project-card-icon green">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
          <path d="M10 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z"/>
        </svg>
      </div>
      <div class="project-card-info">
        <div class="project-card-title-row">
          <span class="project-card-title">指定 NAS 目录</span>
          <span class="project-card-badge green">自定义路径</span>
        </div>
        <span class="project-card-subtitle monospaced">输入 NAS 绝对路径开启新工作区</span>
      </div>
      <svg class="project-card-chevron" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
        <polyline points="9 18 15 12 9 6"></polyline>
      </svg>
    </div>
  `;

  listEl.innerHTML = html;

  listEl.querySelectorAll(".project-select-card").forEach(card => {
    card.addEventListener("click", () => {
      triggerHaptic("medium");
      closeNewSheet();
      const mode = card.getAttribute("data-mode");
      if (mode === "chat") {
        startDraftSession({ isPure: true, name: "新对话", path: "", uri: "", rawId: "outside-of-project" });
      } else if (mode === "custom") {
        const defaultPath = (projects && projects.length > 0 && projects[0].path) ? projects[0].path : "/home/jiuzai";
        const customPath = prompt("请输入 NAS 工作区目录路径 (例如 /home/jiuzai 或 /vol1/1000):", defaultPath);
        if (customPath && customPath.trim()) {
          const trimmed = customPath.trim();
          const folderName = trimmed.split("/").filter(Boolean).pop() || trimmed;
          startDraftSession({
            isPure: false,
            name: folderName,
            path: trimmed,
            uri: trimmed.startsWith("file://") ? trimmed : `file://${trimmed}`
          });
        }
      } else {
        const idx = parseInt(card.getAttribute("data-index"), 10);
        const p = projects[idx];
        if (p) {
          startDraftSession({
            isPure: false,
            name: p.name,
            path: p.path,
            uri: p.uri || p.path,
            rawId: p.rawId || (p.id !== p.path ? p.id : undefined)
          });
        }
      }
    });
  });
}

function startDraftSession(sessionInfo) {
  activeDraftSession = sessionInfo;
  activeCascadeId = null;
  navigateTo("#draft");
}

function closeNewSheet() {
  const sheet = document.getElementById("sheet-new");
  if (sheet) sheet.classList.add("hidden");
}

function updateAuthUI() {
  const statusEl = document.getElementById("settings-auth-status");
  const deviceIdRow = document.getElementById("settings-device-id-row");
  const deviceIdEl = document.getElementById("settings-device-id");
  const btnUnpair = document.getElementById("btn-unpair-device");
  const paired = isDevicePaired();
  const devId = localStorage.getItem("agy_device_id");

  if (paired) {
    if (statusEl) {
      statusEl.textContent = "已配对";
      statusEl.className = "status-badge connected";
    }
    if (deviceIdRow) deviceIdRow.classList.remove("hidden");
    if (deviceIdEl) deviceIdEl.textContent = devId || "已绑定";
    if (btnUnpair) btnUnpair.classList.remove("hidden");
  } else {
    if (statusEl) {
      statusEl.textContent = "未配对";
      statusEl.className = "status-badge disconnected";
    }
    if (deviceIdRow) deviceIdRow.classList.add("hidden");
    if (btnUnpair) btnUnpair.classList.add("hidden");
  }
}

function parsePairingInput(raw) {
  raw = (raw || "").trim();
  if (!raw) return null;

  // Case 1: agy://pair?host=...&port=...&code=...
  if (raw.startsWith("agy://pair")) {
    try {
      const url = new URL(raw.replace("agy://", "http://"));
      const code = url.searchParams.get("code");
      if (code) return code;
    } catch (_) {
      const match = raw.match(/code=([a-zA-Z0-9]+)/);
      if (match) return match[1];
    }
  }

  return raw;
}

function getDeviceKey() {
  try {
    let k = localStorage.getItem("agy_device_key");
    if (!k) {
      k = (window.crypto && crypto.randomUUID) ? crypto.randomUUID() : (Date.now().toString(36) + Math.random().toString(36).slice(2) + Math.random().toString(36).slice(2));
      if (k.length < 16) k += "0000000000000000";
      localStorage.setItem("agy_device_key", k);
    }
    return k;
  } catch (_) {
    return "";
  }
}

async function pairWithCode(code) {
  const resp = await originalFetch("/api/v1/auth/pair", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      pairing_code: code,
      device_name: `Web Browser (${navigator.userAgent.includes("iPhone") ? "iPhone Safari" : "Desktop/PWA"})`,
      platform: "pwa",
      device_key: getDeviceKey()
    })
  });

  const data = await resp.json();
  if (!resp.ok) {
    const serverMsg = String(data.error || "");
    if (/invalid or expired/i.test(serverMsg)) {
      throw new Error("配对码无效或已过期，请在电脑上重新生成配对二维码（mgy pair）");
    }
    throw new Error(serverMsg || `配对失败 (HTTP ${resp.status})`);
  }

  // C-1: Token is securely set as HttpOnly Cookie by the gateway response.
  // We only track pairing state and public device_id in localStorage.
  localStorage.setItem("agy_paired", "1");
  localStorage.setItem("agy_device_id", data.device_id);
  localStorage.removeItem("agy_device_token");
  updateAuthUI();
  return data;
}

function openPairingSheet(errorMsg = "") {
  const sheet = document.getElementById("sheet-pairing");
  const errEl = document.getElementById("pairing-error-msg");
  const inputEl = document.getElementById("input-pairing-code");
  if (errEl) {
    if (errorMsg) {
      errEl.textContent = errorMsg;
      errEl.classList.remove("hidden");
    } else {
      errEl.textContent = "";
      errEl.classList.add("hidden");
    }
  }
  if (inputEl) {
    if (!inputEl.value) {
      inputEl.value = "";
    }
    setTimeout(() => inputEl.focus(), 150);
  }
  if (sheet) sheet.classList.remove("hidden");
}

function closePairingSheet() {
  const sheet = document.getElementById("sheet-pairing");
  if (sheet) sheet.classList.add("hidden");
}

async function submitPairing() {
  const inputEl = document.getElementById("input-pairing-code");
  const errEl = document.getElementById("pairing-error-msg");
  const submitBtn = document.getElementById("btn-sheet-pairing-submit");
  const raw = inputEl ? inputEl.value : "";
  const code = parsePairingInput(raw);

  if (!code) {
    if (errEl) {
      errEl.textContent = "请输入有效的配对码或配对链接";
      errEl.classList.remove("hidden");
    }
    return;
  }

  if (submitBtn) submitBtn.disabled = true;
  if (errEl) errEl.classList.add("hidden");

  try {
    await pairWithCode(code);
    closePairingSheet();
    if (window.innerWidth >= 768) {
      setTimeout(() => {
        window.location.href = "/?view=desktop";
      }, 400);
      return;
    }
    loadConversations();
    checkGatewayStatus();
  } catch (err) {
    if (errEl) {
      errEl.textContent = err.message || "配对失败，请检查配对码是否过期或失效";
      errEl.classList.remove("hidden");
    }
  } finally {
    if (submitBtn) submitBtn.disabled = false;
  }
}

function unpairDevice() {
  if (confirm("确定要解除当前设备的配对绑定吗？")) {
    localStorage.removeItem("agy_paired");
    localStorage.removeItem("agy_device_token");
    localStorage.removeItem("agy_device_id");
    document.cookie = "agy_dt=; Path=/; Expires=Thu, 01 Jan 1970 00:00:01 GMT;";
    updateAuthUI();
    loadConversations();
  }
}

function clearWebCache() {
  triggerHaptic("medium");
  if (!confirm("确定清空本地会话与文档缓存吗？")) return;
  sessionStepsCache = {};
  currentTrajectories = {};
  discoveredProjects = [];
  try {
    const paired = localStorage.getItem("agy_paired");
    const deviceId = localStorage.getItem("agy_device_id");
    const deviceKey = localStorage.getItem("agy_device_key");
    localStorage.clear();
    if (deviceKey) localStorage.setItem("agy_device_key", deviceKey);
    if (paired) localStorage.setItem("agy_paired", paired);
    if (deviceId) localStorage.setItem("agy_device_id", deviceId);
  } catch (_) {}
  alert("本地会话与文档缓存已清空");
  closeSettingsSheet();
  loadConversations();
}

function openSettingsSheet() {
  checkGatewayStatus();
  updateAuthUI();
  const sheet = document.getElementById("sheet-settings");
  if (sheet) sheet.classList.remove("hidden");
}

function closeSettingsSheet() {
  const sheet = document.getElementById("sheet-settings");
  if (sheet) sheet.classList.add("hidden");
}

async function createConversation() {
  const ws = document.getElementById("new-workspace").value.trim();
  const model = document.getElementById("new-model")?.value || activeModel;
  const prompt = document.getElementById("new-prompt").value.trim();

  if (!ws) {
    alert("请选择或输入目标工作区路径");
    return;
  }

  if (!prompt) {
    alert("请输入首条指令");
    return;
  }

  const createBtn = document.getElementById("btn-sheet-new-create");
  if (createBtn) {
    createBtn.textContent = "创建中...";
    createBtn.disabled = true;
  }

  try {
    const isPure = ws === "outside-of-project" || ws.toLowerCase() === "chat";
    const res = await fetch("/gateway/cascade/new", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        workspaceUri: isPure ? "" : ws,
        projectId: isPure ? "outside-of-project" : undefined,
        prompt: prompt,
        model: model || undefined
      })
    });

    const data = await res.json();
    if (!res.ok || data.status === "error" || !data.cascadeId) {
      throw new Error(data.error || "创建会话失败");
    }

    const cascadeId = data.cascadeId;
    closeNewSheet();
    document.getElementById("new-prompt").value = "";
    navigateTo("#c=" + cascadeId);
    await loadConversations();
  } catch (err) {
    alert("创建会话失败: " + err.message);
  } finally {
    if (createBtn) {
      createBtn.textContent = "开始执行";
      createBtn.disabled = false;
    }
  }
}
