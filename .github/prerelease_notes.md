> 🩹 **定向热修复预发布版 / Targeted Hotfix Pre-release (`v1.2.9-hotfix.1`)**
>
> - 本版本基于 **`v1.2.8`** 仅合入 **4 项纯网关（`mgy`）热修复与增强补丁**，用于供遇到相关问题或需要 IPv6 直连的用户提前定向升级验证；**不会推送给正式版全量用户**（普通 `mgy update`、手机端「宿主机更新」、Android 应用内自动更新及一键安装脚本均自动跳过，`v1.2.8` 仍保持为 `Latest` 正式版）。
> - **无需更新 iOS / Android 手机端**，只需在电脑终端执行一行升级网关即可生效（若之前已装过早期 `v1.2.9-hotfix.1`，加 `-force` 覆盖刷新）：
>   ```bash
>   mgy update v1.2.9-hotfix.1
>   # 已安装早期 hotfix.1 时：mgy update -force v1.2.9-hotfix.1
>   ```
> - **回退与后续升级**：如需退回 `v1.2.8` 正式版，执行 `mgy update v1.2.8`；未来 `v1.2.9` 正式版发布后，执行 `mgy update` 或通过手机端「宿主机更新」即可自动检测并无缝升级。
>
> ---
>
> - This is a **targeted Pre-release hotfix** built directly on top of **`v1.2.8`** containing **4 gateway-only (`mgy`) fixes & enhancements** for affected users and IPv6 direct-connect users to verify early. It **will not be pushed to stable users** (`mgy update`, mobile Host Updates, Android in-app updater, and one-line install scripts ignore pre-releases; `v1.2.8` remains `Latest`).
> - **No mobile app update is required** — simply run one command on your computer to install the hotfix gateway (add `-force` if refreshing an earlier `v1.2.9-hotfix.1` install):
>   ```bash
>   mgy update v1.2.9-hotfix.1
>   # Refreshing an earlier hotfix.1 install: mgy update -force v1.2.9-hotfix.1
>   ```
> - **Rollback & future upgrades**: Run `mgy update v1.2.8` at any time to roll back to `v1.2.8`. When stable `v1.2.9` is released, `mgy update` and mobile Host Updates will detect and upgrade seamlessly.

---

## 🇨🇳 简体中文更新内容（基于 `v1.2.8`）

1. **修复聊天中点击相对路径 / 根路径文档（如 `AGENTS.md`）报 `access to file is restricted`**
   - 网关文件预览与下载接口（`/api/v1/files/content`、`/api/v1/files/raw` 及微信归档预览）在收到裸文件名（`AGENTS.md`）、相对路径（`./AGENTS.md`、`docs/guide.md`）或仓库根路径写法（`/AGENTS.md`、`file:///AGENTS.md`）时，在未命中 `brain/<cascade_id>/` 后改为按当前会话（`cascade_id`）关联的工作区目录解析（支持 `#L10-L20` / `:42` 行号后缀与离线本地会话库兜底）。
   - 不再将带会话 ID 的相对路径回落到 `mgy` 进程自身的启动工作目录（CWD），避免后台服务启动于 `/` 或系统目录时误报越权，同时保持目录穿越、敏感目录/文件（`.env`、`.ssh/`、`.codex/` 等）、软链逃逸与 `ALLOWED_WORKSPACE_ROOTS` 安全边界不变，并将越权错误状态码修正为 HTTP `403`。

2. **网关自动兜底升级开头纯文本 `/compact` 等系统斜杠指令**
   - 当客户端发送纯文本开头的已知上游系统斜杠指令（如 `/compact`、`/plan ...`）而未附带结构化 `slashCommand` 节点时，网关自动将其升级为上游原生的结构化 `slashCommand` 对象，确保 `v1.2.8` 手机端或手动输入 `/compact` 即可直接触发原生上下文压缩（文件路径如 `/Users/...` 与未知指令不受影响）。

3. **修复 Windows 传统控制台（conhost）二维码溢出无法扫描**
   - 修复 Windows 传统控制台（尤其使用中文新宋体 `NSimSun` 时）渲染 `U+2584` 下半块字符横向溢出到相邻空白格导致配对二维码错位、扫码失败的问题，自动适配安全的等宽全块渲染与控制台检测。

