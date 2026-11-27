package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/i18n"
	"antigravity-mobile/internal/updater"
)

// GatewaySelfUpdateStatus describes the version status and live self-update job state of mgy itself.
type GatewaySelfUpdateStatus struct {
	OK                 bool   `json:"ok"`
	CurrentVersion     string `json:"currentVersion"`
	LatestVersion      string `json:"latestVersion,omitempty"`
	Tag                string `json:"tag,omitempty"`
	UpdateAvailable    bool   `json:"updateAvailable"`
	CanSelfUpdate      bool   `json:"canSelfUpdate"`
	CannotUpdateReason string `json:"cannotUpdateReason,omitempty"`
	RestartMode        string `json:"restartMode,omitempty"` // "exec" | "service" | "spawn" | "manual"
	OS                 string `json:"os"`
	Arch               string `json:"arch"`
	RunningCount       int    `json:"runningCount"`
	State              string `json:"state"` // "idle" | "checking" | "downloading" | "verifying" | "replacing" | "restarting" | "completed" | "failed"
	StageMessage       string `json:"stageMessage,omitempty"`
	TargetVersion      string `json:"targetVersion,omitempty"`
	Error              string `json:"error,omitempty"`
	CheckError         string `json:"checkError,omitempty"`
	ReleaseURL         string `json:"releaseUrl,omitempty"`
}

type gatewaySelfUpdateState struct {
	mu            sync.Mutex
	state         string
	stageKind     string // structured stage key ("checking" | "up_to_date" | "restarting" | "completed_manual" | "")
	stageMessage  string
	targetVersion string
	latestTag     string
	lastErr       string
	updatedAt     time.Time
}

func formatSelfUpdateStageMessage(lang, stageKind, rawMessage, targetVer string) string {
	switch stageKind {
	case "checking":
		return i18n.T3For(lang,
			"正在检查 mgy 最新版本...",
			"Checking for latest mgy version...",
			"mgy の最新バージョンを確認中...",
		)
	case "up_to_date":
		return i18n.T3For(lang,
			"当前已是最新版本",
			"Already up to date",
			"すでに最新バージョンです",
		)
	case "restarting":
		return i18n.Sprintf3For(lang,
			"已安装 v%s，正在重启网关...",
			"Installed v%s, restarting gateway...",
			"v%s をインストールしました。ゲートウェイを再起動中...",
			targetVer,
		)
	case "completed_manual":
		return i18n.Sprintf3For(lang,
			"已安装 v%s，请重启网关使新版本生效",
			"Installed v%s; please restart the gateway to apply",
			"v%s をインストールしました。ゲートウェイを再起動してください",
			targetVer,
		)
	default:
		return rawMessage
	}
}

func isSelfUpdateActiveState(state string) bool {
	switch state {
	case "checking", "downloading", "verifying", "replacing", "restarting":
		return true
	default:
		return false
	}
}

// SetGatewayVersion sets the running mgy binary version string (without or with leading "v").
func (p *Proxy) SetGatewayVersion(version string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gatewayVersion = strings.TrimSpace(version)
}

// GatewayVersion returns the normalized running mgy binary version string.
func (p *Proxy) GatewayVersion() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return updater.NormalizeVersion(p.gatewayVersion)
}

// SetSelfRestartFunc registers the callback that triggers a graceful gateway self-restart
// after a new binary has been verified and written to disk.
func (p *Proxy) SetSelfRestartFunc(fn func(binaryPath string), mode string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.selfRestartFn = fn
	p.selfRestartMode = strings.TrimSpace(mode)
}

func (p *Proxy) inspectSelfUpdateCapability(curVer, lang string) updater.SelfUpdateCapability {
	if p.selfUpdateCapFn != nil {
		return p.selfUpdateCapFn(curVer)
	}
	return updater.InspectSelfUpdateCapabilityForLang("", curVer, lang)
}

func (p *Proxy) checkLatestSelfTag(ctx context.Context, curVer string, force bool) (string, error) {
	if p.selfUpdateCheckFn != nil {
		return p.selfUpdateCheckFn(ctx, curVer, force)
	}
	tag, _, err := updater.CheckLatestCached(ctx, curVer, force, updater.Options{
		CurrentVersion: curVer,
	})
	return tag, err
}

func (p *Proxy) runSelfUpdate(ctx context.Context, opts updater.Options) (*updater.Result, error) {
	if p.selfUpdateRunFn != nil {
		return p.selfUpdateRunFn(ctx, opts)
	}
	return updater.Run(ctx, opts)
}

func (p *Proxy) selfRestartConfig() (func(string), string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	mode := p.selfRestartMode
	if mode == "" {
		if p.selfRestartFn != nil {
			mode = "exec"
		} else {
			mode = "manual"
		}
	}
	return p.selfRestartFn, mode
}

