import SwiftUI
import UIKit

// MARK: - Swarm Palette (Zero Magenta / Purple)

enum SwarmTheme {
    private static func adaptive(light: UIColor, dark: UIColor) -> Color {
        Color(uiColor: UIColor { traits in
            traits.userInterfaceStyle == .dark ? dark : light
        })
    }

    static let bg = adaptive(
        light: UIColor(red: 0.980, green: 0.973, blue: 0.945, alpha: 1.0), // #FAF8F1
        dark: UIColor(red: 0.082, green: 0.082, blue: 0.094, alpha: 1.0)   // #151518
    )

    static let cardBg = adaptive(
        light: UIColor(red: 0.949, green: 0.937, blue: 0.902, alpha: 1.0), // #F2EFE6
        dark: UIColor(red: 0.122, green: 0.122, blue: 0.145, alpha: 1.0)   // #1F1F25
    )

    static let surfaceBg = adaptive(
        light: UIColor(red: 1.0, green: 0.996, blue: 0.984, alpha: 1.0),   // #FFFEFB
        dark: UIColor(red: 0.149, green: 0.149, blue: 0.176, alpha: 1.0)   // #26262D
    )

    static let fieldBg = adaptive(
        light: UIColor(red: 0.937, green: 0.925, blue: 0.890, alpha: 1.0), // #EFECE3
        dark: UIColor(red: 0.145, green: 0.145, blue: 0.173, alpha: 1.0)   // #25252C
    )

    static let border = adaptive(
        light: UIColor(red: 0.882, green: 0.867, blue: 0.820, alpha: 1.0), // #E1DDD1
        dark: UIColor(red: 0.200, green: 0.200, blue: 0.235, alpha: 1.0)   // #33333C
    )

    static let ink = adaptive(
        light: UIColor(red: 0.125, green: 0.129, blue: 0.141, alpha: 1.0), // #202124
        dark: UIColor(red: 0.953, green: 0.945, blue: 0.922, alpha: 1.0)   // #F3F1EB
    )

    static let muted = adaptive(
        light: UIColor(red: 0.431, green: 0.420, blue: 0.396, alpha: 1.0), // #6E6B65
        dark: UIColor(red: 0.620, green: 0.608, blue: 0.580, alpha: 1.0)   // #9E9B94
    )

    static let faint = adaptive(
        light: UIColor(red: 0.616, green: 0.604, blue: 0.573, alpha: 1.0), // #9D9A92
        dark: UIColor(red: 0.439, green: 0.431, blue: 0.412, alpha: 1.0)   // #706E69
    )

    // Warm terracotta/amber accent (matches desktop active tab border & Working status)
    static let accent = adaptive(
        light: UIColor(red: 0.722, green: 0.353, blue: 0.141, alpha: 1.0), // #B85A24
        dark: UIColor(red: 0.878, green: 0.525, blue: 0.294, alpha: 1.0)   // #E0864B
    )

    static let liveGreen = adaptive(
        light: UIColor(red: 0.243, green: 0.620, blue: 0.294, alpha: 1.0), // #3E9E4B
        dark: UIColor(red: 0.369, green: 0.761, blue: 0.412, alpha: 1.0)   // #5EC269
    )

    static let warnAmber = adaptive(
        light: UIColor(red: 0.745, green: 0.545, blue: 0.118, alpha: 1.0), // #BE8B1E
        dark: UIColor(red: 0.878, green: 0.659, blue: 0.204, alpha: 1.0)   // #E0A834
    )

    static let errorRed = adaptive(
        light: UIColor(red: 0.788, green: 0.231, blue: 0.231, alpha: 1.0), // #C93B3B
        dark: UIColor(red: 0.878, green: 0.376, blue: 0.376, alpha: 1.0)   // #E06060
    )

    static let infoBlue = adaptive(
        light: UIColor(red: 0.157, green: 0.443, blue: 0.690, alpha: 1.0), // #2871B0
        dark: UIColor(red: 0.369, green: 0.651, blue: 0.902, alpha: 1.0)   // #5EA6E6
    )

