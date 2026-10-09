#!/usr/bin/env python3
"""Multigravity 介绍视频一键生成：narration.txt + slides.py -> mp4

流程：edge-tts 逐句配音 -> 量音频时长 -> Chrome 截图画面 -> ffmpeg 推镜头/淡入淡出 -> 拼接。
依赖：ffmpeg、Google Chrome、uv（`uvx edge-tts` 免安装）。仅限 macOS 路径；Linux 改 CHROME 环境变量。

用法：
  scripts/video/build.py                       # 默认音色 晓晓 +22%，输出 dist/video/Multigravity-intro.mp4
  scripts/video/build.py --voice zh-CN-YunxiNeural --rate +10%
  scripts/video/build.py --out ~/Desktop/intro.mp4
  scripts/video/build.py --list-voices         # 列出中文音色
  scripts/video/build.py --fresh               # 忽略缓存，重新配音与渲染
配音按「文案+音色+语速」缓存，只改了某一句时只重配那一句。
"""
import argparse, hashlib, html, importlib.util, os, pathlib, subprocess, sys

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[1]
CHROME = os.environ.get("CHROME", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")
GAP = 0.3  # 每镜头在配音后多留的秒数

CSS = '''*{margin:0;box-sizing:border-box}body{width:1920px;height:1080px;overflow:hidden;background:radial-gradient(1200px 800px at 75% 30%,#1b1b3a,#000 70%);color:#f5f5f7;font-family:"PingFang SC","Hiragino Sans GB","STHeiti",sans-serif;position:relative}
.t{position:absolute;left:140px;top:250px;width:900px}.k{color:#ff9f0a;font-size:34px;font-weight:600;letter-spacing:4px;margin-bottom:24px}
h1{font-size:104px;line-height:1.15;font-weight:800;letter-spacing:-2px}h1 em{font-style:normal;background:linear-gradient(90deg,#5b5bd6,#0a84ff);-webkit-background-clip:text;color:transparent}
p.s{font-size:40px;color:#a1a1a6;margin-top:36px;line-height:1.5}
.ph{position:absolute;right:190px;top:80px;height:920px;border-radius:56px;box-shadow:0 0 0 10px #1c1c1e,0 0 0 12px #3a3a3c,0 40px 120px rgba(91,91,214,.55)}
.sub{position:absolute;left:0;right:0;bottom:44px;text-align:center;font-size:40px;color:#fff;text-shadow:0 2px 12px #000}.sub span{background:rgba(0,0,0,.55);padding:10px 28px;border-radius:14px}
.chips{display:flex;gap:18px;flex-wrap:wrap;margin-top:44px}.chips b{font-size:32px;font-weight:600;padding:12px 26px;border-radius:40px;border:2px solid #3a3a3c;background:#1c1c1e}
.code{background:#111;border:2px solid #333;border-radius:20px;padding:34px 44px;font:34px/1.6 Menlo,monospace;color:#30d158;margin-top:40px}
.card{background:#1c1c1e;border-radius:28px;padding:40px;margin-top:30px;font-size:34px;line-height:1.6;border:2px solid #333}'''


def run(cmd, **kw):
    subprocess.run(cmd, check=True, stdin=subprocess.DEVNULL, **kw)


def duration(path):
    out = subprocess.run(["ffmpeg", "-nostdin", "-i", str(path), "-f", "null", "-"], capture_output=True, text=True).stderr
    t = [l for l in out.splitlines() if "time=" in l][-1].split("time=")[1].split()[0]
    h, m, s = t.split(":")
    return int(h) * 3600 + int(m) * 60 + float(s)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--voice", default="zh-CN-XiaoxiaoNeural")
    ap.add_argument("--rate", default="+22%", help="语速，如 +10%% / -5%%")
    ap.add_argument("--out", default=str(ROOT / "dist/video/Multigravity-intro.mp4"))
    ap.add_argument("--fresh", action="store_true")
    ap.add_argument("--list-voices", action="store_true")
    a = ap.parse_args()
    if a.list_voices:
        os.system("uvx edge-tts --list-voices | grep -E 'zh-CN|Name'")
        return

    spec = importlib.util.spec_from_file_location("slides", HERE / "slides.py")
    mod = importlib.util.module_from_spec(spec); spec.loader.exec_module(mod)
    lines = [l.strip() for l in (HERE / "narration.txt").read_text(encoding="utf-8").splitlines() if l.strip()]
    if len(lines) != len(mod.slides):
        sys.exit(f"narration.txt 有 {len(lines)} 行，slides.py 有 {len(mod.slides)} 个镜头，需一一对应")

    work = ROOT / "dist/video/work"; work.mkdir(parents=True, exist_ok=True)
    segs = []
    for i, (text, body) in enumerate(zip(lines, mod.slides), 1):
        key = hashlib.sha1(f"{a.voice}|{a.rate}|{text}".encode()).hexdigest()[:10]
        mp3 = work / f"a{i}-{key}.mp3"
        if a.fresh or not mp3.exists():
            print(f"[配音 {i}/{len(lines)}] {text}")
            for old in work.glob(f"a{i}-*.mp3"): old.unlink()
            run(["uvx", "edge-tts", "--voice", a.voice, "--rate", a.rate, "--text", text, "--write-media", str(mp3)], stdout=subprocess.DEVNULL)
        wav = work / f"a{i}.wav"
        run(["ffmpeg", "-nostdin", "-loglevel", "error", "-y", "-i", str(mp3), "-ar", "44100", "-ac", "1", str(wav)])
        T = duration(wav) + GAP

        page = work / f"s{i}.html"
        page.write_text(f'<meta charset=utf-8><style>{CSS}</style>{body}<div class=sub><span>{html.escape(text)}</span></div>', encoding="utf-8")
        png = work / f"s{i}.png"
        run([CHROME, "--headless=new", "--disable-gpu", "--hide-scrollbars", "--allow-file-access-from-files",
             "--window-size=1920,1080", f"--screenshot={png}", f"file://{page}"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

        seg = work / f"seg{i}.mp4"
        F = int(T * 30)
        vf = (f"[0:v]scale=2400:-1,zoompan=z='1+0.06*on/{F}':x='iw/2-iw/zoom/2':y='ih/2-ih/zoom/2':d=1:s=1920x1080:fps=30,"
              f"fade=t=in:d=0.25,fade=t=out:st={T-0.25:.2f}:d=0.25,format=yuv420p[v];[1:a]apad=whole_dur={T:.2f}[a]")
        run(["ffmpeg", "-nostdin", "-loglevel", "error", "-y", "-loop", "1", "-framerate", "30", "-t", f"{T:.2f}", "-i", str(png), "-i", str(wav),
             "-filter_complex", vf, "-map", "[v]", "-map", "[a]", "-c:v", "libx264", "-preset", "fast", "-crf", "18",
             "-c:a", "aac", "-b:a", "160k", "-t", f"{T:.2f}", str(seg)])
        segs.append(seg)

    lst = work / "list.txt"
    lst.write_text("".join(f"file '{s}'\n" for s in segs))
    out = pathlib.Path(a.out).expanduser(); out.parent.mkdir(parents=True, exist_ok=True)
    run(["ffmpeg", "-nostdin", "-loglevel", "error", "-y", "-f", "concat", "-safe", "0", "-i", str(lst), "-c", "copy", str(out)])
    print(f"✅ {out}  {duration(out):.1f}s")


if __name__ == "__main__":
    main()