func (p *Proxy) safeRunningCount(ctx context.Context) int {
	if p.insp == nil {
		return 0
	}
	port, _ := p.ActiveUpstream()
	if port <= 0 {
		return 0
	}
	sctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if summary := p.computeGatewaySummary(sctx, false); summary != nil {
		return summary.RunningCount
	}
	return 0
}

func buildReleaseURL(tag string) string {
	t := strings.TrimSpace(tag)
	if t == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/releases/tag/%s", updater.DefaultGitHubBaseURL, updater.RepoFromEnv(), t)
}

// HandleGatewaySelfUpdate serves GET /gateway/self/update and POST /gateway/self/update/apply.
func (p *Proxy) HandleGatewaySelfUpdate(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/gateway/self/update":
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p.handleGatewaySelfUpdateStatus(w, r)

	case "/gateway/self/update/apply":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p.handleGatewaySelfUpdateApply(w, r)

	default:
		http.NotFound(w, r)
	}
}

func (p *Proxy) handleGatewaySelfUpdateStatus(w http.ResponseWriter, r *http.Request) {
	uiLang := resolveRequestUILang(r)
	q := r.URL.Query()
	forceRefresh := q.Get("refresh") == "1" || strings.EqualFold(q.Get("refresh"), "true") ||
		q.Get("force") == "1" || strings.EqualFold(q.Get("force"), "true")

	curVer := p.GatewayVersion()
	capInfo := p.inspectSelfUpdateCapability(curVer, uiLang)
	_, restartMode := p.selfRestartConfig()

	p.selfUpdate.mu.Lock()
	curState := p.selfUpdate.state
	if curState == "" {
		curState = "idle"
	}
	if forceRefresh && !isSelfUpdateActiveState(curState) {
		p.selfUpdate.state = "idle"
		p.selfUpdate.stageKind = ""
		p.selfUpdate.stageMessage = ""
		p.selfUpdate.lastErr = ""
		curState = "idle"
	}
	active := isSelfUpdateActiveState(curState)
	stageKind := p.selfUpdate.stageKind
	rawStageMsg := p.selfUpdate.stageMessage
	targetVer := p.selfUpdate.targetVersion
	jobLatestTag := p.selfUpdate.latestTag
	jobErr := p.selfUpdate.lastErr
	p.selfUpdate.mu.Unlock()
	stageMsg := formatSelfUpdateStageMessage(uiLang, stageKind, rawStageMsg, targetVer)

	var latestTag string
	var checkErrMsg string
	if active {
		latestTag = jobLatestTag
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		tag, err := p.checkLatestSelfTag(ctx, curVer, forceRefresh)
		latestTag = strings.TrimSpace(tag)
		if err != nil {
			checkErrMsg = err.Error()
		}
		if latestTag == "" && jobLatestTag != "" {
			latestTag = jobLatestTag
		}
	}

	latestVer := updater.NormalizeVersion(latestTag)
	if latestTag == "" && targetVer != "" {
		latestVer = updater.NormalizeVersion(targetVer)
		if t, err := updater.EnsureTag(latestVer); err == nil {
			latestTag = t
		}
	}

	updateAvailable := false
	if latestVer != "" && curVer != "" {
		updateAvailable = updater.IsNewer(latestVer, curVer)
	}

	resp := GatewaySelfUpdateStatus{
		OK:                 true,
		CurrentVersion:     curVer,
		LatestVersion:      latestVer,
		Tag:                latestTag,
		UpdateAvailable:    updateAvailable,
		CanSelfUpdate:      capInfo.CanUpdate,
		CannotUpdateReason: capInfo.Reason,
		RestartMode:        restartMode,
		OS:                 runtime.GOOS,
		Arch:               runtime.GOARCH,
		RunningCount:       p.safeRunningCount(r.Context()),
		State:              curState,
		StageMessage:       stageMsg,
		TargetVersion:      targetVer,
		Error:              jobErr,
		CheckError:         checkErrMsg,
		ReleaseURL:         buildReleaseURL(latestTag),
	}
	writeJSONStatus(w, http.StatusOK, resp)
}

