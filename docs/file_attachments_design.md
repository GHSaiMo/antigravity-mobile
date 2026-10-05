# 文件附件与系统分享设计

目标：让 iOS / Android 客户端除图片外，还能把常用文件（办公文档、压缩包、代码等）作为附件发给会话；并支持从微信、Files、其他应用通过「分享 / 用其他应用打开」把文件送进某个会话（或新建会话草稿）。

## 1. 现状

- 图片以 base64 内联在 `SendUserCascadeMessage`（`images` / `media`），网关限制 50MB（`proxy_cascade_handlers.go`）。
- Android 只有 `GetMultipleContents` 图片选择；iOS "+" 直接打开 ZLPhotoPicker。
- 两端都没有入站分享（Android manifest 只有 deep link；iOS 无 entitlements / App Group）。
- `IsSafeFilePath`（`files.go`）把 `/.multigravity/` 列为敏感目录，因此上传文件不能放在那里。

## 2. 核心决策

1. 文档走「上传到电脑 + 路径引用」，不走 base64 内联。网关与 Antigravity 在同一台机器，agent 可直接读本地文件。图片链路保持不变。
2. 两步走：选中后立即后台上传得到 `attachmentId`；发送消息时只带 id。上传未完成时发送置灰。
3. 引用块由网关拼装：客户端发 `attachments:[{id}]`，网关解析为路径并追加到 `text` 与 `items[0].text`，随后删除该字段再转发。`needsModification` 与去重哈希必须把 `attachments` 算进去。
4. 入站分享统一成「发送到…」目标页：顶部「新建会话草稿」，下面是最近会话。选定后文件进入该会话草稿并聚焦输入框，不自动发送。

## 3. 网关

`POST /api/v1/attachments`（经现有鉴权中间件）：

- 原始流请求体。头：`X-File-Name`（URL 编码）、`Content-Type`、可选 `X-Cascade-Id`。
- 边收边算 sha256，`MaxBytesReader` 限长，先写临时文件再 rename；相同 sha256 去重。
- 返回 `{id, path, name, size, mime, sha256, kind}`。
- 存储：`~/Multigravity/Inbox/yyyy-MM/<sha8>-<原文件名>`；目录 0700，文件 0600，无执行位。

限制与校验：

| 项 | 值 |
|---|---|
| 单文件 | 50MB（`MGY_MAX_UPLOAD_MB`） |
| 单条消息 | 最多 5 个、合计 100MB |
| 类型 | 扩展名白名单 + 魔数黑名单（MZ / ELF / Mach-O / dex） |
| 文件名 | 去路径成分、过滤控制与 RTL 覆盖字符、限长、`O_EXCL` |
| 清理 | 默认 30 天 TTL + 磁盘配额 |
| 其他 | 审计、限速；经 Cloudflare 隧道时请求体上限 100MB |

白名单：doc/docx/xls/xlsx/ppt/pptx/pdf/md/txt/csv/rtf/json/odt/ods/odp，zip/tar/gz/tgz/7z，常见源码与配置（py js ts tsx jsx go rs java kt swift c cpp h cs rb php sh sql json yaml yml toml xml html css 等）。

## 4. 客户端

### 4.1 "+" 面板

- Android：`ModalBottomSheet(skipPartiallyExpanded=true)` + `TallSheetBody` + `SheetGrabHandle`，与设置 / 额度 / 新建会话页一致。
- iOS：`.sheet` + `.presentationDetents([.large])` + `.presentationDragIndicator(.visible)`，与 `NewConversationSheet` 一致。

```
┌────────────────────────────┐
│          ▬ 拖拽条           │
│ 取消     最近项目    进入相册 > │
│ ┌────┐ ┌───┐ ┌───┐ ┌───┐   │
│ │ 📷 │ │ ○ │ │ ○ │ │ ○ │   │  最近照片网格，第一格相机
│ │相机│ ├───┤ ├───┤ ├───┤   │
│ └────┘ │ ○ │ │ ○ │ │ ○ │   │
├────────────────────────────┤
│ 📎 添加文件                 │  固定底部横条
│        [ 添加 (3) ]         │  选了照片后出现
└────────────────────────────┘
```

- iOS：自建 PhotoKit 网格（ZLPhotoBrowser 嵌不进 sheet）；`.limited` 授权时提示管理；「进入相册」打开现有 ZLPhotoPicker；相机复用 `CameraPickerView`；「添加文件」用 `.fileImporter`（`.item`，选完校验）。
- Android：MediaStore + Coil；`READ_MEDIA_IMAGES` / `READ_MEDIA_VISUAL_USER_SELECTED`；权限被拒时显示「允许访问照片」格，其余入口仍可用；相机用 `TakePicture` + `FileProvider`；「添加文件」用 `OpenMultipleDocuments(*/*)`，选完按扩展名校验。
- 选择器放开类型，被拒文件 toast 说明原因。

### 4.2 附件条、草稿、气泡

- 输入框上方：图片缩略图与文件 chip 混排（图标 + 名称 + 大小 + 进度环 + ✕），图标复用 `FileIconResolver`。
- 数据模型：先并行新增 `selectedFiles`，不重构 `selectedImages`。
- 草稿：文件拷到 App 私有目录 `drafts/<cid>/`，草稿存元数据；`local_draft_*` 升级为真实 cascadeId 时迁移目录；发送成功 / 删除草稿 / 7 天过期清理。
- 消息气泡与排队消息卡：识别 Inbox 路径行，渲染文件卡片，点击进入现有 QuickLook / `DocumentPreviewSheet`。

