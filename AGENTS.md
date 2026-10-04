# AGENTS.md - Multigravity Agent 开发与交付规范

本项目为 Multigravity 多端协同项目，覆盖 Go 服务端网关、iOS 原生客户端、Android 原生客户端与 Web 控制台。为了防止低级语法、引用或类型错误泄露到远程 CI/CD 导致构建失败，所有 Agent 在执行开发任务时必须严格遵守以下规范与验收标准。

---

## 🚨 核心验收门禁 (Mandatory Acceptance Criteria)

### 1. Android 模块代码修改（强制门禁 ⭐⭐⭐⭐⭐）
**凡是修改了 `android/` 目录下的任何代码（包括 Kotlin 源码、Compose UI、资源 XML、Gradle 配置等），在向用户交付、声称任务完成或提交 Git 之前，必须在本地终端执行编译校验：**

```bash
# 进入 android 目录并执行 release 纯 Kotlin 编译校验
cd android && ./gradlew compileReleaseKotlin
```

- **通过标准**：终端输出 `BUILD SUCCESSFUL`，退出码为 `0`。
- **严禁行为**：
  - 严禁在未经过本地 `./gradlew compileReleaseKotlin` 校验的情况下直接通知用户已完成开发或直接 git push。
  - 严禁凭经验臆造不存在的 ViewModel 方法、不存在的资源 ID 或错误的类型转换。
  - 严禁随意更改 Release 签名配置或删除签名文件，避免造成用户无法覆盖安装。
- **全量打包验证（可选/发布前推荐）**：
  ```bash
  cd android && ./gradlew assembleRelease
  ```

### 2. Android Release 签名一致性规范（强制保持 ⭐⭐⭐⭐⭐）
为确保用户升级客户端时可以**无缝直接覆盖安装**，避免因签名变更导致 Android 系统报 `INSTALL_FAILED_UPDATE_INCOMPATIBLE` 而被迫卸载，项目已配置永久固定的 Release 签名密钥库，所有 Agent 必须严格遵守并保持签名一致：

- **密钥库文件**：`android/app/release.jks`（已纳入 Git 仓库管理）
- **Key Alias**：`multigravity`
- **默认口令**：`antigravity`（可通过环境变量 `MGY_KEYSTORE_PASSWORD` / `MGY_KEY_ALIAS` / `MGY_KEY_PASSWORD` 覆盖）
- **证书所有者**：`CN=Multigravity, OU=Mobile, O=Multigravity, C=CN`
- **有效期**：至 2054 年 2 月（28 年）
- **证书 SHA-256 指纹**：
  `A1:B0:17:A4:1F:61:1D:BE:B8:3C:97:65:B2:D3:BC:3D:34:70:EF:74:D7:6B:58:7F:D6:81:E9:7C:EC:D4:29:00`
- **验证命令**：
  ```bash
  # 校验构建出的 APK 签名指纹
  ~/Library/Android/sdk/build-tools/34.0.0/apksigner verify --print-certs android/app/build/outputs/apk/release/app-release.apk
  ```
- **核心红线**：
  - 严禁将 `build.gradle.kts` 中 `buildTypes.release` 的签名改回临时生成的 `debug` 签名或随意替换 keystore！
  - CI/CD（GitHub Actions `release.yml`）必须统一使用该固定签名。

### 3. iOS 模块代码修改
凡是修改了 `ios/` 目录下的 Swift 源码或工程配置，必须使用 `xcodebuild` 进行本地编译验证：
```bash
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcodebuild \
  -project ios/Multigravity.xcodeproj \
  -scheme Multigravity \
  -destination 'generic/platform=iOS' \
  clean build CODE_SIGN_IDENTITY="" CODE_SIGNING_REQUIRED=NO CODE_SIGNING_ALLOWED=NO
```

### 4. Go 服务端网关代码修改
凡是修改了 `cmd/`、`internal/` 目录下的 Go 代码，必须执行测试与编译：
```bash
go test -count=1 ./...
go build -o /dev/null ./cmd/gateway
```

### 5. Go 代码的 Linux 环境验证（对齐 CI，强制门禁 ⭐⭐⭐⭐）
本机是 macOS，而 GitHub CI 跑在 ubuntu 上。路径约定（如 `Library/Application Support` 与 `~/.config`）、符号链接（`/tmp`、`/var` → `/private/...`）、可用的系统工具等平台差异，会造成“本机全绿、CI 变红”。因此凡是修改了 `cmd/`、`internal/`、`web/` 下的 Go 代码或测试，在 `git push` 之前，除第 4 节外还必须在 Linux 环境复现 CI 的校验：

```bash
# 通过 ssh 在 nas 上的一次性 Linux 容器内执行 go vet + go build + go test（对齐 .github/workflows/ci.yml）
scripts/test-on-nas.sh

# 只复现某个包 / 某个用例（仍会先跑全量 vet 与 build）
scripts/test-on-nas.sh ./internal/cockpit
scripts/test-on-nas.sh -run TestEnsureAntigravityStateDBs -v ./internal/cockpit
```

