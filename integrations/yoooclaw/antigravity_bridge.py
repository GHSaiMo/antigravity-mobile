"""YoooClaw 物理硬件直连 Antigravity 智能体网桥模块.

功能:
1. 关键词判定: 识别 "新建对话/新对话/开个新对话" 等前置词；
2. 会话解析: 新建独立纯净会话 或 锁定当前活跃查看的会话；
3. 模型绑定: 自动从 antigravity_state.pbtxt 读取用户模型配置，防止 executor 报错；
4. 消息注入: 优先走 Antigravity Mobile Gateway (http://127.0.0.1:58900)，带 Admin Token 鉴权，
   自动实现 CSRF 补全、模型覆盖、SSE 缓存失效推送；若 Gateway 异常则安全回退 language_server 直连；
5. 硬件感知: 触发对应 RGB 灯效，并在手机/设备通知栏显式回显目标会话标题；
6. 客户端闭环: 拦截成功后向 YoooClaw App 回显投递状态并触发 run.complete，杜绝界面无限 pending。
"""

from __future__ import annotations

import asyncio
import glob
import json
import logging
import os
import re
import sqlite3
import ssl
import time
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Optional, Tuple

from yoooclaw_hermes.cli.runner import CliRunner

logger = logging.getLogger(__name__)

# 匹配“新建会话/对话/规划/任务”及语音识别(ASR)常见谐音词与口语表达
_PREFIX = r"(?:(?:开|建|起|重开|搞|弄|整|做)(?:一个|个)?新?|新建(?:一个|个)?|创建(?:一个|个)?|建立(?:一个|个)?|新开(?:一个|个)?|新起(?:一个|个)?|开启(?:一个)?新?|发起(?:一个)?新?|来(?:一个|个)?新?|全新|新|最近|最新|先进|新进|星舰|信件)"
_NOUN = r"(?:对话|会话|聊天|任务|规划|计划|主题|窗口|绘画|汇话|回话|对画|鬼话|案子|课题|讨论)"
_STANDALONE = r"(?:新开一个|新建一个|创建新会话|重新开始|重开一个|重新开一个|换个话题|另起一个|另起会话|另开一个)"

NEW_CASCADE_PATTERN = re.compile(
    rf"^(?:(?:{_PREFIX}{_NOUN})|{_STANDALONE})[，,：:\s]*(.*)$",
    re.IGNORECASE,
)

STATE_FILE = Path.home() / ".yoooclaw" / "routing_state.json"
ANNOTATIONS_DIR = Path.home() / ".gemini" / "antigravity" / "annotations"
BRAIN_DIR = Path.home() / ".gemini" / "antigravity" / "brain"
STATE_PBTXT = Path.home() / ".gemini" / "antigravity" / "antigravity_state.pbtxt"
HERMES_STATE_DB = Path.home() / ".hermes" / "state.db"
ADMIN_TOKEN_FILE = Path.home() / ".antigravity-mobile" / "admin_token"

GATEWAY_BASE_URL = "http://127.0.0.1:58900"
GATEWAY_STATUS_URL = f"{GATEWAY_BASE_URL}/gateway/status"
GATEWAY_TOUCH_URL = f"{GATEWAY_BASE_URL}/gateway/cascade/touch"
GATEWAY_API_URL = f"{GATEWAY_BASE_URL}/api/exa.language_server_pb.LanguageServerService"


def get_admin_token() -> str:
    """读取 Mobile Gateway 本地 Admin 访问令牌."""
    if ADMIN_TOKEN_FILE.exists():
        try:
            tok = ADMIN_TOKEN_FILE.read_text(encoding="utf-8").strip()
            if tok:
                return tok
        except Exception:
            pass
    return ""