    static let codeBg = adaptive(
        light: UIColor(red: 0.914, green: 0.898, blue: 0.855, alpha: 1.0), // #E9E5DA
        dark: UIColor(red: 0.180, green: 0.180, blue: 0.212, alpha: 1.0)   // #2E2E36
    )

    static let nodeStroke = adaptive(
        light: UIColor(red: 0.290, green: 0.302, blue: 0.329, alpha: 1.0), // #4A4D54
        dark: UIColor(red: 0.690, green: 0.706, blue: 0.745, alpha: 1.0)   // #B0B4BE
    )

    static let coordHexBg = Color(red: 0.110, green: 0.106, blue: 0.133) // #1C1B22
}

// MARK: - Pixel Bee Avatar View (Ported from swarm_board/hive.js)

enum PixelBeePose: String, Hashable {
    case working
    case asleep
    case landed
}

struct SwarmPixelBeeView: View {
    let name: String
    let pose: PixelBeePose
    var size: CGFloat = 32
    var statusDotColor: Color? = nil

    @State private var wingFrame: Int = 0

    var body: some View {
        ZStack(alignment: .bottomTrailing) {
            ZStack {
                tileBackground
                Image(uiImage: PixelBeeSpriteCache.image(name: name, pose: pose, frame: pose == .working ? wingFrame : 0))
                    .interpolation(.none)
                    .resizable()
                    .aspectRatio(contentMode: .fit)
                    .frame(width: size * 0.76, height: size * 0.62)
            }
            .frame(width: size, height: size)
            .clipShape(RoundedRectangle(cornerRadius: size * 0.24, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: size * 0.24, style: .continuous)
                    .stroke(name == "coordinator" ? Color.white.opacity(0.10) : SwarmTheme.border.opacity(0.8), lineWidth: 0.6)
            )

            if let dotColor = statusDotColor {
                Circle()
                    .fill(dotColor)
                    .frame(width: max(7, size * 0.25), height: max(7, size * 0.25))
                    .overlay(
                        Circle()
                            .stroke(SwarmTheme.bg, lineWidth: 1.5)
                    )
                    .offset(x: 2, y: 2)
            }
        }
        .task(id: pose == .working) {
            guard pose == .working else {
                wingFrame = 0
                return
            }
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 140_000_000)
                if Task.isCancelled { break }
                wingFrame = (wingFrame == 0 ? 1 : 0)
            }
        }
    }

    @ViewBuilder
    private var tileBackground: some View {
        if name == "coordinator" {
            ZStack {
                SwarmTheme.coordHexBg
                HexagonGridOverlay()
            }
        } else {
            PixelBeeSpriteCache.tileColor(name: name)
        }
    }
}

private struct HexagonGridOverlay: View {
    var body: some View {
        Canvas { ctx, size in
            let r: CGFloat = 8
            let h = r * sqrt(3) / 2
            var path = Path()
            let cols = Int(ceil(size.width / (r * 1.5))) + 2
            let rows = Int(ceil(size.height / h)) + 2
            for row in -1..<rows {
                for col in -1..<cols {
                    let cx = CGFloat(col) * r * 1.5
                    let cy = CGFloat(row) * h * 2 + (col % 2 == 0 ? 0 : h)
                    for i in 0..<6 {
                        let angle = CGFloat(i) * .pi / 3
                        let pt = CGPoint(x: cx + r * cos(angle), y: cy + r * sin(angle))
                        if i == 0 {
                            path.move(to: pt)
                        } else {
                            path.addLine(to: pt)
                        }
                    }
                    path.closeSubpath()
                }
            }
            ctx.stroke(path, with: .color(Color.white.opacity(0.08)), lineWidth: 0.7)
        }
    }
}

private enum PixelBeeSpriteCache {
    private static let lock = NSLock()
    private static var cache: [String: UIImage] = [:]