4. **新增可选 IPv6 双栈监听与原生 HTTPS 独立直连端口（默认关闭，零影响现网）**
   - 新增 `-ipv6` 启动参数及环境变量 `MULTIGRAVITY_ENABLE_IPV6=1`（别名 `MGY_ENABLE_IPV6=1`，`mgy service install -ipv6` 同步支持）；**默认保持纯 IPv4 (`tcp4`) 不变**，开启后切换为 IPv4/IPv6 双栈监听（也支持通过 `-host` 直接绑定 `[::]`、`::1` 或指定 IPv6 地址），供具备 IPv6 的用户搭配 DDNS + 反代（Lucky / Caddy / Nginx）或 Tailscale/私网 IPv6 直连。
   - 新增可选原生 HTTPS 独立监听参数 `-tls-cert` / `-tls-key` / `-tls-port`（环境变量 `MGY_TLS_CERT` / `MGY_TLS_KEY` / `MGY_TLS_PORT`，默认独立端口 `58943`，支持 TLS 1.2+ 与 ACME 证书自动热重载）：单进程即可免反代直出 HTTPS，同时保持原 `58900` HTTP 端口兼容本地 CLI（`mgy pair`/`list`/`clear`）与 Wi-Fi 局域网访问；路由器 IPv6 防火墙只需放行 `58943`，手机端在「设置 → 网络连接 → 自定义地址」填入 `https://你的DDNS域名:58943` 即可优先直连（外出不可达时仍自动回退官方 E2EE 中继）。

---

## 🇺🇸 English Changelog (based on `v1.2.8`)

1. **Fix `access to file is restricted` when opening relative / repo-root document links (e.g. `AGENTS.md`) in chat**
   - Gateway file preview/raw endpoints (`/api/v1/files/content`, `/api/v1/files/raw`, and chat archive handlers) now resolve bare filenames (`AGENTS.md`), relative paths (`./AGENTS.md`, `docs/guide.md`), and repo-root links (`/AGENTS.md`, `file:///AGENTS.md`) against the active conversation's (`cascade_id`) bound workspace directory when not found under `brain/<cascade_id>/`, with support for `#L10-L20` / `:42` line locators and offline local summary DB fallback.
   - Stops falling back to the `mgy` process CWD for conversation-scoped relative paths (preventing false `access to file is restricted` errors when launched from `/` or system directories), preserves all traversal/sensitive-path/symlink/`ALLOWED_WORKSPACE_ROOTS` protections, and returns HTTP `403` for restricted access.

2. **Gateway fallback upgrading leading plain-text `/compact` & system slash commands**
   - Automatically upgrades leading plain-text known upstream system commands (such as `/compact` or `/plan ...`) into structured `slashCommand` items on the gateway, enabling native compaction and system command execution even from existing `v1.2.8` mobile clients or manual text input without affecting Unix paths or unknown commands.

3. **Fix Windows conhost QR code overflow with `NSimSun` font**
   - Prevents `U+2584` lower-half-block glyphs from overflowing into adjacent space cells in Windows legacy `conhost` (notably with CJK `NSimSun`), keeping pairing QR codes reliably scannable.

4. **Add opt-in IPv6 dual-stack listening and optional dedicated native HTTPS port (off by default)**
   - Adds `-ipv6` / `MULTIGRAVITY_ENABLE_IPV6=1` (alias `MGY_ENABLE_IPV6=1`, also supported in `mgy service install -ipv6`). **Default remains IPv4-only (`tcp4`)**; when enabled (or when an explicit IPv6 `-host` such as `::` is supplied), `mgy` binds a dual-stack IPv4/IPv6 socket for direct IPv6 DDNS/reverse-proxy (Lucky/Caddy/Nginx) and Tailscale/private IPv6 access.
   - Adds optional `-tls-cert` / `-tls-key` / `-tls-port` (`MGY_TLS_CERT` / `MGY_TLS_KEY` / `MGY_TLS_PORT`, default port `58943`, TLS 1.2+ with automatic ACME certificate hot-reload) on a dedicated HTTPS port so local CLI (`mgy pair`/`list`/`clear`) and LAN HTTP on port `58900` remain unaffected. Expose only the TLS port in your IPv6 firewall and enter `https://<your-ddns-domain>:58943` in mobile **Settings → Custom Server Address** for direct HTTPS priority with automatic cloud E2EE relay fallback.
