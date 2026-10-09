"""镜头定义：SCENES[i] 对应 narration.txt 第 i+1 行。
每个镜头 = 背景色调 + 若干动画图层；图层是一段 HTML（绝对定位，1920x1080 画布，透明底），
anim: fade | up | down | left | right（入场方向），at: 入场秒数，float: 入场后轻微漂浮。
真实截图在 shots/（iOS 模拟器演示模式拍摄，数据均为虚构）。"""
import pathlib

HERE = pathlib.Path(__file__).resolve().parent
SHOTS = HERE / "shots"
LOGO = HERE / "assets" / "logo.svg"
ASPECT = {"phone": 1206 / 2622, "ipad": 2064 / 2752}


def L(html, at=0.2, anim="fade", float_=False):
    return dict(html=html, at=at, anim=anim, float=float_)


def phone(name, cx, top, h, rot=0, kind="phone"):
    w = h * ASPECT[kind]
    r = h * (0.062 if kind == "phone" else 0.035)
    return (f'<img src="file://{SHOTS}/{name}.png" style="position:absolute;left:{cx - w / 2:.0f}px;top:{top}px;'
            f'width:{w:.0f}px;height:{h}px;border-radius:{r:.0f}px;transform:rotate({rot}deg);'
            f'box-shadow:0 0 0 7px #1c1c1e,0 0 0 9px #48484a,0 40px 110px rgba(91,91,214,.55)">')


def text(kicker, l1, l2, left=140, top=240, w=900, size=104):
    return (f'<div style="position:absolute;left:{left}px;top:{top}px;width:{w}px">'
            f'<div class=k>{kicker}</div><h1 style="font-size:{size}px">{l1}<br><em>{l2}</em></h1></div>')


def _cw(s):
    return sum(34 if ord(c) > 255 else 19 for c in s) + 60


def chips(items, left, top, at, step=0.6, maxw=860, colors=None, anim="up"):
    """每个标签一个图层，依次弹出，自动换行。"""
    out, x, y = [], left, top
    for i, s in enumerate(items):
        w = _cw(s)
        if x + w > left + maxw:
            x, y = left, y + 74
        col = colors[i] if colors else "#f5f5f7"
        out.append(L(f'<b class=chip style="left:{x}px;top:{y}px;color:{col}">{s}</b>', at + i * step, anim))
        x += w + 16
    return out


def notif(icon, title, sub, tag, color, top):
    return (f'<div class=notif style="top:{top}px"><span class=ic>{icon}</span>'
            f'<div style="flex:1"><div class=nt>{title}</div><div class=ns>{sub}</div></div>'
            f'<span class=tag style="color:{color};border-color:{color}">{tag}</span></div>')


def fan(at=0.3):
    return [
        L(phone("question", 1160, 150, 780, -8), at, "up", True),
        L(phone("cockpit", 1660, 150, 780, 8), at + 0.3, "up", True),
        L(phone("list", 1410, 90, 880), at + 0.6, "up", True),
    ]


def logo(h, left, top):
    return f'<img src="file://{LOGO}" style="position:absolute;height:{h}px;left:{left}px;top:{top}px">'


def box(inner, left, top, w):
    return f'<div style="position:absolute;left:{left}px;top:{top}px;width:{w}px">{inner}</div>'


def node(emoji, label, sub, x, y):
    return (f'<div class=node style="left:{x}px;top:{y}px"><div class=ne>{emoji}</div>'
            f'<div class=nl>{label}</div><div class=nsub>{sub}</div></div>')


def path(label, sub, y, color):
    return (f'<div class=path style="top:{y}px;border-color:{color}"><div class=pl style="color:{color}">{label}</div>'
            f'<div class=ps>{sub}</div></div>')


def term(lines_html, left, top, w):
    return f'<div class=term style="left:{left}px;top:{top}px;width:{w}px">{lines_html}</div>'


GREEN, RED, ORANGE, BLUE = "#30d158", "#ff453a", "#ff9f0a", "#0a84ff"