    // Non-magenta hue palette preserving exact desktop bee hues where non-magenta
    private static let paletteHues: [Double] = [12, 150, 206, 40, 196, 228, 96, 28, 165, 175, 350, 60]

    private static func nameHash(_ s: String) -> UInt32 {
        var acc: UInt32 = 7
        for scalar in s.unicodeScalars {
            acc = acc &* 31 &+ scalar.value
        }
        return acc
    }

    static func hue(for name: String) -> Double {
        if name == "coordinator" { return 46 }
        let idx = Int(nameHash(name) % UInt32(paletteHues.count))
        return paletteHues[idx]
    }

    static func tileColor(name: String) -> Color {
        let h = hue(for: name)
        let rgb = hslToRGBA(h: h, s: 78, l: 60)
        return Color(uiColor: UIColor { traits in
            let alpha: CGFloat = traits.userInterfaceStyle == .dark ? 0.24 : 0.20
            return UIColor(
                red: CGFloat(rgb.r) / 255.0,
                green: CGFloat(rgb.g) / 255.0,
                blue: CGFloat(rgb.b) / 255.0,
                alpha: alpha
            )
        })
    }

    static func image(name: String, pose: PixelBeePose, frame: Int) -> UIImage {
        let f = pose == .working ? (frame % 2) : 0
        let key = "\(name)|\(pose.rawValue)|\(f)"
        lock.lock()
        if let existing = cache[key] {
            lock.unlock()
            return existing
        }
        lock.unlock()

        let rendered = renderBee(name: name, pose: pose, frame: f)
        lock.lock()
        cache[key] = rendered
        lock.unlock()
        return rendered
    }

    private struct RGBA {
        var r: UInt8
        var g: UInt8
        var b: UInt8
        var a: UInt8
    }

    private static func shiftHue(_ h: Double, target: Double, amount: Double) -> Double {
        let d = ((target - h + 540.0).truncatingRemainder(dividingBy: 360.0)) - 180.0
        return h + d * amount
    }

    private static func hslToRGBA(h rawH: Double, s rawS: Double, l rawL: Double, a: UInt8 = 255) -> RGBA {
        let h = ((rawH.truncatingRemainder(dividingBy: 360.0)) + 360.0).truncatingRemainder(dividingBy: 360.0)
        let s = rawS / 100.0
        let l = rawL / 100.0
        let amp = s * min(l, 1.0 - l)
        func comp(_ n: Double) -> UInt8 {
            let k = (n + h / 30.0).truncatingRemainder(dividingBy: 12.0)
            let val = l - amp * max(-1.0, min(k - 3.0, min(9.0 - k, 1.0)))
            return UInt8(max(0, min(255, Int((val * 255.0).rounded()))))
        }
        return RGBA(r: comp(0), g: comp(8), b: comp(4), a: a)
    }

    private struct Ramp {
        let spec, hi, mid, sh, ol, skh, sk, skd: RGBA
    }

    private static func ramp(for name: String) -> Ramp {
        let h = hue(for: name)
        return Ramp(
            spec: hslToRGBA(h: shiftHue(h, target: 55, amount: 0.28), s: 96, l: 90),
            hi: hslToRGBA(h: shiftHue(h, target: 55, amount: 0.18), s: 90, l: 74),
            mid: hslToRGBA(h: h, s: 78, l: 60),
            sh: hslToRGBA(h: shiftHue(h, target: 220, amount: 0.22), s: 58, l: 44),
            ol: hslToRGBA(h: shiftHue(h, target: 220, amount: 0.35), s: 42, l: 19),
            skh: hslToRGBA(h: shiftHue(h, target: 220, amount: 0.15), s: 20, l: 30),
            sk: hslToRGBA(h: shiftHue(h, target: 220, amount: 0.30), s: 24, l: 18),
            skd: hslToRGBA(h: shiftHue(h, target: 220, amount: 0.45), s: 32, l: 10)
        )
    }

