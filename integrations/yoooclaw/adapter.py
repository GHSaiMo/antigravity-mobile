"""Hermes messaging-platform adapter for YoooClaw APP chat."""

from __future__ import annotations

import asyncio
import contextlib
import logging
import os
import threading
import time
import uuid
from collections import OrderedDict
from typing import Any, Callable

from yoooclaw_hermes.logger import install_plugin_file_logger
from yoooclaw_hermes.runtime import get_lifecycle_coordinator
from yoooclaw_hermes.runtime.openclaw_relay import OpenClawRelayTransport

from .app_transport import AppMessage, AppTransport, RelayAppTransport, resolve_api_key

logger = logging.getLogger(__name__)

try:
    from gateway.config import Platform
    from gateway.platforms.base import BasePlatformAdapter, MessageEvent, MessageType, ProcessingOutcome, SendResult
except ImportError as error:  # Keep source-only unit tests independent of Hermes installation.
    if error.name and error.name.split(".", 1)[0] != "gateway":
        raise
    from dataclasses import dataclass

    class Platform(str):
        @property
        def value(self) -> str:
            return str(self)

    class MessageType:
        TEXT = "text"

    class ProcessingOutcome:
        SUCCESS = "success"
        FAILURE = "failure"
        CANCELLED = "cancelled"

    @dataclass
    class MessageEvent:
        text: str
        message_type: Any
        source: Any
        raw_message: Any
        message_id: str

    @dataclass
    class SendResult:
        success: bool
        message_id: str | None = None
        error: str | None = None
        retryable: bool = False

    class BasePlatformAdapter:
        def __init__(self, config: Any, platform: Platform) -> None:
            self.config = config
            self.platform = platform
            self._running = False

        def _mark_connected(self) -> None:
            self._running = True

        def _mark_disconnected(self) -> None:
            self._running = False

        def build_source(self, **kwargs: Any) -> dict[str, Any]:
            return kwargs

        async def handle_message(self, event: MessageEvent) -> None:
            del event


