# Multigravity (mgy) Windows Complete Uninstaller (zh / en / ja)
# Stops running processes, removes mgy.exe, cloudflared.exe, and ~/.multigravity data
param(
    [switch]$KeepConfig,
    [switch]$KeepCloudflared,
    [string]$Lang = ""
)

$ErrorActionPreference = "SilentlyContinue"

# Force UTF-8 console output
try {
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    $OutputEncoding = [System.Text.Encoding]::UTF8
} catch {}

# Native i18n (zh / en / ja)
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

Write-Host "==================================================" -ForegroundColor Cyan
Write-Host (Get-Msg `
    "  🗑️  Multigravity (mgy) - Windows 彻底卸载与清理  " `
    "  🗑️  Multigravity (mgy) - Windows Uninstaller    " `
    "  🗑️  Multigravity (mgy) - Windows アンインストール ") -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Cyan

# 0. 先停止并移除 Windows 开机自启任务（防止崩溃自动重启在卸载途中复活幽灵进程）
$taskName = "MultigravityGateway"
$scheduledTask = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
if ($scheduledTask) {
    Write-Host (Get-Msg `
        "⏹️  正在停止并移除 Windows 开机自启任务: $taskName" `
        "⏹️  Stopping and removing Windows scheduled task: $taskName" `
        "⏹️  Windows 自動起動タスクを停止・削除しています: $taskName") -ForegroundColor Yellow
    Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 300
    try {
        Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction Stop
        Write-Host (Get-Msg `
            "✅ 已移除 Windows 自启任务: $taskName" `
            "✅ Removed Windows scheduled task: $taskName" `
            "✅ Windows 自動起動タスクを削除しました: $taskName") -ForegroundColor Green
    } catch {
        & schtasks.exe /Delete /TN $taskName /F 2>$null | Out-Null
        if ($LASTEXITCODE -eq 0) {
            Write-Host (Get-Msg `
                "✅ 已移除 Windows 自启任务: $taskName" `
                "✅ Removed Windows scheduled task: $taskName" `
                "✅ Windows 自動起動タスクを削除しました: $taskName") -ForegroundColor Green
        } else {
            Write-Host (Get-Msg `
                "⚠️  移除自启任务 $taskName 需要管理员权限，请以管理员身份运行后执行: schtasks /Delete /TN $taskName /F" `
                "⚠️  Administrator privileges required to remove scheduled task $taskName; run elevated: schtasks /Delete /TN $taskName /F" `
                "⚠️  タスク $taskName の削除には管理者権限が必要です。管理者で次を実行してください: schtasks /Delete /TN $taskName /F") -ForegroundColor Yellow
        }
    }
}

# 1. 停止运行中的 mgy 进程
$mgyProc = Get-Process -Name mgy -ErrorAction SilentlyContinue
if ($mgyProc) {
    Write-Host (Get-Msg `
        "⏹️  正在停止运行中的 mgy 进程..." `
        "⏹️  Stopping running mgy processes..." `
        "⏹️  実行中の mgy プロセスを停止しています...") -ForegroundColor Yellow
    $mgyProc | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 300
}