    private static func renderBee(name: String, pose: PixelBeePose, frame: Int) -> UIImage {
        let w = 22
        let h = 18
        var pixels = [RGBA](repeating: RGBA(r: 0, g: 0, b: 0, a: 0), count: w * h)

        func setPixel(_ x: Int, _ y: Int, _ col: RGBA) {
            guard x >= 0 && y >= 0 && x < w && y < h else { return }
            if col.a == 255 {
                pixels[y * w + x] = col
            } else if col.a > 0 {
                let dst = pixels[y * w + x]
                let sa = Double(col.a) / 255.0
                let da = Double(dst.a) / 255.0
                let outA = sa + da * (1.0 - sa)
                guard outA > 0 else { return }
                let r = (Double(col.r) * sa + Double(dst.r) * da * (1.0 - sa)) / outA
                let g = (Double(col.g) * sa + Double(dst.g) * da * (1.0 - sa)) / outA
                let b = (Double(col.b) * sa + Double(dst.b) * da * (1.0 - sa)) / outA
                pixels[y * w + x] = RGBA(
                    r: UInt8(max(0, min(255, Int(r.rounded())))),
                    g: UInt8(max(0, min(255, Int(g.rounded())))),
                    b: UInt8(max(0, min(255, Int(b.rounded())))),
                    a: UInt8(max(0, min(255, Int((outA * 255.0).rounded()))))
                )
            }
        }

        let r = ramp(for: name)
        let isQueen = (name == "coordinator")
        let landed = (pose == .landed || pose == .asleep)
        let asleep = (pose == .asleep)

        let cx = 11.0, cy = 10.5, rx = 7.3, ry = 4.8
        func inside(_ x: Int, _ y: Int) -> Bool {
            let nx = (Double(x) + 0.5 - cx) / rx
            let ny = (Double(y) + 0.5 - cy) / ry
            return nx * nx + ny * ny <= 1.0
        }

        let rawL = [-0.42, -0.62, 0.66]
        let lenL = sqrt(rawL[0] * rawL[0] + rawL[1] * rawL[1] + rawL[2] * rawL[2])
        let lx = rawL[0] / lenL, ly = rawL[1] / lenL, lz = rawL[2] / lenL

        let n4 = [(1, 0), (-1, 0), (0, 1), (0, -1)]
        func rows(_ spec: [(Int, Int, Int)]) -> [(Int, Int)] {
            var pts: [(Int, Int)] = []
            for (rowY, x0, x1) in spec {
                for x in x0...x1 {
                    pts.append((x, rowY))
                }
            }
            return pts
        }

        func drawWing(_ pts: [(Int, Int)], glint: Bool) {
            let set = Set(pts.map { "\($0.0),\($0.1)" })
            let edgeCol = RGBA(r: 140, g: 160, b: 192, a: 230)
            let fillCol = RGBA(r: 255, g: 255, b: 255, a: 184)
            for (x, y) in pts {
                let isEdge = n4.contains { !set.contains("\(x + $0.0),\(y + $0.1)") }
                setPixel(x, y, isEdge ? edgeCol : fillCol)
            }
            if glint {
                let interior = pts.filter { x, y in
                    !n4.contains { !set.contains("\(x + $0.0),\(y + $0.1)") }
                }.sorted { ($0.1, $0.0) < ($1.1, $1.0) }
                if let tl = interior.first {
                    setPixel(tl.0, tl.1, RGBA(r: 255, g: 255, b: 255, a: 255))
                }
            }
        }

        var backWing: [(Int, Int)]?
        let frontWing: [(Int, Int)]
        if landed {
            frontWing = rows([(5, 14, 17), (6, 13, 18)])
        } else if frame == 0 {
            backWing = rows([(1, 16, 17), (2, 16, 18), (3, 15, 18), (4, 15, 17), (5, 15, 16)])
            frontWing = rows([(0, 13, 14), (1, 12, 15), (2, 11, 15), (3, 11, 14), (4, 11, 13), (5, 11, 12)])
        } else {
            backWing = rows([(3, 16, 18), (4, 15, 19), (5, 16, 18)])
            frontWing = rows([(3, 13, 16), (4, 12, 17), (5, 12, 17), (6, 13, 15)])
        }

        if let bw = backWing {
            drawWing(bw, glint: false)
        }

        func stripe(_ x: Int, _ y: Int) -> Bool {
            let ny = (Double(y) + 0.5 - cy) / ry
            let u = Double(x) + 0.5 + 1.15 * ny * ny
            return (u > 10.6 && u < 12.5) || (u > 14.3 && u < 16.1) || u > 17.9
        }

        for y in 0..<h {
            for x in 0..<w {
                guard inside(x, y) else { continue }
                let nx = (Double(x) + 0.5 - cx) / rx
                let ny = (Double(y) + 0.5 - cy) / ry
                let rr = min(1.0, nx * nx + ny * ny)
                let nz = sqrt(1.0 - rr)
                let intensity = nx * lx + ny * ly + nz * lz
                if stripe(x, y) {
                    setPixel(x, y, intensity > 0.80 ? r.skh : (intensity > 0.30 ? r.sk : r.skd))
                } else {
                    setPixel(x, y, intensity > 0.965 ? r.spec : (intensity > 0.78 ? r.hi : (intensity > 0.26 ? r.mid : r.sh)))
                }
            }
        }

        for y in 0..<h {
            for x in 0..<w {
                guard !inside(x, y) else { continue }
                let nbs = n4.filter { inside(x + $0.0, y + $0.1) }
                guard !nbs.isEmpty else { continue }
                let nx = (Double(x) + 0.5 - cx) / rx
                let ny = (Double(y) + 0.5 - cy) / ry
                let n = sqrt(nx * nx + ny * ny)
                let d = (nx / n) * lx + (ny / n) * ly
                let nextToStripe = nbs.contains { stripe(x + $0.0, y + $0.1) }
                setPixel(x, y, (d > 0.5 && !nextToStripe) ? r.sh : r.ol)
            }
        }

        // Stinger
        setPixel(19, 10, r.ol)
        setPixel(20, 10, r.ol)

        // Face
        if asleep {
            setPixel(6, 10, r.ol)
            setPixel(7, 10, r.ol)
        } else {
            setPixel(6, 9, RGBA(r: 255, g: 255, b: 255, a: 255))
            setPixel(7, 9, r.skd)
            setPixel(6, 10, r.skd)
            setPixel(7, 10, r.skd)
        }
        // Warm coral blush
        setPixel(5, 11, RGBA(r: 245, g: 132, b: 96, a: 140))
        setPixel(4, 11, RGBA(r: 245, g: 132, b: 96, a: 90))

        // Antennae or Queen Crown
        if isQueen {
            let g1 = RGBA(r: 247, g: 206, b: 91, a: 255)
            let g2 = RGBA(r: 255, g: 233, b: 166, a: 255)
            let g3 = RGBA(r: 201, g: 138, b: 43, a: 255)
            let gem = RGBA(r: 228, g: 100, b: 62, a: 255)
            for (x, y) in [(5, 5), (6, 5), (7, 5), (8, 5), (9, 5)] { setPixel(x, y, g1) }
            setPixel(5, 4, g2)
            setPixel(7, 4, g2)
            setPixel(9, 4, g2)
            setPixel(7, 3, g2)
            setPixel(5, 5, g3)
            setPixel(9, 5, g3)
            setPixel(7, 5, gem)
        } else {
            for (x, y) in [(7, 5), (6, 4), (5, 3)] { setPixel(x, y, r.ol) }
            setPixel(4, 3, r.sk)
            for (x, y) in [(9, 5), (9, 4), (9, 3)] { setPixel(x, y, r.ol) }
            setPixel(8, 2, r.sk)
        }

        drawWing(frontWing, glint: true)

        if asleep {
            let zCol = RGBA(r: 140, g: 144, b: 166, a: 255)
            for (x, y) in [(0, 0), (1, 0), (2, 0), (3, 0), (2, 1), (1, 2), (0, 3), (1, 3), (2, 3), (3, 3)] {
                setPixel(x, y, zCol)
            }
        }

        let colorSpace = CGColorSpaceCreateDeviceRGB()
        let bitmapInfo = CGBitmapInfo(rawValue: CGImageAlphaInfo.premultipliedLast.rawValue)
        var premul = [UInt8](repeating: 0, count: w * h * 4)
        for i in 0..<(w * h) {
            let p = pixels[i]
            let a = Int(p.a)
            premul[i * 4 + 0] = UInt8((Int(p.r) * a + 127) / 255)
            premul[i * 4 + 1] = UInt8((Int(p.g) * a + 127) / 255)
            premul[i * 4 + 2] = UInt8((Int(p.b) * a + 127) / 255)
            premul[i * 4 + 3] = p.a
        }
        let data = Data(premul)
        if let provider = CGDataProvider(data: data as CFData),
           let cgImg = CGImage(
               width: w,
               height: h,
               bitsPerComponent: 8,
               bitsPerPixel: 32,
               bytesPerRow: w * 4,
               space: colorSpace,
               bitmapInfo: bitmapInfo,
               provider: provider,
               decode: nil,
               shouldInterpolate: false,
               intent: .defaultIntent
           ) {
            return UIImage(cgImage: cgImg)
        }
        return UIImage()
    }
}