def notify_gateway_cascade_touch(cascade_id: str) -> None:
    """通知 Mobile Gateway 立即清除该会话的本地缓存，促使活跃流即刻向手机推送最新内容."""
    if not cascade_id:
        return
    try:
        url = f"{GATEWAY_TOUCH_URL}?cascadeId={urllib.parse.quote(cascade_id)}"
        req = urllib.request.Request(url, headers={"Accept": "application/json"})
        with urllib.request.urlopen(req, timeout=1.5):
            pass
    except Exception as e:
        logger.debug("Failed to touch gateway cascade cache: %s", e)


def _create_ssl_context() -> ssl.SSLContext:
    ctx = ssl.create_default_context()
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    return ctx


_ssl_ctx = _create_ssl_context()

# 硬件插件注入默认模型: Gemini 3.8 Flash
DEFAULT_HARDWARE_MODEL = "MODEL_PLACEHOLDER_M318"


def get_default_model() -> str:
    """获取硬件插件注入专用的模型枚举，默认走 Gemini 3.8 Flash (MODEL_PLACEHOLDER_M318)."""
    env_model = os.environ.get("ANTIGRAVITY_HARDWARE_MODEL")
    if env_model:
        return env_model.strip()

    if STATE_FILE.exists():
        try:
            with open(STATE_FILE, "r", encoding="utf-8") as f:
                cfg = json.load(f)
            if "default_model" in cfg and cfg["default_model"]:
                return str(cfg["default_model"]).strip()
        except Exception:
            pass

    return DEFAULT_HARDWARE_MODEL