- **通过标准**：脚本末尾输出 `✅ Linux 验证通过`，退出码为 `0`。
- **同步内容**：脚本同步的是本地工作区（含未提交改动），不含 `android/`、`ios/` 等与 Go 无关的目录，因此可以在 `git commit` 之前运行。
- **环境说明**：脚本通过 NAS 上的 Go 模块代理（默认 `http://127.0.0.1:10001`，容器 `dev-goproxy`）下载与 `go.mod` 版本一致的 Go 工具链和依赖，缓存在 NAS 的 `/tmp/mgy-ci`。测试在一次性容器里以普通用户 + 临时 HOME 运行，并把 NAS 宿主机（Debian 12）的 `/usr`、`/lib`、`/bin` 以**只读**方式挂入，因此带有 `sqlite3`、`lsof`、`python3`、`gcc`、`ss`，也启用 cgo，更接近 GitHub 的 ubuntu runner；不会写入宿主机系统目录，也不影响 NAS 上的其他服务。可用环境变量覆盖：`NAS_HOST`、`NAS_WORKDIR`、`NAS_GOPROXY`、`NAS_IMAGE`、`NAS_CGO`（详见脚本头部注释）。
- **已知局限**：该环境不是 GitHub runner 的完整镜像（发行版、系统库版本、预装软件仍有差异），**Linux 验证通过不等于 CI 一定通过**，推送后仍应核对 CI 结果。曾出现过的教训：依赖外部命令的测试（如用 `sqlite3` 的用例）在缺少该命令的环境里会被 `t.Skip`，从而漏掉只在 CI 上才暴露的失败；因此验证环境必须保证这些命令存在，且不要让测试在 CI 上悄悄被跳过。
- **严禁行为**：
  - 严禁在脚本无法执行（例如 `ssh nas` 不通，脚本会以退出码 `3` 提示）时声称“Linux 验证已通过”；此时必须明确告知用户该项未验证。
  - 严禁用 `t.Skip`、删除用例或放宽断言来“绕过”Linux 上的失败；应让测试按平台取路径或按能力跳过，并说明理由。
  - 测试里不得写死 macOS 专属路径或依赖本机真实的 `~/.gemini`、正在运行的 Antigravity；需要 HOME 时使用 `t.Setenv("HOME", t.TempDir())`，并避免让断言依赖临时目录的具体路径。

### 6. 本机网关运行与拉起规范（强制遵守 ⭐⭐⭐⭐⭐）
修改完代码，本机重新拉起网关时，**必须在 tmux 对应的会话（会话名：`mgy`）里重新拉起**，严禁在 Agent 后台以独立子进程/守护进程方式直接拉起，避免端口冲突与会话脱节：

```bash
# 1. 优雅终止已有网关实例（若仍在运行）
tmux send-keys -t mgy C-c

# 2. 在 tmux 对应会话中重新拉起网关
tmux send-keys -t mgy "mgy" Enter
```

- **核心红线**：
  - 严禁通过 `run_command` (IsDaemon=true) 直接在 Agent 环境中独立拉起 `mgy` 常驻，避免导致用户在终端 attached 的 `mgy` 会话中执行时报 `bind: address already in use` 端口冲突。
  - 本机常驻与调试统一归属 `mgy` tmux 会话。

---

## 🛠️ 本机 Android 开发与验证环境

本机已配置 Google 官方轻量级无头构建环境，无需启动庞大的 Android Studio 即可秒级验证：

- **JDK 17**：`~/Library/Java/JavaVirtualMachines/jdk-17.0.20.1+1/Contents/Home`
- **Android SDK**：`~/Library/Android/sdk`
  - 平台：`platforms;android-34`
  - 构建工具：`build-tools;34.0.0`
  - 命令行工具：`cmdline-tools/latest/bin`（提供 `sdkmanager`、`lint` 等）
- **Gradle Wrapper**：项目自带 `android/gradlew`，直接调用 `./gradlew <task>` 即可自动对齐 Gradle 9.7.1，无需全局安装 Gradle。

---

## 📋 典型开发工作流

1. **需求理解与设计**：仔细研读用户要求与交互规范。
2. **代码编写**：修改对应的业务逻辑与组件。
3. **本地编译校验（验收红线）**：
   - Android: `cd android && ./gradlew compileReleaseKotlin`
   - iOS: `xcodebuild ...`
   - Go: `go test ./...`
   - Go（推送前，对齐 CI）: `scripts/test-on-nas.sh`
4. **提交与推送**：只有当上述对应模块的本地编译校验全部成功后，方可进行 `git commit` 与后续交付；涉及 Go 代码的推送还需通过 Linux 验证（第 5 节），推送后核对 CI 结果。
5. **本机网关重启**：交付或验证需要重新拉起网关时，一律通过 `tmux send-keys -t mgy "mgy" Enter` 在 `mgy` 会话中拉起。