class YoooclawAppAdapter(BasePlatformAdapter):
    def __init__(
        self,
        config: Any,
        transport_factory: Callable[..., AppTransport] = RelayAppTransport,
        openclaw_relay_factory: Callable[[], Any] = OpenClawRelayTransport,
    ) -> None:
        super().__init__(config=config, platform=Platform("yoooclaw_app"))
        extra = getattr(config, "extra", {}) or {}
        # 单 WS 架构：openclaw_relay 拥有全部隧道，transport 只是挂在它上面的
        # APP 聊天协议层（进出站帧都经隧道转发）。
        relay_url = extra.get("relay_url")
        self.openclaw_relay = openclaw_relay_factory(relay_url=relay_url) if relay_url else openclaw_relay_factory()
        self.transport = transport_factory(relay=self.openclaw_relay)
        self._lifecycle_watch_task: asyncio.Task[None] | None = None
        self._update_loop: asyncio.AbstractEventLoop | None = None
        # Gateway's plain-text approval fallback does not carry session_key.
        # Remember the key built for recent inbound chats so the plugin can
        # recover that fallback into a structured V1 approval request.
        self._approval_session_keys: OrderedDict[str, str] = OrderedDict()
        # Hermes 0.17 only trusts ``enforces_own_access_policy`` when the
        # adapter advertises an effective allowlist policy.  Relay has already
        # authenticated the API key and resolved the APP user before delivery,
        # so every message that reaches this adapter has passed that gate.
        self._dm_policy = "allowlist"

    @property
    def enforces_own_access_policy(self) -> bool:
        """APP Relay/API-key authentication owns user access before Hermes dispatch."""
        return True

    @property
    def authorization_is_upstream(self) -> bool:
        """Relay authenticated and authorized the APP user before delivery."""
        return True

    async def connect(self, *, is_reconnect: bool = False) -> bool:
        try:
            logger.info(
                "Connecting YoooClaw APP adapter%s",
                " (reconnect)" if is_reconnect else "",
            )
            await get_lifecycle_coordinator().connect_transport(
                self.transport,
                self._handle_app_message,
                reason="app_adapter_connect",
                openclaw_relay=self.openclaw_relay,
            )
            self._mark_connected()
            self._start_lifecycle_watchdog()
            self._setup_update_channel()
            logger.info("YoooClaw APP adapter connected")
            return True
        except Exception as error:
            logger.error("Could not connect YoooClaw APP Relay: %s", error)
            self._mark_disconnected()
            return False

    async def disconnect(self) -> None:
        logger.info("Disconnecting YoooClaw APP adapter")
        await self._stop_lifecycle_watchdog()
        await get_lifecycle_coordinator().disconnect_transport(
            self.transport,
            reason="app_adapter_disconnect",
            openclaw_relay=self.openclaw_relay,
        )
        self._mark_disconnected()
        logger.info("YoooClaw APP adapter disconnected")

    def _start_lifecycle_watchdog(self) -> None:
        if self._lifecycle_watch_task and not self._lifecycle_watch_task.done():
            return
        self._lifecycle_watch_task = asyncio.create_task(self._watch_lifecycle_group())

    async def _stop_lifecycle_watchdog(self) -> None:
        task, self._lifecycle_watch_task = self._lifecycle_watch_task, None
        if not task:
            return
        task.cancel()
        with contextlib.suppress(asyncio.CancelledError):
            await task

    async def _watch_lifecycle_group(self) -> None:
        # 生命周期组语义（有意的设计权衡，非缺陷）：
        # - 隧道掉线先交给 openclaw relay 各隧道的重连循环自愈（含 relay URL
        #   漂移检测）；只有断开持续超过 ws_grace 才把整组（全部隧道）一起
        #   重启，避免把一次网络抖动放大成组级中断。
        # - generation 漂移（profile / relay 环境 / api-key 集合变化）与手动
        #   restart 请求都表现为 status=mismatch/restart_requested，连续
        #   mismatch_streak 次确认后整组重启重建隧道。
        interval = _float_env("YOOOCLAW_HERMES_LIFECYCLE_WATCH_INTERVAL", 5.0)
        ws_grace = _float_env("YOOOCLAW_HERMES_WS_RESTART_AFTER", 30.0)
        max_backoff = max(interval, _float_env("YOOOCLAW_HERMES_LIFECYCLE_MAX_BACKOFF", 60.0))
        mismatch_threshold = max(1, _int_env("YOOOCLAW_HERMES_LIFECYCLE_MISMATCH_STREAK", 2))
        ws_down_since: float | None = None
        consecutive_restarts = 0
        mismatch_streak = 0
        while True:
            # 连续重启按指数拉长观察间隔：持续 mismatch 时避免每 5 秒全量重建
            # 隧道，同时限制网络恢复后的最坏等待时间。
            backoff = min(max_backoff, interval * (2 ** min(consecutive_restarts, 6)))
            await asyncio.sleep(max(1.0, backoff))
            try:
                coordinator = get_lifecycle_coordinator()
                snapshot = await asyncio.to_thread(coordinator.status)
                if snapshot.status != "running":
                    # 手动 restart 请求是确定性指令，不需要连续确认。
                    if snapshot.reason != "restart_requested":
                        mismatch_streak += 1
                        if mismatch_streak < mismatch_threshold:
                            logger.info(
                                "YoooClaw lifecycle watchdog observed generation mismatch (tolerating transient) streak=%d/%d status=%s",
                                mismatch_streak,
                                mismatch_threshold,
                                snapshot.status,
                            )
                            continue
                    logger.warning(
                        "YoooClaw lifecycle watchdog restarting group: status=%s reason=%s generation=%s",
                        snapshot.status,
                        snapshot.reason,
                        snapshot.generation,
                    )
                    await self._restart_lifecycle_group("watchdog_generation_mismatch")
                    ws_down_since = None
                    mismatch_streak = 0
                    consecutive_restarts += 1
                    continue
                mismatch_streak = 0
                openclaw_status = self.openclaw_relay.status()
                openclaw_connected = bool(openclaw_status.get("connected")) if isinstance(openclaw_status, dict) else False
                if openclaw_connected:
                    ws_down_since = None
                    consecutive_restarts = 0
                    continue
                ws_down_since = ws_down_since or time.monotonic()
                if time.monotonic() - ws_down_since >= ws_grace:
                    logger.warning(
                        "YoooClaw lifecycle watchdog restarting group: Relay WS stayed disconnected last_error=%s",
                        openclaw_status.get("lastError") if isinstance(openclaw_status, dict) else None,
                    )
                    await self._restart_lifecycle_group("watchdog_ws_disconnected")
                    ws_down_since = None
                    consecutive_restarts += 1
            except asyncio.CancelledError:
                raise
            except Exception as error:  # noqa: BLE001 - 看门狗自身绝不能死：一轮失败记一次重启计数，下轮退避后再试。
                consecutive_restarts += 1
                logger.warning("YoooClaw lifecycle watchdog iteration failed (attempt backoff %d): %s", consecutive_restarts, error)

    async def _restart_lifecycle_group(self, reason: str) -> None:
        # 重连失败（DNS/网络故障等）直接上抛：transports 已断开，status() 变为
        # stopped ≠ running，后续 watchdog 轮次按退避继续重试整组重连。
        coordinator = get_lifecycle_coordinator()
        await coordinator.disconnect_transport(self.transport, reason=f"{reason}_stop", openclaw_relay=self.openclaw_relay)
        await coordinator.connect_transport(
            self.transport,
            self._handle_app_message,
            reason=f"{reason}_start",
            openclaw_relay=self.openclaw_relay,
        )
        self._mark_connected()

    async def send(self, chat_id: str, content: str, reply_to: str | None = None, metadata: dict[str, Any] | None = None) -> SendResult:
        del metadata
        fallback_approval = _parse_gateway_approval_fallback(content)
        if fallback_approval is not None:
            session_key = self._approval_session_keys.get(chat_id)
            if session_key:
                command, description = fallback_approval
                recovered = await self.send_exec_approval(
                    chat_id=chat_id,
                    command=command,
                    session_key=session_key,
                    description=description,
                )
                if recovered.success:
                    logger.warning(
                        "Recovered Hermes text approval fallback as structured request chat_id=%s approval_id=%s",
                        chat_id,
                        recovered.message_id,
                    )
                    return recovered
                logger.error(
                    "Could not recover Hermes text approval fallback chat_id=%s error=%s; sending text fallback",
                    chat_id,
                    recovered.error,
                )
            else:
                logger.error(
                    "Could not recover Hermes text approval fallback: no session key chat_id=%s; sending text fallback",
                    chat_id,
                )
        streaming = _looks_like_streaming_preview(content)
        await self._send_event(
            chat_id,
            "reply.sending",
            "streaming" if streaming else "sending",
            {"replyToMessageId": reply_to, "chars": len(content), "streaming": streaming},
        )
        try:
            message_id = await self.transport.send(chat_id, content, reply_to)
            if not streaming:
                complete_message = getattr(self.transport, "complete_message", None)
                if callable(complete_message):
                    await complete_message(chat_id, message_id, content)
            await self._send_event(
                chat_id,
                "reply.sent",
                "streaming" if streaming else "final",
                {"messageId": message_id, "replyToMessageId": reply_to, "chars": len(content), "streaming": streaming},
            )
            logger.info("YoooClaw APP adapter sent reply chat_id=%s message_id=%s chars=%d", chat_id, message_id, len(content))
            return SendResult(success=True, message_id=message_id)
        except Exception as error:
            await self._send_event(chat_id, "reply.failed", "failure", {"error": str(error), "replyToMessageId": reply_to})
            logger.warning("YoooClaw APP adapter send failed chat_id=%s reply_to=%s error=%s", chat_id, reply_to, error)
            return SendResult(success=False, error=str(error), retryable=True)

    async def edit_message(
        self,
        chat_id: str,
        message_id: str,
        content: str,
        *,
        finalize: bool = False,
        metadata: dict[str, Any] | None = None,
    ) -> SendResult:
        del metadata
        await self._send_event(
            chat_id,
            "reply.update",
            "final" if finalize else "streaming",
            {"messageId": message_id, "chars": len(content), "final": finalize},
        )
        try:
            updated_message_id = await self.transport.edit(chat_id, message_id, content, finalize=finalize)
            await self._send_event(
                chat_id,
                "reply.updated",
                "final" if finalize else "streaming",
                {"messageId": updated_message_id, "chars": len(content), "final": finalize},
            )
            logger.info("YoooClaw APP adapter edited reply chat_id=%s message_id=%s finalize=%s chars=%d", chat_id, updated_message_id, finalize, len(content))
            return SendResult(success=True, message_id=updated_message_id)
        except Exception as error:
            await self._send_event(chat_id, "reply.update.failed", "failure", {"messageId": message_id, "error": str(error), "final": finalize})
            logger.warning("YoooClaw APP adapter edit failed chat_id=%s message_id=%s error=%s", chat_id, message_id, error)
            return SendResult(success=False, message_id=message_id, error=str(error), retryable=True)

    async def send_typing(self, chat_id: str, metadata: dict[str, Any] | None = None) -> None:
        del metadata
        await self._send_event(chat_id, "typing", "running", {"active": True})
        try:
            await self.transport.send_typing(chat_id, True)
        except Exception as error:
            logger.debug("Could not send YoooClaw APP typing chat_id=%s error=%s", chat_id, error)
        else:
            logger.debug("YoooClaw APP adapter sent typing chat_id=%s", chat_id)

    async def send_exec_approval(
        self,
        chat_id: str,
        command: str,
        session_key: str,
        description: str = "dangerous command",
        metadata: dict[str, Any] | None = None,
    ) -> SendResult:
        """发结构化审批请求给客户端（按钮交互），而非纯文本 /approve fallback。
        登记 approvalId → agent session_key；客户端回传决定走 approval.resolve RPC 唤醒 agent。
        审批 UI 用按钮、独立于输入框，所以不受本轮 pending 锁影响（解死锁的关键）。"""
        del metadata
        approval_id = f"approval_{uuid.uuid4().hex}"
        self._remember_approval_session_key(chat_id, session_key)
        registered = False
        try:
            self.transport.register_approval(approval_id, session_key)
            registered = True
            # Approval delivery is not best-effort: Hermes blocks its agent
            # thread after this call. Propagate transport failures so Hermes
            # can take an explicit fallback path instead of waiting forever on
            # an approval request the App never received.
            await self._send_event(
                chat_id,
                "approval.request",
                "running",
                {
                    "approvalId": approval_id,
                    "command": command,
                    "description": description,
                    "choices": ["once", "session", "always", "deny"],
                },
                raise_on_error=True,
            )
        except Exception as error:  # noqa: BLE001 - convert to SendResult contract.
            if registered:
                pop_approval = getattr(self.transport, "pop_approval", None)
                if callable(pop_approval):
                    pop_approval(approval_id)
            logger.warning(
                "YoooClaw APP exec approval send failed chat_id=%s approval_id=%s error=%s",
                chat_id,
                approval_id,
                error,
                exc_info=True,
            )
            return SendResult(success=False, message_id=approval_id, error=str(error), retryable=True)
        logger.info("YoooClaw APP sent exec approval chat_id=%s approval_id=%s desc=%s", chat_id, approval_id, description)
        return SendResult(success=True, message_id=approval_id)

    async def get_chat_info(self, chat_id: str) -> dict[str, Any]:
        return {"id": chat_id, "name": chat_id, "type": "dm"}

    async def on_processing_start(self, event: MessageEvent) -> None:
        logger.info("YoooClaw APP processing started chat_id=%s message_id=%s", event.source.chat_id, event.message_id)
        await self._send_event(
            event.source.chat_id,
            "processing.start",
            "running",
            {"messageId": event.message_id, "userId": getattr(event.source, "user_id", None), "textPreview": event.text[:120]},
        )

    async def on_processing_complete(self, event: MessageEvent, outcome: ProcessingOutcome) -> None:
        status = _outcome_value(outcome)
        logger.info("YoooClaw APP processing completed chat_id=%s message_id=%s outcome=%s", event.source.chat_id, event.message_id, status)
        await self._send_event(
            event.source.chat_id,
            "processing.complete",
            status,
            {"messageId": event.message_id, "userId": getattr(event.source, "user_id", None), "outcome": status},
        )
        # 整轮唯一结束信号：一条用户消息的 agent loop（含多个工具 turn）真正跑完时只触发一次。
        # 客户端据此收尾 pending + 对齐 history，区别于中间每个 turn 各自的 reply(final)。
        await self._send_event(
            event.source.chat_id,
            "run.complete",
            status,
            {"messageId": event.message_id, "runId": event.message_id, "outcome": status},
        )

    def _setup_update_channel(self) -> None:
        """Give the plugin updater a push channel into the APP chat.

        The updater's watcher/resume paths run on worker threads, so the
        notifier bridges back into this adapter's event loop. Resume must run
        off-loop: it may synchronously wait for a send scheduled on this loop.
        """
        try:
            from yoooclaw_hermes.update import get_update_manager

            manager = get_update_manager()
            self._update_loop = asyncio.get_running_loop()
            manager.register_notifier(self._push_update_notification)
            threading.Thread(
                target=manager.resume_after_reload,
                name="yoooclaw-update-resume",
                daemon=True,
            ).start()
        except Exception as error:  # noqa: BLE001 - update plumbing must never block connect.
            logger.warning("Could not set up YoooClaw update notification channel: %s", error)

    def _push_update_notification(self, chat_id: str, text: str) -> bool:
        loop = self._update_loop
        if loop is None or loop.is_closed():
            return False
        future = asyncio.run_coroutine_threadsafe(self.send(chat_id, text), loop)
        try:
            result = future.result(timeout=15)
        except Exception as error:  # noqa: BLE001 - delivery failure is reported as False.
            logger.warning("YoooClaw update notification push failed chat_id=%s error=%s", chat_id, error)
            return False
        return bool(getattr(result, "success", False))

    async def _handle_app_message(self, message: AppMessage) -> None:
        logger.info("Accepted YoooClaw APP message chat_id=%s message_id=%s user_id=%s chars=%d", message.chat_id, message.message_id, message.user_id, len(message.text))
        # --- Direct Antigravity Interception & Routing ---
        try:
            from .antigravity_bridge import dispatch_to_antigravity
            if await dispatch_to_antigravity(message, adapter=self):
                return
        except Exception as bridge_err:
            logger.warning("[AntigravityBridge] dispatch error, fallback to Hermes: %s", bridge_err, exc_info=True)
        # -------------------------------------------------
        try:
            from yoooclaw_hermes.update import get_update_manager

            get_update_manager().record_last_chat(message.chat_id, message_id=message.message_id)
        except Exception as error:  # noqa: BLE001 - bookkeeping must never break message handling.
            logger.debug("Could not record last APP chat for updates: %s", error)
        # 续聊：target_session_id（与 chat_id 解耦）是要跑/续的会话；老式探针没传则回退用 chat_id。
        # source.chat_id 仍是路由身份（回复经它送回连接），target 则作为 threaded-DM 作用域进入
        # Hermes session_key，避免同一 App 用户下多个会话共享 pending clarify/confirm/agent 锁。
        candidate_session_id = message.target_session_id or message.chat_id
        thread_id = candidate_session_id if candidate_session_id and candidate_session_id != message.chat_id else None
        source = self.build_source(
            chat_id=message.chat_id,
            chat_type="dm",
            user_id=message.user_id,
            thread_id=thread_id,
            message_id=message.message_id,
        )
        try:
            from gateway.session import build_session_key

            self._remember_approval_session_key(message.chat_id, build_session_key(source))
        except Exception:  # noqa: BLE001 - approval fallback recovery is best-effort.
            logger.debug("Could not remember Hermes session key for chat_id=%s", message.chat_id, exc_info=True)
        self._resume_session_if_needed(source, candidate_session_id)
        await self.handle_message(
            MessageEvent(
                text=message.text,
                message_type=MessageType.TEXT,
                source=source,
                raw_message=message,
                message_id=message.message_id,
            )
        )

    def _resume_session_if_needed(self, source: Any, candidate_session_id: str) -> None:
        """客户端点开历史会话续聊时，chat_id 传的是目标 session_id。

        把该路由 (session_key) 切到这个 session_id，否则 Hermes 会按 chat_id
        当成全新路由建新会话。需要：
        1. 用 SessionDB 校验 candidate 确实是已存在的 session_id（routing 的 store
           只按 session_key 索引，没有按 session_id 查的接口）；
        2. switch_session 重绑 session_key→target；
        3. 失效 runner 的 agent 缓存（按 session_key 缓存），否则会复用旧
           session_id 的 AIAgent。对齐 gateway/run.py home-channel 模板。
        """
        store = getattr(self, "_session_store", None)
        if store is None or not candidate_session_id:
            return
        try:
            db = getattr(store, "_db", None) or self._session_db_handle()
            if db is None:
                return
            # App 持有的是稳定的 lineage root；Hermes 在压缩或 `/new` 后会把
            # 它轮换到新的 tip。每条消息都直接切回 root 会撤销 `/new` 的效果，
            # 所以恢复前必须先解析当前 tip。
            target_session_id = db.get_compression_tip(candidate_session_id) or candidate_session_id
            if not db.get_session(target_session_id):
                return  # chat_id 不是已有 session_id，按 chat_id 正常路由（建/续主会话）
            from gateway.session import build_session_key

            session_key = build_session_key(source)
            # get_or_create 会为这个新 session_key 建一条空的 DB session（orphan），
            # 随后 switch_session 把 key 改指向 target。记下 orphan 以便清掉。
            entry = store.get_or_create_session(source)
            orphan_id = getattr(entry, "session_id", None)
            if orphan_id == target_session_id:
                return  # 已经在目标 session 上
            store.switch_session(session_key, target_session_id)
            self._evict_runner_agent(session_key)
            self._delete_empty_session(db, orphan_id)
            logger.info(
                "yoooclaw_app: resumed session %s (lineage root %s) via key %s",
                target_session_id,
                candidate_session_id,
                session_key,
            )
        except Exception as exc:  # noqa: BLE001 - never break message handling on a routing helper
            logger.error("yoooclaw_app: resume session %s failed: %s", candidate_session_id, exc)

    def _session_db_handle(self) -> Any:
        """兜底拿一个进程内 SessionDB（store 没有 _db 时）。"""
        try:
            from hermes_state import SessionDB

            return SessionDB()
        except Exception:  # noqa: BLE001
            return None

    def _evict_runner_agent(self, session_key: str) -> None:
        """失效 runner 按 session_key 缓存的 AIAgent，强制下条消息重建。"""
        handler = getattr(self, "_message_handler", None)
        runner = getattr(handler, "__self__", None)
        evict = getattr(runner, "_evict_cached_agent", None)
        if callable(evict):
            evict(session_key)

    def _delete_empty_session(self, db: Any, session_id: str | None) -> None:
        """清掉 switch 前 get_or_create 留下的空壳 session（仅当确实无消息）。"""
        if db is None or not session_id:
            return
        try:
            if not (db.get_messages(session_id) or []):
                db.delete_session(session_id)
        except Exception as exc:  # noqa: BLE001
            logger.debug("yoooclaw_app: prune orphan session %s skipped: %s", session_id, exc)

    def _remember_approval_session_key(self, chat_id: str, session_key: str) -> None:
        if not chat_id or not session_key:
            return
        self._approval_session_keys[chat_id] = session_key
        self._approval_session_keys.move_to_end(chat_id)
        while len(self._approval_session_keys) > 1024:
            self._approval_session_keys.popitem(last=False)

    async def _send_event(
        self,
        chat_id: str,
        name: str,
        status: str,
        payload: dict[str, Any] | None = None,
        *,
        raise_on_error: bool = False,
    ) -> None:
        try:
            event_payload = {"adapter": "yoooclaw_app", "atMonotonic": time.monotonic(), **(payload or {})}
            await self.transport.send_event(chat_id, name, status, event_payload)
        except Exception:
            if raise_on_error:
                raise
            logger.debug("Could not emit YoooClaw APP event %s/%s", name, status, exc_info=True)


