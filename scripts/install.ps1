# Multigravity (mgy) Windows One-Line Installer (zh / en / ja)
# Usage:
#   irm https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.ps1 | iex
#   $env:MGY_LANG="ja"; irm https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.ps1 | iex
param(
    [string]$Lang = ""
)

$ErrorActionPreference = "Stop"

# 1. Force UTF-8 console & stream encoding to prevent garbled CJK/emoji output on PowerShell 5.1
try {
    [Console]::InputEncoding = [System.Text.Encoding]::UTF8
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    $OutputEncoding = [System.Text.Encoding]::UTF8
} catch {}

# 0. Native i18n (zh / en / ja)
function Normalize-MgyLang([string]$Raw) {
    if ([string]::IsNullOrWhiteSpace($Raw)) { return "" }
    $s = $Raw.Trim().ToLowerInvariant()
    if ($s -match '^(zh($|[-_])|cn$|chinese$)') { return "zh" }
    if ($s -match '^(ja($|[-_])|jp$|jpn$|japanese$)') { return "ja" }
    if ($s -match '^(en($|[-_])|english$)') { return "en" }
    return ""
}

function Get-UiLang([string]$ExplicitLang) {
    $n = Normalize-MgyLang $ExplicitLang
    if ($n) { return $n }

    foreach ($envVal in @($env:MGY_LANG, $env:MULTIGRAVITY_LANG)) {
        $n = Normalize-MgyLang $envVal
        if ($n) { return $n }
    }

    foreach ($locVal in @($env:LC_ALL, $env:LC_MESSAGES, $env:LANG, $env:LANGUAGE)) {
        if ([string]::IsNullOrWhiteSpace($locVal)) { continue }
        $first = ($locVal -split ':')[0].Trim().ToLowerInvariant()
        if ($first -in @("utf-8", "utf8")) { continue }
        if ($first -match '^(c|posix|c\.)') { return "en" }
        $n = Normalize-MgyLang $first
        if ($n) { return $n }
        return "en"
    }

    try {
        $uiCulture = [System.Globalization.CultureInfo]::CurrentUICulture.Name
        $n = Normalize-MgyLang $uiCulture
        if ($n) { return $n }
    } catch {}

    return "en"
}

$uiLang = Get-UiLang $Lang

function Get-Msg([string]$Zh, [string]$En, [string]$Ja) {
    switch ($uiLang) {
        "zh" { return $Zh }
        "ja" { return $Ja }
        default { return $En }
    }
}

$preferMirror = $false
if ($env:MGY_USE_MIRROR -match '^(?i)(1|true|yes)$') {
    $preferMirror = $true
} elseif ($env:MGY_USE_MIRROR -match '^(?i)(0|false|no)$') {
    $preferMirror = $false
} elseif ($uiLang -eq "zh") {
    $preferMirror = $true
}

$appStoreUrl = "https://apps.apple.com/app/id6835508934"

Write-Host "==================================================" -ForegroundColor Cyan
Write-Host (Get-Msg `
    "  📱 Multigravity (mgy) - Windows 一键安装与配置  " `
    "  📱 Multigravity (mgy) - Windows Installer       " `
    "  📱 Multigravity (mgy) - Windows インストーラー   ") -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Cyan

# 2. 架构检测 (Windows 优先使用 amd64 预编译包，ARM64 系统通过内置仿真无缝运行)
$arch = $env:PROCESSOR_ARCHITECTURE
$pkgArch = "amd64"
$archDesc = "Windows x86_64"
if ($arch -match "(?i)ARM64") {
    $archDesc = Get-Msg "Windows ARM64 (amd64 仿真)" "Windows ARM64 (amd64 emulation)" "Windows ARM64 (amd64 エミュレーション)"
}
Write-Host (Get-Msg `
    "🖥️  检测到系统架构: $archDesc ($pkgArch)" `
    "🖥️  Detected architecture: $archDesc ($pkgArch)" `
    "🖥️  システムアーキテクチャを検出: $archDesc ($pkgArch)") -ForegroundColor Cyan

# 3. 准备安装与配置目录 (收紧当前用户专属 ACL 权限)
$installDir = "$HOME\.local\bin"
$confDir = "$HOME\.multigravity"
if (!(Test-Path $installDir)) { New-Item -ItemType Directory -Path $installDir -Force | Out-Null }
if (!(Test-Path "$confDir\logs")) { New-Item -ItemType Directory -Path "$confDir\logs" -Force | Out-Null }
try {
    if ($env:USERNAME) {
        & icacls.exe $confDir /inheritance:r /grant:r "$($env:USERNAME):(OI)(CI)F" "SYSTEM:(OI)(CI)F" "Administrators:(OI)(CI)F" | Out-Null
    }
} catch {}

