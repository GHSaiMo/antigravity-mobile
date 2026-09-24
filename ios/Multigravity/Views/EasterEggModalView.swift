import SwiftUI

// MARK: - Firework Particle & Burst Models

private struct FireworkParticle: Identifiable {
    let id = UUID()
    let angle: Double
    let speed: CGFloat
    let color: Color
    let size: CGFloat
    let initialAlpha: Double
}

private struct FireworkBurst: Identifiable {
    let id = UUID()
    let origin: CGPoint
    let delay: TimeInterval
    let duration: TimeInterval
    let particles: [FireworkParticle]
    
    static func generate(at origin: CGPoint, delay: TimeInterval, count: Int = 30) -> FireworkBurst {
        let colors: [Color] = [
            Color(red: 1.0, green: 0.843, blue: 0.0),    // Gold (0xFFFFD700)
            Color(red: 0.659, green: 0.333, blue: 0.969), // Purple (0xFFA855F7)
            Color(red: 0.024, green: 0.714, blue: 0.831), // Cyan (0xFF06B6D4)
            Color(red: 0.957, green: 0.247, blue: 0.369), // Rose (0xFFF43F5E)
            Color(red: 0.063, green: 0.725, blue: 0.506), // Emerald (0xFF10B981)
            Color(red: 1.0, green: 0.596, blue: 0.0),    // Amber (0xFFFF9800)
            Color.white                                   // Starlight (0xFFFFFFFF)
        ]
        
        let particles = (0..<count).map { i -> FireworkParticle in
            let angle = (Double(i) / Double(count)) * 2.0 * .pi + Double.random(in: -0.15...0.15)
            let speed = CGFloat.random(in: 80...240)
            let color = colors.randomElement() ?? .yellow
            let size = CGFloat.random(in: 4.0...9.0)
            let initialAlpha = Double.random(in: 0.85...1.0)
            return FireworkParticle(angle: angle, speed: speed, color: color, size: size, initialAlpha: initialAlpha)
        }
        
        return FireworkBurst(
            origin: origin,
            delay: delay,
            duration: 1.5,
            particles: particles
        )
    }
}

// MARK: - Fireworks Canvas

private struct FireworksCanvasView: View {
    let bursts: [FireworkBurst]
    let startTime: Date
    
    var body: some View {
        TimelineView(.animation) { timeline in
            Canvas { context, size in
                let elapsed = timeline.date.timeIntervalSince(startTime)
                
                for burst in bursts {
                    let burstElapsed = elapsed - burst.delay
                    guard burstElapsed > 0 && burstElapsed < burst.duration else { continue }
                    
                    let progress = burstElapsed / burst.duration
                    // Ease-out deceleration curve
                    let ease = 1.0 - pow(1.0 - progress, 3)
                    // Slight gravity effect
                    let gravity = pow(progress, 2) * 35.0
                    
                    for particle in burst.particles {
                        let distance = particle.speed * CGFloat(ease)
                        let px = burst.origin.x + cos(particle.angle) * distance
                        let py = burst.origin.y + sin(particle.angle) * distance + CGFloat(gravity)
                        
                        let currentAlpha = particle.initialAlpha * max(0, 1.0 - progress)
                        let currentSize = particle.size * CGFloat(max(0.3, 1.0 - progress * 0.6))
                        
                        let rect = CGRect(
                            x: px - currentSize / 2,
                            y: py - currentSize / 2,
                            width: currentSize,
                            height: currentSize
                        )
                        
                        context.opacity = currentAlpha
                        context.fill(Path(ellipseIn: rect), with: .color(particle.color))
                        
                        // Add bright sparkling core for larger particles
                        if particle.size > 6.0 && progress < 0.6 {
                            let coreSize = currentSize * 0.5
                            let coreRect = CGRect(
                                x: px - coreSize / 2,
                                y: py - coreSize / 2,
                                width: coreSize,
                                height: coreSize
                            )
                            context.opacity = currentAlpha * 0.8
                            context.fill(Path(ellipseIn: coreRect), with: .color(.white))
                        }
                    }
                }
            }
        }
    }
}

// MARK: - Easter Egg Modal View

public struct EasterEggModalView: View {
    @Binding public var isPresented: Bool
    
