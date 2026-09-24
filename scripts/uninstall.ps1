# Multigravity (mgy) Windows 一键彻底卸载脚本
# 包含: 停止运行进程、卸载 mgy、卸载 cloudflared 穿透引擎、彻底清除配置与配对数据 (~/.multigravity)
param(
    [switch]$KeepConfig,
    [switch]$KeepCloudflared
)

$ErrorActionPreference = "SilentlyContinue"

# 强制控制台输出 UTF-8
try {
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    $OutputEncoding = [System.Text.Encoding]::UTF8
} catch {}

Write-Host "==================================================" -ForegroundColor Cyan
Write-Host "  🗑️  Multigravity (mgy) - Windows 彻底卸载与清理  " -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Cyan

# 1. 停止运行中的 mgy 进程
$mgyProc = Get-Process -Name mgy -ErrorAction SilentlyContinue
if ($mgyProc) {
    Write-Host "⏹️  正在停止运行中的 mgy 进程..." -ForegroundColor Yellow
    $mgyProc | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 300
}

# 2. 停止运行中的 cloudflared 进程（仅当未指定 -KeepCloudflared 时）
if (-not $KeepCloudflared) {
    $cfProc = Get-Process -Name cloudflared -ErrorAction SilentlyContinue
    if ($cfProc) {
        Write-Host "⏹️  正在停止运行中的 cloudflared 穿透进程..." -ForegroundColor Yellow
        $cfProc | Stop-Process -Force -ErrorAction SilentlyContinue
        Start-Sleep -Milliseconds 300
    }
}

# 3. 删除 mgy.exe 执行文件与系统快捷方式
$winAppMgy = Join-Path $env:LOCALAPPDATA "Microsoft\WindowsApps\mgy.exe"
if (Test-Path $winAppMgy) {
    Remove-Item $winAppMgy -Force -ErrorAction SilentlyContinue
    Write-Host "✅ 已删除 WindowsApps 快捷指令: $winAppMgy" -ForegroundColor Green
}

$localBinMgy = Join-Path $HOME ".local\bin\mgy.exe"
if (Test-Path $localBinMgy) {
    Remove-Item $localBinMgy -Force -ErrorAction SilentlyContinue
    Write-Host "✅ 已删除 mgy 执行文件: $localBinMgy" -ForegroundColor Green
}

# 4. 删除 cloudflared 穿透引擎
if (-not $KeepCloudflared) {
    $localBinCf = Join-Path $HOME ".local\bin\cloudflared.exe"
    if (Test-Path $localBinCf) {
        Remove-Item $localBinCf -Force -ErrorAction SilentlyContinue
        Write-Host "✅ 已删除 Cloudflare 引擎: $localBinCf" -ForegroundColor Green
    }

    $cfEmbedded = Join-Path $HOME ".multigravity\bin\cloudflared.exe"
    if (Test-Path $cfEmbedded) {
        Remove-Item $cfEmbedded -Force -ErrorAction SilentlyContinue
        Write-Host "✅ 已删除 Cloudflare 引擎: $cfEmbedded" -ForegroundColor Green
    }
} else {
    Write-Host "💡 跳过 Cloudflare 引擎移除 (-KeepCloudflared)" -ForegroundColor Gray
}

# 5. 删除 ~/.multigravity 配置与授权凭据目录
$mgyDataDir = Join-Path $HOME ".multigravity"
if (Test-Path $mgyDataDir) {
    if (-not $KeepConfig) {
        Remove-Item $mgyDataDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Host "✅ 已彻底删除配置与凭据目录: $mgyDataDir" -ForegroundColor Green
    } else {
        Write-Host "💡 已保留配置与凭据目录: $mgyDataDir (-KeepConfig)" -ForegroundColor Gray
    }
}

# 6. 清理源码工程构建产物 bin/ 目录（如存在）
if ($PSScriptRoot) {
    $repoRoot = Split-Path -Parent $PSScriptRoot
    $projectBin = Join-Path $repoRoot "bin"
    if (Test-Path $projectBin) {
        Remove-Item $projectBin -Recurse -Force -ErrorAction SilentlyContinue
        Write-Host "✅ 已清理工程构建产物: $projectBin" -ForegroundColor Green
    }
}

Write-Host "`n🎉 Multigravity (mgy) 及 Cloudflare 组件已彻底从本机移除！" -ForegroundColor Yellow
Write-Host "==================================================" -ForegroundColor Cyan