# 4. 初始化默认配置 .env (若不存在)
if (!(Test-Path "$confDir\.env")) {
    if ($uiLang -eq "zh") {
        $defaultEnv = @"
# Multigravity 全局环境变量配置文件
# 保存路径: ~/.multigravity/.env

# 网关监听端口 (默认 58900)
MULTIGRAVITY_PORT=58900

# 网关监听主机/IP (默认留空绑定所有 IPv4 网卡，设为 127.0.0.1 仅限本机)
# MULTIGRAVITY_HOST=127.0.0.1

# ☁️ Cloudflare Tunnel 专属公网穿透配置 (开箱即用)
# CF_WORKER_URL=https://dispatcher.multigravity.app
# CF_INVITE_CODE=
# CF_TUNNEL_TOKEN=
# CF_TUNNEL_ENABLED=1

# 🔔 iOS 原生 APNs 推送与灵动岛远程更新 (开箱即用，默认开启)
# APNS_ENABLED=1
# LIVE_ACTIVITY_PUSH_ENABLED=1
# APNS_RELAY_URL=https://push.multigravity.app

# 局域网免配对信任模式 (0: 需配对码[默认推荐], 1: 信任局域网免配对直连)
# 本机 127.0.0.1 始终免配对，Cloudflare 公网穿透始终强制验证
MULTIGRAVITY_TRUST_LAN=0
"@
    } elseif ($uiLang -eq "ja") {
        $defaultEnv = @"
# Multigravity グローバル環境設定ファイル
# 保存先: ~/.multigravity/.env

# ゲートウェイの待受ポート (デフォルト: 58900)
MULTIGRAVITY_PORT=58900

# 待受ホスト/IP (空欄で全 IPv4 インターフェースにバインド、127.0.0.1 でローカル限定)
# MULTIGRAVITY_HOST=127.0.0.1

# ☁️ Cloudflare Tunnel リモート公開設定 (そのまま利用可能)
# CF_WORKER_URL=https://dispatcher.multigravity.app
# CF_INVITE_CODE=
# CF_TUNNEL_TOKEN=
# CF_TUNNEL_ENABLED=1

# 🔔 iOS ネイティブ APNs プッシュ通知と Dynamic Island 更新 (デフォルト有効)
# APNS_ENABLED=1
# LIVE_ACTIVITY_PUSH_ENABLED=1
# APNS_RELAY_URL=https://push.multigravity.app

# LAN アクセスセキュリティ設定 (0: ペアリングコード必須 [推奨], 1: LAN を信頼してペアリング不要)
# ローカル 127.0.0.1 は常にペアリング不要、Cloudflare パブリックトンネルは常に認証必須
MULTIGRAVITY_TRUST_LAN=0
"@
    } else {
        $defaultEnv = @"
# Multigravity Global Environment Configuration
# Path: ~/.multigravity/.env

# Gateway listen port (default: 58900)
MULTIGRAVITY_PORT=58900

# Gateway bind host/IP (leave empty to bind all IPv4 interfaces; 127.0.0.1 for local only)
# MULTIGRAVITY_HOST=127.0.0.1

# ☁️ Cloudflare Tunnel remote access configuration (ready out of the box)
# CF_WORKER_URL=https://dispatcher.multigravity.app
# CF_INVITE_CODE=
# CF_TUNNEL_TOKEN=
# CF_TUNNEL_ENABLED=1

# 🔔 Native iOS APNs push & Live Activity Dynamic Island updates (enabled by default)
# APNS_ENABLED=1
# LIVE_ACTIVITY_PUSH_ENABLED=1
# APNS_RELAY_URL=https://push.multigravity.app

# LAN access security policy (0: pairing code required [recommended], 1: trust LAN without pairing)
# Local 127.0.0.1 never requires pairing; public Cloudflare Tunnel always requires pairing
MULTIGRAVITY_TRUST_LAN=0
"@
    }
    [System.IO.File]::WriteAllText("$confDir\.env", $defaultEnv, [System.Text.Encoding]::UTF8)
    try {
        if ($env:USERNAME) {
            & icacls.exe "$confDir\.env" /inheritance:r /grant:r "$($env:USERNAME):F" "SYSTEM:F" "Administrators:F" | Out-Null
        }
    } catch {}
    Write-Host (Get-Msg `
        "📝 已生成全局默认配置: $confDir\.env" `
        "📝 Created default configuration: $confDir\.env" `
        "📝 デフォルト設定ファイルを作成しました: $confDir\.env") -ForegroundColor Green
}

# 5. 下载预编译 Release 包 (多镜像容灾)
$repo = if ($env:MULTIGRAVITY_REPO) { $env:MULTIGRAVITY_REPO } else { "GHSaiMo/antigravity-mobile" }
$zipName = "multigravity-windows-$pkgArch.zip"

# 自动探测本机常用代理端口 (Clash / V2Ray / Surge 等)
$proxyPort = $null
if (-not $env:https_proxy -and -not $env:http_proxy -and -not $env:all_proxy) {
    foreach ($p in @(7890, 10808, 1080, 6152)) {
        try {
            $tcp = New-Object System.Net.Sockets.TcpClient
            $async = $tcp.BeginConnect("127.0.0.1", $p, $null, $null)
            if ($async.AsyncWaitHandle.WaitOne(150, $false) -and $tcp.Connected) {
                $proxyPort = $p
                $tcp.Close()
                if ($preferMirror) {
                    Write-Host (Get-Msg `
                        "⚡ 检测到本机代理环境 (127.0.0.1:${p})，将优先从加速镜像站直连下载（官方源备用加速）" `
                        "⚡ Detected local proxy (127.0.0.1:${p}); trying mirrors first (official source via proxy as fallback)" `
                        "⚡ ローカルプロキシ (127.0.0.1:${p}) を検出しました。加速ミラー優先でダウンロードします（公式ソースはプロキシ経由で予備利用）") -ForegroundColor Cyan
                } else {
                    Write-Host (Get-Msg `
                        "⚡ 检测到本机代理环境 (127.0.0.1:${p})，将通过本机代理加速访问 GitHub 官方源" `
                        "⚡ Detected local proxy (127.0.0.1:${p}); GitHub official downloads will use it" `
                        "⚡ ローカルプロキシ (127.0.0.1:${p}) を検出しました。GitHub 公式ソースのダウンロードに使用します") -ForegroundColor Cyan
                }
                break
            }
            $tcp.Close()
        } catch {}
    }
}

if ($preferMirror) {
    $urls = @(
        "https://ghfast.top/https://github.com/$repo/releases/latest/download/$zipName",
        "https://ghproxy.net/https://github.com/$repo/releases/latest/download/$zipName",
        "https://github.com/$repo/releases/latest/download/$zipName"
    )
} else {
    $urls = @(
        "https://github.com/$repo/releases/latest/download/$zipName",
        "https://ghfast.top/https://github.com/$repo/releases/latest/download/$zipName",
        "https://ghproxy.net/https://github.com/$repo/releases/latest/download/$zipName"
    )
}

$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("mgy-" + [System.Guid]::NewGuid().ToString().Substring(0, 8))
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
$zipFile = Join-Path $tempDir $zipName
$checksumsFile = Join-Path $tempDir "checksums.txt"
$officialChecksumsUrl = "https://github.com/$repo/releases/latest/download/checksums.txt"

# HIGH-02: 仅从 GitHub 官方 HTTPS 源获取 checksums.txt 并执行 Fail-Closed 校验
Write-Host (Get-Msg `
    "🔒 正在从 GitHub 官方源获取 SHA-256 校验清单..." `
    "🔒 Fetching official SHA-256 checksums from GitHub..." `
    "🔒 GitHub 公式ソースから SHA-256 チェックサムを取得しています...") -ForegroundColor Cyan
try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
        $csArgs = @("-fsSL", "--connect-timeout", "10", "--max-time", "25", "-o", $checksumsFile, $officialChecksumsUrl)
        if ($proxyPort) {
            $csArgs = @("--proxy", "http://127.0.0.1:$proxyPort") + $csArgs
        }
        & curl.exe @csArgs
        if ($LASTEXITCODE -ne 0) { throw "curl checksums failed" }
    } else {
        $wcCs = New-Object System.Net.WebClient
        if ($proxyPort) {
            $wcCs.Proxy = New-Object System.Net.WebProxy("http://127.0.0.1:$proxyPort")
        }
        $wcCs.DownloadFile($officialChecksumsUrl, $checksumsFile)
    }
} catch {
    Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host (Get-Msg `
        "❌ 安全校验失败: 无法从 GitHub 官方 HTTPS 源获取 checksums.txt ($officialChecksumsUrl)。" `
        "❌ Security check failed: unable to fetch checksums.txt from GitHub ($officialChecksumsUrl)." `
        "❌ セキュリティ検証に失敗しました: GitHub 公式 HTTPS から checksums.txt を取得できませんでした ($officialChecksumsUrl)。") -ForegroundColor Red
    exit 1
}

$expectedZipSha = $null
foreach ($csLine in (Get-Content $checksumsFile -ErrorAction SilentlyContinue)) {
    $trimmed = $csLine.Trim()
    if ($trimmed -match "^([0-9a-fA-F]{64})\s+\*?(\./)?(.+)$") {
        $fname = [System.IO.Path]::GetFileName($Matches[3].Trim())
        if ($fname -ieq $zipName) {
            $expectedZipSha = $Matches[1].ToLowerInvariant()
            break
        }
    }
}
if (-not $expectedZipSha) {
    Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host (Get-Msg `
        "❌ 安全校验失败: 官方 checksums.txt 中未找到 $zipName 的 SHA-256 记录。" `
        "❌ Security check failed: SHA-256 entry for $zipName not found in official checksums.txt." `
        "❌ セキュリティ検証に失敗しました: 公式 checksums.txt に $zipName の SHA-256 が見つかりません。") -ForegroundColor Red
    exit 1
}