    @State private var bursts: [FireworkBurst] = []
    @State private var startTime = Date()
    @State private var cardScale: CGFloat = 0.82
    @State private var overallOpacity: Double = 0.0
    @State private var isDismissing = false
    
    public init(isPresented: Binding<Bool>) {
        self._isPresented = isPresented
    }
    
    public var body: some View {
        GeometryReader { geometry in
            ZStack {
                // Dimmed translucent backdrop (absorbs taps so rapid clicks do not dismiss early)
                Color.black.opacity(0.45 * overallOpacity)
                    .ignoresSafeArea()
                    .contentShape(Rectangle())
                
                // Fireworks particle layer behind and around the card
                FireworksCanvasView(bursts: bursts, startTime: startTime)
                    .frame(width: geometry.size.width, height: geometry.size.height)
                    .allowsHitTesting(false)
                    .opacity(overallOpacity)
                
                // Centered Easter Egg Card (1:1 aligned with Android)
                VStack(spacing: 16) {
                    // App Logo
                    Image("AppLogo")
                        .resizable()
                        .aspectRatio(contentMode: .fit)
                        .frame(width: 72, height: 72)
                        .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
                        .overlay(
                            RoundedRectangle(cornerRadius: 18, style: .continuous)
                                .stroke(Color.white.opacity(0.2), lineWidth: 1)
                        )
                    
                    // Brand Title
                    Text("Multigravity")
                        .font(.system(size: 22, weight: .bold))
                        .foregroundColor(.primary)
                        .tracking(0.5)
                    
                    // Subtitle / Credit: Design by Jiuge
                    HStack(spacing: 6) {
                        Image(systemName: "sparkles")
                            .font(.system(size: 14, weight: .semibold))
                            .foregroundColor(Color(UIColor.systemIndigo))
                        Text("Design by Jiuge")
                            .font(.system(size: 14, weight: .semibold))
                            .foregroundColor(Color(UIColor.systemIndigo))
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 6)
                    .background(
                        Capsule()
                            .fill(Color(UIColor.systemIndigo).opacity(0.12))
                    )
                }
                .padding(.horizontal, 36)
                .padding(.vertical, 28)
                .background(
                    RoundedRectangle(cornerRadius: 24, style: .continuous)
                        .fill(Color(UIColor.secondarySystemGroupedBackground).opacity(0.94))
                        .overlay(
                            RoundedRectangle(cornerRadius: 24, style: .continuous)
                                .stroke(Color.white.opacity(0.18), lineWidth: 1)
                        )
                        .shadow(color: Color.black.opacity(0.18), radius: 24, x: 0, y: 8)
                )
                .padding(32)
                .scaleEffect(cardScale)
                .opacity(overallOpacity)
            }
            .onAppear {
                setupBursts(in: geometry.size)
                
                // Entrance animation
                withAnimation(.spring(response: 0.35, dampingFraction: 0.72)) {
                    cardScale = 1.0
                    overallOpacity = 1.0
                }
                
                // Auto fade-out after ~2.3 seconds
                Task {
                    try? await Task.sleep(nanoseconds: 2_300_000_000)
                    guard !isDismissing else { return }
                    dismissWithAnimation()
                }
            }
        }
    }
    
    private func setupBursts(in size: CGSize) {
        let cx = size.width / 2
        let cy = size.height / 2
        
        bursts = [
            FireworkBurst.generate(at: CGPoint(x: cx - 80, y: cy - 90), delay: 0.0, count: 30),
            FireworkBurst.generate(at: CGPoint(x: cx + 85, y: cy - 75), delay: 0.15, count: 32),
            FireworkBurst.generate(at: CGPoint(x: cx - 75, y: cy + 70), delay: 0.35, count: 28),
            FireworkBurst.generate(at: CGPoint(x: cx + 80, y: cy + 85), delay: 0.5, count: 30),
            FireworkBurst.generate(at: CGPoint(x: cx, y: cy - 120), delay: 0.75, count: 36)
        ]
        startTime = Date()
    }
    
    private func dismissWithAnimation() {
        guard !isDismissing else { return }
        isDismissing = true
        
        withAnimation(.easeOut(duration: 0.55)) {
            overallOpacity = 0.0
            cardScale = 0.92
        }
        
        Task {
            try? await Task.sleep(nanoseconds: 600_000_000)
            isPresented = false
        }
    }
}