class AntigravityBridge:
    """管理与 Antigravity 本地实例的交互与硬件反馈."""

    def __init__(self) -> None:
        self.cli_runner = CliRunner()

    def get_gateway_status(self) -> Optional[dict]:
        """从 Mobile Gateway /gateway/status 获取完整状态字典 (自动附带本地 admin 鉴权)."""
        try:
            headers = {"Accept": "application/json"}
            admin_tok = get_admin_token()
            if admin_tok:
                headers["Authorization"] = f"Bearer {admin_tok}"
            req = urllib.request.Request(GATEWAY_STATUS_URL, headers=headers)
            with urllib.request.urlopen(req, timeout=2.0) as resp:
                if resp.status == 200:
                    return json.loads(resp.read().decode("utf-8"))
        except Exception as e:
            logger.debug("Failed to query gateway status: %s", e)
        return None

    def is_gateway_available(self) -> bool:
        """检查 Mobile Gateway 是否正常运行并连接了 language_server."""
        status = self.get_gateway_status()
        if status and status.get("status") == "connected":
            return True
        return False

    def _verify_upstream_port(self, port: int, token: str) -> bool:
        """健康检查候选端口，必须是真正的 language_server HTTPS ConnectRPC 端口."""
        try:
            url = f"https://127.0.0.1:{port}/exa.language_server_pb.LanguageServerService/GetStatus"
            req = urllib.request.Request(
                url,
                data=b"{}",
                headers={
                    "Content-Type": "application/json",
                    "Connect-Protocol-Version": "1",
                    "x-codeium-csrf-token": token,
                },
                method="POST",
            )
            with urllib.request.urlopen(req, context=_ssl_ctx, timeout=1.0) as resp:
                return resp.status == 200
        except Exception:
            return False

    def get_upstream_info(self) -> Optional[Tuple[int, str]]:
        """探针获取当前活跃的 language_server port 和 csrf_token (直连兜底使用)."""
        import subprocess

        try:
            out = subprocess.check_output(["ps", "-eo", "pid,command"], text=True, timeout=2.0)
            for line in out.splitlines():
                if "language_server" in line and "--csrf_token" in line and "--standalone" in line:
                    m_csrf = re.search(r"--csrf_token\s+([0-9a-fA-F-]+)", line)
                    parts = line.strip().split()
                    if m_csrf and len(parts) >= 2:
                        pid = parts[0]
                        csrf_tok = m_csrf.group(1)
                        # 注意: macOS lsof 必须加 -a，否则 -p 与 -iTCP 为 OR 逻辑导致误匹配全系统的监听端口
                        lsof_out = subprocess.check_output(
                            ["lsof", "-nP", "-a", "-p", pid, "-iTCP", "-sTCP:LISTEN"],
                            text=True,
                            timeout=2.0,
                        )
                        candidate_ports = []
                        for l_line in lsof_out.splitlines():
                            m_port = re.search(r":(\d+)\s+\(LISTEN\)", l_line)
                            if m_port:
                                p_int = int(m_port.group(1))
                                if p_int not in candidate_ports:
                                    candidate_ports.append(p_int)
                        # 逐个探测 GetStatus，杜绝选错非 HTTPS 端口
                        for p_cand in candidate_ports:
                            if self._verify_upstream_port(p_cand, csrf_tok):
                                return p_cand, csrf_tok
        except Exception as e:
            logger.debug("Failed to inspect process for upstream info: %s", e)

        return None

    def get_pinned_cascade(self) -> Optional[Tuple[str, str, float]]:
        """读取最近通过语音显式新建并置顶的会话及其时间戳."""
        if not STATE_FILE.exists():
            return None
        try:
            with open(STATE_FILE, "r", encoding="utf-8") as f:
                state = json.load(f)
            cid = state.get("pinned_cascade_id")
            title = state.get("pinned_title") or "新会话"
            pinned_at = float(state.get("pinned_at", 0))
            if cid and (time.time() - pinned_at < 7200):
                return cid, title, pinned_at
        except Exception as e:
            logger.debug("Failed to read routing state: %s", e)
        return None

    def set_pinned_cascade(self, cascade_id: str, title: str) -> None:
        """保存当前置顶的新建会话."""
        try:
            STATE_FILE.parent.mkdir(parents=True, exist_ok=True)
            state = {
                "pinned_cascade_id": cascade_id,
                "pinned_title": title,
                "pinned_at": time.time(),
                "updated_at": datetime.now(timezone.utc).isoformat(),
            }
            with open(STATE_FILE, "w", encoding="utf-8") as f:
                json.dump(state, f, ensure_ascii=False, indent=2)
        except Exception as e:
            logger.warning("Failed to save routing state: %s", e)

    def resolve_active_cascade(self) -> Tuple[Optional[str], str]:
        """按照多级仲裁顺序解析当前应注入的活跃会话:

        1. 最高优先级（Follow-Me 跨端统一游标）:
           - 优先命中手机端活跃屏幕或息屏 30 分钟粘性保持；
           - 手机端未聚焦时命中桌面 IDE 最新聚焦会话；
        2. 第二优先级: 手机当前活跃流 (active_stream_cascade_id)；
        3. 第三优先级（时间戳仲裁）: 比对本地 annotations 的最后查看时间与语音置顶时间；
        4. 回退策略: 扫描 annotations 最新的有效会话。
        """
        # 1. 优先采用 Mobile Gateway 统一游标 (Follow-Me 跨端仲裁)
        gw_status = self.get_gateway_status()
        if gw_status:
            cursor = gw_status.get("unified_cursor")
            if cursor and isinstance(cursor, dict):
                cursor_cid = cursor.get("cascade_id")
                if cursor_cid and (BRAIN_DIR / cursor_cid).exists():
                    cursor_title = cursor.get("title") or "当前活跃会话"
                    cursor_src = cursor.get("source") or "gateway"
                    is_sticky = cursor.get("is_sticky", False)
                    logger.info(
                        "[AntigravityBridge] Unified cursor hit: %s (%s, source=%s, sticky=%s)",
                        cursor_cid,
                        cursor_title,
                        cursor_src,
                        is_sticky,
                    )
                    return cursor_cid, cursor_title

            active_stream_id = gw_status.get("active_stream_cascade_id")
            if active_stream_id and (BRAIN_DIR / active_stream_id).exists():
                active_stream_title = gw_status.get("active_stream_title") or "当前屏幕会话"
                logger.info(
                    "[AntigravityBridge] Highest priority hit: Mobile active screen (%s, title: %s)",
                    active_stream_id,
                    active_stream_title,
                )
                return active_stream_id, active_stream_title

        # 2. 收集本地会话查看时间记录 (过滤未命名幽灵会话，必须有真实标题且 brain 存在)
        pbtxt_files = glob.glob(str(ANNOTATIONS_DIR / "*.pbtxt"))
        candidates = []
        for p in pbtxt_files:
            cid = Path(p).stem
            try:
                with open(p, "r", encoding="utf-8", errors="ignore") as f:
                    content = f.read()
                m_time = re.search(r"last_user_view_time:\s*\{seconds:\s*(\d+)", content)
                m_title = re.search(r'title:\s*"([^"]+)"', content)
                if m_time and m_title and m_title.group(1).strip():
                    if (BRAIN_DIR / cid).exists():
                        candidates.append((
                            int(m_time.group(1)),
                            cid,
                            m_title.group(1).strip(),
                        ))
            except Exception:
                continue

        if candidates:
            candidates.sort(key=lambda x: x[0], reverse=True)

        # 3. 动态时间戳仲裁 (View Time vs Pinned Time)
        pinned = self.get_pinned_cascade()
        if pinned:
            pinned_cid, pinned_title, pinned_at = pinned
            if (BRAIN_DIR / pinned_cid).exists():
                if candidates:
                    newest_view_time, newest_cid, newest_title = candidates[0]
                    if newest_view_time > pinned_at + 2:
                        logger.info(
                            "[AntigravityBridge] Attention switched: viewed %s (t=%d) > pinned %s (t=%d)",
                            newest_cid,
                            newest_view_time,
                            pinned_cid,
                            pinned_at,
                        )
                        return newest_cid, newest_title
                return pinned_cid, pinned_title

        if candidates:
            return candidates[0][1], candidates[0][2]

        return None, "未命名会话"

    def create_new_cascade(
        self,
        port: Optional[int] = None,
        token: Optional[str] = None,
        initial_prompt: Optional[str] = None,
        client_msg_id: Optional[str] = None,
    ) -> Tuple[Optional[str], str]:
        """在 Antigravity 中创建一个无工作区的纯净新 Chat，并附带有效模型.
        
        优先通过 Mobile Gateway (58900) 代理创建，若网关不可用则回退至直连 language_server.
        """
        model_enum = get_default_model()
        title = "新对话"
        if initial_prompt and initial_prompt.strip():
            clean_first = initial_prompt.strip().splitlines()[0]
            title = clean_first[:18] + ("..." if len(clean_first) > 18 else "")

        start_payload = {
            "source": "CORTEX_TRAJECTORY_SOURCE_CASCADE_CLIENT",
            "workspaceUris": [],
            "projectEnvConfig": {
                "projectId": "outside-of-project",
                "defaultProjectEnvironment": {},
            },
            "requestedModel": model_enum,
        }
        now_iso = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ")

        # 方案 A: 优先通过 Mobile Gateway 代理创建
        if self.is_gateway_available():
            try:
                admin_tok = get_admin_token()
                gw_headers = {
                    "Content-Type": "application/json",
                    "Connect-Protocol-Version": "1",
                }
                if admin_tok:
                    gw_headers["Authorization"] = f"Bearer {admin_tok}"

                start_url = f"{GATEWAY_API_URL}/StartCascade"
                req = urllib.request.Request(
                    start_url,
                    data=json.dumps(start_payload).encode("utf-8"),
                    headers=gw_headers,
                    method="POST",
                )
                with urllib.request.urlopen(req, timeout=5.0) as resp:
                    if resp.status == 200:
                        res = json.loads(resp.read().decode("utf-8"))
                        cascade_id = res.get("cascadeId")
                        if cascade_id:
                            # 更新标题
                            ann_url = f"{GATEWAY_API_URL}/UpdateConversationAnnotations"
                            ann_payload = {
                                "cascadeId": cascade_id,
                                "annotations": {
                                    "title": title,
                                    "lastUserViewTime": now_iso,
                                },
                            }
                            ann_req = urllib.request.Request(
                                ann_url,
                                data=json.dumps(ann_payload).encode("utf-8"),
                                headers=gw_headers,
                                method="POST",
                            )
                            with urllib.request.urlopen(ann_req, timeout=4.0):
                                pass

                            if initial_prompt and initial_prompt.strip():
                                self.send_message(None, None, cascade_id, initial_prompt, client_msg_id)
                            else:
                                notify_gateway_cascade_touch(cascade_id)

                            self.set_pinned_cascade(cascade_id, title)
                            logger.info("[AntigravityBridge] Created new cascade via Gateway: %s (%s)", cascade_id, title)
                            return cascade_id, title
            except Exception as gw_err:
                logger.warning("[AntigravityBridge] Gateway StartCascade failed, trying direct fallback: %s", gw_err)

        # 方案 B: 直连 language_server 兜底
        if not port or not token:
            upstream = self.get_upstream_info()
            if upstream:
                port, token = upstream

        if not port or not token:
            logger.error("[AntigravityBridge] No available upstream connection for StartCascade")
            return None, ""

        try:
            direct_url = f"https://127.0.0.1:{port}/exa.language_server_pb.LanguageServerService/StartCascade"
            headers = {
                "Content-Type": "application/json",
                "Connect-Protocol-Version": "1",
                "x-codeium-csrf-token": token,
            }
            req = urllib.request.Request(
                direct_url,
                data=json.dumps(start_payload).encode("utf-8"),
                headers=headers,
                method="POST",
            )
            with urllib.request.urlopen(req, context=_ssl_ctx, timeout=6.0) as resp:
                if resp.status != 200:
                    logger.error("Direct StartCascade returned status %d", resp.status)
                    return None, ""
                res = json.loads(resp.read().decode("utf-8"))
                cascade_id = res.get("cascadeId")
                if not cascade_id:
                    return None, ""

            ann_url = f"https://127.0.0.1:{port}/exa.language_server_pb.LanguageServerService/UpdateConversationAnnotations"
            ann_payload = {
                "cascadeId": cascade_id,
                "annotations": {
                    "title": title,
                    "lastUserViewTime": now_iso,
                },
            }
            ann_req = urllib.request.Request(
                ann_url,
                data=json.dumps(ann_payload).encode("utf-8"),
                headers=headers,
                method="POST",
            )
            with urllib.request.urlopen(ann_req, context=_ssl_ctx, timeout=3.0):
                pass

            if initial_prompt and initial_prompt.strip():
                self.send_message(port, token, cascade_id, initial_prompt, client_msg_id)
            else:
                notify_gateway_cascade_touch(cascade_id)

            self.set_pinned_cascade(cascade_id, title)
            logger.info("[AntigravityBridge] Created new cascade via Direct upstream: %s (%s)", cascade_id, title)
            return cascade_id, title
        except Exception as direct_err:
            logger.error("[AntigravityBridge] Direct StartCascade failed: %s", direct_err)
            return None, ""

    def send_message(
        self,
        port: Optional[int] = None,
        token: Optional[str] = None,
        cascade_id: str = "",
        text: str = "",
        client_msg_id: Optional[str] = None,
    ) -> bool:
        """发送消息到指定的 Cascade 会话.
        
        优先走 Mobile Gateway (58900)，自动获得模型配置注入、去重保护与 SSE 广播；
        若 Gateway 不可用则回退至直连 language_server.
        """
        if not cascade_id or not text.strip():
            return False

        model_enum = get_default_model()
        payload = {
            "cascadeId": cascade_id,
            "items": [{"text": text}],
            "cascadeConfig": {
                "plannerConfig": {
                    "planModel": model_enum,
                    "requestedModel": {
                        "model": model_enum,
                    },
                }
            },
        }

        # 方案 A: 优先通过 Mobile Gateway 发送
        if self.is_gateway_available():
            try:
                admin_tok = get_admin_token()
                gw_headers = {
                    "Content-Type": "application/json",
                    "Connect-Protocol-Version": "1",
                }
                if admin_tok:
                    gw_headers["Authorization"] = f"Bearer {admin_tok}"
                if client_msg_id:
                    gw_headers["X-Client-Message-Id"] = client_msg_id

                gw_url = f"{GATEWAY_API_URL}/SendUserCascadeMessage"
                req = urllib.request.Request(
                    gw_url,
                    data=json.dumps(payload).encode("utf-8"),
                    headers=gw_headers,
                    method="POST",
                )
                with urllib.request.urlopen(req, timeout=6.0) as resp:
                    if resp.status == 200:
                        notify_gateway_cascade_touch(cascade_id)
                        logger.info("[AntigravityBridge] Sent message to %s via Gateway", cascade_id)
                        return True
            except Exception as gw_err:
                logger.warning("[AntigravityBridge] Gateway SendUserCascadeMessage failed, trying direct fallback: %s", gw_err)

        # 方案 B: 直连 language_server 兜底
        if not port or not token:
            upstream = self.get_upstream_info()
            if upstream:
                port, token = upstream

        if not port or not token:
            logger.error("[AntigravityBridge] No available upstream connection for SendUserCascadeMessage")
            return False

        url = f"https://127.0.0.1:{port}/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage"
        headers = {
            "Content-Type": "application/json",
            "Connect-Protocol-Version": "1",
            "x-codeium-csrf-token": token,
        }
        if client_msg_id:
            headers["X-Client-Message-Id"] = client_msg_id

        req = urllib.request.Request(
            url,
            data=json.dumps(payload).encode("utf-8"),
            headers=headers,
            method="POST",
        )
        try:
            with urllib.request.urlopen(req, context=_ssl_ctx, timeout=6.0) as resp:
                ok = (resp.status == 200)
                if ok:
                    notify_gateway_cascade_touch(cascade_id)
                    logger.info("[AntigravityBridge] Sent message to %s via Direct upstream", cascade_id)
                return ok
        except Exception as e:
            logger.error("Failed to send message to cascade %s via Direct upstream: %s", cascade_id, e)
            return False

    def feedback_new_cascade(self, title: str) -> None:
        """新建对话硬件反馈: 4 色交织流光 (2.0s) + 显式通知回显."""
        payload = {
            "segments": [
                {"mode": "color_flow", "duration_s": 0.5, "interval_ms": 60, "brightness": 255, "direction": "ltr", "window": 2, "color": {"r": 255, "g": 210, "b": 0}, "background": {"r": 255, "g": 20, "b": 20, "brightness": 180}},
                {"mode": "color_flow", "duration_s": 0.5, "interval_ms": 60, "brightness": 255, "direction": "ltr", "window": 2, "color": {"r": 0, "g": 220, "b": 60}, "background": {"r": 255, "g": 210, "b": 0, "brightness": 180}},
                {"mode": "color_flow", "duration_s": 0.5, "interval_ms": 60, "brightness": 255, "direction": "ltr", "window": 2, "color": {"r": 0, "g": 100, "b": 255}, "background": {"r": 0, "g": 220, "b": 60, "brightness": 180}},
                {"mode": "color_flow", "duration_s": 0.5, "interval_ms": 60, "brightness": 255, "direction": "ltr", "window": 2, "color": {"r": 255, "g": 20, "b": 20}, "background": {"r": 0, "g": 100, "b": 255, "brightness": 180}},
            ],
            "title": "Antigravity",
            "reason": f"✨ 新对话已开启: {title}",
            "repeat": False,
            "repeat_times": 1,
        }
        self._send_light(payload)

    def feedback_sent_to_cascade(self, title: str) -> None:
        """普通对讲发送反馈: 科技蔚蓝波浪 (1.0s) + 显式通知回显."""
        payload = {
            "segments": [
                {"mode": "wave", "duration_s": 1.0, "interval_ms": 70, "brightness": 255, "color": {"r": 0, "g": 130, "b": 255}, "background": {"r": 0, "g": 20, "b": 60, "brightness": 120}},
            ],
            "title": "Antigravity",
            "reason": f"📌 已发送至: {title}",
            "repeat": False,
            "repeat_times": 1,
        }
        self._send_light(payload)

    def feedback_error(self, message: str) -> None:
        """异常报警反馈: 红色爆闪 3 次."""
        payload = {
            "segments": [
                {"mode": "strobe", "duration_s": 1.0, "interval_ms": 100, "brightness": 255, "color": {"r": 255, "g": 0, "b": 0}},
            ],
            "title": "Antigravity",
            "reason": f"❌ {message}",
            "repeat": False,
            "repeat_times": 1,
        }
        self._send_light(payload)

    def _send_light(self, payload: dict[str, Any]) -> None:
        try:
            self.cli_runner.run_json(
                ["light", "+gateway", "light.send"],
                input_text=json.dumps(payload, ensure_ascii=False),
                timeout_seconds=5,
            )
        except Exception as e:
            logger.debug("Failed to send light feedback: %s", e)


