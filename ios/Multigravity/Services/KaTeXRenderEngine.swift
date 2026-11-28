import Foundation
import SwiftUI
import UIKit
import WebKit

/// Rendered KaTeX formula bitmap and typographic layout metrics.
public struct RenderedMathFormula: @unchecked Sendable {
    public let image: UIImage
    public let width: CGFloat
    public let height: CGFloat
    /// Distance in logical points from the bottom of `image` to the math text baseline.
    public let baselineFromBottom: CGFloat
    /// True when the formula carries no explicit `\color` directive and can be tinted with `.template`.
    public let isMonochrome: Bool
}

private final class RenderedMathCacheEntry: NSObject {
    let formula: RenderedMathFormula?
    init(_ formula: RenderedMathFormula?) {
        self.formula = formula
    }
}

/// Global singleton offscreen KaTeX renderer backed by a single lazily-created WKWebView.
/// Produces transparent Retina UIImages cached in memory so LazyVStack scrolling,
/// theme switching, and ImageRenderer share-card exports remain 120fps and WebView-free.
@MainActor
public final class KaTeXRenderEngine: NSObject, WKNavigationDelegate {
    public static let shared = KaTeXRenderEngine()

    nonisolated(unsafe) private let cache: NSCache<NSString, RenderedMathCacheEntry> = {
        let c = NSCache<NSString, RenderedMathCacheEntry>()
        c.countLimit = 400
        return c
    }()

    private var webView: WKWebView?
    private var hostContainer: UIView?
    private var isPageLoaded = false
    private var pageReadyWaiters: [CheckedContinuation<Bool, Never>] = []
    private var inFlightWaiters: [String: [CheckedContinuation<RenderedMathFormula?, Never>]] = [:]
    private var pendingQueue: [RenderJob] = []
    private var isProcessingQueue = false
    private var memoryWarningObserver: NSObjectProtocol?

    private struct RenderJob {
        let cacheKey: String
        let latex: String
        let displayMode: Bool
        let fontSize: CGFloat
        let textColorHex: String
        let isMonochrome: Bool
    }

