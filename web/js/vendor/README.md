# 第三方脚本

| 文件 | 来源 | 版本 | 许可 |
|---|---|---|---|
| `jsQR.js` | npm `jsqr`（https://github.com/cozmo/jsQR） | 1.4.0（tarball sha1 `8efb8d0a7cc6863cb6d95116b9069123ce9eb2d1`，与 npm 登记一致） | Apache-2.0，见 `jsQR.LICENSE.txt` |
| `katex/` | npm `katex`（https://katex.org） | 0.19.1（含 `katex.min.js`、`mhchem.min.js`、`katex.min.css` 与 20 个 `.woff2` 字体） | MIT，见 `katex/LICENSE` |

- `jsQR.js` 原样收录官方 `dist/jsQR.js`，未修改；纯计算代码，不含网络、DOM、`eval` 调用；仅在 Web 配对页点「扫描二维码」时按需加载（`js/scan.js` 里的 `loadJsQR`）。
- `katex/` 收录官方 `dist/katex.min.js`、`dist/contrib/mhchem.min.js`、全部 `.woff2` 字体，以及去除 `.woff`/`.ttf` 回退后的 `katex.min.css`，离线同步渲染 LaTeX 数学与化学公式（开启 `trust: false`）。
- 升级时重新核对 tarball 校验和，并重新审查是否出现网络 / 动态执行相关调用。