$downloadSuccess = $false
Write-Host (Get-Msg `
    "📥 正在获取 Multigravity ($pkgArch) 最新发行版..." `
    "📥 Downloading latest Multigravity release ($pkgArch)..." `
    "📥 Multigravity ($pkgArch) の最新リリースをダウンロードしています...") -ForegroundColor Cyan

foreach ($url in $urls) {
    $isOfficial = ($url -like "https://github.com/*")
    if ($isOfficial) {
        if ($proxyPort) {
            Write-Host (Get-Msg `
                "🔗 尝试从 GitHub 官方源（走本机代理加速）下载: $url" `
                "🔗 Downloading from GitHub official source (via local proxy): $url" `
                "🔗 GitHub 公式ソース（ローカルプロキシ経由）からダウンロード中: $url") -ForegroundColor Gray
        } else {
            Write-Host (Get-Msg `
                "🔗 尝试从 GitHub 官方源下载: $url" `
                "🔗 Downloading from GitHub official source: $url" `
                "🔗 GitHub 公式ソースからダウンロード中: $url") -ForegroundColor Gray
        }
    } else {
        Write-Host (Get-Msg `
            "🔗 尝试从加速镜像站直连下载: $url" `
            "🔗 Downloading from mirror: $url" `
            "🔗 加速ミラーからダウンロード中: $url") -ForegroundColor Gray
    }

    try {
        if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
            $curlArgs = @("-fL", "--connect-timeout", "8", "--speed-limit", "10240", "--speed-time", "10", "-#", "-o", $zipFile, $url)
            if ($isOfficial -and $proxyPort) {
                $curlArgs = @("--proxy", "http://127.0.0.1:$proxyPort") + $curlArgs
            } elseif (-not $isOfficial) {
                $curlArgs = @("--noproxy", "*") + $curlArgs
            }
            & curl.exe @curlArgs
            if ($LASTEXITCODE -ne 0) { throw "curl download failed" }
        } else {
            [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
            $wc = New-Object System.Net.WebClient
            if ($isOfficial -and $proxyPort) {
                $wc.Proxy = New-Object System.Net.WebProxy("http://127.0.0.1:$proxyPort")
            }
            $wc.DownloadFile($url, $zipFile)
        }
        if (Test-Path $zipFile) {
            $gotSha = (Get-FileHash -Path $zipFile -Algorithm SHA256).Hash.ToLowerInvariant()
            if ($gotSha -eq $expectedZipSha) {
                Write-Host (Get-Msg `
                    "🔒 SHA-256 校验通过 ($($gotSha.Substring(0, 12))...)" `
                    "🔒 SHA-256 verified ($($gotSha.Substring(0, 12))...)" `
                    "🔒 SHA-256 検証に成功しました ($($gotSha.Substring(0, 12))...)") -ForegroundColor Green
                $downloadSuccess = $true
                break
            }
            Write-Host (Get-Msg `
                "⚠️  SHA-256 校验不匹配 (got $gotSha)，已丢弃并切换备用源..." `
                "⚠️  SHA-256 mismatch (got $gotSha); discarding and trying next source..." `
                "⚠️  SHA-256 チェックサムが一致しません (got $gotSha)。破棄して次のソースを試行します...") -ForegroundColor Yellow
            Remove-Item $zipFile -Force -ErrorAction SilentlyContinue
        }
    } catch {
        Write-Host (Get-Msg `
            "⚠️  下载异常或连接超时，正在切换下一个镜像源..." `
            "⚠️  Download failed or timed out; trying fallback source..." `
            "⚠️  ダウンロードに失敗またはタイムアウトしました。予備ソースを試行します...") -ForegroundColor Yellow
        if (Test-Path $zipFile) { Remove-Item $zipFile -Force -ErrorAction SilentlyContinue }
    }
}

$wasServiceRunning = $false
if ($downloadSuccess) {
    Write-Host (Get-Msg `
        "📦 校验通过，正在定向提取 mgy.exe..." `
        "📦 Verified. Extracting mgy.exe..." `
        "📦 検証完了。mgy.exe を展開しています...") -ForegroundColor Green

    # 若已注册后台自启任务且正在运行，先暂停防止升级期间文件被占用或触发重启
    try {
        $mgyTask = Get-ScheduledTask -TaskName "MultigravityGateway" -ErrorAction SilentlyContinue
        if ($mgyTask -and ($mgyTask.State -eq "Running")) {
            $wasServiceRunning = $true
            Stop-ScheduledTask -TaskName "MultigravityGateway" -ErrorAction SilentlyContinue
            Start-Sleep -Milliseconds 300
        }
    } catch {}

    # 停止旧版本 mgy 进程以防文件占用锁
    Get-Process -Name mgy -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 200

    # HIGH-02: 仅定向提取单一 mgy.exe 条目，防止 Zip-Slip 路径穿越
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = [System.IO.Compression.ZipFile]::OpenRead($zipFile)
    try {
        $targetEntry = $archive.Entries | Where-Object { $_.Name -ieq "mgy.exe" -and $_.FullName -notmatch "\.\." } | Select-Object -First 1
        if (-not $targetEntry) {
            throw (Get-Msg `
                "归档中未找到合法的 mgy.exe" `
                "Valid mgy.exe not found in archive" `
                "アーカイブ内に有効な mgy.exe が見つかりませんでした")
        }
        $stagedExe = "$installDir\mgy.exe.tmp"
        [System.IO.Compression.ZipFileExtensions]::ExtractToFile($targetEntry, $stagedExe, $true)
        Move-Item -Path $stagedExe -Destination "$installDir\mgy.exe" -Force
    } finally {
        $archive.Dispose()
    }
} else {
    Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host (Get-Msg `
        "❌ 未能下载并通过官方 SHA-256 校验获取发行包 ($zipName)。" `
        "❌ Failed to download and verify release package ($zipName)." `
        "❌ リリースパッケージ ($zipName) のダウンロードおよび SHA-256 検証に失敗しました。") -ForegroundColor Red
    Write-Host (Get-Msg `
        "   请检查网络连接或前往官方 Release 下载: https://github.com/$repo/releases/latest" `
        "   Please check your network or download manually from GitHub Releases: https://github.com/$repo/releases/latest" `
        "   ネットワーク接続を確認するか、公式 Releases から手動でダウンロードしてください: https://github.com/$repo/releases/latest") -ForegroundColor Yellow
    exit 1
}

# 6. 安装到 WindowsApps (Windows 默认已在 PATH 中的用户级目录，免重启即生效)
$windowsApps = "$env:LOCALAPPDATA\Microsoft\WindowsApps"
if (Test-Path $windowsApps) {
    try {
        Copy-Item -Path "$installDir\mgy.exe" -Destination "$windowsApps\mgy.exe" -Force
    } catch {}
}

# 若升级前后台自启服务正在运行，自动恢复启动新版
if ($wasServiceRunning) {
    try {
        Start-ScheduledTask -TaskName "MultigravityGateway" -ErrorAction SilentlyContinue
        Write-Host (Get-Msg `
            "🔄 已自动重启后台自启服务 (MultigravityGateway)" `
            "🔄 Automatically restarted background service (MultigravityGateway)" `
            "🔄 バックグラウンドサービス (MultigravityGateway) を自動再起動しました") -ForegroundColor Green
    } catch {}
}

# 7. 确保 ~/.local/bin 写入系统用户 PATH 环境变量
try {
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$installDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
    }
} catch {}

# 刷新当前会话的 PATH
$env:PATH = "$installDir;$windowsApps;$env:PATH"

# 8. 预先检测并安装 Cloudflare 穿透引擎 (用于远程外网安全直连)
$cfBinDir = "$confDir\bin"
if (!(Test-Path $cfBinDir)) { New-Item -ItemType Directory -Path $cfBinDir -Force | Out-Null }
$cfTarget = "$cfBinDir\cloudflared.exe"

$cfExisting = $null
if (Get-Command cloudflared.exe -ErrorAction SilentlyContinue) {
    $cfCmd = (Get-Command cloudflared.exe).Source
    $cfItem = Get-Item $cfCmd -ErrorAction SilentlyContinue
    if ($cfItem -and ($cfItem.Length -gt 10000000)) {
        $cfExisting = $cfCmd
    }
}
if (!$cfExisting -and (Test-Path $cfTarget)) {
    $cfTargetItem = Get-Item $cfTarget -ErrorAction SilentlyContinue
    if ($cfTargetItem -and ($cfTargetItem.Length -gt 10000000)) {
        $cfExisting = $cfTarget
    }
}

if ($cfExisting) {
    Write-Host (Get-Msg `
        "✅ 检测到 Cloudflare 穿透引擎已就绪: $cfExisting" `
        "✅ Cloudflare tunnel engine already installed: $cfExisting" `
        "✅ Cloudflare トンネルエンジンを検出済み: $cfExisting") -ForegroundColor Green
    if (!(Test-Path "$installDir\cloudflared.exe") -and (Test-Path $cfTarget)) {
        try { Copy-Item -Path $cfTarget -Destination "$installDir\cloudflared.exe" -Force } catch {}
    }
} else {
    $cfVersion = "2026.11.1"
    $cfExpectedSha = "341deda3db5ec498a0482254d6c38c0b7cf01a30c37bb5dd95d2216d08955e35"
    if ($preferMirror) {
        $cfUrls = @(
            "https://ghfast.top/https://github.com/cloudflare/cloudflared/releases/download/$cfVersion/cloudflared-windows-amd64.exe",
            "https://ghproxy.net/https://github.com/cloudflare/cloudflared/releases/download/$cfVersion/cloudflared-windows-amd64.exe",
            "https://github.com/cloudflare/cloudflared/releases/download/$cfVersion/cloudflared-windows-amd64.exe"
        )
    } else {
        $cfUrls = @(
            "https://github.com/cloudflare/cloudflared/releases/download/$cfVersion/cloudflared-windows-amd64.exe",
            "https://ghfast.top/https://github.com/cloudflare/cloudflared/releases/download/$cfVersion/cloudflared-windows-amd64.exe",
            "https://ghproxy.net/https://github.com/cloudflare/cloudflared/releases/download/$cfVersion/cloudflared-windows-amd64.exe"
        )
    }

    $cfDone = $false
    $keepTrying = $true

    while ($keepTrying -and -not $cfDone) {
        Write-Host ""
        Write-Host (Get-Msg `
            "⏬ 正在预先获取 Cloudflare 穿透引擎 (cloudflared.exe, 约 65MB)..." `
            "⏬ Pre-downloading Cloudflare tunnel engine (cloudflared.exe, ~65MB)..." `
            "⏬ Cloudflare トンネルエンジン (cloudflared.exe, 約 65MB) を事前ダウンロードしています...") -ForegroundColor Cyan

        # 动态重新探测本机代理端口（支持用户切换节点后重试）
        $cfProxyPort = $proxyPort
        if (-not $env:https_proxy -and -not $env:http_proxy -and -not $env:all_proxy) {
            foreach ($p in @(7890, 10808, 1080, 6152)) {
                try {
                    $tcp = New-Object System.Net.Sockets.TcpClient
                    $async = $tcp.BeginConnect("127.0.0.1", $p, $null, $null)
                    if ($async.AsyncWaitHandle.WaitOne(150, $false) -and $tcp.Connected) {
                        $cfProxyPort = $p
                        $tcp.Close()
                        break
                    }
                    $tcp.Close()
                } catch {}
            }
        }

        foreach ($url in $cfUrls) {
            $isOfficial = ($url -like "https://github.com/*")
            if ($isOfficial) {
                if ($cfProxyPort) {
                    Write-Host (Get-Msg `
                        "🔗 尝试从 Cloudflare 官方源（走本机代理 127.0.0.1:${cfProxyPort}）下载..." `
                        "🔗 Downloading from Cloudflare official source (via local proxy 127.0.0.1:${cfProxyPort})..." `
                        "🔗 Cloudflare 公式ソース（ローカルプロキシ 127.0.0.1:${cfProxyPort} 経由）からダウンロード中...") -ForegroundColor Gray
                } else {
                    Write-Host (Get-Msg `
                        "🔗 尝试从 Cloudflare 官方源下载: $url" `
                        "🔗 Downloading from Cloudflare official source: $url" `
                        "🔗 Cloudflare 公式ソースからダウンロード中: $url") -ForegroundColor Gray
                }
            } else {
                Write-Host (Get-Msg `
                    "🔗 尝试从加速镜像站直连下载: $url" `
                    "🔗 Downloading from mirror: $url" `
                    "🔗 加速ミラーからダウンロード中: $url") -ForegroundColor Gray
            }

            $tmpFile = "$cfTarget.tmp"
            if (Test-Path $tmpFile) { Remove-Item $tmpFile -Force }

            try {
                if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
                    # 超时放宽至 300 秒 (5分钟)，连接超时 15 秒，显示进度条
                    $curlArgs = @("-fL", "--connect-timeout", "15", "--max-time", "300", "-#", "-o", $tmpFile, $url)
                    if ($isOfficial -and $cfProxyPort) {
                        $curlArgs = @("--proxy", "http://127.0.0.1:$cfProxyPort") + $curlArgs
                    } elseif (-not $isOfficial) {
                        $curlArgs = @("--noproxy", "*") + $curlArgs
                    }
                    & curl.exe @curlArgs
                } else {
                    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
                    $wc = New-Object System.Net.WebClient
                    if ($isOfficial -and $cfProxyPort) {
                        $wc.Proxy = New-Object System.Net.WebProxy("http://127.0.0.1:$cfProxyPort")
                    }
                    $wc.DownloadFile($url, $tmpFile)
                }

                if (Test-Path $tmpFile) {
                    $gotCfSha = (Get-FileHash -Path $tmpFile -Algorithm SHA256).Hash.ToLowerInvariant()
                    if ($gotCfSha -eq $cfExpectedSha) {
                        Move-Item -Path $tmpFile -Destination $cfTarget -Force
                        try { Copy-Item -Path $cfTarget -Destination "$installDir\cloudflared.exe" -Force } catch {}
                        $cfDone = $true
                        Write-Host (Get-Msg `
                            "✅ Cloudflare 穿透引擎 SHA-256 校验通过并安装成功: $cfTarget" `
                            "✅ Cloudflare tunnel engine verified and installed: $cfTarget" `
                            "✅ Cloudflare トンネルエンジンの SHA-256 を検証しインストールしました: $cfTarget") -ForegroundColor Green
                        break
                    }
                    Write-Host (Get-Msg `
                        "⚠️  cloudflared SHA-256 校验不匹配 (got $gotCfSha)，已丢弃并切换备用源..." `
                        "⚠️  cloudflared SHA-256 mismatch (got $gotCfSha); discarding and trying next source..." `
                        "⚠️  cloudflared の SHA-256 が一致しません (got $gotCfSha)。破棄して次のソースを試行します...") -ForegroundColor Yellow
                    Remove-Item $tmpFile -Force -ErrorAction SilentlyContinue
                }
            } catch {
                Write-Host (Get-Msg `
                    "⚠️  当前源下载异常或超时，尝试下一个备用源..." `
                    "⚠️  Download failed or timed out; trying fallback source..." `
                    "⚠️  ダウンロードに失敗またはタイムアウトしました。予備ソースを試行します...") -ForegroundColor Yellow
                if (Test-Path $tmpFile) { Remove-Item $tmpFile -Force }
            }
        }

        if (-not $cfDone) {
            Write-Host ""
            Write-Host (Get-Msg `
                "⚠️  Cloudflare 穿透引擎下载失败（所有镜像与官方源均连接超时或受阻）。" `
                "⚠️  Failed to download Cloudflare tunnel engine (all sources timed out or failed)." `
                "⚠️  Cloudflare トンネルエンジンのダウンロードに失敗しました（全ソースでタイムアウトまたは接続失敗）。") -ForegroundColor Yellow
            Write-Host (Get-Msg `
                "💡 提示: cloudflared 体积约 65MB，国内部分网络访问可能受限。可切换代理节点或开启代理客户端后重试。" `
                "💡 Tip: cloudflared is ~65MB and may fail on restricted networks. Switch networks or enable a proxy and retry." `
                "💡 ヒント: cloudflared は約 65MB あり、ネットワーク制限により失敗することがあります。プロキシや回線を切り替えて再試行してください。") -ForegroundColor Gray

            # 检测是否支持交互式输入
            $isInteractive = [Environment]::UserInteractive -and -not [Console]::IsInputRedirected
            if ($isInteractive) {
                Write-Host ""
                $userChoice = Read-Host (Get-Msg `
                    "❓ 是否尝试切换代理节点/网络后重试下载？(Y: 切换后重试 / N: 跳过稍后)" `
                    "❓ Retry download after switching network/proxy? (Y: retry / N: skip for now)" `
                    "❓ ネットワークやプロキシを切り替えて再試行しますか？ (Y: 再試行 / N: スキップ)")
                if ($userChoice -and $userChoice -match "^[Nn]$") {
                    $keepTrying = $false
                    Write-Host (Get-Msg `
                        "⚠️  已跳过 Cloudflare 引擎预装。后续运行 mgy 时若开启穿透，引擎仍会自动尝试拉取；亦可手动放置 cloudflared.exe 至 $cfTarget" `
                        "⚠️  Skipped cloudflared pre-install. mgy will retry automatically when tunneling is enabled, or you can place cloudflared.exe manually at $cfTarget" `
                        "⚠️  cloudflared の事前インストールをスキップしました。次回 mgy 起動時に自動で再取得されます（手動配置先: $cfTarget）。") -ForegroundColor Yellow
                } else {
                    Write-Host (Get-Msg `
                        "🔄 正在准备重试下载，请确保网络或代理已切换就绪..." `
                        "🔄 Preparing to retry download. Please make sure your network/proxy is ready..." `
                        "🔄 ダウンロードを再試行します。ネットワークまたはプロキシの準備を確認してください...") -ForegroundColor Cyan
                    Start-Sleep -Seconds 1
                }
            } else {
                $keepTrying = $false
                Write-Host (Get-Msg `
                    "⚠️  非交互式终端，已跳过预下载。后续运行 mgy 时仍会自动尝试拉取。" `
                    "⚠️  Non-interactive terminal; skipping pre-download. mgy will retry automatically on launch." `
                    "⚠️  非対話型ターミナルのため事前ダウンロードをスキップしました。mgy 起動時に自動で再取得されます。") -ForegroundColor Yellow
            }
        }
    }
}

