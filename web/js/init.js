// --- Initialization ---

function initApp() {
  // Initialize Image Viewer Modal
  ImageViewerManager.init();

  // === Security: Event delegation for dynamic UI actions (replaces inline onclick) ===
  document.addEventListener("click", function(e) {
    // 1. Click on thumbnail card anywhere opens the lightbox
    const thumbCard = e.target.closest(".image-thumb-card");
    if (thumbCard) {
      const origUrl = thumbCard.dataset.originalUrl;
      const thumbUrl = thumbCard.dataset.thumbUrl;
      const title = thumbCard.dataset.title;
      if (origUrl) {
        ImageViewerManager.open(origUrl, title, thumbUrl);
      }
      return;
    }

    const btn = e.target.closest("[data-action]");
    if (!btn) return;
    const action = btn.dataset.action;
    if (action === "open-image") {
      const img = btn.tagName === "IMG" ? btn : btn.querySelector("img");
      if (img && img.src) {
        ImageViewerManager.open(img.src, "上传图片", img.src);
      }
    } else if (action === "open-lightbox") {
      const origUrl = btn.dataset.originalUrl;
      const thumbUrl = btn.dataset.thumbUrl;
      const title = btn.dataset.title;
      if (origUrl) {
        ImageViewerManager.open(origUrl, title, thumbUrl);
      }
    } else if (action === "stop-task") {
      RunningTasksManager.stopTask(Number(btn.dataset.step), btn.dataset.id);
    } else if (action === "queue-send") {
      LocalQueueManager.sendNow(btn.dataset.id);
    } else if (action === "queue-edit") {
      LocalQueueManager.edit(btn.dataset.id);
    } else if (action === "queue-remove") {
      LocalQueueManager.remove(btn.dataset.id);
    }
  });

  if ("serviceWorker" in navigator) {
    navigator.serviceWorker.register("/sw.js").catch(() => {});
  }

  window.addEventListener("hashchange", renderRoute);

  checkGatewayStatus();
  setInterval(() => {
    if (document.visibilityState === "visible") {
      checkGatewayStatus();
    }
  }, 6000);

  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") {
      checkGatewayStatus();
      if (activeCascadeId) {
        if (!activeWs || activeWs.readyState !== WebSocket.OPEN) {
          connectStreamWs(activeCascadeId);
        }
      }
      if (typeof fetchCockpitQuotas === "function") {
        fetchCockpitQuotas();
      }
    }
  });

  // Navigation & Sheets
  document.getElementById("btn-back")?.addEventListener("click", () => {
    triggerHaptic("selection");
    navigateTo("#");
  });
  document.getElementById("btn-new")?.addEventListener("click", openNewSheet);
  document.getElementById("btn-settings")?.addEventListener("click", openSettingsSheet);

  // New Conversation Sheet
  document.getElementById("btn-sheet-new-close")?.addEventListener("click", () => {
    triggerHaptic("light");
    closeNewSheet();
  });
  const sheetNew = document.getElementById("sheet-new");
  sheetNew?.addEventListener("click", (e) => {
    if (e.target === sheetNew) closeNewSheet();
  });
  enableSheetPullToDismiss(sheetNew, closeNewSheet);

  // Settings Sheet
  document.getElementById("btn-sheet-settings-close")?.addEventListener("click", () => {
    triggerHaptic("light");
    closeSettingsSheet();
  });
  document.getElementById("btn-rescan-gateway")?.addEventListener("click", rescanGateway);
  document.getElementById("btn-open-pairing")?.addEventListener("click", () => {
    closeSettingsSheet();
    openPairingSheet();
  });
  document.getElementById("btn-unpair-device")?.addEventListener("click", unpairDevice);
  document.getElementById("btn-clear-web-cache")?.addEventListener("click", clearWebCache);
  const sheetSettings = document.getElementById("sheet-settings");
  sheetSettings?.addEventListener("click", (e) => {
    if (e.target === sheetSettings) closeSettingsSheet();
  });
  enableSheetPullToDismiss(sheetSettings, closeSettingsSheet);

  // Pairing Sheet
  document.getElementById("btn-sheet-pairing-cancel")?.addEventListener("click", closePairingSheet);
  document.getElementById("btn-sheet-pairing-submit")?.addEventListener("click", submitPairing);
  const sheetPairing = document.getElementById("sheet-pairing");
  sheetPairing?.addEventListener("click", (e) => {
    if (e.target === sheetPairing) closePairingSheet();
  });

  // iOS Alert Dialog: Rename Conversation
  document.getElementById("btn-alert-rename-cancel")?.addEventListener("click", closeRenameAlert);
  document.getElementById("btn-alert-rename-save")?.addEventListener("click", submitRenameConversation);
  document.getElementById("input-rename-title")?.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      submitRenameConversation();
    }
  });
  const alertRename = document.getElementById("alert-rename");
  alertRename?.addEventListener("click", (e) => {
    if (e.target === alertRename) closeRenameAlert();
  });

  // iOS ActionSheet: Delete Conversation Confirmation
  document.getElementById("btn-actionsheet-delete-cancel")?.addEventListener("click", closeDeleteActionSheet);
  document.getElementById("actionsheet-delete-backdrop")?.addEventListener("click", closeDeleteActionSheet);
  document.getElementById("btn-actionsheet-delete-confirm")?.addEventListener("click", confirmDeleteConversation);

  // Initialize iOS Gestures & Viewport Handling
  initPullToRefresh();
  initEdgeSwipeBack();
  initVisualViewportHandling();

  // Pairing codes in the URL must never silently replace an existing device token.
  const urlParams = new URLSearchParams(window.location.search);
  const autoPairCode = urlParams.get("pair_code") || urlParams.get("code");
  if (autoPairCode) {
    window.history.replaceState({}, document.title, window.location.pathname + window.location.hash);
    const alreadyPaired = isDevicePaired();
    const hint = alreadyPaired
      ? "链接包含配对码。当前设备已配对，确认后才会替换现有凭据。"
      : "链接包含配对码，请确认后再配对。";
    openPairingSheet(hint);
    const inputEl = document.getElementById("input-pairing-code");
    if (inputEl) inputEl.value = autoPairCode;
  }

  updateAuthUI();

  // Action Button (Send / Stop / Queue Toggle)
  const sendBtn = document.getElementById("btn-send");
  sendBtn?.addEventListener("click", () => {
    const summary = currentTrajectories[activeCascadeId];
    const isRunning = summary?.status === "CASCADE_RUN_STATUS_RUNNING";
    const hasContent = (document.getElementById("chat-input")?.value.trim().length > 0) || (pendingImages && pendingImages.length > 0);
    if (isRunning && !hasContent) {
      cancelCurrentTask();
    } else {
      sendMessage();
    }
  });

  // Expand / Collapse Running Tasks Card
  document.getElementById("btn-tasks-expand")?.addEventListener("click", () => {
    RunningTasksManager.toggleExpand();
  });

  // Expand / Collapse Queued Messages Card
  document.getElementById("btn-queued-expand")?.addEventListener("click", () => {
    LocalQueueManager.toggleExpand();
  });

  // Add Image (+) Chip & File Input
  const addImgBtn = document.getElementById("btn-add-image");
  const imgFileInput = document.getElementById("image-file-input");
  addImgBtn?.addEventListener("click", () => {
    imgFileInput?.click();
  });
  imgFileInput?.addEventListener("change", (e) => {
    if (e.target.files && e.target.files.length) {
      handleFilesSelected(e.target.files);
      e.target.value = "";
    }
  });

  // Model Switch Chip
  const modelSwitchBtn = document.getElementById("btn-model-switch");
  modelSwitchBtn?.addEventListener("click", () => {
    toggleModel();
  });
  updateModelSwitchUI();

  // Chips
  document.getElementById("btn-commit-push")?.addEventListener("click", () => {
    const input = document.getElementById("chat-input");
    if (!input) return;
    const toAppend = "Commit and Push";
    if (!input.value.trim()) {
      input.value = toAppend;
    } else {
      input.value += "\n" + toAppend;
    }
    input.focus();
    if (activeCascadeId) {
      DraftManager.set(activeCascadeId, input.value);
    }
    if (sendBtn && sendBtn.classList.contains("send-mode")) {
      sendBtn.classList.add("active");
    }
  });

  document.getElementById("btn-continue")?.addEventListener("click", handleContinue);
  document.getElementById("btn-proceed")?.addEventListener("click", handleProceed);

  // Search Filter with iOS Clear Button
  const searchInput = document.getElementById("conv-search");
  const searchClearBtn = document.getElementById("btn-search-clear");
  if (searchInput) {
    searchInput.addEventListener("input", () => {
      if (searchClearBtn) {
        if (searchInput.value.trim().length > 0) {
          searchClearBtn.classList.remove("hidden");
        } else {
          searchClearBtn.classList.add("hidden");
        }
      }
      renderConversationList(currentTrajectories);
    });
  }
  if (searchClearBtn) {
    searchClearBtn.addEventListener("click", () => {
      if (searchInput) {
        searchInput.value = "";
        searchInput.focus();
      }
      searchClearBtn.classList.add("hidden");
      renderConversationList(currentTrajectories);
    });
  }

  // Chat Input Auto-grow, Keyboard Dismissal Recovery & Dynamic Queue / Stop Controls
  const chatInput = document.getElementById("chat-input");
  const messagesStream = document.getElementById("messages-stream");

  // Tap conversation view to dismiss keyboard smoothly (parity with iOS)
  if (messagesStream) {
    messagesStream.addEventListener("pointerdown", (e) => {
      // Don't blur if tapping inside an input, button, or interactive control
      if (e.target.closest("button, a, input, select, textarea, summary, .chip-pill")) return;
      if (document.activeElement === chatInput) {
        chatInput.blur();
      }
    });
  }

  if (chatInput) {
    chatInput.addEventListener("input", () => {
      chatInput.style.height = "auto";
      chatInput.style.height = Math.min(chatInput.scrollHeight, 120) + "px";
      if (activeCascadeId) {
        clearTimeout(draftDebounceTimer);
        const textToSave = chatInput.value;
        const targetId = activeCascadeId;
        draftDebounceTimer = setTimeout(() => {
          DraftManager.set(targetId, textToSave);
        }, 300);
      }
      const isRunning = currentTrajectories[activeCascadeId]?.status === "CASCADE_RUN_STATUS_RUNNING";
      updateChatControls(isRunning, null, false);
    });

    chatInput.addEventListener("focus", () => {
      // Lock window displacement when virtual keyboard rises (parity with iOS)
      window.scrollTo(0, 0);
      document.body.scrollTop = 0;
      document.documentElement.scrollTop = 0;
      setTimeout(() => {
        window.scrollTo(0, 0);
        if (messagesStream && userIsNearBottom) {
          messagesStream.scrollTop = messagesStream.scrollHeight;
        }
      }, 100);
      setTimeout(() => {
        window.scrollTo(0, 0);
        if (messagesStream && userIsNearBottom) {
          messagesStream.scrollTop = messagesStream.scrollHeight;
        }
      }, 320);
    });

    chatInput.addEventListener("blur", () => {
      // Ensure window is not displaced when keyboard retracts without jerking messagesStream
      window.scrollTo(0, 0);
      document.body.scrollTop = 0;
      document.documentElement.scrollTop = 0;
    });

    chatInput.addEventListener("keydown", (e) => {
      if (e.isComposing || e.keyCode === 229) return; // Ignore IME composition (Chinese, Japanese, Korean)
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        sendMessage();
      }
    });

    // Support pasting image screenshots directly into chat input
    chatInput.addEventListener("paste", (e) => {
      const items = e.clipboardData?.items;
      if (!items) return;
      const files = [];
      for (let i = 0; i < items.length; i++) {
        if (items[i].type.startsWith("image/")) {
          const file = items[i].getAsFile();
          if (file) files.push(file);
        }
      }
      if (files.length > 0) {
        handleFilesSelected(files);
      }
    });
  }

  renderRoute();
  initQuotaModule();
  initMarkdownViewer();

  // Handle PWA shortcut action query params e.g. /?action=new
  const urlParams = new URLSearchParams(window.location.search);
  if (urlParams.get("action") === "new") {
    window.history.replaceState({}, document.title, window.location.pathname + window.location.hash);
    setTimeout(() => {
      openNewSheet();
    }, 150);
  }
}

if (document.readyState === "loading") {
  window.addEventListener("DOMContentLoaded", initApp);
} else {
  initApp();
}