// MARK: - Swarm Shared Formatting & Metadata Helpers

enum SwarmHelpers {
    private static let dateParsers: [DateFormatter] = {
        let f1 = DateFormatter()
        f1.locale = Locale(identifier: "en_US_POSIX")
        f1.dateFormat = "yyyy-MM-dd HH:mm:ss"

        let f2 = DateFormatter()
        f2.locale = Locale(identifier: "en_US_POSIX")
        f2.dateFormat = "yyyy-MM-dd'T'HH:mm:ssZ"

        let f3 = DateFormatter()
        f3.locale = Locale(identifier: "en_US_POSIX")
        f3.dateFormat = "yyyy-MM-dd'T'HH:mm:ss.SSSZ"

        return [f1, f2, f3]
    }()

    static func parseDate(_ raw: String) -> Date? {
        let s = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !s.isEmpty else { return nil }
        for fmt in dateParsers {
            if let d = fmt.date(from: s) { return d }
        }
        let iso = ISO8601DateFormatter()
        iso.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let d = iso.date(from: s) { return d }
        iso.formatOptions = [.withInternetDateTime]
        return iso.date(from: s)
    }

    static func formatClock(_ raw: String) -> String {
        if let d = parseDate(raw) {
            let cal = Calendar.current
            let h = cal.component(.hour, from: d)
            let m = cal.component(.minute, from: d)
            return String(format: "%d:%02d", h, m)
        }
        // Fallback: extract HH:mm from "YYYY-MM-DD HH:MM:SS"
        let parts = raw.split(separator: " ")
        if parts.count >= 2 {
            let comps = parts[1].split(separator: ":")
            if comps.count >= 2, let h = Int(comps[0]), let m = Int(comps[1]) {
                return String(format: "%d:%02d", h, m)
            }
        }
        return raw
    }