## 5. 入站分享

### Android

- 独立半透明 `ShareReceiverActivity`；intent-filter：`SEND` / `SEND_MULTIPLE` / `VIEW`，MIME 为办公类 + 压缩包 + `text/*` + `image/*` + `application/octet-stream` 兜底，之后按扩展名校验。
- 微信「用其他应用打开」是 `VIEW` + `content://`，读权限临时，必须在 `onCreate` 内同步拷到 cache。
- 随后弹「发送到…」，写入草稿，跳转 MainActivity。后续可加 Direct Share 快捷方式。

### iOS

- 保底档（不需要 App Group）：`CFBundleDocumentTypes`（`LSSupportsOpeningDocumentsInPlace=false`，md 需 `UTImportedTypeDeclarations`）+ `onOpenURL`（`url.isFileURL` 与 `multigravity://` 区分），立即拷出并删除原文件。
- 增强档：Share Extension + App Group `group.com.taojiuzhen.antigravity`；extension 流式拷入共享容器，用会话快照做目标选择，主 App 在 `scenePhase == .active` 时消费「待处理分享」。唤起主 App 仅尽力而为，兜底本地通知。
- 免费开发者账号是否支持 App Groups 需在 Xcode 实测；若不支持，只做保底档。

## 6. 实施阶段

| 阶段 | 内容 | 验收 |
|---|---|---|
| 0 验证 | 见 §7 | 结论写入本文档 |
| 1 网关 | 上传接口、引用拼装、清理、测试 | `go test` + `scripts/test-on-nas.sh` |
| 2 "+" 面板 | 两端面板 + 文件选择 + 附件条 + 草稿 + 气泡 | `compileReleaseKotlin` / `xcodebuild` |
| 3 Android 入站 | ShareReceiverActivity + 目标选择页 | 真机从微信分享 |
| 4 iOS 入站 | Open-in（保底）→ Share Extension（视 App Group） | 真机验证 |
| 5 增强 | Direct Share、Office 文本提取、Web 控制台上传 | — |

## 7. 阶段 0 待验证

1. agent 能否直接读 Inbox 文件，非工作区路径是否触发权限提示（若会，改存工作区 `.mgy-inbox/`）。
2. 桌面端 @ 文件时上游实际发送的结构；若 `items` 有原生 file 项，优先使用。
3. 真机抓取微信在两个平台发出的 intent / UTI。

### 阶段 0 实测结果（2026-10-05，本机网关 v1.0.5 + 真实 agent）

- 问题 1：通过。纯对话会话里，agent 直接读取了 `~/Multigravity/Inbox/` 下的 `.md` 并正确回复文件内容，未弹出权限提示。**尚未验证绑定工作区的会话**，若那里会弹提示，再改存工作区 `.mgy-inbox/`。
- 问题 2：未验证。当前方案依赖路径文本，实测可用，暂不需要原生 file 项。
- 问题 3：未验证（需真机）。
- 网关链路：上传 / 去重 / 类型拒绝 / 伪装可执行文件拒绝 / 50MB+1 返回 413 / 未配对 401 / 路径穿越与未知 id 返回 400，均符合预期。
- 注意：请求里不带 `model` 时，上游会报 `plan model not specified`；真实客户端始终带 model，无需处理。

## 8. 测试矩阵

文件类型 × 来源（Files、iCloud 未下载、微信、邮件）× 大小（0 字节 / 50MB / 51MB）；中文 / 空格 / emoji 文件名；上传中断网；分享后 App 被杀；agent 运行中排队发送；草稿升级为真实会话；伪装成文档的可执行文件被拒。

## 9. 交付门禁

遵循 `AGENTS.md`：Android `./gradlew compileReleaseKotlin`；iOS `xcodebuild`；Go `go test -count=1 ./...` + `go build`，推送前 `scripts/test-on-nas.sh`；重启网关一律在 tmux `mgy` 会话。

## 10. 演示模式（Demo Mode）

目的：App Store / Google Play 审核员没有桌面网关，需要一个无需配对即可体验全部功能的入口；开发时也可在模拟器里直接验证 UI，不依赖真实网关。

- 入口：欢迎页「没有安装网关？体验演示模式」；设置页里的「解除配对」在演示模式下变为「退出演示」。
- iOS 实现：`Services/Demo/DemoGateway.swift`（内存数据、脚本化回复、模拟上传存储）+ `DemoURLProtocol.swift`（拦截发往 `127.0.0.1:9` 的请求）。`AppSettings.isDemoMode` 为真时 `serverURL` 指向该地址，`isPaired` 视为已配对；WebSocket 改为 HTTP 轮询；文件下载（后台 session 不走 URLProtocol）在 `APIClient.downloadFile` 里直连演示网关。
- 演示数据：3 个会话（含带附件卡片的消息）、2 个项目、额度；发送消息后先显示运行中 + 工具步骤，约 3 秒后给出示例回复；上传校验沿用 `AttachmentRules`。
- 待办：Android 端同样实现（OkHttp Interceptor 拦截演示地址）；提审时在 App Review Notes 写明「欢迎页点击『体验演示模式』即可体验，无需账号」。