func (p *Proxy) handleGatewaySelfUpdateApply(w http.ResponseWriter, r *http.Request) {
	uiLang := resolveRequestUILang(r)
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
		return
	}
	if !req.Confirm {
		writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "请确认后再执行 mgy 网关更新", "Please confirm before updating the mgy gateway", "mgy ゲートウェイを更新する前に確認してください"))
		return
	}

	curVer := p.GatewayVersion()
	capInfo := p.inspectSelfUpdateCapability(curVer, uiLang)
	if !capInfo.CanUpdate {
		reason := capInfo.Reason
		if reason == "" {
			reason = i18n.T3For(uiLang,
				"当前环境不支持远程自动更新，请在电脑端执行 mgy update",
				"Remote auto-update is not supported in this environment. Run `mgy update` on your computer.",
				"この環境ではリモート自動更新に対応していません。PCで `mgy update` を実行してください。",
			)
		}
		writeRPCError(w, http.StatusBadRequest, reason)
		return
	}

	p.selfUpdate.mu.Lock()
	if isSelfUpdateActiveState(p.selfUpdate.state) {
		curState := p.selfUpdate.state
		stageMsg := formatSelfUpdateStageMessage(uiLang, p.selfUpdate.stageKind, p.selfUpdate.stageMessage, p.selfUpdate.targetVersion)
		p.selfUpdate.mu.Unlock()
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":       true,
			"accepted": true,
			"state":    curState,
			"message": firstNonEmpty(stageMsg, i18n.T3For(uiLang,
				"mgy 网关更新正在进行中",
				"mgy gateway update is already in progress",
				"mgy ゲートウェイの更新が進行中です",
			)),
		})
		return
	}
	p.selfUpdate.state = "checking"
	p.selfUpdate.stageKind = "checking"
	p.selfUpdate.stageMessage = formatSelfUpdateStageMessage(uiLang, "checking", "", "")
	p.selfUpdate.targetVersion = ""
	p.selfUpdate.lastErr = ""
	p.selfUpdate.updatedAt = time.Now()
	p.selfUpdate.mu.Unlock()

	restartFn, restartMode := p.selfRestartConfig()
	restartDelay := p.selfRestartDelay
	if restartDelay <= 0 {
		restartDelay = 600 * time.Millisecond
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer cancel()

		setStage := func(stage, detail string) {
			p.selfUpdate.mu.Lock()
			p.selfUpdate.state = stage
			p.selfUpdate.stageKind = ""
			p.selfUpdate.stageMessage = detail
			p.selfUpdate.updatedAt = time.Now()
			p.selfUpdate.mu.Unlock()
		}

		res, err := p.runSelfUpdate(ctx, updater.Options{
			CurrentVersion: curVer,
			Out:            io.Discard,
			Lang:           uiLang,
			OnProgress: func(stage, detail string) {
				setStage(stage, detail)
			},
		})
		if err != nil {
			slog.Warn("[SelfUpdate] mgy 远程自动更新失败", "err", err)
			p.selfUpdate.mu.Lock()
			p.selfUpdate.state = "failed"
			p.selfUpdate.stageKind = ""
			p.selfUpdate.stageMessage = ""
			p.selfUpdate.lastErr = err.Error()
			p.selfUpdate.updatedAt = time.Now()
			p.selfUpdate.mu.Unlock()
			return
		}

		if res == nil || (!res.Updated && res.UpToDate) {
			latestTag := ""
			if res != nil {
				latestTag = res.Tag
			}
			p.selfUpdate.mu.Lock()
			p.selfUpdate.state = "completed"
			p.selfUpdate.stageKind = "up_to_date"
			p.selfUpdate.stageMessage = formatSelfUpdateStageMessage(uiLang, "up_to_date", "", "")
			if latestTag != "" {
				p.selfUpdate.latestTag = latestTag
			}
			p.selfUpdate.lastErr = ""
			p.selfUpdate.updatedAt = time.Now()
			p.selfUpdate.mu.Unlock()
			return
		}

		if res.Tag != "" {
			updater.SaveCachedLatestTag(res.Tag)
		}

		slog.Info("✅ [SelfUpdate] mgy 新版本已校验并替换就绪",
			"from", res.CurrentVersion,
			"to", res.LatestVersion,
			"target", res.TargetPath,
			"restart_mode", restartMode)

		if restartFn != nil {
			p.selfUpdate.mu.Lock()
			p.selfUpdate.state = "restarting"
			p.selfUpdate.stageKind = "restarting"
			p.selfUpdate.targetVersion = res.LatestVersion
			p.selfUpdate.latestTag = res.Tag
			p.selfUpdate.stageMessage = formatSelfUpdateStageMessage(uiLang, "restarting", "", res.LatestVersion)
			p.selfUpdate.lastErr = ""
			p.selfUpdate.updatedAt = time.Now()
			p.selfUpdate.mu.Unlock()

			time.Sleep(restartDelay)
			restartFn(res.TargetPath)
			return
		}

		p.selfUpdate.mu.Lock()
		p.selfUpdate.state = "completed"
		p.selfUpdate.stageKind = "completed_manual"
		p.selfUpdate.targetVersion = res.LatestVersion
		p.selfUpdate.latestTag = res.Tag
		p.selfUpdate.stageMessage = formatSelfUpdateStageMessage(uiLang, "completed_manual", "", res.LatestVersion)
		p.selfUpdate.lastErr = ""
		p.selfUpdate.updatedAt = time.Now()
		p.selfUpdate.mu.Unlock()
	}()

	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"ok":          true,
		"accepted":    true,
		"state":       "checking",
		"restartMode": restartMode,
		"message": i18n.T3For(uiLang,
			"已开始下载并更新 mgy 网关",
			"Started downloading and updating mgy gateway",
			"mgy ゲートウェイのダウンロードと更新を開始しました",
		),
	})
}
