// --- Initialization ---

function initApp() {
  if (window.I18n) {
    window.I18n.translateDOM(document);
  }

  // Initialize Image Viewer Modal
  ImageViewerManager.init();

  // === Security: Event delegation for dynamic UI actions (replaces inline onclick) ===
  document.addEventListener("click", function(e) {
    // 1. Click on thumbnail card anywhere opens the lightbox
    const thumbCard = e.target.closest(".image-thumb-card");
    if (thumbCard) {
      const origUrl = thumbCard.dataset.originalUrl;
      const thumbUrl = thumbCard.dataset.thumbUrl;
      const fallbackUrl = thumbCard.dataset.fallbackUrl;
      const title = thumbCard.dataset.title;
      if (origUrl) {
        ImageViewerManager.open(origUrl, title, thumbUrl, fallbackUrl);
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
      const fallbackUrl = btn.dataset.fallbackUrl;
      const title = btn.dataset.title;
      if (origUrl) {
        ImageViewerManager.open(origUrl, title, thumbUrl, fallbackUrl);
      }
    } else if (action === "open-attachment-file") {
      const uri = btn.dataset.uri;
      const name = btn.dataset.name || "";
      if (uri) {
        if (typeof isWeChatChatArchiveName === "function" && isWeChatChatArchiveName(name) && typeof WeChatArchiveModal !== "undefined") {
          WeChatArchiveModal.open(uri, name);
        } else if (typeof resolveMediaRawUrl === "function") {
          window.open(resolveMediaRawUrl(uri), "_blank", "noopener");
        }
      }
    } else if (action === "stop-task") {
      e.stopPropagation();
      RunningTasksManager.stopTask(Number(btn.dataset.step) || 0, btn.dataset.id);
    } else if (action === "install-plugin") {
      const pluginId = btn.dataset.pluginId || btn.closest(".plugin-card-view")?.dataset.pluginId;
      if (pluginId && typeof window.installPluginFromCard === "function") {
        window.installPluginFromCard(pluginId);
      }
    } else if (action === "remove-pending-image") {
      if (btn.dataset.id && typeof removePendingImage === "function") {
        removePendingImage(btn.dataset.id);
      }
    } else if (action === "remove-pending-attachment") {
      if (btn.dataset.id && typeof removePendingAttachment === "function") {
        removePendingAttachment(btn.dataset.id);
      }
    } else if (action === "queue-send") {
      LocalQueueManager.sendNow(btn.dataset.id);
    } else if (action === "queue-edit") {
      LocalQueueManager.edit(btn.dataset.id);
    } else if (action === "queue-remove") {
      LocalQueueManager.remove(btn.dataset.id);
    }
  });

  if ("serviceWorker" in navigator) {
    navigator.serviceWorker.register("/sw.js").then((reg) => {
      reg.update().catch(() => {});
    }).catch(() => {});
  }

  window.addEventListener("hashchange", renderRoute);

  // The status pill / upstream port live in the settings sheet, so only poll while it is open
  // (openSettingsSheet refreshes it immediately).
  const isSettingsOpen = () => {
    const sheet = document.getElementById("sheet-settings");
    return !!sheet && !sheet.classList.contains("hidden");
  };
  setInterval(() => {
    if (document.visibilityState === "visible" && isSettingsOpen()) {
      checkGatewayStatus();
    }
  }, 6000);

  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") {
      if (isSettingsOpen()) checkGatewayStatus();
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
    navigateTo(SubagentManager.backHash());
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

  if (typeof initPairingScan === "function") initPairingScan();
  initContentSearch();
  initMessageActions();
  initSlashCommands();
  if (typeof initWorkflowTemplates === "function") initWorkflowTemplates();
  initChangesSheet();
  initGitSheet();
  initModelSettings();
  initSubagents();
  initBottomStackObserver();

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

  // Action Button (Send / Stop / Voice Mode / Hangup)
  const sendBtn = document.getElementById("btn-send");
  sendBtn?.addEventListener("click", () => {
    const isRunning = isCascadeRunning(activeCascadeId);
    const hasContent = (document.getElementById("chat-input")?.value.trim().length > 0) || (pendingImages && pendingImages.length > 0) || (typeof pendingAttachments !== "undefined" && pendingAttachments.length > 0) || !!getSelectedSlashCommand();
    const inVoiceCall = typeof WebVoiceCallManager !== "undefined" && WebVoiceCallManager.isInCall;
    const voiceEnabled = typeof WebVoiceCallManager !== "undefined" && WebVoiceCallManager.isVoiceModeEnabled();

    if (inVoiceCall && !hasContent) {
      WebVoiceCallManager.endCall();
    } else if (hasContent) {
      sendMessage();
    } else if (isRunning) {
      cancelCurrentTask();
    } else if (voiceEnabled) {
      WebVoiceCallManager.startCall();
    }
  });

  // Voice Mode Toggle in Settings
  const voiceModeCheckbox = document.getElementById("settings-enable-voice-mode");
  if (voiceModeCheckbox && typeof WebVoiceCallManager !== "undefined") {
    voiceModeCheckbox.checked = WebVoiceCallManager.isVoiceModeEnabled();
    voiceModeCheckbox.addEventListener("change", (e) => {
      WebVoiceCallManager.setVoiceModeEnabled(!!e.target.checked);
    });
  }

  // Font Size Scale in Settings (Antigravity 2.31+)
  const fontSelect = document.getElementById("settings-font-size-scale");
  const savedFontScale = localStorage.getItem("antigravity_font_scale") || "default";
  if (savedFontScale === "compact") {
    document.body.classList.add("font-compact");
  } else if (savedFontScale === "comfortable") {
    document.body.classList.add("font-comfortable");
  }
  if (fontSelect) {
    fontSelect.value = savedFontScale;
    fontSelect.addEventListener("change", (e) => {
      const val = e.target.value;
      localStorage.setItem("antigravity_font_scale", val);
      document.body.classList.remove("font-compact", "font-comfortable");
      if (val === "compact") document.body.classList.add("font-compact");
      if (val === "comfortable") document.body.classList.add("font-comfortable");
    });
  }

  // Compact Mode & Progressive Disclosure Sub-options
  window.CompactModePrefs = {
    isEnabled() {
      return localStorage.getItem("agy_knowledge_mode") === "true";
    },
    collapseTools() {
      const v = localStorage.getItem("agy_compact_collapse_tools");
      return v === null ? true : v === "true";
    },
    collapseTasks() {
      const v = localStorage.getItem("agy_compact_collapse_tasks");
      return v === null ? true : v === "true";
    },
    hideHealthyContext() {
      return localStorage.getItem("agy_compact_hide_healthy_ctx") === "true";
    },
    shouldCollapseTools() {
      return this.isEnabled() && this.collapseTools();
    },
    shouldCollapseTasks() {
      return this.isEnabled() && this.collapseTasks();
    },
    shouldShowContextMeter(meter, isRunning = false, hasPendingInteraction = false) {
      if (!meter) return false;
      if (!(this.isEnabled() && this.hideHealthyContext())) return true;
      if (Boolean(meter.isOverflow) || (Number(meter.percentage) || 0) >= 70) return true;
      if (meter.cacheStatus === "miss_streak" || (Number(meter.cacheMissStreak) || 0) >= 2) return true;
      if (!isRunning && !hasPendingInteraction && (Number(meter.usedTokens) || 0) >= 40000 && meter.lastActivityAt) {
        const ts = Date.parse(meter.lastActivityAt);
        if (!Number.isNaN(ts) && (Date.now() - ts) / 60000 >= 15) return true;
      }
      return false;
    }
  };

  const kmCheckbox = document.getElementById("settings-knowledge-mode");
  const kmSubpanel = document.getElementById("settings-compact-subpanel");
  const toolsCheckbox = document.getElementById("settings-compact-collapse-tools");
  const tasksCheckbox = document.getElementById("settings-compact-collapse-tasks");
  const ctxCheckbox = document.getElementById("settings-compact-hide-healthy-ctx");

  const kmSummary = document.getElementById("settings-compact-summary");

  const syncCompactUI = () => {
    const enabled = window.CompactModePrefs.isEnabled();
    const tools = window.CompactModePrefs.collapseTools();
    const tasks = window.CompactModePrefs.collapseTasks();
    const hideCtx = window.CompactModePrefs.hideHealthyContext();
    if (kmCheckbox) kmCheckbox.checked = enabled;
    if (kmSubpanel) kmSubpanel.classList.toggle("compact-subpanel-disabled", !enabled);
    if (toolsCheckbox) {
      toolsCheckbox.checked = tools;
      toolsCheckbox.disabled = !enabled;
    }
    if (tasksCheckbox) {
      tasksCheckbox.checked = tasks;
      tasksCheckbox.disabled = !enabled;
    }
    if (ctxCheckbox) {
      ctxCheckbox.checked = hideCtx;
      ctxCheckbox.disabled = !enabled;
    }
    if (kmSummary) {
      if (!enabled) {
        kmSummary.textContent = typeof t === "function" ? t("已关闭") : "已关闭";
      } else {
        const count = [tools, tasks, hideCtx].filter(Boolean).length;
        kmSummary.textContent = typeof t === "function" ? `${t("已开启")} (${count}/3)` : `已开启 (${count}/3)`;
      }
    }
  };

  window.addEventListener("mgy:languagechange", () => {
    syncCompactUI();
  });

  const applyCompactLiveEffects = () => {
    if (typeof currentContextMeter !== "undefined" && typeof updateContextMeterUI === "function") {
      updateContextMeterUI(currentContextMeter);
    }
    if (typeof RunningTasksManager !== "undefined" && typeof RunningTasksManager.syncCompactDefault === "function") {
      RunningTasksManager.syncCompactDefault();
    }
    if (typeof activeCascadeId !== "undefined" && activeCascadeId && typeof getSessionStepsCache === "function" && typeof renderMessages === "function") {
      const cached = getSessionStepsCache(activeCascadeId);
      if (cached && Array.isArray(cached.steps)) {
        renderMessages(cached.steps, !!cached.isRunning);
      }
    }
  };

  syncCompactUI();
  kmCheckbox?.addEventListener("change", (e) => {
    localStorage.setItem("agy_knowledge_mode", e.target.checked ? "true" : "false");
    syncCompactUI();
    applyCompactLiveEffects();
  });
  toolsCheckbox?.addEventListener("change", (e) => {
    localStorage.setItem("agy_compact_collapse_tools", e.target.checked ? "true" : "false");
    applyCompactLiveEffects();
  });
  tasksCheckbox?.addEventListener("change", (e) => {
    localStorage.setItem("agy_compact_collapse_tasks", e.target.checked ? "true" : "false");
    applyCompactLiveEffects();
  });
  ctxCheckbox?.addEventListener("change", (e) => {
    localStorage.setItem("agy_compact_hide_healthy_ctx", e.target.checked ? "true" : "false");
    applyCompactLiveEffects();
  });

  // Gateway Smart Alert Preferences (二级页「提醒内容」实时同步)
  const alertKeys = [
    "task_completion",
    "task_stall",
    "cache_miss",
    "quota_low",
    "quota_bonus_reset",
    "quota_anomaly",
  ];
  let currentAlertPrefs = {
    task_completion: true,
    task_stall: true,
    cache_miss: true,
    cache_miss_mode: "shadow",
    quota_low: true,
    quota_bonus_reset: true,
    quota_anomaly: true,
    stall_minutes: 8,
  };

  const syncAlertPrefsUI = (errorText = "") => {
    const rawLang = (document.documentElement.getAttribute("data-lang") || navigator.language || "zh").toLowerCase();
    const lang = rawLang.startsWith("ja") ? "ja" : rawLang.startsWith("en") ? "en" : "zh";
    let enabled = 0;
    for (const k of alertKeys) {
      if (currentAlertPrefs[k] !== false) enabled++;
      const el = document.querySelector(`#settings-sub-alerts input[data-alert-key="${k}"]`);
      if (el) el.checked = currentAlertPrefs[k] !== false;
    }
    const summaryEl = document.getElementById("settings-alerts-summary");
    if (summaryEl) {
      if (enabled === alertKeys.length) {
        summaryEl.textContent = lang === "en" ? "All On" : lang === "ja" ? "すべてオン" : "全部开启";
      } else if (enabled === 0) {
        summaryEl.textContent = lang === "en" ? "All Off" : lang === "ja" ? "すべてオフ" : "全部关闭";
      } else {
        summaryEl.textContent = lang === "en" ? `${enabled}/${alertKeys.length} On` : lang === "ja" ? `${enabled}/${alertKeys.length} オン` : `已开 ${enabled}/${alertKeys.length}`;
      }
    }
    const stallDesc = document.getElementById("settings-alert-stall-desc");
    if (stallDesc) {
      const mins = Number(currentAlertPrefs.stall_minutes) > 0 ? Number(currentAlertPrefs.stall_minutes) : 8;
      stallDesc.textContent =
        lang === "en"
          ? `Notify after ${mins} min with no new output or step progress`
          : lang === "ja"
            ? `${mins} 分間新しい出力や進行がない場合に通知`
            : `连续 ${mins} 分钟无新输出或步骤推进时提醒`;
    }
    const cacheMissDesc = document.getElementById("settings-alert-cache-miss-desc");
    if (cacheMissDesc) {
      const isLive = String(currentAlertPrefs.cache_miss_mode || "shadow").toLowerCase() === "live";
      cacheMissDesc.textContent = isLive
        ? (lang === "en"
            ? "Notify after 3 consecutive large-context cache misses"
            : lang === "ja"
              ? "大コンテキストで3ステップ連続キャッシュ未命中時に通知"
              : "长任务连续 3 步大上下文未命中缓存时提醒")
        : (lang === "en"
            ? "Shadow audit · background report only, no push alerts"
            : lang === "ja"
              ? "シャドウ監査 · バックグラウンド記録のみ（通知なし）"
              : "影子审计模式 · 仅后台记录并生成报告，不推送提醒");
    }
    const footerEl = document.getElementById("settings-alerts-footer");
    if (footerEl) {
      footerEl.textContent =
        errorText ||
        (lang === "en"
          ? "Toggles sync live to your local gateway across paired devices."
          : lang === "ja"
            ? "設定はローカルゲートウェイへ即時同期され、全端末に反映されます。"
            : "开关实时同步至本机网关并跨设备生效。");
    }
  };

  window.PushAlertPrefs = {
    async refresh() {
      try {
        const resp = await fetch("/api/v1/push/preferences");
        if (resp && resp.ok) {
          const data = await resp.json();
          if (data && typeof data === "object") {
            currentAlertPrefs = { ...currentAlertPrefs, ...data };
          }
        }
      } catch (_) {}
      syncAlertPrefsUI();
    },
    async update(key, value) {
      const prev = { ...currentAlertPrefs };
      currentAlertPrefs[key] = Boolean(value);
      syncAlertPrefsUI();
      try {
        const resp = await fetch("/api/v1/push/preferences", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ [key]: Boolean(value) }),
        });
        if (!resp || !resp.ok) {
          currentAlertPrefs = prev;
          const rawLang = (document.documentElement.getAttribute("data-lang") || navigator.language || "zh").toLowerCase();
          const errMsg = rawLang.startsWith("ja")
            ? "同期に失敗しました。接続を確認してください。"
            : rawLang.startsWith("en")
              ? "Sync failed. Check gateway connection."
              : "同步失败，请检查网关连接";
          syncAlertPrefsUI(errMsg);
          return;
        }
        const data = await resp.json();
        if (data && typeof data === "object") {
          currentAlertPrefs = { ...currentAlertPrefs, ...data };
        }
        syncAlertPrefsUI();
      } catch (_) {
        currentAlertPrefs = prev;
        syncAlertPrefsUI("同步失败，请检查网关连接");
      }
    },
  };

  document.querySelectorAll("#settings-sub-alerts input[data-alert-key]").forEach((input) => {
    input.addEventListener("change", (e) => {
      const key = e.target.getAttribute("data-alert-key");
      if (key) window.PushAlertPrefs.update(key, e.target.checked);
    });
  });
  syncAlertPrefsUI();
  window.PushAlertPrefs.refresh();

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

  // Model Switch Chip (click to toggle favorites; long-press or right-click for full 2-level model menu)
  const modelSwitchBtn = document.getElementById("btn-model-switch");
  if (modelSwitchBtn) {
    let modelLongPressTimer = null;
    let suppressNextClick = false;
    modelSwitchBtn.addEventListener("pointerdown", (e) => {
      if (e.button === 2) return;
      suppressNextClick = false;
      clearTimeout(modelLongPressTimer);
      modelLongPressTimer = setTimeout(() => {
        suppressNextClick = true;
        triggerHaptic("medium");
        if (typeof openWebModelPicker === "function") openWebModelPicker(modelSwitchBtn);
      }, 220);
    });
    const cancelPress = () => clearTimeout(modelLongPressTimer);
    modelSwitchBtn.addEventListener("pointerup", cancelPress);
    modelSwitchBtn.addEventListener("pointercancel", cancelPress);
    modelSwitchBtn.addEventListener("pointerleave", cancelPress);
    modelSwitchBtn.addEventListener("contextmenu", (e) => {
      e.preventDefault();
      cancelPress();
      if (typeof openWebModelPicker === "function") openWebModelPicker(modelSwitchBtn);
    });
    modelSwitchBtn.addEventListener("click", () => {
      if (suppressNextClick) {
        suppressNextClick = false;
        return;
      }
      if (typeof webPickerState !== "undefined" && webPickerState) {
        dismissWebModelPicker();
        return;
      }
      toggleModel();
    });
  }
  updateModelSwitchUI();

  // Chips

  document.getElementById("btn-continue")?.addEventListener("click", handleContinue);
  document.getElementById("btn-proceed")?.addEventListener("click", handleProceed);

  // Search Filter with iOS Clear Button
  const searchInput = document.getElementById("conv-search");
  const searchClearBtn = document.getElementById("btn-search-clear");
  let convSearchDebounceTimer = 0;
  if (searchInput) {
    searchInput.closest(".search-capsule")?.addEventListener("click", (e) => {
      if (e.target !== searchInput && !e.target.closest("button")) {
        searchInput.focus();
      }
    });
    searchInput.addEventListener("input", () => {
      if (searchClearBtn) {
        if (searchInput.value.trim().length > 0) {
          searchClearBtn.classList.remove("hidden");
        } else {
          searchClearBtn.classList.add("hidden");
        }
      }
      clearTimeout(convSearchDebounceTimer);
      convSearchDebounceTimer = setTimeout(() => {
        renderConversationList(currentTrajectories);
      }, 120);
      scheduleContentSearch();
    });
  }
  if (searchClearBtn) {
    searchClearBtn.addEventListener("click", () => {
      clearTimeout(convSearchDebounceTimer);
      if (searchInput) {
        searchInput.value = "";
        searchInput.focus();
      }
      searchClearBtn.classList.add("hidden");
      renderConversationList(currentTrajectories);
      scheduleContentSearch();
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
    // Forward taps anywhere inside the 48px capsule padding synchronously to textarea
    // so iOS WebKit treats the user gesture as direct input focus and summons the keyboard.
    chatInput.closest(".textfield-wrapper")?.addEventListener("click", (e) => {
      if (e.target !== chatInput) {
        chatInput.focus();
      }
    });

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
      const isRunning = isCascadeRunning(activeCascadeId);
      updateChatControls(isRunning, null, false);
    });

    chatInput.addEventListener("focus", () => {
      // Keep messages pinned to bottom if already near bottom; avoid unconditional
      // window.scrollTo(0, 0) here so iOS Standalone WebClips that lift the scroll view
      // instead of shrinking visualViewport.height never yank the input back behind the keyboard.
      setTimeout(() => {
        if (messagesStream && userIsNearBottom) {
          messagesStream.scrollTop = messagesStream.scrollHeight;
        }
      }, 100);
      setTimeout(() => {
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
  if (window.Phase3 && typeof window.Phase3.init === "function") {
    window.Phase3.init();
  }

  // Handle PWA shortcut action query params e.g. /?action=new (reuses urlParams declared above)
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
