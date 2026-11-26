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

  // --- AI Assist (Title Auto/Batch Translation & Next-Prompt Suggestions) ---
  let aiAssistState = {
    prefs: {
      titleTranslateEnabled: false,
      titleTranslateLang: "",
      titleTranslateLangName: "",
      stepPlanTranslateEnabled: false,
      focusTranslateEnabled: false,
      nextPromptEnabled: false,
      nextPromptTapAction: "fill",
    },
    modelName: "Gemini 3.5 Flash Lite",
    resolvedLangName: "",
    translatedCount: 0,
    available: true,
    loadedAt: 0,
    busyBatch: false,
    busyRestore: false,
    feedback: "",
    feedbackIsError: false,
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
    const showTitleSection = !dev.isEnglish || Boolean(p.titleTranslateEnabled);
    const showLiveSection = !dev.isEnglish || Boolean(p.stepPlanTranslateEnabled) || Boolean(p.focusTranslateEnabled);
    const activeTargetName = !dev.isEnglish
      ? dev.englishName
      : (p.titleTranslateLangName || aiAssistState.resolvedLangName || "Local Language");

    const uiLang = window.I18n?.getLang?.() || "zh";
    const summaryEl = document.getElementById("settings-ai-assist-summary");
    if (summaryEl) {
      const titleOn = showTitleSection && Boolean(p.titleTranslateEnabled);
      const stepOn = showLiveSection && Boolean(p.stepPlanTranslateEnabled);
      const focusOn = showLiveSection && Boolean(p.focusTranslateEnabled);
      const nextOn = Boolean(p.nextPromptEnabled);
      const count = [titleOn, stepOn, focusOn, nextOn].filter(Boolean).length;
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
      }
      summaryEl.textContent = label;
    }

    const titleSection = document.getElementById("ai-assist-title-section");
    if (titleSection) {
      titleSection.classList.toggle("hidden", !showTitleSection);
    }
    if (showTitleSection) {
      const hdr = document.getElementById("ai-assist-title-header");
      const lbl = document.getElementById("settings-ai-title-label");
      const desc = document.getElementById("settings-ai-title-desc");
      const btnBatch = document.getElementById("btn-ai-translate-existing");
      const btnBatchLbl = document.getElementById("btn-ai-translate-existing-label");
      const btnRestore = document.getElementById("btn-ai-restore-titles");
      const btnRestoreLbl = document.getElementById("btn-ai-restore-titles-label");
      const footer = document.getElementById("settings-ai-title-footer");
      const fb = document.getElementById("settings-ai-title-feedback");

      if (dev.kind === "zh-Hans" || dev.kind === "zh-Hant") {
        if (hdr) hdr.textContent = "会话标题翻译";
        if (lbl) lbl.textContent = dev.kind === "zh-Hant" ? "自动翻译为繁体中文" : "自动翻译为简体中文";
        if (desc) desc.textContent = "Google 生成英文标题后自动翻译并同步到桌面与手机";
        if (btnBatchLbl) btnBatchLbl.textContent = aiAssistState.busyBatch ? "正在翻译历史英文标题…" : "立即翻译历史英文标题";
        if (btnRestoreLbl) btnRestoreLbl.textContent = aiAssistState.busyRestore ? "正在还原原始标题…" : `还原已翻译标题（${aiAssistState.translatedCount}）`;
        if (footer) footer.textContent = "仅翻译未归档活跃会话，不会覆盖手动重命名的标题；翻译结果直接写回 Antigravity，桌面端与手机端同步生效。";
      } else if (dev.kind === "ja") {
        if (hdr) hdr.textContent = "会話タイトルの翻訳";
        if (lbl) lbl.textContent = "英語タイトルを日本語に自動翻訳";
        if (desc) desc.textContent = "Google 生成の英語タイトルを自動翻訳しデスクトップと同期";
        if (btnBatchLbl) btnBatchLbl.textContent = aiAssistState.busyBatch ? "英語タイトルを翻訳中…" : "過去の英語タイトルを一括翻訳";
        if (btnRestoreLbl) btnRestoreLbl.textContent = aiAssistState.busyRestore ? "元のタイトルを復元中…" : `翻訳済みタイトルを元に戻す（${aiAssistState.translatedCount}）`;
        if (footer) footer.textContent = "アーカイブ済みを除く会話のみ対象。手動変更したタイトルは上書きされず、デスクトップと同期されます。";
      } else {
        if (hdr) hdr.textContent = "Conversation Title Translation";
        if (lbl) lbl.textContent = `Auto-translate to ${activeTargetName}`;
        if (desc) desc.textContent = `Translate English titles into ${activeTargetName} and sync across desktop & mobile`;
        if (btnBatchLbl) btnBatchLbl.textContent = aiAssistState.busyBatch ? `Translating titles to ${activeTargetName}…` : `Translate Existing Titles to ${activeTargetName}`;
        if (btnRestoreLbl) btnRestoreLbl.textContent = aiAssistState.busyRestore ? "Restoring original titles…" : `Restore Original Titles (${aiAssistState.translatedCount})`;
        if (footer) footer.textContent = "Applies to active non-archived conversations only. Manually renamed titles are preserved.";
      }

      if (btnBatch) {
        btnBatch.classList.toggle("hidden", dev.isEnglish);
        btnBatch.disabled = !aiAssistState.available || aiAssistState.busyBatch;
      }
      if (btnRestore) {
        btnRestore.classList.toggle("hidden", aiAssistState.translatedCount <= 0);
        btnRestore.disabled = aiAssistState.busyRestore || aiAssistState.busyBatch;
      }
      if (fb) {
        if (aiAssistState.feedback) {
          fb.textContent = aiAssistState.feedback;
          fb.style.color = aiAssistState.feedbackIsError ? "#ff453a" : "";
          fb.classList.remove("hidden");
        } else {
          fb.classList.add("hidden");
        }
      }
    }

    const liveSection = document.getElementById("ai-assist-live-section");
    if (liveSection) {
      liveSection.classList.toggle("hidden", !showLiveSection);
    }
    if (showLiveSection) {
      const hdr = document.getElementById("ai-assist-live-header");
      const stepLbl = document.getElementById("settings-ai-step-plan-label");
      const stepDesc = document.getElementById("settings-ai-step-plan-desc");
      const focusLbl = document.getElementById("settings-ai-focus-label");
      const focusDesc = document.getElementById("settings-ai-focus-desc");
      const footer = document.getElementById("settings-ai-live-footer");
      if (dev.kind === "zh-Hans" || dev.kind === "zh-Hant") {
        const langLabel = dev.kind === "zh-Hant" ? "繁体中文" : "简体中文";
        if (hdr) hdr.textContent = "进度与状态翻译";
        if (stepLbl) stepLbl.textContent = "步骤清单实时翻译";
        if (stepDesc) stepDesc.textContent = `将英文待办步骤翻译为${langLabel}（含灵动岛）`;
        if (focusLbl) focusLbl.textContent = "实时状态行翻译";
        if (focusDesc) focusDesc.textContent = `将执行中的英文状态摘要翻译为${langLabel}（含灵动岛）`;
        if (footer) footer.textContent = "仅在客户端与灵动岛展示译文，不修改桌面端原始记录；相同条目本地缓存复用，随时可独立开关。";
      } else if (dev.kind === "ja") {
        if (hdr) hdr.textContent = "進捗・ステータスの翻訳";
        if (stepLbl) stepLbl.textContent = "ステップ一覧を自動翻訳";
        if (stepDesc) stepDesc.textContent = "英語のタスクリストを日本語に翻訳（Dynamic Island 含む）";
        if (focusLbl) focusLbl.textContent = "リアルタイム状況を自動翻訳";
        if (focusDesc) focusDesc.textContent = "実行中の英語ステータスを日本語に翻訳（Dynamic Island 含む）";
        if (footer) footer.textContent = "クライアントと Dynamic Island の表示のみ翻訳され、デスクトップの元記録は変更されません。同じ項目はキャッシュで再利用されます。";
      } else {
        if (hdr) hdr.textContent = "Progress & Status Translation";
        if (stepLbl) stepLbl.textContent = "Translate Step Checklist";
        if (stepDesc) stepDesc.textContent = `Translate English checklist steps into ${activeTargetName} (incl. Live Activity)`;
        if (focusLbl) focusLbl.textContent = "Translate Live Status Line";
        if (focusDesc) focusDesc.textContent = `Translate live working status into ${activeTargetName} (incl. Live Activity)`;
        if (footer) footer.textContent = "Translates display on mobile, Web, and Live Activity only without modifying desktop history. Cached locally for reuse.";
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
    const tapRow = document.getElementById("settings-ai-next-prompt-tap-row");
    const tapSel = document.getElementById("settings-ai-next-prompt-tap");
    if (tapRow) tapRow.classList.toggle("hidden", !p.nextPromptEnabled);
    if (tapSel) tapSel.value = p.nextPromptTapAction === "send" ? "send" : "fill";

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
    getTapAction() {
      return aiAssistState.prefs?.nextPromptTapAction === "send" ? "send" : "fill";
    },
    async ensureLoaded() {
      if (aiAssistState.loadedAt > 0 && Date.now() - aiAssistState.loadedAt < 300000) return;
      await this.refresh();
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
            aiAssistState.translatedCount = Number(data.translatedCount || 0);
            aiAssistState.available = data.available !== false;
            aiAssistState.loadedAt = Date.now();
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
          aiAssistState.translatedCount = Number(data.translatedCount || 0);
          aiAssistState.available = data.available !== false;
          aiAssistState.loadedAt = Date.now();
        }
        syncAIAssistUI();
      } catch (_) {
        aiAssistState.prefs = prev;
        syncAIAssistUI(typeof t === "function" ? t("同步失败，请检查网关连接") : "同步失败，请检查网关连接");
      }
    },
    async translateExisting() {
      if (aiAssistState.busyBatch) return;
      const dev = resolveWebTitleTargetLanguage();
      if (dev.isEnglish) return;
      aiAssistState.busyBatch = true;
      aiAssistState.feedback = "";
      aiAssistState.feedbackIsError = false;
      syncAIAssistUI();
      try {
        const resp = await fetch("/gateway/ai-assist/translate-titles", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            targetLang: dev.bcp47,
            targetLangName: dev.englishName,
            includeArchived: false,
          }),
        });
        const data = resp && resp.ok ? await resp.json() : null;
        if (data) {
          if (typeof data.translatedCount === "number") aiAssistState.translatedCount = data.translatedCount;
          const tr = Number(data.translated || 0);
          const sk = Number(data.skipped || 0);
          const fl = Number(data.failed || 0);
          aiAssistState.feedbackIsError = tr === 0 && fl > 0;
          if (data.alreadyRunning) {
            aiAssistState.feedback = dev.kind === "ja" ? "翻訳処理が実行中です" : (dev.kind.startsWith("zh") ? "后台正在翻译中，请稍候" : "Translation is already running in the background");
          } else if (tr === 0 && fl === 0) {
            aiAssistState.feedback = dev.kind === "ja" ? `翻訳対象の英語タイトルはありませんでした（${sk} 件スキップ）` : (dev.kind.startsWith("zh") ? `所有活跃会话标题已是目标语言（跳过 ${sk} 条）` : `All active titles are already up to date (${sk} skipped)`);
          } else {
            aiAssistState.feedback = dev.kind === "ja"
              ? (fl > 0 ? `${tr} 件翻訳、${sk} 件スキップ、${fl} 件失敗` : `${tr} 件のタイトルを翻訳しました（${sk} 件スキップ）`)
              : (dev.kind.startsWith("zh")
                ? (fl > 0 ? `已翻译 ${tr} 条，跳过 ${sk} 条，失败 ${fl} 条` : `已翻译 ${tr} 条标题，跳过 ${sk} 条`)
                : (fl > 0 ? `Translated ${tr}, skipped ${sk}, failed ${fl}` : `Translated ${tr} titles (${sk} skipped)`));
          }
          if (tr > 0 && typeof loadConversations === "function") loadConversations();
        } else {
          aiAssistState.feedbackIsError = true;
          aiAssistState.feedback = typeof t === "function" ? t("翻译失败，请确认网关已连接 Antigravity") : "翻译失败，请确认网关已连接 Antigravity";
        }
      } catch (_) {
        aiAssistState.feedbackIsError = true;
        aiAssistState.feedback = typeof t === "function" ? t("翻译请求超时或连接中断") : "翻译请求超时或连接中断";
      } finally {
        aiAssistState.busyBatch = false;
        syncAIAssistUI();
      }
    },
    async restoreOriginal() {
      if (aiAssistState.busyRestore || aiAssistState.busyBatch) return;
      const dev = resolveWebTitleTargetLanguage();
      const msg = dev.kind === "ja"
        ? "自動・一括翻訳された会話タイトルを元の英語タイトルに戻します（手動変更したタイトルは維持されます）。よろしいですか？"
        : (dev.kind.startsWith("zh")
          ? "将把此前自动或批量翻译的会话标题恢复为原始标题（手动重命名的会话不受影响），确认还原？"
          : "Restore titles previously translated by AI Assist back to their original text? (Manually renamed titles are preserved.)");
      if (!window.confirm(msg)) return;
      aiAssistState.busyRestore = true;
      aiAssistState.feedback = "";
      aiAssistState.feedbackIsError = false;
      syncAIAssistUI();
      try {
        const resp = await fetch("/gateway/ai-assist/restore-titles", { method: "POST" });
        const data = resp && resp.ok ? await resp.json() : null;
        if (data) {
          const restored = Number(data.restored || 0);
          if (typeof data.translatedCount === "number") aiAssistState.translatedCount = data.translatedCount;
          aiAssistState.feedback = dev.kind === "ja"
            ? `${restored} 件の元のタイトルを復元しました`
            : (dev.kind.startsWith("zh") ? `已还原 ${restored} 条原始标题` : `Restored ${restored} original titles`);
          if (restored > 0 && typeof loadConversations === "function") loadConversations();
        } else {
          aiAssistState.feedbackIsError = true;
          aiAssistState.feedback = typeof t === "function" ? t("还原失败，请稍后重试") : "还原失败，请稍后重试";
        }
      } catch (_) {
        aiAssistState.feedbackIsError = true;
        aiAssistState.feedback = typeof t === "function" ? t("还原失败，请检查网关连接") : "还原失败，请检查网关连接";
      } finally {
        aiAssistState.busyRestore = false;
        syncAIAssistUI();
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
  document.getElementById("btn-ai-translate-existing")?.addEventListener("click", () => {
    window.AIAssist.translateExisting();
  });
  document.getElementById("btn-ai-restore-titles")?.addEventListener("click", () => {
    window.AIAssist.restoreOriginal();
  });
  document.getElementById("settings-ai-next-prompt")?.addEventListener("change", (e) => {
    window.AIAssist.patch({ nextPromptEnabled: Boolean(e.target.checked) });
  });
  document.getElementById("settings-ai-next-prompt-tap")?.addEventListener("change", (e) => {
    window.AIAssist.patch({ nextPromptTapAction: e.target.value === "send" ? "send" : "fill" });
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

  // Model Switch Chip (click to toggle favorites; long-press + slide up/horizontal or right-click for inline effort picker)
  const modelSwitchBtn = document.getElementById("btn-model-switch");
  if (modelSwitchBtn) {
    let modelLongPressTimer = null;
    let suppressNextClick = false;
    let activePointerId = null;
    let isDraggingPicker = false;

    const cleanupDragListeners = () => {
      window.removeEventListener("pointermove", onWindowPointerMove);
      window.removeEventListener("pointerup", onWindowPointerUp);
      window.removeEventListener("pointercancel", onWindowPointerCancel);
      activePointerId = null;
      isDraggingPicker = false;
    };

    const onWindowPointerMove = (ev) => {
      if (activePointerId !== null && ev.pointerId !== activePointerId) return;
      if (isDraggingPicker && typeof updateWebModelPickerDrag === "function") {
        ev.preventDefault();
        updateWebModelPickerDrag(ev.clientX, ev.clientY);
      }
    };

    const onWindowPointerUp = (ev) => {
      if (activePointerId !== null && ev.pointerId !== activePointerId) return;
      clearTimeout(modelLongPressTimer);
      if (isDraggingPicker && typeof endWebModelPickerDrag === "function") {
        endWebModelPickerDrag();
      }
      cleanupDragListeners();
    };

    const onWindowPointerCancel = () => {
      clearTimeout(modelLongPressTimer);
      if (isDraggingPicker && typeof dismissWebModelPicker === "function") {
        dismissWebModelPicker();
      }
      cleanupDragListeners();
    };

    modelSwitchBtn.addEventListener("pointerdown", (e) => {
      if (e.button === 2) return;
      suppressNextClick = false;
      activePointerId = e.pointerId;
      isDraggingPicker = false;
      clearTimeout(modelLongPressTimer);
      window.addEventListener("pointermove", onWindowPointerMove, { passive: false });
      window.addEventListener("pointerup", onWindowPointerUp);
      window.addEventListener("pointercancel", onWindowPointerCancel);
      const startX = e.clientX;
      const startY = e.clientY;
      modelLongPressTimer = setTimeout(() => {
        suppressNextClick = true;
        isDraggingPicker = true;
        triggerHaptic("medium");
        if (typeof openWebModelPicker === "function") {
          openWebModelPicker(modelSwitchBtn, { dragMode: true, pointerX: startX, pointerY: startY });
        }
      }, 210);
    });

    modelSwitchBtn.addEventListener("contextmenu", (e) => {
      e.preventDefault();
      clearTimeout(modelLongPressTimer);
      cleanupDragListeners();
      if (typeof openWebModelPicker === "function") {
        openWebModelPicker(modelSwitchBtn, { dragMode: false });
      }
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