    static func formatDelta(from startRaw: String?, to endRaw: String) -> String? {
        guard let startRaw, let d1 = parseDate(startRaw), let d2 = parseDate(endRaw) else { return nil }
        let sec = max(0, Int(d2.timeIntervalSince(d1).rounded()))
        if sec < 60 { return "+\(sec)s" }
        let mins = sec / 60
        let rem = sec % 60
        if mins < 60 { return "+\(mins)m \(rem)s" }
        let hrs = mins / 60
        let remMins = mins % 60
        return "+\(hrs)h \(remMins)m"
    }

    static func shortModel(_ model: String) -> String {
        var out = model.trimmingCharacters(in: .whitespacesAndNewlines)
        for suffix in ["-dropdown", "-high", "-medium", "-low"] {
            if out.lowercased().hasSuffix(suffix) {
                out = String(out.dropLast(suffix.count))
            }
        }
        return out
    }

    static func isRoleWorker(name: String, role: String = "") -> Bool {
        let r = role.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if !r.isEmpty && r != "worker" && r != "coordinator" {
            return true
        }
        let n = name.lowercased()
        return n == "narrator" || n == "monitor" || n.hasPrefix("narrator-") || n.hasPrefix("monitor-")
    }

    static func isBroadcast(_ msg: SwarmBoardMessage) -> Bool {
        msg.mentions.contains { $0.caseInsensitiveCompare("all") == .orderedSame }
    }

