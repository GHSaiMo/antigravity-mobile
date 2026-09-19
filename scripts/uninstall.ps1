# Multigravity (mgy) Windows 卸载与清理脚本
$ErrorActionPreference = "SilentlyContinue"

# 强制控制台输出 UTF-8
try {
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    $OutputEncoding = [System.Text.Encoding]::UTF8
} catch {}

Write-Host "==================================================" -ForegroundColor Cyan
Write-Host "  🗑️  Multigravity (mgy) - Windows 卸载与清理  " -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Cyan

# 1. 停止运行中的 mgy 进程
Get-Process -Name mgy -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 200

# 2. 删除 WindowsApps 全局快捷指令
$winAppMgy = Join-Path $env:LOCALAPPDATA "Microsoft\WindowsApps\mgy.exe"
if (Test-Path $winAppMgy) {
    Remove-Item $winAppMgy -Force -ErrorAction SilentlyContinue
    Write-Host "✅ 已删除 WindowsApps 快捷指令: $winAppMgy" -ForegroundColor Green
} else {
    Write-Host "ℹ️  WindowsApps 中未发现 mgy.exe" -ForegroundColor Gray
}

# 3. 删除 ~/.local/bin 中的 mgy.exe（保留用户的 uv/python 等其他工具）
$localBinMgy = Join-Path $HOME ".local\bin\mgy.exe"
if (Test-Path $localBinMgy) {
    Remove-Item $localBinMgy -Force -ErrorAction SilentlyContinue
    Write-Host "✅ 已删除用户执行文件: $localBinMgy" -ForegroundColor Green
} else {
    Write-Host "ℹ️  ~/.local/bin 中未发现 mgy.exe" -ForegroundColor Gray
}

# 4. 删除 ~/.multigravity 配置与授权凭据目录
$mgyDataDir = Join-Path $HOME ".multigravity"
if (Test-Path $mgyDataDir) {
    Remove-Item $mgyDataDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host "✅ 已删除数据与凭据目录: $mgyDataDir" -ForegroundColor Green
} else {
    Write-Host "ℹ️  未发现 ~/.multigravity 目录" -ForegroundColor Gray
}

# 5. 清理源码工程构建产物 bin/ 目录
$repoRoot = Split-Path -Parent $PSScriptRoot
$projectBin = Join-Path $repoRoot "bin"
if (Test-Path $projectBin) {
    Remove-Item $projectBin -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host "✅ 已清理工程构建产物: $projectBin" -ForegroundColor Green
}

# 6. 清理本次下载的便携式 Go SDK 与构建缓存
$goSdkDir = Join-Path $HOME "go-sdk"
if (Test-Path $goSdkDir) {
    Remove-Item $goSdkDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host "✅ 已清理便携式 Go SDK: $goSdkDir" -ForegroundColor Green
}

$goCacheDir = Join-Path $HOME "go"
if (Test-Path $goCacheDir) {
    Remove-Item $goCacheDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host "✅ 已清理 Go 构建缓存: $goCacheDir" -ForegroundColor Green
}

# 7. 从用户环境变量 PATH 中移除 go-sdk 路径
try {
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath) {
        $cleanItems = @()
        foreach ($p in ($userPath -split ';')) {
            $trimmed = $p.Trim()
            if ($trimmed -and ($trimmed -notlike "*\go-sdk\*") -and ($trimmed -notlike "*taoji\go\bin*")) {
                $cleanItems += $trimmed
            }
        }
        $newPath = $cleanItems -join ';'
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        Write-Host "✅ 已从用户环境变量 PATH 中移除 Go SDK 路径" -ForegroundColor Green
    }
} catch {
    Write-Host "⚠️  环境变量清理异常: $_" -ForegroundColor Yellow
}

Write-Host "`n🎉 Multigravity (mgy) 及相关环境配置已彻底从本机移除！" -ForegroundColor Yellow
Write-Host "==================================================" -ForegroundColor Cyan