def register(ctx: Any) -> None:
    logs_dir = install_plugin_file_logger()
    logger.info("YoooClaw APP adapter file logging enabled: %s", logs_dir)
    _log_startup_context()
    ctx.register_platform(
        name="yoooclaw_app",
        label="YoooClaw APP",
        adapter_factory=lambda config: YoooclawAppAdapter(config),
        check_fn=lambda: True,
        validate_config=lambda config: bool(resolve_api_key() or (getattr(config, "extra", {}) or {}).get("api_key")),
        env_enablement_fn=_env_enablement,
        platform_hint="You are replying inside the YoooClaw APP. Keep responses concise and mobile-friendly.",
        pii_safe=True,
    )


def _env_enablement() -> dict[str, Any] | None:
    api_key = resolve_api_key()
    if not api_key:
        return None
    return {"api_key": api_key}


def _log_startup_context() -> None:
    """Emit the effective runtime identity once per process (docs §7.2.C).

    Answers "which profile's instance connected to which Relay" without needing
    any secret value — every field is either an id, a path, or a fingerprint.
    """
    try:
        from importlib import metadata
        from urllib.parse import urlsplit, urlunsplit

        from yoooclaw_hermes.install import default_hermes_home
        from yoooclaw_hermes.paths import resolve_hermes_config_path, resolve_hermes_profile, resolve_profile_paths
        from yoooclaw_hermes.runtime.lifecycle import _api_key_fingerprint, get_instance_id
        from yoooclaw_hermes.runtime.openclaw_relay import resolve_openclaw_relay_url

        try:
            version = metadata.version("yoooclaw-hermes-plugin")
        except metadata.PackageNotFoundError:
            version = "dev"
        hermes_profile = resolve_hermes_profile()
        relay_url = resolve_openclaw_relay_url()
        try:
            split = urlsplit(relay_url)
            relay_host = urlunsplit((split.scheme, split.netloc, split.path, "", ""))
        except ValueError:
            relay_host = relay_url
        logger.info(
            "YoooClaw plugin startup context package_version=%s pid=%s instance_id=%s hermes_profile=%s yoooclaw_profile=%s effective_config=%s relay_host=%s api_key_fingerprint=%s",
            version,
            os.getpid(),
            get_instance_id(),
            hermes_profile or "",
            resolve_profile_paths().profile,
            resolve_hermes_config_path(default_hermes_home(), hermes_profile),
            relay_host,
            _api_key_fingerprint(),
        )
    except Exception:  # noqa: BLE001 - startup context must never block registration.
        logger.debug("Could not log YoooClaw startup context", exc_info=True)


