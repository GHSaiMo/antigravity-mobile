#!/usr/bin/env python3
"""Multigravity 介绍视频一键生成：narration.txt + slides.py + shots/ -> mp4

流程：TTS 逐句配音 -> 量音频时长 -> Chrome 渲染每个动画图层(透明PNG) -> ffmpeg 叠加入场动画 -> 拼接。
依赖：ffmpeg、Google Chrome、uv（`uvx edge-tts` 免安装）。仅限 macOS 路径；Linux 改 CHROME 环境变量。

用法：
  scripts/video/build.py                          # 豆包配音（需密钥，见下），输出 dist/video/Multigravity-intro.mp4
  scripts/video/build.py --engine edge            # 免费 edge-tts（晓晓），无需密钥
  scripts/video/build.py --voice zh_female_vv_uranus_bigtts --engine volc
  scripts/video/build.py --out ~/Desktop/intro.mp4
  scripts/video/build.py --fresh                  # 忽略缓存重新配音与渲染
豆包密钥：环境变量 VOLC_API_KEY，或 ~/.config/multigravity/volc.env 里的 VOLC_API_KEY=...（勿提交仓库）。
配音与图层按内容缓存，只改一句文案只重配那一句。
截图来自 iOS 模拟器「演示模式」（数据均为虚构），放在 shots/，更新 App 后重拍即可。
"""
import argparse, base64, hashlib, html, importlib.util, json, os, pathlib, subprocess, sys, urllib.request, uuid

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[1]
CHROME = os.environ.get("CHROME", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")
GAP = 0.35
FPS = 30

CSS = '''*{margin:0;box-sizing:border-box}html,body{width:1920px;height:1080px;overflow:hidden;background:transparent;color:#f5f5f7;font-family:"PingFang SC","Hiragino Sans GB","STHeiti",sans-serif}
.k{color:#ff9f0a;font-size:34px;font-weight:600;letter-spacing:4px;margin-bottom:24px}
h1{font-weight:800;line-height:1.15;letter-spacing:-2px}h1 em{font-style:normal;background:linear-gradient(90deg,#7b7bff,#0a84ff);-webkit-background-clip:text;color:transparent}
p.s{font-size:42px;color:#a1a1a6;line-height:1.5}
.chip{position:absolute;font-size:32px;font-weight:600;padding:12px 26px;border-radius:40px;border:2px solid #3a3a3c;background:#1c1c1e;white-space:nowrap}
.sub{position:absolute;left:0;right:0;bottom:36px;text-align:center;font-size:38px;color:#fff;text-shadow:0 2px 12px #000}.sub span{background:rgba(0,0,0,.6);padding:10px 28px;border-radius:14px}
.notif{position:absolute;left:360px;width:1200px;height:128px;display:flex;align-items:center;gap:26px;padding:0 36px;border-radius:30px;background:rgba(44,44,46,.92);border:2px solid #48484a;box-shadow:0 20px 60px rgba(0,0,0,.5)}
.notif .ic{font-size:54px}.nt{font-size:38px;font-weight:700}.ns{font-size:27px;color:#a1a1a6;margin-top:6px}.tag{font-size:26px;font-weight:700;border:2px solid;border-radius:12px;padding:6px 16px}
.node{position:absolute;width:400px;height:300px;border-radius:36px;background:#1c1c1e;border:2px solid #48484a;text-align:center;padding-top:38px;box-shadow:0 30px 80px rgba(91,91,214,.4)}
.ne{font-size:96px}.nl{font-size:38px;font-weight:700;margin-top:10px}.nsub{font-size:26px;color:#a1a1a6;margin-top:8px}
.path{position:absolute;left:600px;width:720px;height:150px;border-top:4px dashed;text-align:center;padding-top:18px}.pl{font-size:38px;font-weight:700}.ps{font-size:26px;color:#a1a1a6;margin-top:8px}
.term{position:absolute;font:40px/1.3 Menlo,monospace;background:#111;border:2px solid #333;border-radius:18px;padding:14px 36px;color:#f5f5f7;height:72px}
.term .g{color:#30d158}.term .d{color:#a1a1a6}.term .w{color:#fff;font-family:"PingFang SC",sans-serif;font-weight:700}'''


def run(cmd, **kw):
    subprocess.run(cmd, check=True, stdin=subprocess.DEVNULL, **kw)


def duration(path):
    out = subprocess.run(["ffmpeg", "-nostdin", "-i", str(path), "-f", "null", "-"], capture_output=True, text=True).stderr
    t = [l for l in out.splitlines() if "time=" in l][-1].split("time=")[1].split()[0]
    h, m, s = t.split(":")
    return int(h) * 3600 + int(m) * 60 + float(s)


def volc_key():
    k = os.environ.get("VOLC_API_KEY")
    f = pathlib.Path.home() / ".config/multigravity/volc.env"
    if not k and f.exists():
        for line in f.read_text().splitlines():
            if line.startswith("VOLC_API_KEY="):
                k = line.split("=", 1)[1].strip()
    if not k:
        sys.exit("缺少 VOLC_API_KEY（环境变量或 ~/.config/multigravity/volc.env），或改用 --engine edge")
    return k


def tts(engine, voice, rate, text, out):
    if engine == "edge":
        run(["uvx", "edge-tts", "--voice", voice, "--rate", rate, "--text", text, "--write-media", str(out)], stdout=subprocess.DEVNULL)
        return
    body = {"req_params": {"speaker": voice, "text": text, "audio_params": {"format": "mp3", "sample_rate": 24000, "speech_rate": int(rate)}}}
    req = urllib.request.Request("https://openspeech.bytedance.com/api/v3/tts/unidirectional", data=json.dumps(body).encode(), method="POST",
                                 headers={"Content-Type": "application/json", "X-Api-Key": volc_key(),
                                          "X-Api-Resource-Id": "seed-tts-2.0", "X-Api-Request-Id": str(uuid.uuid4())})
    audio = bytearray()
    with urllib.request.urlopen(req, timeout=90) as r:
        for line in r:
            line = line.strip()
            if not line:
                continue
            j = json.loads(line)
            if j.get("data"):
                audio += base64.b64decode(j["data"])
            elif j.get("code") not in (0, 20000000):
                sys.exit(f"豆包 TTS 失败: {j}")
    if not audio:
        sys.exit("豆包 TTS 未返回音频")
    out.write_bytes(audio)


def shot(body, png, bg=None):
    """Chrome 渲染一个图层；bg=None 为透明底。"""
    page = png.with_suffix(".html")
    page.write_text(f'<meta charset=utf-8><style>{CSS}</style>{body}', encoding="utf-8")
    args = [CHROME, "--headless=new", "--disable-gpu", "--hide-scrollbars", "--allow-file-access-from-files",
            "--window-size=1920,1080", f"--screenshot={png}"]
    if bg is None:
        args.append("--default-background-color=00000000")
    run(args + [f"file://{page}"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def cached_shot(work, body, bg=None):
    key = hashlib.sha1((body + str(bg)).encode()).hexdigest()[:12]
    png = work / "layers" / f"{key}.png"
    if not png.exists():
        png.parent.mkdir(exist_ok=True)
        shot(body, png, bg)
    return png


def bg_html(glow):
    a, b = glow
    return (f'<body style="background:radial-gradient(1100px 800px at 78% 25%,{a}66,transparent 70%),'
            f'radial-gradient(900px 700px at 8% 95%,{b}44,transparent 70%),#000"></body>')


OFF = {"fade": (0, 0), "up": (0, 120), "down": (0, -90), "left": (-160, 0), "right": (160, 0)}


def compose(work, i, scene, sub_png, bg_png, wav, T, seg):
    inputs = ["-loop", "1", "-framerate", str(FPS), "-t", f"{T:.2f}", "-i", str(bg_png)]
    layers = [(cached_shot(work, l["html"]), l) for l in scene["layers"] if l["at"] < T - 0.3]
    layers.append((sub_png, dict(at=0.15, anim="fade", float=False)))
    for png, _ in layers:
        inputs += ["-loop", "1", "-framerate", str(FPS), "-t", f"{T:.2f}", "-i", str(png)]
    inputs += ["-i", str(wav)]
    f, prev = [], "0:v"
    for k, (_, l) in enumerate(layers, 1):
        at = l["at"]; dx, dy = OFF[l["anim"]]
        E = f"(1-pow(1-min(max((t-{at})/0.8,0),1),3))"
        fl = f"+7*sin((t-{at})*1.5)*min(max(t-{at}-0.8,0),1)" if l.get("float") else ""
        f.append(f"[{k}:v]format=rgba,fade=t=in:st={at}:d=0.5:alpha=1[l{k}]")
        f.append(f"[{prev}][l{k}]overlay=x='{dx}*(1-{E})':y='{dy}*(1-{E}){fl}':format=auto[o{k}]")
        prev = f"o{k}"
    n = len(layers) + 1
    f.append(f"[{prev}]fade=t=in:d=0.25,fade=t=out:st={T - 0.25:.2f}:d=0.25,format=yuv420p[v]")
    f.append(f"[{n}:a]apad=whole_dur={T:.2f}[a]")
    run(["ffmpeg", "-nostdin", "-loglevel", "error", "-y", *inputs, "-filter_complex", ";".join(f), "-map", "[v]", "-map", "[a]",
         "-c:v", "libx264", "-preset", "fast", "-crf", "19", "-c:a", "aac", "-b:a", "160k", "-t", f"{T:.2f}", str(seg)])


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--engine", choices=["volc", "edge"], default="volc")
    ap.add_argument("--voice", help="音色；volc 默认 zh_female_vv_uranus_bigtts，edge 默认 zh-CN-XiaoxiaoNeural")
    ap.add_argument("--rate", help="语速；volc 为 -50..100 整数（默认 15），edge 如 +20%%（默认 +22%%）")
    ap.add_argument("--out", default=str(ROOT / "dist/video/Multigravity-intro.mp4"))
    ap.add_argument("--fresh", action="store_true")
    a = ap.parse_args()
    voice = a.voice or ("zh_female_vv_uranus_bigtts" if a.engine == "volc" else "zh-CN-XiaoxiaoNeural")
    rate = a.rate or ("15" if a.engine == "volc" else "+22%")

    spec = importlib.util.spec_from_file_location("slides", HERE / "slides.py")
    mod = importlib.util.module_from_spec(spec); spec.loader.exec_module(mod)
    lines = [l.strip() for l in (HERE / "narration.txt").read_text(encoding="utf-8").splitlines() if l.strip()]
    if len(lines) != len(mod.SCENES):
        sys.exit(f"narration.txt 有 {len(lines)} 行，slides.py 有 {len(mod.SCENES)} 个镜头，需一一对应")

    work = ROOT / "dist/video/work"; work.mkdir(parents=True, exist_ok=True)
    segs = []
    for i, (text, scene) in enumerate(zip(lines, mod.SCENES), 1):
        key = hashlib.sha1(f"{a.engine}|{voice}|{rate}|{text}".encode()).hexdigest()[:10]
        mp3 = work / f"a{i}-{key}.mp3"
        if a.fresh or not mp3.exists():
            print(f"[配音 {i}/{len(lines)}] {text}")
            for old in work.glob(f"a{i}-*.mp3"): old.unlink()
            tts(a.engine, voice, rate, text, mp3)
        wav = work / f"a{i}.wav"
        run(["ffmpeg", "-nostdin", "-loglevel", "error", "-y", "-i", str(mp3), "-ar", "44100", "-ac", "1", str(wav)])
        T = duration(wav) + GAP
        bg_png = cached_shot(work, bg_html(scene["glow"]), bg="#000")
        sub_png = cached_shot(work, f'<div class=sub><span>{html.escape(text)}</span></div>')
        seg = work / f"seg{i}.mp4"
        print(f"[合成 {i}/{len(lines)}] {T:.1f}s")
        compose(work, i, scene, sub_png, bg_png, wav, T, seg)
        segs.append(seg)

    lst = work / "list.txt"
    lst.write_text("".join(f"file '{s}'\n" for s in segs))
    out = pathlib.Path(a.out).expanduser(); out.parent.mkdir(parents=True, exist_ok=True)
    run(["ffmpeg", "-nostdin", "-loglevel", "error", "-y", "-f", "concat", "-safe", "0", "-i", str(lst), "-c", "copy", str(out)])
    print(f"✅ {out}  {duration(out):.1f}s")


if __name__ == "__main__":
    main()
