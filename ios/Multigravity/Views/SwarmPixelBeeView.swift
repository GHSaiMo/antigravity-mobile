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

/// Bare pixel bee sprite (no square tile background) for Hive honeycomb cells, worker pills, and Mention graph nodes.
struct SwarmPixelBeeSpriteView: View {
    let name: String
    let pose: PixelBeePose
    var width: CGFloat = 40
    var height: CGFloat = 33
    var statusDotColor: Color? = nil
    var dotBorderColor: Color = SwarmTheme.surfaceBg
    var isTilted: Bool = false
    var isRetired: Bool = false
    var bobbing: Bool = false

    @State private var wingFrame: Int = 0
    @State private var bobOffset: CGFloat = 0

    var body: some View {
        ZStack(alignment: .topTrailing) {
            Image(uiImage: PixelBeeSpriteCache.image(name: name, pose: pose, frame: pose == .working ? wingFrame : 0))
                .interpolation(.none)
                .resizable()
                .aspectRatio(contentMode: .fit)
                .frame(width: width, height: height)
                .rotationEffect(.degrees(isTilted ? -14 : 0))
                .saturation(isRetired ? 0 : 1)
                .opacity(isRetired ? 0.36 : 1)
                .offset(y: bobbing ? bobOffset : 0)

            if let dotColor = statusDotColor {
                let dSize: CGFloat = width < 26 ? 6.5 : 8.5
                Circle()
                    .fill(dotColor)
                    .frame(width: dSize, height: dSize)
                    .overlay(Circle().stroke(dotBorderColor, lineWidth: 1.6))
                    .offset(x: 1, y: -1)
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
        .task(id: bobbing) {
            guard bobbing else {
                bobOffset = 0
                return
            }
            withAnimation(.easeInOut(duration: 0.95).repeatForever(autoreverses: true)) {
                bobOffset = -2.2
            }
        }
    }
}

/// "All" 3-bee mini swarm tile in The Team filter strip (matches o2-swarm in hive.js).
struct SwarmMiniTrioTileView: View {
    let names: [String]
    var size: CGFloat = 44
    var isActive: Bool = true

    var body: some View {
        let trio = Array((names + ["worker-a", "worker-b", "worker-c"]).prefix(3))
        ZStack(alignment: .topLeading) {
            SwarmTheme.fieldBg
            SwarmPixelBeeSpriteView(
                name: trio[0],
                pose: isActive ? .working : .landed,
                width: 20,
                height: 16
            )
            .offset(x: 3, y: 7)
            SwarmPixelBeeSpriteView(
                name: trio[1],
                pose: isActive ? .working : .landed,
                width: 20,
                height: 16
            )
            .offset(x: 19, y: 4)
            SwarmPixelBeeSpriteView(
                name: trio[2],
                pose: isActive ? .working : .landed,
                width: 20,
                height: 16
            )
            .offset(x: 11, y: 22)
        }
        .frame(width: size, height: size)
        .clipShape(RoundedRectangle(cornerRadius: size * 0.22, style: .continuous))
    }
}

/// Crisp nearest-neighbor full-color Queen Pixel Bee icon for inline `/swarm` slash command bubbles.
enum SwarmPixelBeeInlineIcon {
    private static let cache = NSCache<NSString, UIImage>()

    static func cachedImage(height: CGFloat = 15.5) -> UIImage {
        let scale = max(2.0, UITraitCollection.current.displayScale)
        let key = "\(Int((height * 10).rounded()))_\(Int(scale))" as NSString
        if let hit = cache.object(forKey: key) {
            return hit
        }
        let src = PixelBeeSpriteCache.image(name: "coordinator", pose: .landed, frame: 0)
        let targetW = round(height * 22.0 / 18.0)
        let targetSize = CGSize(width: targetW, height: height)
        let format = UIGraphicsImageRendererFormat()
        format.scale = scale
        format.opaque = false
        let renderer = UIGraphicsImageRenderer(size: targetSize, format: format)
        let img = renderer.image { ctx in
            ctx.cgContext.interpolationQuality = .none
            src.draw(in: CGRect(origin: .zero, size: targetSize))
        }.withRenderingMode(.alwaysOriginal)
        cache.setObject(img, forKey: key)
        return img
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
        let lower = out.lowercased()
        if lower == "gemini-pro-orchestrator-neon" {
            return "Orchestrator Neon"
        }
        if lower.hasPrefix("gemini-pro-orchestrator-") {
            let suffix = out.dropFirst("gemini-pro-orchestrator-".count)
            return "Orch · \(suffix.capitalized)"
        }
        if lower == "gemini-pro-orchestrator" {
            return "Orchestrator"
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
            return String(localized: "Coordinator")
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

    static func formatSpanDuration(from startRaw: String?, to endRaw: String?) -> String? {
        guard let startRaw, let endRaw,
              let d1 = parseDate(startRaw), let d2 = parseDate(endRaw) else { return nil }
        let sec = max(0, Int(d2.timeIntervalSince(d1).rounded()))
        if sec <= 0 { return nil }
        if sec < 60 { return String(localized: "\(sec)秒") }
        let mins = sec / 60
        let rem = sec % 60
        if mins < 60 {
            return rem > 0 ? String(localized: "\(mins)分\(rem)秒") : String(localized: "\(mins)分")
        }
        let hrs = mins / 60
        let remMins = mins % 60
        return remMins > 0 ? String(localized: "\(hrs)小时\(remMins)分") : String(localized: "\(hrs)小时")
    }
}

// MARK: - Swarm Hive & Overview Synthesis Engine (Ported from swarm_board/hive.js)

enum SwarmTaskBucketState: String, Equatable {
    case done
    case running
    case blocked
    case queued
}

enum SwarmThreadState: String, Equatable {
    case needs
    case waiting
    case stuck
    case running
    case queued
    case done
}

enum SwarmHiveBeeState: String, Equatable {
    case working
    case wait
    case needs
    case issue
    case done
    case idle
    case retired
    case unborn
}

struct SwarmSynthesizedMember: Identifiable, Equatable {
    var id: String { name }
    let name: String
    let conversationId: String
    let role: String
    let title: String
    let model: String
    let status: String // "needs_you", "working", "waiting_on", "parked"
    let waitingOn: String?
    let now: String
    let liveText: String?
    let liveKind: String?
    let hasIssue: Bool
    let issueKind: String?
    let issueText: String?
    let killed: Bool
    let posts: Int
    let lastPostSeq: Int
}

struct SwarmSynthesizedTask: Identifiable, Equatable {
    let id: String
    let title: String
    let owners: [String]
    let state: SwarmTaskBucketState
    let rawState: String // "active", "review", "blocked", "done", "queued"
    let progressLabel: String?
    let progressFraction: Double?
    let note: String
    let liveActivity: String?
    let waitingOn: String?
    let seq: Int
}

struct SwarmSynthesizedThread: Identifiable, Equatable {
    let id: String
    let title: String
    let goal: String
    let now: String
    let state: SwarmThreadState
    let statusLabel: String
    let tasks: [SwarmSynthesizedTask]
    let doneCount: Int
    let runningCount: Int
    let blockedCount: Int
    let queuedCount: Int

    var totalCount: Int { doneCount + runningCount + blockedCount + queuedCount }
}

struct SwarmSynthesizedModel: Equatable {
    let goalTitle: String
    let goalLine: String
    let runningSummary: String
    let phaseLabel: String?
    let finished: Bool
    let stallSeq: Int?
    let stallAgent: String?
    let stallText: String?
    let members: [String: SwarmSynthesizedMember]
    let orderedCrew: [SwarmSynthesizedMember]
    let tasks: [SwarmSynthesizedTask]
    let threads: [SwarmSynthesizedThread]
    let doneCount: Int
    let runningCount: Int
    let blockedCount: Int
    let queuedCount: Int
    let reviewCount: Int
    let openThreadsCount: Int
    let needsYouCount: Int
    let stuckCount: Int
    let waitingChips: [String]

    var totalTasksCount: Int { doneCount + runningCount + blockedCount + queuedCount }
}

struct SwarmHiveCell: Identifiable, Equatable {
    let id: String
    let title: String
    let goal: String
    let bees: [String]
    let done: Int
    let total: Int
    let countOnly: Int?
    let q: Int
    let r: Int
}

struct SwarmHiveMapModel: Equatable {
    let cells: [SwarmHiveCell]
    let beeStates: [String: SwarmHiveBeeState]
    let beeWaitingOn: [String: String]
    let allDone: Int
    let allTotal: Int
    let coordinatorNeedsYou: Bool
    let beamSeq: Int?
    let beamFrom: String?
    let beamTargets: [String]
    let waitLinks: [(from: String, to: String)]

    static func == (lhs: SwarmHiveMapModel, rhs: SwarmHiveMapModel) -> Bool {
        lhs.cells == rhs.cells
            && lhs.beeStates == rhs.beeStates
            && lhs.beeWaitingOn == rhs.beeWaitingOn
            && lhs.allDone == rhs.allDone
            && lhs.allTotal == rhs.allTotal
            && lhs.coordinatorNeedsYou == rhs.coordinatorNeedsYou
            && lhs.beamSeq == rhs.beamSeq
            && lhs.beamFrom == rhs.beamFrom
            && lhs.beamTargets == rhs.beamTargets
            && lhs.waitLinks.count == rhs.waitLinks.count
    }
}

enum SwarmHiveEngine {
    private static let askQuestionRegex = try? NSRegularExpression(
        pattern: #"\?|could you|can you|please|ping me|waiting on|请|能否"#,
        options: [.caseInsensitive]
    )
    private static let doneTagRegex = try? NSRegularExpression(
        pattern: #"^(?:@[A-Za-z0-9_\-]+[\s,:–—\-]*)*(?:\*\*|\*|`|_)?\[done\]"#,
        options: [.caseInsensitive]
    )

    private static func isDonePost(_ text: String) -> Bool {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let re = doneTagRegex else { return trimmed.lowercased().contains("[done]") }
        let range = NSRange(trimmed.startIndex..<trimmed.endIndex, in: trimmed)
        return re.firstMatch(in: trimmed, range: range) != nil
    }

    private static func looksLikeQuestion(_ text: String) -> Bool {
        guard let re = askQuestionRegex else { return text.contains("?") }
        let range = NSRange(text.startIndex..<text.endIndex, in: text)
        return re.firstMatch(in: text, range: range) != nil
    }

    private static func cleanInlineText(_ raw: String, maxChars: Int = 180) -> String {
        var s = raw
            .replacingOccurrences(of: "**", with: "")
            .replacingOccurrences(of: "`", with: "")
            .replacingOccurrences(of: "\n", with: " ")
        while s.contains("  ") {
            s = s.replacingOccurrences(of: "  ", with: " ")
        }
        s = s.trimmingCharacters(in: .whitespacesAndNewlines)
        if s.count > maxChars {
            return String(s.prefix(maxChars - 1)).trimmingCharacters(in: .whitespaces) + "…"
        }
        return s
    }

    static func synthesize(_ snap: SwarmSnapshot, filterMember: String? = nil) -> SwarmSynthesizedModel {
        let leadMember = snap.members.first(where: { $0.role == "coordinator" || $0.name == "coordinator" })
        let leadName = leadMember?.name ?? "coordinator"
        func normKey(_ n: String) -> String { n == leadName ? "coordinator" : n }

        let teamMembers = snap.members.filter { m in
            !m.roleWorker && !SwarmHelpers.isRoleWorker(name: m.name, role: m.role)
        }
        let teamByName = Dictionary(uniqueKeysWithValues: snap.members.map { ($0.name, $0) })

        let teamMsgs = snap.messages.filter { m in
            guard !m.text.hasPrefix("[channel]") else { return false }
            if let info = teamByName[m.agent] {
                return !info.roleWorker && !SwarmHelpers.isRoleWorker(name: info.name, role: info.role)
            }
            return !SwarmHelpers.isRoleWorker(name: m.agent)
        }

        let headSeq = max(snap.brief.latestSeq, teamMsgs.last?.seq ?? 0)
        let hasLiveWorking = teamMembers.contains { !$0.killed && $0.status.lowercased() == "working" }
        let hasSystemError = teamMembers.contains { !$0.killed && ($0.status.lowercased() == "error" || $0.status.lowercased() == "stuck") }
        let lastPost = teamMsgs.last
        let finished = !hasLiveWorking && !hasSystemError && (lastPost.map { normKey($0.agent) == "coordinator" && isDonePost($0.text) } ?? false)

        let narratorView = snap.view
        let plan = (narratorView == nil) ? snap.plan : (snap.view?.workstreams.isEmpty == true && snap.view?.threads.isEmpty == true ? snap.plan : nil)
        let planTasks = snap.plan?.tasks ?? [:]

        func lastPostBy(_ key: String) -> SwarmBoardMessage? {
            teamMsgs.last(where: { normKey($0.agent) == key })
        }

        // Attention asks from view.json
        var askByMember: [String: SwarmViewAttention] = [:]
        if !finished, let atts = narratorView?.attention {
            for a in atts where a.kind != "fyi" && a.kind != "failure" && a.kind != "stuck" && a.kind != "error" {
                if askByMember["coordinator"] == nil && a.kind == "decision" {
                    askByMember["coordinator"] = a
                }
            }
        }

        var synthMembers: [String: SwarmSynthesizedMember] = [:]
        for m in teamMembers {
            let k = normKey(m.name)
            let lp = lastPostBy(k)
            let rawSt = m.status.lowercased()
            let hasIssue = !m.killed && !finished && (rawSt == "error" || rawSt == "stuck")
            var st = m.killed ? "parked" : (rawSt == "working" ? "working" : "parked")
            var waitingOn: String? = nil

            if st == "parked", !finished, !m.killed, let lp {
                let directTargets = SwarmHelpers.targets(of: lp).filter { $0.lowercased() != "all" && !SwarmHelpers.isRoleWorker(name: $0) }.map(normKey)
                if let firstTarget = directTargets.first, looksLikeQuestion(lp.text) {
                    let answered = teamMsgs.contains { $0.seq > lp.seq && normKey($0.agent) == firstTarget }
                    if !answered {
                        st = "waiting_on"
                        waitingOn = firstTarget
                    }
                }
            }

            let actTrimmed = (m.lastActivity ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            let isGenericWaitLabel = actTrimmed.contains("swarm_wait") || actTrimmed == "等待新消息" || actTrimmed == "Waiting for posts"
            let effectiveLive = (rawSt == "working" && !m.killed && !hasIssue && !actTrimmed.isEmpty && !isGenericWaitLabel) ? cleanInlineText(actTrimmed, maxChars: 120) : nil

            let nowLine: String = {
                if let live = effectiveLive { return live }
                if let narratorNow = narratorView?.now[m.name], !narratorNow.isEmpty {
                    return cleanInlineText(narratorNow, maxChars: 120)
                }
                if let lp {
                    return cleanInlineText(SwarmHelpers.stripLeadingMentions(lp.text), maxChars: 140)
                }
                return ""
            }()

            let roleTitle: String = {
                if k == "coordinator" { return String(localized: "Coordinator") }
                if let pt = planTasks[m.name]?.trimmingCharacters(in: .whitespacesAndNewlines), !pt.isEmpty {
                    return pt
                }
                return SwarmHelpers.extractRoleHint(for: m.name, role: m.role, messages: snap.messages)
            }()

            let postCount = m.posts > 0 ? m.posts : teamMsgs.filter { normKey($0.agent) == k }.count
            let lastSeq = m.lastPostSeq > 0 ? m.lastPostSeq : (lp?.seq ?? 0)

            synthMembers[k] = SwarmSynthesizedMember(
                name: k,
                conversationId: m.conversationId,
                role: m.role,
                title: roleTitle,
                model: m.model,
                status: st,
                waitingOn: waitingOn,
                now: nowLine,
                liveText: effectiveLive,
                liveKind: m.activityKind,
                hasIssue: hasIssue,
                issueKind: hasIssue ? rawSt : nil,
                issueText: hasIssue ? actTrimmed : nil,
                killed: m.killed,
                posts: postCount,
                lastPostSeq: lastSeq
            )
        }

        if synthMembers["coordinator"] == nil {
            synthMembers["coordinator"] = SwarmSynthesizedMember(
                name: "coordinator",
                conversationId: snap.coordinatorId,
                role: "coordinator",
                title: String(localized: "Coordinator"),
                model: "",
                status: "parked",
                waitingOn: nil,
                now: lastPostBy("coordinator").map { cleanInlineText(SwarmHelpers.stripLeadingMentions($0.text), maxChars: 140) } ?? "",
                liveText: nil,
                liveKind: nil,
                hasIssue: false,
                issueKind: nil,
                issueText: nil,
                killed: false,
                posts: 0,
                lastPostSeq: 0
            )
        }

        // Synthesize Tasks (Workstreams)
        var allTasks: [SwarmSynthesizedTask] = []
        if let vws = narratorView?.workstreams, !vws.isEmpty {
            for w in vws {
                let owners = w.owners.isEmpty ? ["coordinator"] : w.owners.map(normKey)
                let raw0 = w.state.lowercased()
                let rawState = (finished && raw0 != "failed") ? "done" : raw0
                var bucket: SwarmTaskBucketState = {
                    switch rawState {
                    case "done": return .done
                    case "active", "review": return .running
                    case "blocked", "failed": return .blocked
                    default: return .queued
                    }
                }()
                var waitTarget: String? = nil
                if bucket != .done && bucket != .blocked, let firstOwner = owners.first, let om = synthMembers[firstOwner], om.status == "waiting_on" {
                    bucket = .blocked
                    waitTarget = om.waitingOn
                }
                let liveOwner = bucket == .running ? owners.compactMap { synthMembers[$0] }.first(where: { !$0.killed && $0.liveText != nil }) : nil
                allTasks.append(SwarmSynthesizedTask(
                    id: w.id,
                    title: w.title,
                    owners: owners,
                    state: bucket,
                    rawState: rawState,
                    progressLabel: nil,
                    progressFraction: nil,
                    note: cleanInlineText(w.note ?? liveOwner?.now ?? ""),
                    liveActivity: liveOwner?.liveText,
                    waitingOn: waitTarget,
                    seq: headSeq
                ))
            }
        } else {
            for m in teamMembers where m.role != "coordinator" && m.name != leadName {
                let k = normKey(m.name)
                guard let sm = synthMembers[k] else { continue }
                let lp = lastPostBy(k)
                let rawState: String = {
                    if sm.killed { return "done" }
                    if sm.status == "waiting_on" { return "blocked" }
                    if sm.status == "working" || sm.status == "needs_you" { return "active" }
                    if let lp {
                        return (finished || isDonePost(lp.text)) ? "done" : "review"
                    }
                    return "queued"
                }()
                let bucket: SwarmTaskBucketState = {
                    switch rawState {
                    case "done": return .done
                    case "active", "review": return .running
                    case "blocked": return .blocked
                    default: return .queued
                    }
                }()
                let taskTitle = planTasks[m.name]?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
                    ? planTasks[m.name]!
                    : (!sm.title.isEmpty ? sm.title : k)
                allTasks.append(SwarmSynthesizedTask(
                    id: k,
                    title: taskTitle,
                    owners: [k],
                    state: bucket,
                    rawState: rawState,
                    progressLabel: nil,
                    progressFraction: nil,
                    note: sm.now,
                    liveActivity: sm.liveText,
                    waitingOn: sm.waitingOn,
                    seq: lp?.seq ?? headSeq
                ))
            }
        }

        let taskById = Dictionary(uniqueKeysWithValues: allTasks.map { ($0.id, $0) })
        let activeFilter = filterMember?.isEmpty == false ? filterMember : nil
        func matchesFilter(_ t: SwarmSynthesizedTask) -> Bool {
            guard let f = activeFilter else { return true }
            return t.owners.contains(f)
        }

        // Raw thread specs from view.threads or plan.threads
        struct RawThreadSpec {
            let id: String
            let title: String
            let goal: String
            let now: String
            let taskIds: [String]
        }
        let rawSpecs: [RawThreadSpec] = {
            if let vt = narratorView?.threads, !vt.isEmpty {
                return vt.map { RawThreadSpec(id: $0.id, title: $0.title, goal: $0.goal ?? "", now: $0.now ?? "", taskIds: $0.workstreams.map(normKey)) }
            }
            if let pt = snap.plan?.threads, !pt.isEmpty {
                return pt.map { RawThreadSpec(id: $0.id, title: $0.title, goal: $0.goal ?? "", now: "", taskIds: $0.tasks.map(normKey)) }
            }
            return []
        }()

        var usedIds = Set<String>()
        var synthThreads: [SwarmSynthesizedThread] = []

        func buildThread(id: String, title: String, goal: String, explicitNow: String, items: [SwarmSynthesizedTask]) -> SwarmSynthesizedThread? {
            guard !items.isEmpty else { return nil }
            var d = 0, r = 0, b = 0, q = 0
            var waiters = Set<String>()
            for item in items {
                switch item.state {
                case .done: d += 1
                case .running: r += 1
                case .blocked:
                    b += 1
                    if let w = item.waitingOn { waiters.insert(w) }
                case .queued: q += 1
                }
            }
            let hasStuck = items.contains { t in t.owners.contains { synthMembers[$0]?.hasIssue == true } }
            let tState: SwarmThreadState = {
                if !waiters.isEmpty || b > 0 { return .waiting }
                if hasStuck { return .stuck }
                if r > 0 { return .running }
                if q > 0 && d == 0 { return .queued }
                if q > 0 { return .running }
                return .done
            }()
            let statusLabel: String = {
                switch tState {
                case .needs: return String(localized: "待你确认")
                case .waiting:
                    if waiters.count == 1, let only = waiters.first {
                        return String(localized: "Waiting on @\(only)")
                    }
                    return waiters.count > 1 ? String(localized: "Waiting on \(waiters.count)") : String(localized: "Waiting")
                case .stuck: return String(localized: "Stuck")
                case .running: return String(localized: "Moving")
                case .queued: return String(localized: "Not started")
                case .done: return String(localized: "Done")
                }
            }()
            let derivedNow: String = {
                if !explicitNow.isEmpty { return explicitNow }
                if let workingOwner = items.flatMap(\.owners).compactMap({ synthMembers[$0] }).first(where: { !$0.killed && $0.status == "working" && !$0.now.isEmpty }) {
                    return workingOwner.liveText ?? workingOwner.now
                }
                return ""
            }()
            let sortedTasks = items.sorted { a, b in
                let rank: (SwarmTaskBucketState) -> Int = {
                    switch $0 {
                    case .blocked: return 0
                    case .running: return 1
                    case .queued: return 2
                    case .done: return 3
                    }
                }
                return rank(a.state) < rank(b.state)
            }
            return SwarmSynthesizedThread(
                id: id,
                title: title,
                goal: goal,
                now: derivedNow,
                state: tState,
                statusLabel: statusLabel,
                tasks: sortedTasks,
                doneCount: d,
                runningCount: r,
                blockedCount: b,
                queuedCount: q
            )
        }

        for spec in rawSpecs {
            let ws = spec.taskIds.compactMap { taskById[$0] }.filter(matchesFilter)
            spec.taskIds.forEach { usedIds.insert($0) }
            if let th = buildThread(id: spec.id, title: spec.title, goal: spec.goal, explicitNow: spec.now, items: ws) {
                synthThreads.append(th)
            }
        }

        let remaining = allTasks.filter { !usedIds.contains($0.id) }.filter(matchesFilter)
        if !remaining.isEmpty, !rawSpecs.isEmpty {
            if let otherTh = buildThread(id: "t-other", title: String(localized: "Other"), goal: "", explicitNow: "", items: remaining) {
                synthThreads.append(otherTh)
            }
        }

        let threadRank: (SwarmThreadState) -> Int = {
            switch $0 {
            case .needs: return 0
            case .waiting: return 1
            case .stuck: return 2
            case .running: return 3
            case .queued: return 4
            case .done: return 5
            }
        }
        synthThreads.sort { threadRank($0.state) < threadRank($1.state) }

        let visibleTasks = allTasks.filter(matchesFilter)
        var totDone = 0, totRun = 0, totBlk = 0, totQue = 0, totRev = 0
        var waitMap: [String: Int] = [:]
        for t in visibleTasks {
            switch t.state {
            case .done: totDone += 1
            case .running:
                totRun += 1
                if t.rawState == "review" { totRev += 1 }
            case .blocked:
                totBlk += 1
                if let w = t.waitingOn { waitMap[w, default: 0] += 1 }
            case .queued: totQue += 1
            }
        }

        let waitingChips = waitMap.sorted { $0.value > $1.value }.prefix(2).map { String(localized: "\($0.value) waiting on @\($0.key)") }
        let stuckCount = synthMembers.values.filter { !$0.killed && $0.hasIssue }.count
        let openThreads = synthThreads.filter { $0.state != .done }.count
        let needsYouCount = askByMember["coordinator"] != nil ? 1 : 0

        // Ordered crew for The Team strip
        let crewRank: (SwarmSynthesizedMember) -> Int = { m in
            if m.status == "needs_you" { return 0 }
            if m.status == "working" { return 1 }
            if m.status == "waiting_on" || m.hasIssue { return 2 }
            return 3
        }
        let orderedCrew = synthMembers.values
            .filter { $0.name != "coordinator" && !$0.killed }
            .sorted { a, b in
                let ra = crewRank(a), rb = crewRank(b)
                if ra != rb { return ra < rb }
                return a.name < b.name
            }

        // Stall banner check (@monitor posted [stall] after latest team post while nobody is working)
        var stallSeq: Int? = nil
        var stallAgent: String? = nil
        var stallText: String? = nil
        if !finished && !hasLiveWorking {
            if let monMsg = snap.messages.last(where: { m in
                SwarmHelpers.isRoleWorker(name: m.agent) && m.text.range(of: "[stall]", options: .caseInsensitive) != nil
            }), monMsg.seq >= (lastPost?.seq ?? 0) {
                stallSeq = monMsg.seq
                stallAgent = monMsg.agent
                stallText = cleanInlineText(monMsg.text.replacingOccurrences(of: "[stall]", with: "", options: .caseInsensitive), maxChars: 200)
            }
        }

        let goalObj = narratorView?.goal ?? snap.plan?.goal ?? plan?.goal
        let goalTitle = goalObj?.title.isEmpty == false ? goalObj!.title : (narratorView?.headline ?? "")
        let goalLine = goalObj?.line ?? (goalObj?.title.isEmpty == false ? (narratorView?.headline ?? "") : "")
        let phaseLabel: String? = {
            if let p = narratorView?.phase, p.total > 1 {
                return "\(p.label) · \(p.index)/\(p.total)"
            }
            return narratorView?.phase?.label
        }()

        return SwarmSynthesizedModel(
            goalTitle: goalTitle,
            goalLine: goalLine,
            runningSummary: narratorView?.runningSummary ?? "",
            phaseLabel: phaseLabel,
            finished: finished,
            stallSeq: stallSeq,
            stallAgent: stallAgent,
            stallText: stallText,
            members: synthMembers,
            orderedCrew: orderedCrew,
            tasks: visibleTasks,
            threads: synthThreads,
            doneCount: totDone,
            runningCount: totRun,
            blockedCount: totBlk,
            queuedCount: totQue,
            reviewCount: totRev,
            openThreadsCount: openThreads,
            needsYouCount: needsYouCount,
            stuckCount: stuckCount,
            waitingChips: waitingChips
        )
    }

    // MARK: - Honeycomb Hive Map Layout & Replay Engine

    private static let hvRing: [(Int, Int)] = [(0, -1), (1, -1), (1, 0), (0, 1), (-1, 1), (-1, 0)]
    private static let hvFew: [Int: [Int]] = [
        1: [0],
        2: [5, 1],
        3: [0, 2, 4],
        4: [5, 1, 4, 2],
        5: [5, 0, 1, 4, 2]
    ]

    static func hiveSlots(count n: Int) -> [(Int, Int)] {
        guard n >= 1 else { return [] }
        if let preset = hvFew[n] {
            return preset.map { hvRing[$0] }
        }
        var out: [(Int, Int)] = []
        let dirs = [(1, 0), (0, 1), (-1, 1), (-1, 0), (0, -1), (1, -1)]
        var ring = 1
        while out.count < n {
            var q = 0, r = -ring
            for (dq, dr) in dirs {
                for _ in 0..<ring {
                    out.append((q, r))
                    q += dq
                    r += dr
                }
            }
            ring += 1
        }
        return Array(out.prefix(n))
    }

    static func buildHiveMap(snap: SwarmSnapshot, synth: SwarmSynthesizedModel, replaySeq: Int?) -> SwarmHiveMapModel {
        let leadName = snap.members.first(where: { $0.role == "coordinator" || $0.name == "coordinator" })?.name ?? "coordinator"
        func normKey(_ n: String) -> String { n == leadName ? "coordinator" : n }

        let teamMsgs = snap.messages.filter { m in
            !m.text.hasPrefix("[channel]") && !SwarmHelpers.isRoleWorker(name: m.agent)
        }
        let names = Array(synth.members.keys)

        var beeStates: [String: SwarmHiveBeeState] = [:]
        var beeWaitingOn: [String: String] = [:]
        var replayDoneCheck: ((String) -> Bool)? = nil
        var beamMsg: SwarmBoardMessage? = nil

        if let seq = replaySeq {
            let prefixMsgs = teamMsgs.filter { $0.seq <= seq }
            var finIdx: [String: Int] = [:]
            for (idx, m) in teamMsgs.enumerated() {
                finIdx[normKey(m.agent)] = idx
            }
            var born: Set<String> = ["coordinator"]
            var lastPostIdx: [String: Int] = [:]
            var lastInIdx: [String: Int] = [:]

            for (idx, m) in prefixMsgs.enumerated() {
                let sender = normKey(m.agent)
                born.insert(sender)
                lastPostIdx[sender] = idx
                if !SwarmHelpers.isBroadcast(m) {
                    for t in SwarmHelpers.targets(of: m) where !SwarmHelpers.isRoleWorker(name: t) {
                        let tk = normKey(t)
                        born.insert(tk)
                        lastInIdx[tk] = idx
                    }
                }
            }

            let curIdx = prefixMsgs.count - 1
            let nowDate = curIdx >= 0 ? SwarmHelpers.parseDate(prefixMsgs[curIdx].time) : nil

            for n in names {
                guard let sm = synth.members[n] else { continue }
                let lp = lastPostIdx[n] ?? -1
                let li = lastInIdx[n] ?? -1
                var st: SwarmHiveBeeState = .idle
                var who: String? = nil

                if !born.contains(n) {
                    st = .unborn
                } else if sm.killed, let fn = finIdx[n], curIdx >= fn && curIdx > fn + 8 {
                    st = .retired
                } else if li > lp {
                    st = .working
                    if li >= 0 { who = normKey(prefixMsgs[li].agent) }
                } else if lp >= 0 {
                    let post = prefixMsgs[lp]
                    let targets = SwarmHelpers.isBroadcast(post) ? [] : SwarmHelpers.targets(of: post).filter { !SwarmHelpers.isRoleWorker(name: $0) }.map(normKey)
                    if let firstT = targets.first, looksLikeQuestion(post.text), (lastPostIdx[firstT] ?? -1) <= lp {
                        st = .wait
                        who = firstT
                    } else if let fn = finIdx[n], fn <= curIdx, n != "coordinator" {
                        st = .done
                    } else if curIdx - lp < 3 {
                        if let dNow = nowDate, let dLp = SwarmHelpers.parseDate(post.time), dNow.timeIntervalSince(dLp) < 240 {
                            st = .working
                        } else if nowDate == nil {
                            st = .working
                        }
                    }
                }
                beeStates[n] = st
                if let w = who, w != n { beeWaitingOn[n] = w }
            }
            replayDoneCheck = { worker in
                let st = beeStates[worker]
                if st == .retired { return true }
                if let fn = finIdx[worker], fn <= curIdx { return true }
                return false
            }
            beamMsg = prefixMsgs.last
        } else {
            for n in names {
                guard let sm = synth.members[n] else { continue }
                let ownedTasks = synth.tasks.filter { $0.owners.contains(n) }
                var st: SwarmHiveBeeState = .idle
                if sm.killed {
                    st = .retired
                } else if synth.finished {
                    st = (n == "coordinator") ? .idle : .done
                } else if sm.hasIssue {
                    st = .issue
                } else if sm.status == "working" {
                    st = .working
                } else if sm.status == "waiting_on" {
                    st = .wait
                    if let w = sm.waitingOn { beeWaitingOn[n] = w }
                } else if n != "coordinator" && !ownedTasks.isEmpty && ownedTasks.allSatisfy({ $0.state == .done }) {
                    st = .done
                }
                beeStates[n] = st
            }
            if !synth.finished && synth.needsYouCount > 0 && beeStates["coordinator"] != .retired {
                beeStates["coordinator"] = .needs
            }
            if let lp = teamMsgs.last {
                if let d = SwarmHelpers.parseDate(lp.time) {
                    if Date().timeIntervalSince(d) < 180 {
                        beamMsg = lp
                    }
                } else {
                    beamMsg = lp
                }
            }
        }

        let workers = names.filter { $0 != "coordinator" }.sorted()
        let taskById = Dictionary(uniqueKeysWithValues: synth.tasks.map { ($0.id, $0) })
        func isTaskDone(_ t: SwarmSynthesizedTask) -> Bool {
            if let replayDone = replayDoneCheck {
                return t.owners.allSatisfy { $0 == "coordinator" || replayDone($0) }
            }
            return t.state == .done
        }

        // Thread definitions (unfiltered from plan/view)
        let rawThreads: [(id: String, title: String, goal: String, wsIds: [String])] = {
            if let vt = snap.view?.threads, !vt.isEmpty {
                return vt.map { ($0.id, $0.title, $0.goal ?? "", $0.workstreams.map(normKey)) }
            }
            if let pt = snap.plan?.threads, !pt.isEmpty {
                return pt.map { ($0.id, $0.title, $0.goal ?? "", $0.tasks.map(normKey)) }
            }
            return []
        }()

        struct TempCell {
            let id: String
            let title: String
            let goal: String
            let ws: [SwarmSynthesizedTask]
            var bees: [String]
            let done: Int
            let total: Int
            let countOnly: Int?
        }

        var tempCells: [TempCell] = []
        if !rawThreads.isEmpty {
            for rt in rawThreads {
                let ws = rt.wsIds.compactMap { taskById[$0] }
                let doneCnt = ws.filter(isTaskDone).count
                tempCells.append(TempCell(id: rt.id, title: rt.title, goal: rt.goal, ws: ws, bees: [], done: doneCnt, total: ws.count, countOnly: nil))
            }
            var homed = Set<String>()
            for w in workers {
                let matchingIndices = tempCells.indices.filter { idx in
                    tempCells[idx].ws.contains { $0.owners.contains(w) }
                }
                guard !matchingIndices.isEmpty else { continue }
                let chosen = matchingIndices.first(where: { idx in
                    tempCells[idx].ws.contains { $0.owners.contains(w) && !isTaskDone($0) }
                }) ?? matchingIndices[0]
                tempCells[chosen].bees.append(w)
                homed.insert(w)
            }
            let rest = workers.filter { !homed.contains($0) }
            if !rest.isEmpty {
                let doneRest = rest.filter { beeStates[$0] == .done || beeStates[$0] == .retired }.count
                tempCells.append(TempCell(id: "_other", title: String(localized: "Other"), goal: "", ws: [], bees: rest, done: doneRest, total: rest.count, countOnly: nil))
            }
        } else {
            let buckets: [(String, String, (SwarmHiveBeeState) -> Bool)] = [
                ("Working", String(localized: "Working"), { $0 == .working }),
                ("Waiting", String(localized: "Waiting"), { $0 == .wait || $0 == .issue }),
                ("Resting", String(localized: "Resting"), { $0 == .idle }),
                ("Done", String(localized: "Done"), { $0 == .done || $0 == .retired })
            ]
            for (bucketKey, title, pred) in buckets {
                let matched = workers.filter { pred(beeStates[$0] ?? .idle) }
                if !matched.isEmpty {
                    tempCells.append(TempCell(
                        id: "_\(bucketKey)",
                        title: title,
                        goal: "",
                        ws: [],
                        bees: matched,
                        done: title == "Done" ? matched.count : 0,
                        total: matched.count,
                        countOnly: matched.count
                    ))
                }
            }
        }

        let slots = hiveSlots(count: tempCells.count)
        let finalCells: [SwarmHiveCell] = tempCells.enumerated().map { idx, tc in
            let (q, r) = idx < slots.count ? slots[idx] : (0, 0)
            return SwarmHiveCell(
                id: tc.id,
                title: tc.title,
                goal: tc.goal,
                bees: tc.bees,
                done: tc.done,
                total: tc.total,
                countOnly: tc.countOnly,
                q: q,
                r: r
            )
        }

        let allDone = replayDoneCheck != nil ? finalCells.reduce(0) { $0 + $1.done } : synth.doneCount
        let allTotal = replayDoneCheck != nil ? finalCells.reduce(0) { $0 + $1.total } : max(1, synth.totalTasksCount)

        var beamFrom: String? = nil
        var beamTargets: [String] = []
        if let bm = beamMsg {
            let sender = normKey(bm.agent)
            beamFrom = sender
            if !SwarmHelpers.isBroadcast(bm) {
                beamTargets = SwarmHelpers.targets(of: bm)
                    .filter { !SwarmHelpers.isRoleWorker(name: $0) }
                    .map(normKey)
                    .filter { beeStates[$0] != nil && beeStates[$0] != .unborn }
            }
        }

        var waitLinks: [(from: String, to: String)] = []
        for (n, st) in beeStates where st == .wait {
            if let w = beeWaitingOn[n], beeStates[n] != .unborn, beeStates[w] != nil, beeStates[w] != .unborn {
                waitLinks.append((from: n, to: w))
            }
        }

        return SwarmHiveMapModel(
            cells: finalCells,
            beeStates: beeStates,
            beeWaitingOn: beeWaitingOn,
            allDone: allDone,
            allTotal: allTotal,
            coordinatorNeedsYou: beeStates["coordinator"] == .needs,
            beamSeq: beamMsg?.seq,
            beamFrom: beamFrom,
            beamTargets: beamTargets,
            waitLinks: waitLinks
        )
    }
}
