"""每个镜头的画面。slides[i] 对应 narration.txt 第 i+1 行；返回 HTML 片段（字幕由 build.py 叠加）。
改画面只需编辑本文件，改文案编辑 narration.txt。"""
import pathlib

ROOT = pathlib.Path(__file__).resolve().parents[2]
IMG = ROOT / "images"
LOGO = pathlib.Path(__file__).resolve().parent / "assets" / "logo.svg"


def phone(name):
    return f'<img class=ph src="file://{IMG}/{name}">'


def chips(*items):
    return '<div class=chips>' + ''.join(f'<b>{c}</b>' for c in items) + '</div>'


def left(kicker, line1, line2, extra=''):
    return f'<div class=t><div class=k>{kicker}</div><h1>{line1}<br><em>{line2}</em></h1>{extra}</div>'


slides = [
    f'<div class=t style="top:300px"><img src="file://{LOGO}" style="height:150px;margin-bottom:30px"><h1>Multi<em>gravity</em></h1><p class=s>Google Antigravity 的原生移动伴侣<br>随时随地，唤醒你的 Agent</p></div>',
    '<div class=t style="left:0;width:1920px;text-align:center;top:300px"><div class=k>PAIN POINT</div><h1>Agent 在等你点头<br><em>你却不在电脑前</em></h1><p class=s>通勤、用餐、开会 — 一次确认卡住，整条流水线停摆</p></div>',
    left('全局视野', '所有任务', '一眼看清',
         '<div class=chips><b style="color:#30d158">RUNNING</b><b style="color:#ff453a">ERROR</b><b style="color:#ff9f0a">ACTION</b><b>额度常驻顶部</b></div>') + phone('session_list.jpg'),
    left('一触即决', 'Agent 提问', '你只需轻点', chips('多题一次提交', '高危命令审批', '推送直达会话')) + phone('native_components.jpg'),
    left('方案放行', '随手看', '一键 Proceed',
         '<div class=card>📄 implementation_plan.md<br>① 首屏只保留一个主行动按钮<br>② 客户案例上移到第二屏<br>③ 图片转 WebP 并懒加载<br><span style="color:#ff9f0a;font-weight:700">▶ Proceed</span> &nbsp;&nbsp;↩ 撤回 &nbsp;&nbsp;⟲ 回滚</div>'),
    left('配额罗盘', '额度告急？', '一键换号', chips('Claude · Gemini 四象限', '多账号热切', '失败自动回滚')) + phone('cockpit_tools.jpg'),
    left('连接无忧', '家里直连', '外出加密隧道', chips('局域网低延迟', 'Cloudflare HTTPS', '扫码配对', '一键测速选路')) + phone('network_settings.jpg'),
    f'<div class=t style="top:140px;width:760px"><div class=k>全平台原生</div><h1>一套体验<br><em>所有设备</em></h1>{chips("Android","iPhone","iPad","Web","全文搜索","长图分享","Git 提交")}</div>'
    f'<img class=ph style="right:80px;top:200px;height:auto;width:1000px;border-radius:28px" src="file://{IMG}/ipad_overview.jpg">',
    left('一分钟上手', '一条命令', '扫码即连',
         '<div class=code>$ curl -fsSL …/install.sh | bash<br>$ mgy<br><span style="color:#fff">▣▣▣ 终端打印配对二维码 ▣▣▣</span></div>'),
    f'<div class=t style="left:0;width:1920px;text-align:center;top:300px"><img src="file://{LOGO}" style="height:130px;margin-bottom:24px"><h1>把 Antigravity<br><em>装进口袋</em></h1><p class=s>github.com/GHSaiMo/antigravity-mobile · macOS / Windows / Linux / Android / iOS</p></div>',
]