# 8.5 首次安装交互式安全向导：局域网配对策略配置
$envFile = "$confDir\.env"
$lanPrefConfigured = $false
if (Test-Path $envFile) {
    $envContent = Get-Content $envFile -Raw -ErrorAction SilentlyContinue
    if ($envContent -and ($envContent -match "(?m)^MULTIGRAVITY_TRUST_LAN=")) {
        $lanPrefConfigured = $true
    }
}

$isInteractive = [Environment]::UserInteractive -and -not [Console]::IsInputRedirected
if (-not $lanPrefConfigured -or ($env:FORCE_CONFIG -eq "1")) {
    if ($isInteractive) {
        Write-Host ""
        Write-Host "==================================================" -ForegroundColor Cyan
        Write-Host (Get-Msg `
            "⚙️  Multigravity 局域网访问安全偏好配置向导" `
            "⚙️  Multigravity LAN Access Security Setup" `
            "⚙️  Multigravity LAN アクセスセキュリティ設定") -ForegroundColor Cyan
        Write-Host "==================================================" -ForegroundColor Cyan
        Write-Host (Get-Msg `
            "当您在手机、iPad 或同局域网其他电脑访问本网关时，是否需要配对码？" `
            "Require a pairing code when phones, iPads, or other devices on your LAN connect?" `
            "スマートフォン・iPad・同一 LAN 内の他端末から接続する際、ペアリングコードを必須にしますか？")
        Write-Host ""
        Write-Host (Get-Msg `
            "  [1] 🔐 开启局域网配对校验（默认推荐）" `
            "  [1] 🔐 Require LAN pairing code (Recommended)" `
            "  [1] 🔐 LAN ペアリング認証を有効にする（推奨・デフォルト）") -ForegroundColor Green
        Write-Host (Get-Msg `
            "      适合办公网络、公共 Wi-Fi 或有多人/访客环境。" `
            "      Best for office networks, public Wi-Fi, or shared environments." `
            "      オフィス・公衆 Wi-Fi・共有ネットワーク環境に最適です。") -ForegroundColor Gray
        Write-Host (Get-Msg `
            "      新设备首次接入必须输入配对码授权。" `
            "      New devices must authorize with a pairing code on first connection." `
            "      新しいデバイスは初回接続時にペアリングコードでの認可が必要です。") -ForegroundColor Gray
        Write-Host ""
        Write-Host (Get-Msg `
            "  [2] 🚀 信任局域网免配对（极简直连）" `
            "  [2] 🚀 Trust LAN without pairing (Instant access)" `
            "  [2] 🚀 LAN を信頼してペアリング不要にする（ダイレクト接続）") -ForegroundColor Yellow
        Write-Host (Get-Msg `
            "      适合家庭纯净内网、私人可控网络。" `
            "      Best for private, trusted home networks." `
            "      自宅など完全に管理されたプライベートネットワーク向けです。") -ForegroundColor Gray
        Write-Host (Get-Msg `
            "      同局域网设备打开网页即可直接进入桌面工作台，公网穿透仍强制验证。" `
            "      LAN devices connect directly; public Cloudflare Tunnel always requires pairing." `
            "      同一 LAN からは即座に接続できます（パブリックトンネル経由は常に認証必須）。") -ForegroundColor Gray
        Write-Host ""

        $lanChoice = Read-Host (Get-Msg `
            "请选择 [1-2] (默认: 1)" `
            "Select [1-2] (default: 1)" `
            "[1-2] を選択してください (デフォルト: 1)")
        if ($lanChoice -eq "2") {
            $trustVal = "1"
            Write-Host (Get-Msg `
                "✅ 已启用局域网信任免配对直连模式 (MULTIGRAVITY_TRUST_LAN=1)" `
                "✅ Enabled trusted LAN direct mode (MULTIGRAVITY_TRUST_LAN=1)" `
                "✅ 信頼済み LAN ダイレクト接続を有効にしました (MULTIGRAVITY_TRUST_LAN=1)") -ForegroundColor Green
        } else {
            $trustVal = "0"
            Write-Host (Get-Msg `
                "✅ 已保持标准安全配对模式 (MULTIGRAVITY_TRUST_LAN=0)" `
                "✅ Kept standard LAN pairing verification (MULTIGRAVITY_TRUST_LAN=0)" `
                "✅ 標準の LAN ペアリング認証を維持しました (MULTIGRAVITY_TRUST_LAN=0)") -ForegroundColor Green
        }

        # 更新或追加到 .env
        if (Test-Path $envFile) {
            $lines = Get-Content $envFile
            $newLines = @()
            $found = $false
            foreach ($line in $lines) {
                if ($line -match "^MULTIGRAVITY_TRUST_LAN=") {
                    $newLines += "MULTIGRAVITY_TRUST_LAN=$trustVal"
                    $found = $true
                } else {
                    $newLines += $line
                }
            }
            if (-not $found) {
                $newLines += "MULTIGRAVITY_TRUST_LAN=$trustVal"
            }
            [System.IO.File]::WriteAllLines($envFile, $newLines, [System.Text.Encoding]::UTF8)
        } else {
            [System.IO.File]::WriteAllText($envFile, "MULTIGRAVITY_TRUST_LAN=$trustVal`n", [System.Text.Encoding]::UTF8)
        }
        Write-Host (Get-Msg `
            "💡 提示: 之后可随时在 $confDir\.env 中修改 MULTIGRAVITY_TRUST_LAN 的值" `
            "💡 Tip: You can change MULTIGRAVITY_TRUST_LAN anytime in $confDir\.env" `
            "💡 ヒント: MULTIGRAVITY_TRUST_LAN はいつでも $confDir\.env で変更できます") -ForegroundColor Gray
    } else {
        # 非交互环境追加默认值
        if (Test-Path $envFile) {
            $raw = Get-Content $envFile -Raw -ErrorAction SilentlyContinue
            if (-not ($raw -match "(?m)^MULTIGRAVITY_TRUST_LAN=")) {
                [System.IO.File]::AppendAllText($envFile, "`nMULTIGRAVITY_TRUST_LAN=0`n", [System.Text.Encoding]::UTF8)
            }
        }
    }
}

# 9. 验证与打印完成信息
$installedVer = & "$installDir\mgy.exe" version 2>$null
if (!$installedVer) { $installedVer = "Multigravity (mgy)" }

Write-Host ""
Write-Host "==================================================" -ForegroundColor Cyan
Write-Host (Get-Msg `
    "🎉 安装完成！$installedVer" `
    "🎉 Installation complete! $installedVer" `
    "🎉 インストールが完了しました！ $installedVer") -ForegroundColor Green
Write-Host "==================================================" -ForegroundColor Cyan
Write-Host (Get-Msg `
    "📍 二进制安装位置:   $installDir\mgy.exe" `
    "📍 Binary path:      $installDir\mgy.exe" `
    "📍 バイナリ配置先:   $installDir\mgy.exe")
Write-Host (Get-Msg `
    "📁 全局配置与数据:   $confDir\" `
    "📁 Config & data:    $confDir\" `
    "📁 設定・データ:     $confDir\")
Write-Host (Get-Msg `
    "📄 配置文件路径:     $confDir\.env" `
    "📄 Config file:      $confDir\.env" `
    "📄 設定ファイル:     $confDir\.env")
Write-Host ""
Write-Host (Get-Msg "🚀 常用指令:" "🚀 Common commands:" "🚀 主なコマンド:") -ForegroundColor Yellow
Write-Host (Get-Msg "   • 启动网关主服务:   mgy"                 "   • Start gateway server:     mgy"                 "   • ゲートウェイを起動:       mgy") -ForegroundColor White
Write-Host (Get-Msg "   • 开机后台自动接管: mgy service install" "   • Auto-start on boot:       mgy service install" "   • 自動起動サービス登録:     mgy service install") -ForegroundColor White
Write-Host (Get-Msg "   • 终端打印配对码:   mgy pair"            "   • Show pairing QR code:     mgy pair"            "   • ペアリング QR を表示:     mgy pair") -ForegroundColor White
Write-Host (Get-Msg "   • 配置座舱报表:     mgy cockpit"    "   • Configure Cockpit sync:   mgy cockpit"    "   • Cockpit レポート設定:     mgy cockpit") -ForegroundColor White
Write-Host (Get-Msg "   • 配置公网穿透:     mgy cloudflare" "   • Configure public tunnel:  mgy cloudflare" "   • パブリックトンネル設定:   mgy cloudflare") -ForegroundColor White
Write-Host (Get-Msg "   • 查看已连接设备:   mgy list"       "   • List paired devices:      mgy list"       "   • ペアリング済み端末一覧:   mgy list") -ForegroundColor White
Write-Host (Get-Msg "   • 清空已配对设备:   mgy clear all"  "   • Revoke all devices:       mgy clear all"  "   • 全ペアリング端末を解除:   mgy clear all") -ForegroundColor White
Write-Host (Get-Msg "   • 查看命令帮助:     mgy help"       "   • Show CLI help:            mgy help"       "   • コマンドヘルプを表示:     mgy help") -ForegroundColor White
Write-Host ""
Write-Host (Get-Msg "📱 手机端使用:" "📱 Mobile app:" "📱 モバイルアプリ:") -ForegroundColor Yellow
Write-Host "   • iOS (App Store): $appStoreUrl" -ForegroundColor White
Write-Host "   • Android (APK):   https://github.com/$repo/releases/latest" -ForegroundColor White
Write-Host (Get-Msg `
    "   打开 App 扫描终端打印的二维码即可完成配对！" `
    "   Open the app and scan the QR code printed in your terminal to pair!" `
    "   アプリを開き、ターミナルに表示される QR コードを読み取ってペアリングしてください！") -ForegroundColor White
Write-Host "==================================================" -ForegroundColor Cyan

# 清理临时文件
Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
