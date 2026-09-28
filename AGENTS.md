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
- **全量打包验证（可选/发布前推荐）**：
  ```bash
  cd android && ./gradlew assembleRelease
  ```

### 2. iOS 模块代码修改
凡是修改了 `ios/` 目录下的 Swift 源码或工程配置，必须使用 `xcodebuild` 进行本地编译验证：
```bash
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcodebuild \
  -project ios/Multigravity.xcodeproj \
  -scheme Multigravity \
  -destination 'generic/platform=iOS' \
  clean build CODE_SIGN_IDENTITY="" CODE_SIGNING_REQUIRED=NO CODE_SIGNING_ALLOWED=NO
```

### 3. Go 服务端网关代码修改
凡是修改了 `cmd/`、`internal/` 目录下的 Go 代码，必须执行测试与编译：
```bash
go test -count=1 ./...
go build -o /dev/null ./cmd/gateway
```

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
4. **提交与推送**：只有当上述对应模块的本地编译校验全部成功后，方可进行 `git commit` 与后续交付。