_bridge = AntigravityBridge()


def get_hermes_session_title(session_id: Optional[str]) -> Optional[str]:
    """从 Hermes SQLite (state.db) 读取指定 session_id 的会话标题."""
    if not session_id:
        return None
    if not HERMES_STATE_DB.exists():
        return None
    try:
        with sqlite3.connect(f"file:{HERMES_STATE_DB}?mode=ro", uri=True, timeout=1.5) as conn:
            cursor = conn.cursor()
            cursor.execute(
                "SELECT title FROM sessions WHERE id = ? AND title IS NOT NULL AND title != ''",
                (session_id,),
            )
            row = cursor.fetchone()
            if row and row[0]:
                return str(row[0]).strip()

            cursor.execute(
                "SELECT title FROM sessions WHERE parent_session_id = ? AND title IS NOT NULL AND title != '' ORDER BY started_at DESC LIMIT 1",
                (session_id,),
            )
            row = cursor.fetchone()
            if row and row[0]:
                return str(row[0]).strip()
    except Exception as e:
        logger.debug("[AntigravityBridge] Failed to query session title for %s: %s", session_id, e)
    return None


def is_antigravity_session(title: Optional[str]) -> bool:
    """判定会话标题是否为用户指定的 Antigravity 专属通道 (标题包含'反重力'或'antigravity')."""
    if not title:
        return False
    normalized = title.strip().lower()
    return "反重力" in normalized or "antigravity" in normalized


