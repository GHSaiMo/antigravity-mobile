# Multigravity Windows 一键构建与安装脚本
$ErrorActionPreference = "Stop"

# 1. 强制控制台与输出流使用 UTF-8 编码，彻底防止 Windows PowerShell 默认 GBK 导致中文乱码
try {
    [Console]::InputEncoding = [System.Text.Encoding]::UTF8
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    $OutputEncoding = [System.Text.Encoding]::UTF8
} catch {}

Write-Host "==================================================" -ForegroundColor Cyan
Write-Host "  📱 Multigravity (mgy) - Windows 一键安装与构建  " -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Cyan

# 2. 查找或准备 Go 环境
$goCmd = "go"
if (!(Get-Command go -ErrorAction SilentlyContinue)) {
    if (Test-Path "$HOME\go-sdk\go\bin\go.exe") {
        $goCmd = "$HOME\go-sdk\go\bin\go.exe"
        $env:PATH = "$HOME\go-sdk\go\bin;$env:PATH"
    } else {
        Write-Host "⚠️ 未检测到系统 Go 环境，正在下载便携式 Go SDK..." -ForegroundColor Yellow
        $sdkDir = "$HOME\go-sdk"
        if (!(Test-Path $sdkDir)) { New-Item -ItemType Directory -Path $sdkDir -Force | Out-Null }
        $zipFile = "$sdkDir\go.zip"
        curl.exe -L -o $zipFile "https://dl.google.com/go/go1.22.10.windows-amd64.zip"
        if (Get-Command tar.exe -ErrorAction SilentlyContinue) {
            tar.exe -xf $zipFile -C $sdkDir
        } else {
            Expand-Archive -Path $zipFile -DestinationPath $sdkDir -Force
        }
        Remove-Item $zipFile -Force
        $goCmd = "$sdkDir\go\bin\go.exe"
        $env:PATH = "$sdkDir\go\bin;$env:PATH"
    }
}

Write-Host "✅ 编译器就绪: $(& $goCmd version)" -ForegroundColor Green

# 3. 编译项目
$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

$binDir = "$repoRoot\bin"
if (!(Test-Path $binDir)) { New-Item -ItemType Directory -Path $binDir -Force | Out-Null }

Write-Host "🔨 正在编译 Windows 单体二进制包 (bin\mgy.exe)..." -ForegroundColor Cyan
& $goCmd build -ldflags="-s -w -X 'main.Version=1.0.0'" -o "$binDir\mgy.exe" ./cmd/gateway

if (Test-Path "$binDir\mgy.exe") {
    $size = (Get-Item "$binDir\mgy.exe").Length / 1MB
    Write-Host ("🎉 构建成功: bin\mgy.exe ({0:N1} MB)" -f $size) -ForegroundColor Green
} else {
    Write-Host "❌ 构建失败，未能生成 bin\mgy.exe" -ForegroundColor Red
    exit 1
}

# 4. 安装到系统用户 PATH，支持全局直接键入 mgy 命令
# 如果旧版本 mgy 正在运行，先优雅关闭以释放文件锁
Get-Process -Name mgy -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 200

$installDir = "$HOME\.local\bin"
if (!(Test-Path $installDir)) { New-Item -ItemType Directory -Path $installDir -Force | Out-Null }
Copy-Item -Path "$binDir\mgy.exe" -Destination "$installDir\mgy.exe" -Force

# 确保 ~/.local/bin 写入用户环境变量 PATH
try {
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$installDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
    }
} catch {}

# 同步安装到 WindowsApps（Windows 默认免配置全局 PATH，无需重启当前终端即可秒级生效）
$windowsApps = "$env:LOCALAPPDATA\Microsoft\WindowsApps"
if (Test-Path $windowsApps) {
    try {
        Copy-Item -Path "$binDir\mgy.exe" -Destination "$windowsApps\mgy.exe" -Force
    } catch {
        Write-Warning "未能复制到 $windowsApps，请确保无其他进程正在使用 mgy.exe"
    }
}

# 刷新当前会话的 PATH 变量
$env:PATH = "$installDir;$windowsApps;$env:PATH"

Write-Host "📦 已安装至全局快捷指令目录: $installDir\mgy.exe" -ForegroundColor Green

# 提示运行
Write-Host "`n🚀 安装完成！现在您可以像在 Mac 上一样，在任何终端直接输入 mgy 指令:" -ForegroundColor Yellow
Write-Host "   mgy                   (前台启动主网关服务并生成配对二维码)" -ForegroundColor White
Write-Host "   mgy pair              (申请新配对码与 URI)" -ForegroundColor White
Write-Host "   mgy list              (查看已授权设备)" -ForegroundColor White
Write-Host "   mgy clear all         (清除所有设备授权)" -ForegroundColor White
Write-Host "   mgy --help            (查看完整参数帮助)" -ForegroundColor White
Write-Host "==================================================" -ForegroundColor Cyan
