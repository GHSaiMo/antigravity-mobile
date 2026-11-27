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

  // iOS ActionSheet: Archive / Unarchive Conversation Confirmation
  document.getElementById("btn-actionsheet-archive-cancel")?.addEventListener("click", closeArchiveActionSheet);
  document.getElementById("actionsheet-archive-backdrop")?.addEventListener("click", closeArchiveActionSheet);
  document.getElementById("btn-actionsheet-archive-confirm")?.addEventListener("click", confirmArchiveConversation);

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

  // Language Selector in Settings (supports system / zh-CN / en / ja)
  const langSelect = document.getElementById("settings-language-select");
  if (langSelect && window.I18n) {
    langSelect.value = window.I18n.getPreference ? window.I18n.getPreference() : "system";
    langSelect.addEventListener("change", (e) => {
      window.I18n.setLanguage(e.target.value);
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
  const launchpadCheckbox = document.getElementById("settings-new-chat-launchpad");

  const kmSummary = document.getElementById("settings-compact-summary");

  if (launchpadCheckbox) {
    launchpadCheckbox.checked = localStorage.getItem("agy_new_chat_launchpad") !== "false";
    launchpadCheckbox.addEventListener("change", () => {
      localStorage.setItem("agy_new_chat_launchpad", launchpadCheckbox.checked ? "true" : "false");
      if (typeof window.refreshNewChatLaunchpad === "function") {
        window.refreshNewChatLaunchpad();
      }
    });
  }

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
    if (langSelect && window.I18n?.getPreference) {
      langSelect.value = window.I18n.getPreference();
    }
    if (typeof syncAlertPrefsUI === "function") {
      syncAlertPrefsUI();
    }
    if (typeof updateModelSwitchUI === "function") {
      updateModelSwitchUI();
    }
    if (typeof updateQuotaDisplaySummary === "function") {
      updateQuotaDisplaySummary();
    }
    if (typeof currentCockpitQuotas !== "undefined" && currentCockpitQuotas) {
      if (typeof renderQuotaStatusBar === "function") {
        renderQuotaStatusBar(currentCockpitQuotas);
      }
      if (typeof renderQuotaSheet === "function") {
        renderQuotaSheet(currentCockpitQuotas);
      }
    }
    if (typeof currentContextMeter !== "undefined" && currentContextMeter && typeof renderAuditSheetContent === "function") {
      const auditSheet = document.getElementById("sheet-audit");
      if (auditSheet && !auditSheet.classList.contains("hidden")) {
        renderAuditSheetContent(currentContextMeter);
      }
    }
    if (window.Phase4) {
      window.Phase4.refreshSettingsDesktopUpdateRow?.();
      window.Phase4.rerenderHostUpdatesSubpage?.();
    }
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
    const lang = window.I18n?.isZh?.() ? "zh" : (window.I18n?.isJa?.() ? "ja" : "en");
    const isCacheMissLive = String(currentAlertPrefs.cache_miss_mode || "shadow").toLowerCase() === "live";
    const cacheMissRow = document.getElementById("settings-alert-cache-miss-row");
    if (cacheMissRow) {
      cacheMissRow.classList.toggle("hidden", !isCacheMissLive);
    }
    const visibleAlertKeys = isCacheMissLive
      ? alertKeys
      : alertKeys.filter((k) => k !== "cache_miss");
    let enabled = 0;
    for (const k of alertKeys) {
      const el = document.querySelector(`#settings-sub-alerts input[data-alert-key="${k}"]`);
      if (el) el.checked = currentAlertPrefs[k] !== false;
    }
    for (const k of visibleAlertKeys) {
      if (currentAlertPrefs[k] !== false) enabled++;
    }
    const summaryEl = document.getElementById("settings-alerts-summary");
    if (summaryEl) {
      if (enabled === visibleAlertKeys.length) {
        summaryEl.textContent = lang === "en" ? "All On" : lang === "ja" ? "すべてオン" : "全部开启";
      } else if (enabled === 0) {
        summaryEl.textContent = lang === "en" ? "All Off" : lang === "ja" ? "すべてオフ" : "全部关闭";
      } else {
        summaryEl.textContent = lang === "en" ? `${enabled}/${visibleAlertKeys.length} On` : lang === "ja" ? `${enabled}/${visibleAlertKeys.length} オン` : `已开 ${enabled}/${visibleAlertKeys.length}`;
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
      cacheMissDesc.textContent =
        lang === "en"
          ? "Notify after 3 consecutive large-context cache misses"
          : lang === "ja"
            ? "大コンテキストで3ステップ連続キャッシュ未命中時に通知"
            : "长任务连续 3 步大上下文未命中缓存时提醒";
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
          const lang = window.I18n?.isZh?.() ? "zh" : (window.I18n?.isJa?.() ? "ja" : "en");
          const errMsg = lang === "ja"
            ? "同期に失敗しました。接続を確認してください。"
            : lang === "en"
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
        syncAlertPrefsUI(typeof t === "function" ? t("同步失败，请检查网关连接") : "同步失败，请检查网关连接");
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

  // --- AI Assist (Smart Translation & Next-Prompt Suggestions) ---
  let aiAssistState = {
    prefs: {
      titleTranslateEnabled: false,
      titleTranslateLang: "",
      titleTranslateLangName: "",
      stepPlanTranslateEnabled: false,
      focusTranslateEnabled: false,
      nextPromptEnabled: false,
      postCompactRehydrateEnabled: false,
    },
    modelName: "Gemini 3.5 Flash Lite",
    resolvedLangName: "",
    available: true,
    rehydrateHookInstalled: false,
  };

  function resolveWebTitleTargetLanguage() {
    const pref = window.I18n?.getPreference?.() || localStorage.getItem("antigravity_language") || "system";
    if (pref === "en") return { kind: "en", bcp47: "en", englishName: "English", isEnglish: true };
    if (pref === "zh-CN") return { kind: "zh-Hans", bcp47: "zh-Hans", englishName: "Simplified Chinese", isEnglish: false };
    if (pref === "ja") return { kind: "ja", bcp47: "ja", englishName: "Japanese", isEnglish: false };

    const raw = (
      (navigator.languages && navigator.languages[0]) ||
      navigator.language ||
      "en"
    ).replace(/_/g, "-").trim();
    const lower = raw.toLowerCase();
    if (lower === "en" || lower.startsWith("en-")) {
      return { kind: "en", bcp47: "en", englishName: "English", isEnglish: true };
    }
    if (lower.startsWith("zh-hant") || lower.startsWith("zh-tw") || lower.startsWith("zh-hk") || lower.startsWith("zh-mo")) {
      return { kind: "zh-Hant", bcp47: "zh-Hant", englishName: "Traditional Chinese", isEnglish: false };
    }
    if (lower === "zh" || lower.startsWith("zh-")) {
      return { kind: "zh-Hans", bcp47: "zh-Hans", englishName: "Simplified Chinese", isEnglish: false };
    }
    if (lower === "ja" || lower.startsWith("ja-")) {
      return { kind: "ja", bcp47: "ja", englishName: "Japanese", isEnglish: false };
    }
    const base = lower.split("-")[0] || lower;
    let displayName = raw;
    try {
      if (typeof Intl !== "undefined" && typeof Intl.DisplayNames === "function") {
        const dn = new Intl.DisplayNames(["en"], { type: "language" });
        displayName = dn.of(base) || dn.of(raw) || raw;
      }
    } catch (_) {}
    if (displayName) {
      displayName = displayName.charAt(0).toUpperCase() + displayName.slice(1);
    }
    return { kind: "other", bcp47: raw || base, englishName: displayName || raw, isEnglish: false };
  }

  function syncAIAssistUI(syncErrorText = "") {
    const dev = resolveWebTitleTargetLanguage();
    const p = aiAssistState.prefs;
    const showSmartTranslation = !dev.isEnglish;

    const uiLang = window.I18n?.isZh?.() ? "zh" : (window.I18n?.isJa?.() ? "ja" : "en");
    const summaryEl = document.getElementById("settings-ai-assist-summary");
    if (summaryEl) {
      const titleOn = showSmartTranslation && Boolean(p.titleTranslateEnabled);
      const stepOn = showSmartTranslation && Boolean(p.stepPlanTranslateEnabled);
      const focusOn = showSmartTranslation && Boolean(p.focusTranslateEnabled);
      const nextOn = Boolean(p.nextPromptEnabled);
      const rehydrateOn = Boolean(p.postCompactRehydrateEnabled);
      const count = [titleOn, stepOn, focusOn, nextOn, rehydrateOn].filter(Boolean).length;
      let label = uiLang === "en" ? "Off" : (uiLang === "ja" ? "オフ" : "已关闭");
      if (count >= 2) {
        label = uiLang === "en" ? `${count} Enabled` : (uiLang === "ja" ? `${count} 件有効` : `已开启 ${count} 项`);
      } else if (titleOn) {
        label = uiLang === "en" ? "Title Translation" : (uiLang === "ja" ? "タイトル翻訳" : "标题翻译");
      } else if (stepOn) {
        label = uiLang === "en" ? "Step Translation" : (uiLang === "ja" ? "ステップ翻訳" : "步骤翻译");
      } else if (focusOn) {
        label = uiLang === "en" ? "Status Translation" : (uiLang === "ja" ? "ステータス翻訳" : "状态翻译");
      } else if (nextOn) {
        label = uiLang === "en" ? "Prompt Suggestions" : (uiLang === "ja" ? "プロンプト提案" : "提示词建议");
      } else if (rehydrateOn) {
        label = uiLang === "en" ? "Context Rehydration" : (uiLang === "ja" ? "圧縮後コンテキスト復元" : "压缩后回灌");
      }
      summaryEl.textContent = label;
    }

    const translationSection = document.getElementById("ai-assist-translation-section");
    if (translationSection) {
      translationSection.classList.toggle("hidden", !showSmartTranslation);
    }
    if (showSmartTranslation) {
      const hdr = document.getElementById("ai-assist-translation-header");
      const titleLbl = document.getElementById("settings-ai-title-label");
      const titleDesc = document.getElementById("settings-ai-title-desc");
      const stepLbl = document.getElementById("settings-ai-step-plan-label");
      const stepDesc = document.getElementById("settings-ai-step-plan-desc");
      const focusLbl = document.getElementById("settings-ai-focus-label");
      const focusDesc = document.getElementById("settings-ai-focus-desc");
      const footer = document.getElementById("settings-ai-translation-footer");

      if (dev.kind === "zh-Hans" || dev.kind === "zh-Hant") {
        const langLabel = dev.kind === "zh-Hant" ? "繁体中文" : "简体中文";
        if (hdr) hdr.textContent = "智能翻译";
        if (titleLbl) titleLbl.textContent = "会话标题翻译";
        if (titleDesc) titleDesc.textContent = `新会话生成英文标题后自动翻译为${langLabel}并覆盖`;
        if (stepLbl) stepLbl.textContent = "步骤清单实时翻译";
        if (stepDesc) stepDesc.textContent = `将英文待办步骤翻译为${langLabel}（含灵动岛）`;
        if (focusLbl) focusLbl.textContent = "实时状态行翻译";
        if (focusDesc) focusDesc.textContent = `将执行中的英文状态摘要翻译为${langLabel}（含灵动岛）`;
        if (footer) footer.textContent = "标题翻译仅覆盖开启后的新会话，不改动已有会话；步骤与状态仅在客户端与灵动岛展示译文，不修改桌面端原始记录。";
      } else if (dev.kind === "ja") {
        if (hdr) hdr.textContent = "スマート翻訳";
        if (titleLbl) titleLbl.textContent = "会話タイトルの翻訳";
        if (titleDesc) titleDesc.textContent = "新しい会話の英語タイトルを日本語に自動翻訳して上書き";
        if (stepLbl) stepLbl.textContent = "ステップ一覧を自動翻訳";
        if (stepDesc) stepDesc.textContent = "英語のタスクリストを日本語に翻訳（Dynamic Island 含む）";
        if (focusLbl) focusLbl.textContent = "リアルタイム状況を自動翻訳";
        if (focusDesc) focusDesc.textContent = "実行中の英語ステータスを日本語に翻訳（Dynamic Island 含む）";
        if (footer) footer.textContent = "タイトル翻訳はオン後の新しい会話のみ上書き同期されます（既存会話は変更しません）。ステップ・状況は表示のみ翻訳され、元の記録は保持されます。";
      } else {
        const targetName = dev.englishName;
        if (hdr) hdr.textContent = "Smart Translation";
        if (titleLbl) titleLbl.textContent = "Conversation Title Translation";
        if (titleDesc) titleDesc.textContent = `Auto-translate new English titles into ${targetName}`;
        if (stepLbl) stepLbl.textContent = "Translate Step Checklist";
        if (stepDesc) stepDesc.textContent = `Translate English checklist steps into ${targetName} (incl. Live Activity)`;
        if (focusLbl) focusLbl.textContent = "Translate Live Status Line";
        if (focusDesc) focusDesc.textContent = `Translate live working status into ${targetName} (incl. Live Activity)`;
        if (footer) footer.textContent = "Title translation overwrites new conversations going forward without modifying existing ones. Step and status translations are display-only.";
      }
    }

    const chkTitle = document.getElementById("settings-ai-title-translate");
    if (chkTitle) {
      chkTitle.checked = Boolean(p.titleTranslateEnabled);
      chkTitle.disabled = !aiAssistState.available;
    }
    const chkStepPlan = document.getElementById("settings-ai-step-plan-translate");
    if (chkStepPlan) {
      chkStepPlan.checked = Boolean(p.stepPlanTranslateEnabled);
      chkStepPlan.disabled = !aiAssistState.available;
    }
    const chkFocus = document.getElementById("settings-ai-focus-translate");
    if (chkFocus) {
      chkFocus.checked = Boolean(p.focusTranslateEnabled);
      chkFocus.disabled = !aiAssistState.available;
    }
    const chkNext = document.getElementById("settings-ai-next-prompt");
    if (chkNext) {
      chkNext.checked = Boolean(p.nextPromptEnabled);
      chkNext.disabled = !aiAssistState.available;
    }

    const chkRehydrate = document.getElementById("settings-ai-rehydrate");
    if (chkRehydrate) {
      chkRehydrate.checked = Boolean(p.postCompactRehydrateEnabled);
    }
    const rehydrateDesc = document.getElementById("settings-ai-rehydrate-desc");
    if (rehydrateDesc) {
      if (p.postCompactRehydrateEnabled && aiAssistState.rehydrateHookInstalled) {
        rehydrateDesc.textContent = uiLang === "en"
          ? "PreInvocation hook installed in hooks.json"
          : (uiLang === "ja" ? "hooks.json に登録済み" : "已在本机 hooks.json 注册生效");
      } else {
        rehydrateDesc.textContent = uiLang === "en"
          ? "Preserve recent intent & edited files after compaction"
          : (uiLang === "ja" ? "圧縮後の文脈欠落を防ぐネイティブフック" : "压缩后保留近期上下文与编辑文件要点");
      }
    }

    const modelEl = document.getElementById("settings-ai-model-name");
    if (modelEl) modelEl.textContent = aiAssistState.modelName || "Gemini 3.5 Flash Lite";

    const engFooter = document.getElementById("settings-ai-engine-footer");
    if (engFooter) {
      if (syncErrorText) {
        engFooter.textContent = syncErrorText;
      } else if (!aiAssistState.available) {
        engFooter.textContent = uiLang === "en"
          ? "This Antigravity version does not support native GetModelResponse."
          : (uiLang === "ja" ? "現在の Antigravity バージョンでは GetModelResponse に対応していません。" : "当前 Antigravity 版本不支持原生 GetModelResponse 接口。");
      } else {
        engFooter.textContent = uiLang === "en"
          ? "Runs statelessly via local Antigravity native RPC without creating extra sessions. Syncs across devices."
          : (uiLang === "ja" ? "ローカルの Antigravity ネイティブ API でステートレス実行され、追加会話は作成されません。設定は全端末に同期されます。" : "通过本机 Antigravity 原生接口无状态执行，不产生额外会话，微量共享当前账号 Gemini 额度，开关跨设备同步至网关。");
      }
    }
  }

  window.AIAssist = {
    getTargetLangTag() {
      return resolveWebTitleTargetLanguage().bcp47;
    },
    async refresh() {
      try {
        const resp = await fetch("/gateway/ai-assist/prefs");
        if (resp && resp.ok) {
          const data = await resp.json();
          if (data && data.prefs) {
            aiAssistState.prefs = { ...aiAssistState.prefs, ...data.prefs };
            if (data.modelName) aiAssistState.modelName = data.modelName;
            aiAssistState.resolvedLangName = data.resolvedLangName || "";
            aiAssistState.available = data.available !== false;
            aiAssistState.rehydrateHookInstalled = Boolean(data.rehydrateHookInstalled);
          }
        }
      } catch (_) {}
      syncAIAssistUI();
    },
    async patch(patchObj) {
      const prev = { ...aiAssistState.prefs };
      aiAssistState.prefs = { ...aiAssistState.prefs, ...patchObj };
      syncAIAssistUI();
      try {
        const resp = await fetch("/gateway/ai-assist/prefs", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(patchObj),
        });
        if (!resp || !resp.ok) {
          aiAssistState.prefs = prev;
          syncAIAssistUI(typeof t === "function" ? t("同步失败，请稍后重试") : "同步失败，请稍后重试");
          return;
        }
        const data = await resp.json();
        if (data && data.prefs) {
          aiAssistState.prefs = { ...aiAssistState.prefs, ...data.prefs };
          if (data.modelName) aiAssistState.modelName = data.modelName;
          aiAssistState.resolvedLangName = data.resolvedLangName || "";
          aiAssistState.available = data.available !== false;
          aiAssistState.rehydrateHookInstalled = Boolean(data.rehydrateHookInstalled);
        }
        syncAIAssistUI();
      } catch (_) {
        aiAssistState.prefs = prev;
        syncAIAssistUI(typeof t === "function" ? t("同步失败，请检查网关连接") : "同步失败，请检查网关连接");
      }
    },
  };

  document.getElementById("settings-ai-title-translate")?.addEventListener("change", (e) => {
    const enabled = Boolean(e.target.checked);
    const dev = resolveWebTitleTargetLanguage();
    const patch = { titleTranslateEnabled: enabled };
    if (enabled && !dev.isEnglish) {
      patch.titleTranslateLang = dev.bcp47;
      patch.titleTranslateLangName = dev.englishName;
    }
    window.AIAssist.patch(patch);
  });
  document.getElementById("settings-ai-step-plan-translate")?.addEventListener("change", (e) => {
    const enabled = Boolean(e.target.checked);
    const dev = resolveWebTitleTargetLanguage();
    const patch = { stepPlanTranslateEnabled: enabled };
    if (enabled && !dev.isEnglish) {
      patch.titleTranslateLang = dev.bcp47;
      patch.titleTranslateLangName = dev.englishName;
    }
    window.AIAssist.patch(patch);
  });
  document.getElementById("settings-ai-focus-translate")?.addEventListener("change", (e) => {
    const enabled = Boolean(e.target.checked);
    const dev = resolveWebTitleTargetLanguage();
    const patch = { focusTranslateEnabled: enabled };
    if (enabled && !dev.isEnglish) {
      patch.titleTranslateLang = dev.bcp47;
      patch.titleTranslateLangName = dev.englishName;
    }
    window.AIAssist.patch(patch);
  });
  document.getElementById("settings-ai-next-prompt")?.addEventListener("change", (e) => {
    window.AIAssist.patch({ nextPromptEnabled: Boolean(e.target.checked) });
  });
  document.getElementById("settings-ai-rehydrate")?.addEventListener("change", (e) => {
    window.AIAssist.patch({ postCompactRehydrateEnabled: Boolean(e.target.checked) });
  });
  window.addEventListener("mgy:languagechange", () => syncAIAssistUI());
  syncAIAssistUI();
  window.AIAssist.refresh();

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