async def dispatch_to_antigravity(message: Any, adapter: Any = None) -> bool:
    """尝试将 YoooClaw 消息拦截并直连注入 Antigravity.

    返回值:
    - True: 命中反重力专属会话并成功注入 Antigravity (阻断 Hermes 处理)；
    - False: 未命中反重力会话或未能投递，100% 放行给 Hermes 兜底，防止丢消息。
    """
    raw_text = getattr(message, "text", "") or ""
    text = raw_text.strip()
    if not text:
        return False

    session_id = getattr(message, "target_session_id", None) or getattr(message, "chat_id", None)
    session_title = get_hermes_session_title(session_id)

    # 核心分流开关: 只有会话标题明确包含 "反重力" 或 "antigravity" 时才执行截流！
    if not is_antigravity_session(session_title):
        logger.info(
            "[AntigravityBridge] Bypass: session '%s' (title: %r) is not an Antigravity channel. Letting Hermes handle it.",
            session_id,
            session_title,
        )
        return False

    logger.info(
        "[AntigravityBridge] Intercepting message for Antigravity session '%s' (title: %r, chars=%d)",
        session_id,
        session_title,
        len(text),
    )

    # 检查通路是否就绪 (优先 Gateway，兜底 Direct upstream)
    if not _bridge.is_gateway_available() and not _bridge.get_upstream_info():
        logger.warning("[AntigravityBridge] Neither Antigravity gateway nor language_server is available, falling back to Hermes.")
        loop = asyncio.get_running_loop()
        await loop.run_in_executor(None, _bridge.feedback_error, "反重力服务未连接")
        return False

    client_msg_id = getattr(message, "message_id", None)

    # 关键词判定: 是否为“新建对话”
    match = NEW_CASCADE_PATTERN.match(text)
    if match:
        prompt = match.group(1).strip()
        logger.info("[AntigravityBridge] Detected NEW cascade command in Antigravity session. prompt_len=%d", len(prompt))

        loop = asyncio.get_running_loop()
        cascade_id, title = await loop.run_in_executor(
            None, _bridge.create_new_cascade, None, None, prompt, client_msg_id
        )
        if cascade_id:
            await loop.run_in_executor(None, _bridge.feedback_new_cascade, title)
            logger.info("[AntigravityBridge] Successfully created cascade %s (title: %s)", cascade_id, title)
            # 向 YoooClaw App 回显并收尾 pending，杜绝前端无限等待
            if adapter and getattr(message, "chat_id", None):
                try:
                    await adapter.send(
                        message.chat_id,
                        f"✨ 已为您开启新反重力会话【{title}】\n\n已在后台开始执行，您可在反重力客户端查看实时进度。",
                    )
                    await adapter._send_event(
                        message.chat_id,
                        "run.complete",
                        "success",
                        {"messageId": message.message_id, "runId": message.message_id, "outcome": "success"},
                    )
                except Exception as resp_err:
                    logger.debug("Failed to emit adapter confirmation: %s", resp_err)
            return True
        else:
            await loop.run_in_executor(None, _bridge.feedback_error, "新建反重力会话失败")
            return False

    # 普通发话: 路由到当前活跃的有效会话 (Follow-Me 跨端统一游标仲裁)
    cascade_id, title = _bridge.resolve_active_cascade()
    loop = asyncio.get_running_loop()

    if not cascade_id:
        logger.info("[AntigravityBridge] No active cascade found, creating one on the fly.")
        cascade_id, title = await loop.run_in_executor(
            None, _bridge.create_new_cascade, None, None, text, client_msg_id
        )
        if cascade_id:
            await loop.run_in_executor(None, _bridge.feedback_new_cascade, title)
            if adapter and getattr(message, "chat_id", None):
                try:
                    await adapter.send(
                        message.chat_id,
                        f"✨ 已为您开启新反重力会话【{title}】\n\n已在后台开始执行，您可在反重力客户端查看实时进度。",
                    )
                    await adapter._send_event(
                        message.chat_id,
                        "run.complete",
                        "success",
                        {"messageId": message.message_id, "runId": message.message_id, "outcome": "success"},
                    )
                except Exception as resp_err:
                    logger.debug("Failed to emit adapter confirmation: %s", resp_err)
            return True
        return False

    logger.info("[AntigravityBridge] Routing to active cascade %s (title: %s)", cascade_id, title)
    ok = await loop.run_in_executor(
        None, _bridge.send_message, None, None, cascade_id, text, client_msg_id
    )
    if ok:
        await loop.run_in_executor(None, _bridge.feedback_sent_to_cascade, title)
        logger.info("[AntigravityBridge] Message delivered to cascade %s (%s)", cascade_id, title)
        if adapter and getattr(message, "chat_id", None):
            try:
                await adapter.send(
                    message.chat_id,
                    f"📌 已投递至反重力会话【{title}】\n\n已在后台开始执行，您可在反重力客户端查看实时进度。",
                )
                await adapter._send_event(
                    message.chat_id,
                    "run.complete",
                    "success",
                    {"messageId": message.message_id, "runId": message.message_id, "outcome": "success"},
                )
            except Exception as resp_err:
                logger.debug("Failed to emit adapter confirmation: %s", resp_err)
        return True
    else:
        await loop.run_in_executor(None, _bridge.feedback_error, "发送到反重力失败")
        logger.error("[AntigravityBridge] Delivery failed, falling back to Hermes.")
        return False
