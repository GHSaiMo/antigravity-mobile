# Multigravity Windows 一键卸载脚本
$ErrorActionPreference = "SilentlyContinue"

try {
    [Console]::InputEncoding = [System.Text.Encoding]::UTF8
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    $OutputEncoding = [System.Text.Encoding]::UTF8
} catch {}

Write-Host "==================================================" -ForegroundColor Cyan
Write-Host "🗑️  正在卸载 Multigravity (mgy) for Windows...  " -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Cyan

# 1. 停止运行中的进程
Get-Process -Name mgy -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 200

# 2. 移除二进制
$targets = @(
    "$HOME\.local\bin\mgy.exe",
    "$HOME\.local\bin\mgy",
    "$env:LOCALAPPDATA\Microsoft\WindowsApps\mgy.exe"
)

foreach ($target in $targets) {
    if (Test-Path $target) {
        Remove-Item -Path $target -Force
        Write-Host "✅ 已移除二进制: $target" -ForegroundColor Green
    }
}

# 3. 检查并可选清理数据目录
$confDir = "$HOME\.multigravity"
if (Test-Path $confDir) {
    if ($args[0] -eq "--all" -or $args[0] -eq "-a") {
        Remove-Item -Path $confDir -Recurse -Force
        Write-Host "✅ 已彻底清除数据与配置目录: $confDir" -ForegroundColor Green
    } else {
        Write-Host "💡 保留了数据与配置目录: $confDir" -ForegroundColor Yellow
        Write-Host "   (若需彻底清除配置与配对数据，可执行: Remove-Item -Recurse -Force ~\.multigravity)" -ForegroundColor Gray
    }
}

Write-Host "==================================================" -ForegroundColor Cyan
Write-Host "🎉 Multigravity 卸载完成！" -ForegroundColor Green
Write-Host "==================================================" -ForegroundColor Cyan
