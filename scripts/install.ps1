# Multigravity Windows 一键安装脚本
$ErrorActionPreference = "Stop"

# 1. 强制控制台与输出流使用 UTF-8 编码，防止 Windows PowerShell 默认 GBK 导致乱码
try {
    [Console]::InputEncoding = [System.Text.Encoding]::UTF8
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    $OutputEncoding = [System.Text.Encoding]::UTF8
} catch {}

Write-Host "==================================================" -ForegroundColor Cyan
Write-Host "  📱 Multigravity (mgy) - Windows 一键安装与配置  " -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Cyan

# 2. 架构检测 (Windows 优先使用 amd64 预编译包，ARM64 系统通过内置仿真无缝运行)
$arch = $env:PROCESSOR_ARCHITECTURE
$pkgArch = "amd64"
$archDesc = "Windows x86_64 (amd64)"
Write-Host "🖥️  检测到系统架构: $archDesc ($pkgArch)" -ForegroundColor Cyan

# 3. 准备安装与配置目录
$installDir = "$HOME\.local\bin"
$confDir = "$HOME\.multigravity"
if (!(Test-Path $installDir)) { New-Item -ItemType Directory -Path $installDir -Force | Out-Null }
if (!(Test-Path "$confDir\logs")) { New-Item -ItemType Directory -Path "$confDir\logs" -Force | Out-Null }

# 4. 初始化默认配置 .env (若不存在)
if (!(Test-Path "$confDir\.env")) {
    $defaultEnv = @"
# Multigravity 全局环境变量配置文件
# 保存路径: ~/.multigravity/.env

# 网关监听端口 (默认 58900)
MULTIGRAVITY_PORT=58900

# 网关监听主机/IP (默认留空双栈绑定所有网卡，设为 127.0.0.1 仅限本机)
# MULTIGRAVITY_HOST=127.0.0.1

# 公网 DDNS 域名或固定 IPv6 地址 (若需要外网直连)
# DDNS_HOST=agy.example.com

# 公网 IPv6 自动广播 (默认 1：检测到公网 IPv6 时自动打入复合配对二维码与链接；设为 0 关闭)
INCLUDE_PUBLIC_IPV6=1

# 是否默认优先使用纯 IPv6 作为二维码 (默认 0 生成双栈复合码；设为 1 纯 IPv6 码)
# MULTIGRAVITY_PREFER_IPV6=0

# HTTPS / SSL 加密访问 (启用需设为 1 并指定证书和私钥文件)
# MULTIGRAVITY_SSL=0
# MULTIGRAVITY_TLS_CERT=~/.multigravity/certs/fullchain.cer
# MULTIGRAVITY_TLS_KEY=~/.multigravity/certs/private.key

# iOS Bark 实时推送通知 (填入 Device Key 或 Bark 完整 URL)
# BARK_URL=
# BARK_ICON=https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/web/icons/icon-192.png
# BARK_GROUP=Antigravity
# BARK_SOUND_ACTION=alarm
# BARK_SOUND_COMPLETE=glass

# ☁️ Cloudflare Tunnel 专属公网穿透配置 (开箱即用)
# CF_WORKER_URL=https://dispatcher.jiuge.space
# CF_INVITE_CODE=
# CF_TUNNEL_TOKEN=
# CF_TUNNEL_ENABLED=1
"@
    [System.IO.File]::WriteAllText("$confDir\.env", $defaultEnv, [System.Text.Encoding]::UTF8)
    Write-Host "📝 已生成全局默认配置: $confDir\.env" -ForegroundColor Green
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
                Write-Host "⚡ 检测到本机代理环境 (127.0.0.1:$p)，将优先从加速镜像站直连下载（官方源备用加速）" -ForegroundColor Cyan
                break
            }
            $tcp.Close()
        } catch {}
    }
}

# 优先镜像站直连加速下载，官方源排在最后作为兜底
$urls = @(
    "https://ghfast.top/https://github.com/$repo/releases/latest/download/$zipName",
    "https://ghproxy.net/https://github.com/$repo/releases/latest/download/$zipName",
    "https://github.com/$repo/releases/latest/download/$zipName"
)

$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("mgy-" + [System.Guid]::NewGuid().ToString().Substring(0, 8))
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
$zipFile = Join-Path $tempDir $zipName

$downloadSuccess = $false
Write-Host "📥 正在获取 Multigravity ($pkgArch) 最新发行版..." -ForegroundColor Cyan

foreach ($url in $urls) {
    $isOfficial = ($url -like "https://github.com/*")
    if ($isOfficial) {
        if ($proxyPort) {
            Write-Host "🔗 尝试从 GitHub 官方源（走本机代理加速）下载: $url" -ForegroundColor Gray
        } else {
            Write-Host "🔗 尝试从 GitHub 官方源下载: $url" -ForegroundColor Gray
        }
    } else {
        Write-Host "🔗 尝试从加速镜像站直连下载: $url" -ForegroundColor Gray
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
        } else {
            [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
            $wc = New-Object System.Net.WebClient
            if ($isOfficial -and $proxyPort) {
                $wc.Proxy = New-Object System.Net.WebProxy("http://127.0.0.1:$proxyPort")
            }
            $wc.DownloadFile($url, $zipFile)
        }
        if ((Test-Path $zipFile) -and ((Get-Item $zipFile).Length -gt 100000)) {
            $downloadSuccess = $true
            break
        }
    } catch {
        Write-Host "⚠️  下载异常或连接超时，正在切换下一个镜像源..." -ForegroundColor Yellow
        if (Test-Path $zipFile) { Remove-Item $zipFile -Force }
    }
}