SCENES = [
    # 1 开场
    dict(glow=("#3b3bb8", "#0a84ff"), layers=[
        L(logo(130, 140, 270), 0.1, "left"),
        L('<h1 style="position:absolute;left:140px;top:420px;font-size:120px">Multi<em>gravity</em></h1>', 0.3, "left"),
        L('<p class=s style="position:absolute;left:140px;top:600px">Google Antigravity 的原生移动伴侣</p>', 0.9, "fade"),
    ] + fan(0.4)),
    # 2 痛点：通知卡片依次掉落
    dict(glow=("#7a2b2b", "#3b3bb8"), layers=[
        L('<div style="position:absolute;left:0;width:1920px;text-align:center;top:90px"><div class=k>PAIN POINT</div>'
          '<h1 style="font-size:96px">Agent 在等你点头，<em>你却不在电脑前</em></h1></div>', 0.1, "down"),
        L(notif("⚠️", "登录接口 500 错误排查", "backend · 3 步骤 · 刚刚", "ERROR", RED, 400), 1.2, "up"),
        L(notif("❓", "数据库迁移方案评审", "需要你确认 3 个问题 · 等待回应", "ACTION", BLUE, 560), 2.6, "up"),
        L(notif("⏸", "整条流水线已停住", "你在通勤、用餐、开会……", "WAITING", ORANGE, 720), 4.2, "up"),
    ]),
    # 3 会话列表
    dict(glow=("#2f6b3d", "#3b3bb8"), layers=[
        L(text("全局视野", "所有任务", "一眼看清"), 0.1, "left"),
        *chips(["RUNNING", "ERROR", "ACTION", "额度常驻顶部"], 140, 600, 2.2, 0.8, colors=[GREEN, RED, BLUE, "#f5f5f7"]),
        L(phone("list", 1460, 80, 900), 0.3, "up", True),
    ]),
    # 4 选择题 / 审批
    dict(glow=("#a35a00", "#3b3bb8"), layers=[
        L(text("一触即决", "Agent 提问", "你只需轻点"), 0.1, "left"),
        *chips(["多题一次提交", "高危命令审批", "推送直达会话"], 140, 600, 2.4, 0.9),
        L(phone("chat_conclusion", 1230, 220, 700, -7), 3.6, "up", True),
        L(phone("question", 1580, 80, 900), 0.3, "up", True),
    ]),
    # 5 方案 Proceed
    dict(glow=("#3b3bb8", "#0a84ff"), layers=[
        L(text("方案放行", "随手看", "一键 Proceed"), 0.1, "left"),
        *chips(["原生预览", "可撤回", "可回滚"], 140, 600, 2.6, 0.9),
        L(phone("plan", 1210, 140, 800, -5), 0.3, "up", True),
        L(phone("plan_preview", 1640, 80, 900), 2.2, "right", True),
    ]),
    # 6 Cockpit
    dict(glow=("#3b3bb8", "#30d158"), layers=[
        L(text("配额罗盘", "额度告急？", "一键换号"), 0.1, "left"),
        *chips(["Claude · Gemini 四象限", "多账号热切", "失败自动回滚"], 140, 600, 2.0, 0.9),
        L(phone("cockpit", 1460, 80, 900), 0.3, "up", True),
    ]),
    # 7 连接：示意图依次出现
    dict(glow=("#0a5a8a", "#3b3bb8"), layers=[
        L('<div style="position:absolute;left:0;width:1920px;text-align:center;top:90px"><div class=k>连接无忧</div>'
          '<h1 style="font-size:96px">家里直连，<em>外出加密隧道</em></h1></div>', 0.1, "down"),
        L(node("📱", "Multigravity", "iOS · Android · Web", 150, 470), 0.8, "left"),
        L(node("💻", "你的电脑", "Antigravity + mgy 网关", 1370, 470), 1.2, "right"),
        L(path("局域网直连 · 低延迟", "同一 Wi-Fi 下自动走内网", 440, GREEN), 2.2, "fade"),
        L(path("Cloudflare HTTPS 加密隧道", "外出或异网自动切换，免翻墙免配置", 640, BLUE), 3.8, "fade"),
        *chips(["▣ 扫码配对", "一键测速选路"], 700, 860, 5.2, 0.8),
    ]),
    # 8 全平台
    dict(glow=("#3b3bb8", "#ff9f0a"), layers=[
        L(text("全平台原生", "一套体验", "所有设备", top=150, w=700), 0.1, "left"),
        *chips(["Android", "iPhone", "iPad", "Web", "全文搜索", "长图分享", "Git 提交"], 140, 520, 3.0, 0.55, maxw=620),
        L(phone("ipad", 1020, 130, 820, 0, "ipad"), 0.3, "up", True),
        L(phone("pdf", 1560, 150, 860, 5), 1.8, "right", True),
    ]),
    # 9 安装
    dict(glow=("#1f6b3a", "#3b3bb8"), layers=[
        L(text("一分钟上手", "一条命令", "扫码即连", top=170), 0.1, "left"),
        L(term('<span class=g>$</span> curl -fsSL …/install.sh | bash', 140, 620, 1640), 0.4, "up"),
        L(term('<span class=d>✓ 已安装 mgy</span>', 140, 700, 1640), 1.1, "up"),
        L(term('<span class=g>$</span> mgy', 140, 780, 1640), 1.7, "up"),
        L(term('<span class=w>▣▣▣  终端打印配对二维码，手机扫一下就连上</span>', 140, 860, 1640), 2.3, "up"),
    ]),
    # 10 收尾
    dict(glow=("#3b3bb8", "#0a84ff"), layers=[
        L(logo(110, 140, 250), 0.1, "left"),
        L('<h1 style="position:absolute;left:140px;top:390px;font-size:104px">把 Antigravity<br><em>装进口袋</em></h1>', 0.3, "left"),
        L('<p class=s style="position:absolute;left:140px;top:700px;font-size:34px">github.com/GHSaiMo/antigravity-mobile<br>macOS · Windows · Linux · Android · iOS</p>', 1.0, "fade"),
    ] + fan(0.3)),
]