    static func targets(of msg: SwarmBoardMessage) -> [String] {
        var seen = Set<String>()
        var result: [String] = []
        for m in msg.mentions {
            if m.caseInsensitiveCompare(msg.agent) == .orderedSame { continue }
            let key = m.lowercased()
            if !seen.contains(key) {
                seen.insert(key)
                result.append(m)
            }
        }
        return result
    }

    static func stripLeadingMentions(_ text: String) -> String {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.hasPrefix("@") else { return text }
        // Strip consecutive leading @mention tokens followed by whitespace/comma/colon
        let pattern = #"^(?:\s*@[A-Za-z0-9_\-\.]+[,:]?)+\s*"#
        if let regex = try? NSRegularExpression(pattern: pattern) {
            let range = NSRange(trimmed.startIndex..<trimmed.endIndex, in: trimmed)
            let stripped = regex.stringByReplacingMatches(in: trimmed, range: range, withTemplate: "")
            if !stripped.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                return stripped
            }
        }
        return text
    }

    static func extractRoleHint(for memberName: String, role: String, messages: [SwarmBoardMessage]) -> String {
        if memberName == "coordinator" || role == "coordinator" {
            return "Coordinator"
        }
        // Look at earliest brief from coordinator mentioning this member
        if let brief = messages.first(where: { m in
            m.agent == "coordinator" && m.mentions.contains(where: { $0.caseInsensitiveCompare(memberName) == .orderedSame })
        }) {
            var body = stripLeadingMentions(brief.text)
            if let nl = body.firstIndex(of: "\n") {
                body = String(body[..<nl])
            }
            for prefix in ["You own ", "You are the ", "You are this swarm's ", "You are "] {
                if body.hasPrefix(prefix) {
                    body = String(body.dropFirst(prefix.count))
                    break
                }
            }
            body = body.trimmingCharacters(in: .whitespacesAndNewlines)
            if body.hasSuffix(".") || body.hasSuffix(":") {
                body = String(body.dropLast())
            }
            if !body.isEmpty {
                return body
            }
        }
        return role.isEmpty ? "worker" : role
    }

    static func inlineAttributed(_ text: String, baseSize: CGFloat = 13) -> AttributedString {
        // Simple inline code & bold parser without any purple/magenta tint
        var result = AttributedString()
        let parts = text.components(separatedBy: "`")
        for (idx, part) in parts.enumerated() {
            if idx % 2 == 1 {
                var codeAttr = AttributedString(" \(part) ")
                codeAttr.font = .system(size: max(11, baseSize - 1), weight: .medium, design: .monospaced)
                codeAttr.backgroundColor = SwarmTheme.codeBg
                codeAttr.foregroundColor = SwarmTheme.ink
                result.append(codeAttr)
            } else {
                var plain = AttributedString(part)
                plain.font = .system(size: baseSize)
                plain.foregroundColor = SwarmTheme.ink
                result.append(plain)
            }
        }
        return result
    }
}