def _float_env(name: str, default: float) -> float:
    try:
        value = float(os.environ.get(name, ""))
    except ValueError:
        return default
    return value if value > 0 else default


def _int_env(name: str, default: int) -> int:
    try:
        value = int(os.environ.get(name, ""))
    except ValueError:
        return default
    return value if value > 0 else default


def _looks_like_streaming_preview(content: str) -> bool:
    return content.endswith("▉")


def _parse_gateway_approval_fallback(content: str) -> tuple[str, str] | None:
    """Recognize Hermes' exact plain-text dangerous-command fallback.

    This is deliberately strict so ordinary assistant prose mentioning
    ``/approve`` is never converted into an actionable confirmation card.
    """
    prefix = "⚠️ **Dangerous command requires approval:**\n```\n"
    reason_marker = "\n```\nReason: "
    reply_marker = "\n\nReply `"
    if not content.startswith(prefix):
        return None
    reason_at = content.find(reason_marker, len(prefix))
    if reason_at < 0:
        return None
    reply_at = content.find(reply_marker, reason_at + len(reason_marker))
    if reply_at < 0:
        return None
    instructions = content[reply_at:]
    if "approve" not in instructions or "deny" not in instructions or "to cancel." not in instructions:
        return None
    command = content[len(prefix):reason_at].strip()
    description = content[reason_at + len(reason_marker):reply_at].strip()
    if not command or not description:
        return None
    return command, description


def _outcome_value(outcome: Any) -> str:
    value = getattr(outcome, "value", outcome)
    if isinstance(value, str):
        return value
    return str(value).lower()