if ($downloadSuccess) {
    Write-Host "📦 下载完成，正在解压安装..." -ForegroundColor Green
    
    # 停止旧版本 mgy 进程以防文件占用锁
    Get-Process -Name mgy -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 200

    if (Get-Command tar.exe -ErrorAction SilentlyContinue) {
        tar.exe -xf $zipFile -C $tempDir
    } else {
        Expand-Archive -Path $zipFile -DestinationPath $tempDir -Force
    }

    $exeSource = Join-Path $tempDir "mgy.exe"
    if (Test-Path $exeSource) {
        Copy-Item -Path $exeSource -Destination "$installDir\mgy.exe" -Force
    } else {
        Write-Host "❌ 解压归档中未找到 mgy.exe" -ForegroundColor Red
        exit 1
    }
} else {
    Write-Host "⚠️  未能从网络镜像获取预编译包，尝试回退本地编译..." -ForegroundColor Yellow
    
    # 查找本地 Go
    $goCmd = "go"
    if (!(Get-Command go -ErrorAction SilentlyContinue)) {
        if (Test-Path "$HOME\go-sdk\go\bin\go.exe") {
            $goCmd = "$HOME\go-sdk\go\bin\go.exe"
            $env:PATH = "$HOME\go-sdk\go\bin;$env:PATH"
        } else {
            Write-Host "⚠️ 未检测到系统 Go 环境，正在下载便携式 Go SDK..." -ForegroundColor Yellow
            $sdkDir = "$HOME\go-sdk"
            if (!(Test-Path $sdkDir)) { New-Item -ItemType Directory -Path $sdkDir -Force | Out-Null }
            $goZip = "$sdkDir\go.zip"
            curl.exe -L -o $goZip "https://dl.google.com/go/go1.22.10.windows-amd64.zip"
            if (Get-Command tar.exe -ErrorAction SilentlyContinue) {
                tar.exe -xf $goZip -C $sdkDir
            } else {
                Expand-Archive -Path $goZip -DestinationPath $sdkDir -Force
            }
            Remove-Item $goZip -Force
            $goCmd = "$sdkDir\go\bin\go.exe"
            $env:PATH = "$sdkDir\go\bin;$env:PATH"
        }
    }

    $repoRoot = $PSScriptRoot
    if ($repoRoot -and (Test-Path "$repoRoot\..\cmd\gateway")) {
        Set-Location (Join-Path $repoRoot "..")
    }
    & $goCmd build -ldflags="-s -w -X 'main.Version=1.0.2'" -o "$installDir\mgy.exe" ./cmd/gateway
}

# 6. 安装到 WindowsApps (Windows 默认已在 PATH 中的用户级目录，免重启即生效)
$windowsApps = "$env:LOCALAPPDATA\Microsoft\WindowsApps"
if (Test-Path $windowsApps) {
    try {
        Copy-Item -Path "$installDir\mgy.exe" -Destination "$windowsApps\mgy.exe" -Force
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

# 8. 验证与打印完成信息
$installedVer = & "$installDir\mgy.exe" version 2>$null
if (!$installedVer) { $installedVer = "Multigravity (mgy) 1.0.2" }

Write-Host ""
Write-Host "==================================================" -ForegroundColor Cyan
Write-Host "🎉 安装完成！$installedVer" -ForegroundColor Green
Write-Host "==================================================" -ForegroundColor Cyan
Write-Host "📍 二进制安装位置:   $installDir\mgy.exe"
Write-Host "📁 全局配置与数据:   $confDir\"
Write-Host "📄 配置文件路径:     $confDir\.env"
Write-Host ""
Write-Host "🚀 常用指令:" -ForegroundColor Yellow
Write-Host "   • 启动网关主服务:   mgy" -ForegroundColor White
Write-Host "   • 终端打印配对码:   mgy pair" -ForegroundColor White
Write-Host "   • 查看已连接设备:   mgy list" -ForegroundColor White
Write-Host "   • 清空已配对设备:   mgy clear all" -ForegroundColor White
Write-Host "   • 查看命令帮助:     mgy help" -ForegroundColor White
Write-Host ""
Write-Host "📱 手机端使用:" -ForegroundColor Yellow
Write-Host "   请在 GitHub Releases 下载安装 Multigravity-*.apk，" -ForegroundColor White
Write-Host "   打开 App 扫描终端打印的二维码即可完成配对！" -ForegroundColor White
Write-Host "==================================================" -ForegroundColor Cyan

# 清理临时文件
Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