    private override init() {
        super.init()
        memoryWarningObserver = NotificationCenter.default.addObserver(
            forName: UIApplication.didReceiveMemoryWarningNotification,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor in
                self?.cache.removeAllObjects()
            }
        }
    }

    nonisolated public static func cacheKey(latex: String, displayMode: Bool, fontSize: CGFloat, isDark: Bool) -> String {
        let normalizedLatex = latex.trimmingCharacters(in: .whitespacesAndNewlines)
        let bucketFont = (fontSize * 2).rounded() / 2
        let monochrome = !MathSymbolProcessor.hasExplicitColor(normalizedLatex)
        let themeTag = monochrome ? "M" : (isDark ? "D" : "L")
        let modeTag = displayMode ? "B" : "I"
        return "\(modeTag)|\(themeTag)|\(bucketFont)|\(normalizedLatex)"
    }

    nonisolated private func makeCacheKey(latex: String, displayMode: Bool, fontSize: CGFloat, isDark: Bool) -> (key: String, isMonochrome: Bool, colorHex: String) {
        let normalizedLatex = latex.trimmingCharacters(in: .whitespacesAndNewlines)
        let monochrome = !MathSymbolProcessor.hasExplicitColor(normalizedLatex)
        let colorHex = monochrome ? "#000000" : (isDark ? "#F2F2F7" : "#1C1C1E")
        let key = Self.cacheKey(latex: normalizedLatex, displayMode: displayMode, fontSize: fontSize, isDark: isDark)
        return (key, monochrome, colorHex)
    }

    /// Synchronous thread-safe cache lookup so previously rendered formulas paint on the first frame with zero flicker.
    nonisolated public func cached(latex: String, displayMode: Bool, fontSize: CGFloat, isDark: Bool) -> RenderedMathFormula? {
        let info = makeCacheKey(latex: latex, displayMode: displayMode, fontSize: fontSize, isDark: isDark)
        return cache.object(forKey: info.key as NSString)?.formula
    }

    /// Renders LaTeX using the offline bundled KaTeX engine, returning nil on syntax error so callers
    /// can cleanly fall back to Unicode typography.
    public func render(latex: String, displayMode: Bool, fontSize: CGFloat, isDark: Bool) async -> RenderedMathFormula? {
        let normalizedLatex = latex.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalizedLatex.isEmpty else { return nil }

        let info = makeCacheKey(latex: normalizedLatex, displayMode: displayMode, fontSize: fontSize, isDark: isDark)
        if let entry = cache.object(forKey: info.key as NSString) {
            return entry.formula
        }

        return await withCheckedContinuation { continuation in
            if inFlightWaiters[info.key] != nil {
                inFlightWaiters[info.key]?.append(continuation)
                return
            }
            inFlightWaiters[info.key] = [continuation]
            pendingQueue.append(RenderJob(
                cacheKey: info.key,
                latex: normalizedLatex,
                displayMode: displayMode,
                fontSize: (fontSize * 2).rounded() / 2,
                textColorHex: info.colorHex,
                isMonochrome: info.isMonochrome
            ))
            processNextIfNeeded()
        }
    }

    private func processNextIfNeeded() {
        guard !isProcessingQueue else { return }
        guard !pendingQueue.isEmpty else { return }
        isProcessingQueue = true

        Task { @MainActor [weak self] in
            guard let self else { return }
            while !self.pendingQueue.isEmpty {
                let job = self.pendingQueue.removeFirst()
                let result = await self.executeJob(job)
                self.cache.setObject(RenderedMathCacheEntry(result), forKey: job.cacheKey as NSString)
                let waiters = self.inFlightWaiters.removeValue(forKey: job.cacheKey) ?? []
                for waiter in waiters {
                    waiter.resume(returning: result)
                }
            }
            self.isProcessingQueue = false
        }
    }

    private func ensureWebViewReady() async -> WKWebView? {
        if let existing = webView {
            attachToHostWindowIfNeeded(existing)
            if isPageLoaded { return existing }
            let ok = await withCheckedContinuation { continuation in
                pageReadyWaiters.append(continuation)
            }
            return ok ? existing : nil
        }

        guard let baseURL = Self.katexBundleDirectoryURL() else {
            return nil
        }

        let config = WKWebViewConfiguration()
        config.suppressesIncrementalRendering = false
        let wv = WKWebView(frame: CGRect(x: 0, y: 0, width: 1200, height: 480), configuration: config)
        wv.navigationDelegate = self
        wv.isOpaque = true
        wv.backgroundColor = .white
        wv.scrollView.isOpaque = true
        wv.scrollView.backgroundColor = .white
        wv.scrollView.isScrollEnabled = false
        wv.scrollView.contentInsetAdjustmentBehavior = .never
        wv.scrollView.automaticallyAdjustsScrollIndicatorInsets = false
        wv.scrollView.contentInset = .zero
        if #available(iOS 26.0, *) {
            wv.scrollView.topEdgeEffect.isHidden = true
            wv.scrollView.bottomEdgeEffect.isHidden = true
            wv.scrollView.leftEdgeEffect.isHidden = true
            wv.scrollView.rightEdgeEffect.isHidden = true
        }
        wv.isUserInteractionEnabled = false
        wv.accessibilityElementsHidden = true
        wv.alpha = 1.0

        // Place behind the opaque root view inside window bounds so WebKit actively rasterizes
        // every tile at full alpha=1.0 without ever appearing on screen or receiving touches.
        let container = UIView(frame: CGRect(x: 0, y: 0, width: 1200, height: 480))
        container.clipsToBounds = true
        container.isUserInteractionEnabled = false
        container.accessibilityElementsHidden = true
        container.backgroundColor = .white
        container.addSubview(wv)

        self.webView = wv
        self.hostContainer = container
        attachToHostWindowIfNeeded(wv)
        wv.loadHTMLString(Self.rendererHTML, baseURL: baseURL)

        let ok = await withCheckedContinuation { continuation in
            pageReadyWaiters.append(continuation)
        }
        return ok ? wv : nil
    }

    private func attachToHostWindowIfNeeded(_ wv: WKWebView) {
        guard let container = hostContainer else { return }
        if container.superview != nil { return }
        guard let window = UIApplication.shared.connectedScenes
            .compactMap({ $0 as? UIWindowScene })
            .flatMap({ $0.windows })
            .first(where: { $0.isKeyWindow })
            ?? UIApplication.shared.connectedScenes
                .compactMap({ $0 as? UIWindowScene })
                .flatMap({ $0.windows })
                .first else {
            return
        }
        window.insertSubview(container, at: 0)
    }

    private func executeJob(_ job: RenderJob) async -> RenderedMathFormula? {
        guard let wv = await ensureWebViewReady() else { return nil }
        attachToHostWindowIfNeeded(wv)

        let js = """
        return await window.renderFormula(latex, displayMode, fontSizePx, textColorHex, bgHex);
        """
        let args: [String: Any] = [
            "latex": job.latex,
            "displayMode": job.displayMode,
            "fontSizePx": Double(job.fontSize),
            "textColorHex": job.textColorHex,
            "bgHex": "#FFFFFF"
        ]

        let rawResponse: Any?
        do {
            rawResponse = try await wv.callAsyncJavaScript(js, arguments: args, in: nil, contentWorld: .page)
        } catch {
            return nil
        }

        guard let dict = rawResponse as? [String: Any],
              let ok = dict["ok"] as? Bool, ok,
              let widthNum = dict["width"] as? Double,
              let heightNum = dict["height"] as? Double,
              widthNum > 1, heightNum > 1 else {
            return nil
        }

        let originX = CGFloat((dict["left"] as? Double) ?? 0)
        let originY = CGFloat((dict["top"] as? Double) ?? 0)
        let baselineNum = (dict["baselineFromBottom"] as? Double) ?? 0
        let targetWidth = min(CGFloat(ceil(widthNum)), 2400)
        let targetHeight = min(CGFloat(ceil(heightNum)), 1600)

        let requiredW = max(1200, originX + targetWidth + 32)
        let requiredH = max(480, originY + targetHeight + 32)
        if wv.frame.width < requiredW || wv.frame.height < requiredH {
            wv.frame = CGRect(x: 0, y: 0, width: requiredW, height: requiredH)
            hostContainer?.frame = CGRect(x: 0, y: 0, width: requiredW, height: requiredH)
            wv.layoutIfNeeded()
            try? await Task.sleep(nanoseconds: 16_000_000)
        }
        wv.scrollView.contentOffset = .zero

        let snapConfig = WKSnapshotConfiguration()
        snapConfig.rect = CGRect(x: max(0, originX), y: max(0, originY), width: targetWidth, height: targetHeight)
        snapConfig.snapshotWidth = NSNumber(value: Double(targetWidth))
        snapConfig.afterScreenUpdates = true

        guard let whiteSnap = try? await wv.takeSnapshot(configuration: snapConfig) else {
            return nil
        }

        let finalImage: UIImage
        if job.isMonochrome {
            if let masked = Self.extractTemplateAlphaMask(fromWhiteBgImage: whiteSnap) {
                finalImage = masked
            } else {
                try? await Task.sleep(nanoseconds: 60_000_000)
                guard let retrySnap = try? await wv.takeSnapshot(configuration: snapConfig),
                      let maskedRetry = Self.extractTemplateAlphaMask(fromWhiteBgImage: retrySnap) else {
                    return nil
                }
                finalImage = maskedRetry
            }
        } else {
            // Exact two-matte alpha extraction for colored formulas (\color / \textcolor)
            wv.backgroundColor = .black
            wv.scrollView.backgroundColor = .black
            let blackArgs: [String: Any] = ["bgHex": "#000000"]
            _ = try? await wv.callAsyncJavaScript("return await window.setStageBg(bgHex);", arguments: blackArgs, in: nil, contentWorld: .page)
            let blackSnap = try? await wv.takeSnapshot(configuration: snapConfig)
            wv.backgroundColor = .white
            wv.scrollView.backgroundColor = .white
            if let blackSnap,
               let exact = Self.extractColoredTransparentImage(whiteBg: whiteSnap, blackBg: blackSnap) {
                finalImage = exact
            } else {
                finalImage = whiteSnap.withRenderingMode(.alwaysOriginal)
            }
        }

        return RenderedMathFormula(
            image: finalImage,
            width: targetWidth,
            height: targetHeight,
            baselineFromBottom: CGFloat(max(0, min(baselineNum, Double(targetHeight)))),
            isMonochrome: job.isMonochrome
        )
    }

    // MARK: - Lossless Alpha Extraction

    /// Converts a black-glyph-on-white-background WKWebView snapshot into a crisp transparent
    /// `.alwaysTemplate` UIImage preserving full anti-aliased glyph coverage.
    private static func extractTemplateAlphaMask(fromWhiteBgImage image: UIImage) -> UIImage? {
        guard let cg = image.cgImage else { return nil }
        let width = cg.width
        let height = cg.height
        guard width > 0, height > 0 else { return nil }

        let bytesPerRow = width * 4
        var buf = [UInt8](repeating: 255, count: height * bytesPerRow)
        let colorSpace = CGColorSpaceCreateDeviceRGB()
        let bitmapInfo = CGImageAlphaInfo.premultipliedLast.rawValue | CGBitmapInfo.byteOrder32Big.rawValue

        guard let ctx = CGContext(
            data: &buf,
            width: width,
            height: height,
            bitsPerComponent: 8,
            bytesPerRow: bytesPerRow,
            space: colorSpace,
            bitmapInfo: bitmapInfo
        ) else {
            return nil
        }

        ctx.setFillColor(UIColor.white.cgColor)
        ctx.fill(CGRect(x: 0, y: 0, width: width, height: height))
        ctx.draw(cg, in: CGRect(x: 0, y: 0, width: width, height: height))

        var visiblePixels = 0
        let totalBytes = height * bytesPerRow
        var idx = 0
        while idx < totalBytes {
            let r = Int(buf[idx])
            let g = Int(buf[idx + 1])
            let b = Int(buf[idx + 2])
            let luma = (r * 77 + g * 150 + b * 29) >> 8
            let alpha = 255 - luma
            buf[idx] = 0
            buf[idx + 1] = 0
            buf[idx + 2] = 0
            if alpha <= 2 {
                buf[idx + 3] = 0
            } else {
                buf[idx + 3] = UInt8(min(255, alpha))
                visiblePixels += 1
            }
            idx += 4
        }

        guard visiblePixels > 0, let outCG = ctx.makeImage() else {
            return nil
        }
        return UIImage(cgImage: outCG, scale: image.scale, orientation: .up).withRenderingMode(.alwaysTemplate)
    }

    /// Exact difference-matting alpha recovery from white-matte and black-matte snapshots:
    /// C_black = C_fg * alpha (already premultiplied RGB), alpha = 1 - (C_white - C_black).
    private static func extractColoredTransparentImage(whiteBg: UIImage, blackBg: UIImage) -> UIImage? {
        guard let wCG = whiteBg.cgImage, let bCG = blackBg.cgImage else { return nil }
        let width = wCG.width
        let height = wCG.height
        guard width > 0, height > 0, bCG.width == width, bCG.height == height else { return nil }

        let bytesPerRow = width * 4
        let count = height * bytesPerRow
        var wBuf = [UInt8](repeating: 255, count: count)
        var bBuf = [UInt8](repeating: 0, count: count)
        let colorSpace = CGColorSpaceCreateDeviceRGB()
        let bitmapInfo = CGImageAlphaInfo.premultipliedLast.rawValue | CGBitmapInfo.byteOrder32Big.rawValue

        guard let wCtx = CGContext(data: &wBuf, width: width, height: height, bitsPerComponent: 8, bytesPerRow: bytesPerRow, space: colorSpace, bitmapInfo: bitmapInfo),
              let bCtx = CGContext(data: &bBuf, width: width, height: height, bitsPerComponent: 8, bytesPerRow: bytesPerRow, space: colorSpace, bitmapInfo: bitmapInfo) else {
            return nil
        }
        let rect = CGRect(x: 0, y: 0, width: width, height: height)
        wCtx.setFillColor(UIColor.white.cgColor)
        wCtx.fill(rect)
        wCtx.draw(wCG, in: rect)

        bCtx.setFillColor(UIColor.black.cgColor)
        bCtx.fill(rect)
        bCtx.draw(bCG, in: rect)

        var visiblePixels = 0
        var idx = 0
        while idx < count {
            let diffR = max(0, Int(wBuf[idx]) - Int(bBuf[idx]))
            let diffG = max(0, Int(wBuf[idx + 1]) - Int(bBuf[idx + 1]))
            let diffB = max(0, Int(wBuf[idx + 2]) - Int(bBuf[idx + 2]))
            let invAlpha = (diffR + diffG + diffB) / 3
            let alpha = max(0, min(255, 255 - invAlpha))
            if alpha <= 2 {
                bBuf[idx] = 0
                bBuf[idx + 1] = 0
                bBuf[idx + 2] = 0
                bBuf[idx + 3] = 0
            } else {
                // Clamp premultiplied channels <= alpha to keep valid premultiplied RGBA
                bBuf[idx] = UInt8(min(Int(bBuf[idx]), alpha))
                bBuf[idx + 1] = UInt8(min(Int(bBuf[idx + 1]), alpha))
                bBuf[idx + 2] = UInt8(min(Int(bBuf[idx + 2]), alpha))
                bBuf[idx + 3] = UInt8(alpha)
                visiblePixels += 1
            }
            idx += 4
        }

        guard visiblePixels > 0, let outCG = bCtx.makeImage() else {
            return nil
        }
        return UIImage(cgImage: outCG, scale: whiteBg.scale, orientation: .up).withRenderingMode(.alwaysOriginal)
    }

    // MARK: - WKNavigationDelegate

    public func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        isPageLoaded = true
        let waiters = pageReadyWaiters
        pageReadyWaiters.removeAll()
        for waiter in waiters {
            waiter.resume(returning: true)
        }
    }

    public func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
        let waiters = pageReadyWaiters
        pageReadyWaiters.removeAll()
        for waiter in waiters {
            waiter.resume(returning: false)
        }
    }

    public func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
        let waiters = pageReadyWaiters
        pageReadyWaiters.removeAll()
        for waiter in waiters {
            waiter.resume(returning: false)
        }
    }

    public func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
        self.webView?.removeFromSuperview()
        self.hostContainer?.removeFromSuperview()
        self.webView = nil
        self.hostContainer = nil
        self.isPageLoaded = false
        let waiters = pageReadyWaiters
        pageReadyWaiters.removeAll()
        for waiter in waiters {
            waiter.resume(returning: false)
        }
    }

    // MARK: - Bundle Assets & Renderer HTML

    private static func katexBundleDirectoryURL() -> URL? {
        if let url = Bundle.main.url(forResource: "katex.min", withExtension: "js") {
            return url.deletingLastPathComponent()
        }
        if let url = Bundle.main.url(forResource: "katex.min", withExtension: "js", subdirectory: "katex") {
            return url.deletingLastPathComponent()
        }
        if let url = Bundle.main.url(forResource: "katex.min", withExtension: "js", subdirectory: "Resources/katex") {
            return url.deletingLastPathComponent()
        }
        return nil
    }

    private static let rendererHTML: String = """
    <!DOCTYPE html>
    <html>
    <head>
      <meta charset="utf-8" />
      <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0" />
      <link rel="stylesheet" href="katex.min.css" />
      <style>
        * { box-sizing: border-box; }
        html, body {
          margin: 0;
          padding: 0;
          background: #FFFFFF;
          overflow: hidden;
          -webkit-font-smoothing: antialiased;
          font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Hiragino Sans GB", sans-serif;
        }
        #stage {
          position: fixed;
          top: 0;
          left: 0;
          display: inline-flex;
          align-items: baseline;
          padding: 1px 2px;
          background: transparent;
          color: #000000;
          white-space: nowrap;
        }
        #stage.display-mode {
          padding: 3px 5px;
        }
        #stage .katex-display {
          margin: 0 !important;
          padding: 0 !important;
          display: inline-block !important;
        }
        #stage .katex-display > .katex {
          display: inline-block !important;
          text-align: left !important;
        }
        #baseline-probe {
          display: inline-block;
          width: 0;
          height: 0;
          overflow: hidden;
        }
      </style>
      <script src="katex.min.js"></script>
      <script src="mhchem.min.js"></script>
    </head>
    <body>
      <div id="stage"><span id="formula"></span><span id="baseline-probe"></span></div>
      <script>
        let __fontsWarmed = false;
        window.setStageBg = async function(bgHex) {
          document.documentElement.style.background = bgHex || '#FFFFFF';
          document.body.style.background = bgHex || '#FFFFFF';
          await new Promise(function(resolve) { requestAnimationFrame(resolve); });
          return true;
        };
        window.renderFormula = async function(latex, displayMode, fontSizePx, textColorHex, bgHex) {
          if (typeof katex === 'undefined') {
            return { ok: false, error: 'katex_missing' };
          }
          document.documentElement.style.background = bgHex || '#FFFFFF';
          document.body.style.background = bgHex || '#FFFFFF';
          const stage = document.getElementById('stage');
          const formulaEl = document.getElementById('formula');
          const probe = document.getElementById('baseline-probe');
          stage.className = displayMode ? 'display-mode' : '';
          stage.style.fontSize = fontSizePx + 'px';
          stage.style.color = textColorHex || '#000000';
          formulaEl.innerHTML = '';
          try {
            katex.render(latex, formulaEl, {
              displayMode: !!displayMode,
              throwOnError: true,
              strict: 'ignore',
              trust: false,
              output: 'html',
              maxSize: 50,
              maxExpand: 300
            });
          } catch (e) {
            return { ok: false, error: String(e && e.message || e) };
          }
          if (!__fontsWarmed && document.fonts && document.fonts.ready) {
            try { await document.fonts.ready; } catch (_) {}
            __fontsWarmed = true;
          }
          await new Promise(function(resolve) {
            requestAnimationFrame(resolve);
          });
          const stageRect = stage.getBoundingClientRect();
          const formulaRect = formulaEl.getBoundingClientRect();
          const probeRect = probe.getBoundingClientRect();
          const left = Math.max(0, Math.floor(Math.min(stageRect.left, formulaRect.left)));
          const top = Math.max(0, Math.floor(Math.min(stageRect.top, formulaRect.top)));
          const right = Math.ceil(Math.max(stageRect.right, formulaRect.right) + 1);
          const bottom = Math.ceil(Math.max(stageRect.bottom, formulaRect.bottom) + 1);
          const w = Math.max(1, right - left);
          const h = Math.max(1, bottom - top);
          const baselineFromBottom = Math.max(0, bottom - probeRect.bottom);
          return {
            ok: true,
            left: left,
            top: top,
            width: w,
            height: h,
            baselineFromBottom: baselineFromBottom
          };
        };
      </script>
    </body>
    </html>
    """
}