# 2. 停止运行中的 cloudflared 进程（仅当未指定 -KeepCloudflared 时）
if (-not $KeepCloudflared) {
    $cfProc = Get-Process -Name cloudflared -ErrorAction SilentlyContinue
    if ($cfProc) {
        Write-Host (Get-Msg `
            "⏹️  正在停止运行中的 cloudflared 穿透进程..." `
            "⏹️  Stopping running cloudflared tunnel processes..." `
            "⏹️  実行中の cloudflared トンネルプロセスを停止しています...") -ForegroundColor Yellow
        $cfProc | Stop-Process -Force -ErrorAction SilentlyContinue
        Start-Sleep -Milliseconds 300
    }
}

# 3. 删除 mgy.exe 执行文件与系统快捷方式
$winAppMgy = Join-Path $env:LOCALAPPDATA "Microsoft\WindowsApps\mgy.exe"
if (Test-Path $winAppMgy) {
    Remove-Item $winAppMgy -Force -ErrorAction SilentlyContinue
    Write-Host (Get-Msg `
        "✅ 已删除 WindowsApps 快捷指令: $winAppMgy" `
        "✅ Removed WindowsApps shim: $winAppMgy" `
        "✅ WindowsApps ショートカットを削除しました: $winAppMgy") -ForegroundColor Green
}

$localBinMgy = Join-Path $HOME ".local\bin\mgy.exe"
if (Test-Path $localBinMgy) {
    Remove-Item $localBinMgy -Force -ErrorAction SilentlyContinue
    Write-Host (Get-Msg `
        "✅ 已删除 mgy 执行文件: $localBinMgy" `
        "✅ Removed mgy binary: $localBinMgy" `
        "✅ mgy バイナリを削除しました: $localBinMgy") -ForegroundColor Green
}

# 4. 删除 cloudflared 穿透引擎
if (-not $KeepCloudflared) {
    $localBinCf = Join-Path $HOME ".local\bin\cloudflared.exe"
    if (Test-Path $localBinCf) {
        Remove-Item $localBinCf -Force -ErrorAction SilentlyContinue
        Write-Host (Get-Msg `
            "✅ 已删除 Cloudflare 引擎: $localBinCf" `
            "✅ Removed Cloudflare engine: $localBinCf" `
            "✅ Cloudflare エンジンを削除しました: $localBinCf") -ForegroundColor Green
    }

    $cfEmbedded = Join-Path $HOME ".multigravity\bin\cloudflared.exe"
    if (Test-Path $cfEmbedded) {
        Remove-Item $cfEmbedded -Force -ErrorAction SilentlyContinue
        Write-Host (Get-Msg `
            "✅ 已删除 Cloudflare 引擎: $cfEmbedded" `
            "✅ Removed Cloudflare engine: $cfEmbedded" `
            "✅ Cloudflare エンジンを削除しました: $cfEmbedded") -ForegroundColor Green
    }
} else {
    Write-Host (Get-Msg `
        "💡 跳过 Cloudflare 引擎移除 (-KeepCloudflared)" `
        "💡 Keeping Cloudflare tunnel engine (-KeepCloudflared)" `
        "💡 Cloudflare トンネルエンジンの削除をスキップしました (-KeepCloudflared)") -ForegroundColor Gray
}

# 5. 删除 ~/.multigravity 配置与授权凭据目录
$mgyDataDir = Join-Path $HOME ".multigravity"
if (Test-Path $mgyDataDir) {
    if (-not $KeepConfig) {
        Remove-Item $mgyDataDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Host (Get-Msg `
            "✅ 已彻底删除配置与凭据目录: $mgyDataDir" `
            "✅ Removed config & credentials directory: $mgyDataDir" `
            "✅ 設定・認証情報ディレクトリを削除しました: $mgyDataDir") -ForegroundColor Green
    } else {
        Write-Host (Get-Msg `
            "💡 已保留配置与凭据目录: $mgyDataDir (-KeepConfig)" `
            "💡 Keeping config & credentials directory: $mgyDataDir (-KeepConfig)" `
            "💡 設定・認証情報ディレクトリを保持しました: $mgyDataDir (-KeepConfig)") -ForegroundColor Gray
    }
}

# 6. 清理源码工程构建产物 bin/ 目录（如存在）
if ($PSScriptRoot) {
    $repoRoot = Split-Path -Parent $PSScriptRoot
    $projectBin = Join-Path $repoRoot "bin"
    if (Test-Path $projectBin) {
        Remove-Item $projectBin -Recurse -Force -ErrorAction SilentlyContinue
        Write-Host (Get-Msg `
            "✅ 已清理工程构建产物: $projectBin" `
            "✅ Cleaned project build output: $projectBin" `
            "✅ プロジェクトのビルド成果物を削除しました: $projectBin") -ForegroundColor Green
    }
}

Write-Host ("`n" + (Get-Msg `
    "🎉 Multigravity (mgy) 及 Cloudflare 组件已彻底从本机移除！" `
    "🎉 Multigravity (mgy) and Cloudflare components completely uninstalled!" `
    "🎉 Multigravity (mgy) および Cloudflare コンポーネントのアンインストールが完了しました！")) -ForegroundColor Yellow
Write-Host "==================================================" -ForegroundColor Cyan
