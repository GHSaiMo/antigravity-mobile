# 🚀 Multigravity v1.2.1

> 🎉 **Multigravity（反重力智能体移动伴侣）iOS 版现已正式上架 Apple App Store！**  
> 无需本地 Xcode 编译或自签，直接前往 👉 **[App Store 下载 Multigravity](https://apps.apple.com/app/id6835508934)**（海外 174 地区，中文系统设备自动优先展示简体中文商店页与展示图，并内置零配置**离线演示模式**）；如需第一时间抢先体验最新构建，也可通过 👉 **[TestFlight 公开测试通道](https://testflight.apple.com/join/YM7f4aSK)** 安装。

### ✨ 核心修复与重要更新

- 🌐 **设置页自定义与局域网网络保存体验全面升级（Android / iOS）**：
  - 新增显式交互「保存 / 已保存」状态按钮，未修改时展示「已保存」，输入变更时高亮触发「保存」；
  - 接入软键盘 `Done` / `回车` 快捷提交保存，并在设置面板导航栏右上角补充「完成」按钮，离开面板时自动持久化脏数据；
  - 自动为缺少协议头的自定义输入（如 `100.x.x.x:58900`）补齐 `http://` 规范化协议头，防止网络请求解析异常。
- 🐛 **修复底层误清除自定义网络地址的关键 Bug**：
  - 彻底修复 `PreferencesManager`（Android）与 `AppSettings`（iOS）在自定义网络连接成功或应用重启时，误将非局域网的活跃网关判定为旧残留而静默删除的 Bug；
  - 确保 Tailscale（`100.x.x.x` / `*.ts.net`）、WireGuard、私有 VPS、DDNS 等自定义直连通道在重连与重启后稳定留存。
- 🔌 **兼容 Cockpit Tools v1.4+ SQLite 账号索引（感谢 PR #6 `@iamtmac`）**：
  - 适配 Cockpit Tools v1.4+ 将账号索引从 `accounts.json` 迁移至 `antigravity_accounts.sqlite`（`summaries` / `account_index_state`）的存储变更，解决升级后手机端提示「未检测到 Cockpit Tools」及无法查看多账号额度、切换账号的问题；
  - 优先读取新版 SQLite 索引以兼容 v1.3 → v1.4+ 原地升级后残留的过期 `accounts.json`，无新库时平滑回退旧版索引；
  - `sqlite3` CLI 不可用或执行失败时自动以只读方式回退 Python `sqlite3`，并基于文件修改时间与大小做轻量缓存避免高频轮询反复创建子进程。
- 📦 **双端版本号与客户端对齐**：
  - 同步更新 Android 版本号至 `1.2.1`（Version Code `21`）；
  - 同步更新 iOS 版本号至 `1.2.1`（Project Version `20`）；
  - 网关 `mgy` 桌面与服务器端版本号同步升至 `1.2.1`。
