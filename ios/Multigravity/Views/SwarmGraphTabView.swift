import SwiftUI

struct SwarmGraphTabView: View {
    let snapshot: SwarmSnapshot
    let onSelectConversation: ((String) -> Void)?
    let onDismissSheet: () -> Void

    enum GraphMode: String, CaseIterable {
        case hive = "Hive"
        case mentions = "Mentions"
    }

    enum ReplayPacing: String, CaseIterable {
        case even = "Even pacing"
        case realtime = "Real-time gaps"

        var shortLabel: String {
            switch self {
            case .even: return "Even"
            case .realtime: return "Real-time"
            }
        }
    }

    @State private var graphMode: GraphMode = .hive
    @State private var replayIndex: Int? = nil // nil == Live (all messages)
    @State private var isPlaying: Bool = false
    @State private var replayPacing: ReplayPacing = .even
    @State private var replaySpeed: Double = 1.0

    // Hive interactive state
    @State private var selectedCellId: String? = nil
    @State private var hiveZoom: CGFloat = 1.0
    @State private var beamProgress: CGFloat = 1.0
    @State private var lastAnimatedBeamSeq: Int? = nil
    @State private var waitDashPhase: CGFloat = 0

    // Mentions graph state
    @State private var hideInactive: Bool = false
    @State private var expandAll: Bool = false
    @State private var selectedNodeId: String? = nil

    private static let speedOptions: [Double] = [0.5, 1, 2, 4, 8]

    private var allMessages: [SwarmBoardMessage] {
        snapshot.messages.filter { !$0.text.hasPrefix("[channel]") }
    }

    private var visibleMessages: [SwarmBoardMessage] {
        let all = allMessages
        guard let idx = replayIndex else { return all }
        let clamped = max(0, min(idx, all.count))
        return Array(all.prefix(clamped))
    }

    private var currentReplayMessage: SwarmBoardMessage? {
        let all = allMessages
        guard let idx = replayIndex, idx > 0, idx <= all.count else { return nil }
        return all[idx - 1]
    }

    private var displayDetailMessage: SwarmBoardMessage? {
        if let cur = currentReplayMessage {
            return cur
        }
        if graphMode == .mentions {
            if let sel = selectedNodeId {
                return allMessages.last(where: { $0.agent == sel }) ?? allMessages.last(where: {
                    SwarmHelpers.targets(of: $0).contains(where: { $0.caseInsensitiveCompare(sel) == .orderedSame })
                })
            }
            return allMessages.last
        }
        return nil
    }

    private var memberByName: [String: SwarmMemberInfo] {
        Dictionary(uniqueKeysWithValues: snapshot.members.map { ($0.name, $0) })
    }

    private var taskMembers: [SwarmMemberInfo] {
        let filtered = snapshot.members.filter { !$0.roleWorker && !SwarmHelpers.isRoleWorker(name: $0.name, role: $0.role) }
        return filtered.sorted { a, b in
            if a.name == "coordinator" || a.role == "coordinator" { return true }
            if b.name == "coordinator" || b.role == "coordinator" { return false }
            return a.name < b.name
        }
    }

    var body: some View {
        let synth = SwarmHiveEngine.synthesize(snapshot)
        let replaySeq: Int? = replayIndex != nil ? (currentReplayMessage?.seq ?? 0) : nil
        let hiveModel = SwarmHiveEngine.buildHiveMap(snap: snapshot, synth: synth, replaySeq: replaySeq)

        GeometryReader { rootGeo in
            let hiveAvailW = max(280, rootGeo.size.width - 20)

            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    replayControlsBar
                        .padding(.horizontal, 16)
                        .padding(.top, 14)

                    if graphMode == .hive {
                        hiveHoneycombSection(availW: hiveAvailW, hiveModel: hiveModel, synth: synth)
                            .padding(.horizontal, 10)

                        hiveLegendRow
                            .padding(.horizontal, 16)

                        if let selId = selectedCellId,
                           let selCell = hiveModel.cells.first(where: { $0.id == selId }) {
                            selectedHiveCellCard(cell: selCell, hiveModel: hiveModel, synth: synth)
                                .padding(.horizontal, 16)
                        }
                    } else {
                        mentionsFilterRow
                            .padding(.horizontal, 16)

                        graphCanvasCard
                            .padding(.horizontal, 12)

                        mentionsLegendRow
                            .padding(.horizontal, 16)
                    }

                    if let detailMsg = displayDetailMessage {
                        detailCard(detailMsg)
                            .padding(.horizontal, 16)
                    }
                }
                .frame(width: rootGeo.size.width, alignment: .leading)
                .padding(.bottom, 22)
            }
            .frame(width: rootGeo.size.width, height: rootGeo.size.height)
            .background(SwarmTheme.bg)
        }
        .background(SwarmTheme.bg)
        .onAppear {
            withAnimation(.linear(duration: 1.0).repeatForever(autoreverses: false)) {
                waitDashPhase = -8
            }
        }
        .onChange(of: hiveModel.beamSeq) { _, newSeq in
            guard let seq = newSeq, seq != lastAnimatedBeamSeq else { return }
            lastAnimatedBeamSeq = seq
            beamProgress = 0
            withAnimation(.timingCurve(0.3, 0, 0.2, 1, duration: 0.68)) {
                beamProgress = 1.0
            }
        }
        .task(id: isPlaying) {
            guard isPlaying else { return }
            let total = allMessages.count
            guard total > 0 else {
                isPlaying = false
                return
            }
            if replayIndex == nil || (replayIndex ?? 0) >= total {
                replayIndex = 1
            }
            while isPlaying && !Task.isCancelled {
                let cur = replayIndex ?? total
                let delayMs = stepDelayMilliseconds(messages: allMessages, nextIndex: cur)
                try? await Task.sleep(nanoseconds: UInt64(delayMs * 1_000_000))
                if Task.isCancelled || !isPlaying { break }
                let latestAll = allMessages
                let nowPos = replayIndex ?? latestAll.count
                if nowPos < latestAll.count {
                    replayIndex = nowPos + 1
                } else {
                    isPlaying = false
                }
            }
        }
    }

    private func stepDelayMilliseconds(messages: [SwarmBoardMessage], nextIndex: Int) -> Double {
        let speed = max(0.25, replaySpeed)
        if replayPacing == .even || nextIndex <= 0 || nextIndex >= messages.count {
            return 1400.0 / speed
        }
        if let d1 = SwarmHelpers.parseDate(messages[nextIndex - 1].time),
           let d2 = SwarmHelpers.parseDate(messages[nextIndex].time) {
            let gapMs = max(0, d2.timeIntervalSince(d1) * 1000.0)
            // 1 minute wall time = 1 second at 1x, clamped 250..4000ms (matches app.js)
            return min(4000.0, max(250.0, gapMs / 60.0 / speed))
        }
        return 1100.0 / speed
    }

    private func formatSpeed(_ s: Double) -> String {
        s == floor(s) ? "\(Int(s))×" : String(format: "%.1f×", s)
    }

    // MARK: - Replay & Hive/Mentions Controls Bar

    private var replayControlsBar: some View {
        let total = allMessages.count
        let currentPos = replayIndex ?? total
        let isLive = replayIndex == nil

        return VStack(alignment: .leading, spacing: 10) {
            // Row 1: Step buttons + Scrubber + Live pill
            HStack(spacing: 8) {
                HStack(spacing: 2) {
                    Button(action: {
                        isPlaying = false
                        replayIndex = total > 0 ? 1 : 0
                    }) {
                        Image(systemName: "backward.end.fill")
                            .font(.system(size: 11.5))
                            .foregroundColor(total > 0 && !(!isLive && currentPos <= 1) ? SwarmTheme.ink : SwarmTheme.faint)
                            .frame(width: 26, height: 26)
                    }
                    .disabled(total == 0)

                    Button(action: {
                        isPlaying = false
                        let cur = replayIndex ?? total
                        replayIndex = max(1, cur - 1)
                    }) {
                        Image(systemName: "chevron.left")
                            .font(.system(size: 12, weight: .semibold))
                            .foregroundColor(currentPos > 1 ? SwarmTheme.ink : SwarmTheme.faint)
                            .frame(width: 24, height: 26)
                    }
                    .disabled(total == 0 || currentPos <= 1)

                    Button(action: {
                        if isPlaying {
                            isPlaying = false
                        } else {
                            if replayIndex == nil || (replayIndex ?? 0) >= total {
                                replayIndex = 1
                            }
                            isPlaying = true
                        }
                    }) {
                        Image(systemName: isPlaying ? "pause.fill" : "play.fill")
                            .font(.system(size: 12.5))
                            .foregroundColor(SwarmTheme.ink)
                            .frame(width: 28, height: 26)
                            .background(SwarmTheme.fieldBg)
                            .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
                    }
                    .disabled(total == 0)

                    Button(action: {
                        isPlaying = false
                        let cur = replayIndex ?? total
                        if cur + 1 >= total {
                            replayIndex = nil
                        } else {
                            replayIndex = cur + 1
                        }
                    }) {
                        Image(systemName: "chevron.right")
                            .font(.system(size: 12, weight: .semibold))
                            .foregroundColor(!isLive ? SwarmTheme.ink : SwarmTheme.faint)
                            .frame(width: 24, height: 26)
                    }
                    .disabled(isLive || total == 0)
                }

                Slider(
                    value: Binding(
                        get: { Double(replayIndex ?? total) },
                        set: { val in
                            isPlaying = false
                            let v = Int(val.rounded())
                            if v >= total {
                                replayIndex = nil
                            } else {
                                replayIndex = max(1, v)
                            }
                        }
                    ),
                    in: 0...Double(max(1, total)),
                    step: 1
                )
                .tint(SwarmTheme.nodeStroke)
                .disabled(total == 0)

                Button(action: {
                    isPlaying = false
                    replayIndex = nil
                }) {
                    HStack(spacing: 4) {
                        Circle()
                            .fill(isLive ? SwarmTheme.liveGreen : SwarmTheme.faint)
                            .frame(width: 6, height: 6)
                        Text("Live")
                            .font(.system(size: 11.5, weight: isLive ? .semibold : .regular))
                            .foregroundColor(isLive ? SwarmTheme.ink : SwarmTheme.muted)
                    }
                    .padding(.horizontal, 8)
                    .padding(.vertical, 4)
                    .background(isLive ? SwarmTheme.fieldBg : Color.clear)
                    .clipShape(Capsule())
                }
                .buttonStyle(.plain)
            }

            // Row 2: Post span summary + Hive/Mentions segmented control + Pacing & Speed menus
            HStack(spacing: 8) {
                if let cur = currentReplayMessage {
                    let clock = SwarmHelpers.formatClock(cur.time)
                    Text("\(currentPos) of \(total) · #\(cur.seq) · \(clock)")
                        .font(.system(size: 11.5, weight: .medium, design: .rounded))
                        .foregroundColor(SwarmTheme.muted)
                        .lineLimit(1)
                } else {
                    let span = SwarmHelpers.formatSpanDuration(from: allMessages.first?.time, to: allMessages.last?.time)
                    HStack(spacing: 3) {
                        Text("\(total) \(total == 1 ? "post" : "posts")")
                            .font(.system(size: 12, weight: .semibold))
                            .foregroundColor(SwarmTheme.ink)
                        if let span {
                            Text("over \(span)")
                                .font(.system(size: 12))
                                .foregroundColor(SwarmTheme.muted)
                        }
                    }
                    .lineLimit(1)
                }

                Spacer(minLength: 4)

                // Hive | Mentions Segmented Pill
                HStack(spacing: 2) {
                    ForEach(GraphMode.allCases, id: \.rawValue) { mode in
                        let selected = (graphMode == mode)
                        Button(action: {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            withAnimation(.easeInOut(duration: 0.16)) {
                                graphMode = mode
                            }
                        }) {
                            Text(mode.rawValue)
                                .font(.system(size: 11.5, weight: selected ? .semibold : .medium))
                                .foregroundColor(selected ? SwarmTheme.ink : SwarmTheme.muted)
                                .padding(.horizontal, 9)
                                .padding(.vertical, 4.5)
                                .background(
                                    RoundedRectangle(cornerRadius: 6, style: .continuous)
                                        .fill(selected ? SwarmTheme.surfaceBg : Color.clear)
                                        .shadow(color: selected ? Color.black.opacity(0.06) : .clear, radius: 1.5, y: 0.5)
                                )
                        }
                        .buttonStyle(.plain)
                    }
                }
                .padding(2)
                .background(SwarmTheme.fieldBg)
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))

                // Pacing Menu
                Menu {
                    ForEach(ReplayPacing.allCases, id: \.rawValue) { p in
                        Button(action: { replayPacing = p }) {
                            HStack {
                                Text(p.rawValue)
                                if replayPacing == p { Image(systemName: "checkmark") }
                            }
                        }
                    }
                } label: {
                    HStack(spacing: 3) {
                        Text(replayPacing.shortLabel)
                            .font(.system(size: 11.5, weight: .medium))
                        Image(systemName: "chevron.down")
                            .font(.system(size: 8.5, weight: .semibold))
                    }
                    .foregroundColor(SwarmTheme.muted)
                    .padding(.horizontal, 7)
                    .padding(.vertical, 5)
                    .background(SwarmTheme.fieldBg.opacity(0.75))
                    .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
                }

                // Speed Menu
                Menu {
                    ForEach(Self.speedOptions, id: \.self) { sp in
                        Button(action: { replaySpeed = sp }) {
                            HStack {
                                Text(formatSpeed(sp))
                                if replaySpeed == sp { Image(systemName: "checkmark") }
                            }
                        }
                    }
                } label: {
                    HStack(spacing: 3) {
                        Text(formatSpeed(replaySpeed))
                            .font(.system(size: 11.5, weight: .medium, design: .monospaced))
                        Image(systemName: "chevron.down")
                            .font(.system(size: 8.5, weight: .semibold))
                    }
                    .foregroundColor(SwarmTheme.muted)
                    .padding(.horizontal, 6.5)
                    .padding(.vertical, 5)
                    .background(SwarmTheme.fieldBg.opacity(0.75))
                    .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
                }
            }
        }
    }

    private var mentionsFilterRow: some View {
        HStack(spacing: 8) {
            toggleChip(title: String(localized: "隐藏未活跃"), isOn: $hideInactive)
            toggleChip(title: String(localized: "展开 @all"), isOn: $expandAll)
            Spacer()
        }
    }

    private func toggleChip(title: String, isOn: Binding<Bool>) -> some View {
        Button(action: { isOn.wrappedValue.toggle() }) {
            HStack(spacing: 4) {
                Circle()
                    .fill(isOn.wrappedValue ? SwarmTheme.accent : SwarmTheme.faint.opacity(0.6))
                    .frame(width: 6, height: 6)
                Text(title)
                    .font(.system(size: 11, weight: .medium))
                    .foregroundColor(isOn.wrappedValue ? SwarmTheme.ink : SwarmTheme.muted)
            }
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
            .background(isOn.wrappedValue ? SwarmTheme.cardBg : SwarmTheme.fieldBg.opacity(0.6))
            .clipShape(Capsule())
            .overlay(
                Capsule()
                    .stroke(isOn.wrappedValue ? SwarmTheme.accent.opacity(0.5) : SwarmTheme.border, lineWidth: 0.7)
            )
        }
        .buttonStyle(.plain)
    }

    // MARK: - Honeycomb Hive Map (Ported from swarm_board/hive.js)

    private struct HiveBeePlacement: Identifiable {
        var id: String { name }
        let name: String
        let x: CGFloat
        let y: CGFloat
        let spriteW: CGFloat
        let spriteH: CGFloat
        let padRadius: CGFloat
        let labelMaxW: CGFloat
        let showName: Bool
        let isQueen: Bool
    }

    private struct HiveCellPlacement: Identifiable {
        let id: String
        let cell: SwarmHiveCell? // nil == Coordinator hub
        let cx: CGFloat
        let cy: CGFloat
        let fractionDone: Double
        let isSelected: Bool
    }

    @ViewBuilder
    private func hiveHoneycombSection(availW: CGFloat, hiveModel: SwarmHiveMapModel, synth: SwarmSynthesizedModel) -> some View {
        let baseLayout = computeHiveGeometry(availW: availW, zoom: 1.0, hiveModel: hiveModel)
        let layout = computeHiveGeometry(availW: availW, zoom: hiveZoom, hiveModel: hiveModel)
        let needsInnerScroll = layout.canvasW > availW + 1.0 || hiveZoom > 1.01
        let viewportH: CGFloat = needsInnerScroll
            ? min(layout.canvasH, max(baseLayout.canvasH, 420))
            : layout.canvasH

        ZStack(alignment: .topTrailing) {
            if needsInnerScroll {
                ScrollView([.horizontal, .vertical], showsIndicators: false) {
                    hiveCanvasContent(layout: layout, hiveModel: hiveModel)
                }
            } else {
                hiveCanvasContent(layout: layout, hiveModel: hiveModel)
            }

            // Compact Zoom Pill (shown when zoomed in or when many cells shrink R)
            if layout.maxZoom > 1.15 && (hiveZoom > 1.01 || layout.baseRadius < 68) {
                HStack(spacing: 2) {
                    Button(action: {
                        withAnimation(.easeInOut(duration: 0.18)) {
                            hiveZoom = max(1.0, hiveZoom / 1.35)
                        }
                    }) {
                        Image(systemName: "minus")
                            .font(.system(size: 11, weight: .semibold))
                            .frame(width: 24, height: 24)
                    }
                    .disabled(hiveZoom <= 1.01)

                    Button(action: {
                        withAnimation(.easeInOut(duration: 0.18)) {
                            hiveZoom = min(layout.maxZoom, hiveZoom * 1.35)
                        }
                    }) {
                        Image(systemName: "plus")
                            .font(.system(size: 11, weight: .semibold))
                            .frame(width: 24, height: 24)
                    }
                    .disabled(hiveZoom >= layout.maxZoom - 0.02)

                    if hiveZoom > 1.01 {
                        Button(action: {
                            withAnimation(.easeInOut(duration: 0.18)) { hiveZoom = 1.0 }
                        }) {
                            Text("Fit")
                                .font(.system(size: 10.5, weight: .semibold))
                                .padding(.horizontal, 5)
                                .frame(height: 24)
                        }
                    }
                }
                .foregroundColor(SwarmTheme.ink)
                .padding(.horizontal, 4)
                .background(SwarmTheme.surfaceBg.opacity(0.92))
                .clipShape(Capsule())
                .overlay(Capsule().stroke(SwarmTheme.border, lineWidth: 0.8))
                .padding(6)
            }
        }
        .frame(width: availW, height: viewportH, alignment: .topLeading)
    }

    @ViewBuilder
    private func hiveCanvasContent(layout: ComputedHiveLayout, hiveModel: SwarmHiveMapModel) -> some View {
        ZStack(alignment: .topLeading) {
                        // 1. Hexagonal cells, green task-done borders, message beams & waiting arrows
                        Canvas { ctx, _ in
                            let r = layout.radius
                            for cp in layout.cells {
                                let hexPath = flatTopHexagonPath(center: CGPoint(x: cp.cx, y: cp.cy), radius: r)
                                let isDoneCell = cp.fractionDone >= 1.0 && (cp.cell?.total ?? hiveModel.allTotal) > 0
                                let fillCol: Color = {
                                    if cp.isSelected { return SwarmTheme.accent.opacity(0.08) }
                                    if isDoneCell { return SwarmTheme.liveGreen.opacity(0.08) }
                                    return SwarmTheme.surfaceBg
                                }()
                                ctx.fill(hexPath, with: .color(fillCol))
                                ctx.stroke(hexPath, with: .color(SwarmTheme.border), lineWidth: 1.0)

                                // Green task-done perimeter arc starting at top-left vertex (-120 deg)
                                if cp.fractionDone > 0 {
                                    let progHex = flatTopHexagonPath(center: CGPoint(x: cp.cx, y: cp.cy), radius: r - 2.0, startAngleDeg: -120)
                                    let trimmed = progHex.trimmedPath(from: 0, to: min(1.0, CGFloat(cp.fractionDone)))
                                    ctx.stroke(
                                        trimmed,
                                        with: .color(SwarmTheme.liveGreen),
                                        style: StrokeStyle(lineWidth: 2.4, lineCap: .round, lineJoin: .round)
                                    )
                                }

                                if cp.isSelected {
                                    let selRing = flatTopHexagonPath(center: CGPoint(x: cp.cx, y: cp.cy), radius: r + 1.5)
                                    ctx.stroke(selRing, with: .color(SwarmTheme.accent), lineWidth: 2.2)
                                }
                            }

                            // Waiting dashed links (from waiting bee -> awaited bee)
                            for wl in hiveModel.waitLinks {
                                guard let a = layout.beeMap[wl.from], let b = layout.beeMap[wl.to] else { continue }
                                if let curve = hiveCurveGeometry(from: a, to: b, cutEnd: 8) {
                                    var path = Path()
                                    path.move(to: curve.start)
                                    path.addQuadCurve(to: curve.end, control: curve.control)
                                    ctx.stroke(
                                        path,
                                        with: .color(SwarmTheme.warnAmber.opacity(0.7)),
                                        style: StrokeStyle(lineWidth: 1.6, lineCap: .round, dash: [4, 4], dashPhase: waitDashPhase)
                                    )
                                    let arrow = arrowheadPath(tip: curve.rawEnd, from: curve.control, size: 7.5)
                                    ctx.fill(arrow, with: .color(SwarmTheme.warnAmber.opacity(0.75)))
                                }
                            }

                            // Recent / Replay Message Beam (warm terracotta curve + flying dot)
                            if let fromName = hiveModel.beamFrom, let a = layout.beeMap[fromName] {
                                for targetName in hiveModel.beamTargets {
                                    guard let b = layout.beeMap[targetName],
                                          let curve = hiveCurveGeometry(from: a, to: b, cutEnd: 0) else { continue }
                                    var path = Path()
                                    path.move(to: curve.start)
                                    path.addQuadCurve(to: curve.end, control: curve.control)
                                    ctx.stroke(
                                        path,
                                        with: .color(SwarmTheme.accent.opacity(0.62)),
                                        style: StrokeStyle(lineWidth: 1.6, lineCap: .round)
                                    )
                                    let pt = quadPoint(start: curve.start, control: curve.control, end: curve.end, t: beamProgress)
                                    let dotRect = CGRect(x: pt.x - 3.0, y: pt.y - 3.0, width: 6.0, height: 6.0)
                                    ctx.fill(Path(ellipseIn: dotRect), with: .color(SwarmTheme.accent))
                                }
                            }
                        }

                        // 2. Cell Tappable Hit Areas + Cell Titles & Counts
                        ForEach(layout.cells) { cp in
                            let r = layout.radius
                            let k = CGFloat(sqrt(3.0) / 2.0)
                            if let cell = cp.cell {
                                // Thread Hexagon Title + Done/Total Count + Tap Handler
                                ZStack(alignment: .top) {
                                    flatTopHexagonPath(center: CGPoint(x: r, y: k * r), radius: r)
                                        .fill(Color.white.opacity(0.001))
                                        .onTapGesture {
                                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                            withAnimation(.easeInOut(duration: 0.18)) {
                                                selectedCellId = (selectedCellId == cell.id) ? nil : cell.id
                                            }
                                        }

                                    if r >= 38 {
                                        VStack(spacing: 1.5) {
                                            Text(cell.title)
                                                .font(.system(size: r >= 80 ? 12 : 10.5, weight: .semibold))
                                                .foregroundColor(SwarmTheme.ink)
                                                .multilineTextAlignment(.center)
                                                .lineLimit(r >= 56 ? 2 : 1)
                                                .frame(maxWidth: r * 1.24)

                                            Text(cell.countOnly.map { "\($0)" } ?? "\(cell.done)/\(cell.total)")
                                                .font(.system(size: r >= 80 ? 10.5 : 9.5, design: .rounded))
                                                .foregroundColor(SwarmTheme.muted)
                                        }
                                        .padding(.top, r >= 80 ? 12 : 9)
                                        .allowsHitTesting(false)
                                    }
                                }
                                .frame(width: r * 2, height: k * r * 2)
                                .position(x: cp.cx, y: cp.cy)
                            } else {
                                // Coordinator Hub Labels
                                VStack(spacing: 1.5) {
                                    if r >= 52 {
                                        Text("Coordinator")
                                            .font(.system(size: r >= 80 ? 12.5 : 11, weight: .semibold))
                                            .foregroundColor(SwarmTheme.ink)
                                    }
                                    Text(hiveModel.coordinatorNeedsYou
                                         ? String(localized: "Needs you")
                                         : (r >= 52 ? "\(hiveModel.allDone) of \(hiveModel.allTotal) done" : "\(hiveModel.allDone)/\(hiveModel.allTotal)"))
                                        .font(.system(size: r >= 80 ? 11 : 9.5, weight: hiveModel.coordinatorNeedsYou ? .semibold : .regular, design: .rounded))
                                        .foregroundColor(hiveModel.coordinatorNeedsYou ? SwarmTheme.warnAmber : SwarmTheme.muted)
                                }
                                .position(x: cp.cx, y: cp.cy + r * (r >= 52 ? 0.48 : 0.56))
                                .allowsHitTesting(false)
                            }
                        }

                        // 3. Animated Pixel Bees inside their Honeycomb Cells
                        ForEach(layout.bees) { bp in
                            let st = hiveModel.beeStates[bp.name] ?? .idle
                            if st != .unborn {
                                let pose: PixelBeePose = (st == .working) ? .working : .landed
                                let dotCol: Color? = {
                                    switch st {
                                    case .wait, .needs: return SwarmTheme.warnAmber
                                    case .issue: return SwarmTheme.errorRed
                                    case .done: return SwarmTheme.liveGreen
                                    default: return nil
                                    }
                                }()

                                Button(action: {
                                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                    if let convId = snapshot.members.first(where: { $0.name == bp.name || (bp.isQueen && $0.role == "coordinator") })?.conversationId,
                                       !convId.isEmpty, onSelectConversation != nil {
                                        onDismissSheet()
                                        onSelectConversation?(convId)
                                    }
                                }) {
                                    VStack(spacing: 1.5) {
                                        SwarmPixelBeeSpriteView(
                                            name: bp.name,
                                            pose: pose,
                                            width: bp.spriteW,
                                            height: bp.spriteH,
                                            statusDotColor: dotCol,
                                            dotBorderColor: SwarmTheme.surfaceBg,
                                            isTilted: st == .wait,
                                            isRetired: st == .retired,
                                            bobbing: st == .working
                                        )

                                        if bp.showName && !bp.isQueen {
                                            Text(bp.name)
                                                .font(.system(size: layout.radius >= 80 ? 9.5 : 8.5))
                                                .foregroundColor(SwarmTheme.muted)
                                                .lineLimit(1)
                                                .truncationMode(.tail)
                                                .frame(maxWidth: bp.labelMaxW)
                                        }
                                    }
                                }
                                .buttonStyle(.plain)
                                .position(x: bp.x, y: bp.y + (bp.showName && !bp.isQueen ? 6 : 0))
                                .animation(.spring(response: 0.45, dampingFraction: 0.82), value: bp.x)
                                .animation(.spring(response: 0.45, dampingFraction: 0.82), value: bp.y)
                            }
                        }
        }
        .frame(width: layout.canvasW, height: layout.canvasH)
    }

    private struct ComputedHiveLayout {
        let baseRadius: CGFloat
        let radius: CGFloat
        let maxZoom: CGFloat
        let canvasW: CGFloat
        let canvasH: CGFloat
        let cells: [HiveCellPlacement]
        let bees: [HiveBeePlacement]
        let beeMap: [String: HiveBeePlacement]
    }

    private func computeHiveGeometry(availW: CGFloat, zoom: CGFloat, hiveModel: SwarmHiveMapModel) -> ComputedHiveLayout {
        let k = CGFloat(sqrt(3.0) / 2.0)
        func ux(_ q: Int) -> CGFloat { 1.5 * CGFloat(q) }
        func uy(_ q: Int, _ r: Int) -> CGFloat { 2.0 * k * (CGFloat(r) + CGFloat(q) / 2.0) }

        let xs: [CGFloat] = [0] + hiveModel.cells.map { ux($0.q) }
        let ys: [CGFloat] = [0] + hiveModel.cells.map { uy($0.q, $0.r) }
        let minX = xs.min() ?? 0, maxX = xs.max() ?? 0
        let minY = ys.min() ?? 0, maxY = ys.max() ?? 0
        let spanX = max(2.0, (maxX - minX) + 2.0)

        let padX: CGFloat = 8.0
        let padY: CGFloat = 10.0

        let fit = (availW - padX * 2.0) / spanX
        let gap: CGFloat = fit < 44 ? 3.5 : 5.5
        let rFit = max(18.0, min(104.0, fit - gap / 2.0))
        let maxZ = max(1.0, 104.0 / rFit)
        let clampedZoom = max(1.0, min(maxZ, zoom))
        let r = min(104.0, rFit * clampedZoom)
        let stepG = r + gap / 2.0

        let contentW = (maxX - minX) * stepG + 2.0 * r
        let contentH = (maxY - minY) * stepG + 2.0 * k * r
        let canvasW = max(availW, ceil(contentW + padX * 2.0))
        let canvasH = ceil(contentH + padY * 2.0)
        let ox = canvasW / 2.0 - ((maxX + minX) / 2.0) * stepG
        let oy = padY + k * r - minY * stepG

        var cellPlacements: [HiveCellPlacement] = []
        var beePlacements: [HiveBeePlacement] = []

        // Coordinator Hub at (ox, oy)
        let hubFraction = hiveModel.allTotal > 0 ? Double(hiveModel.allDone) / Double(hiveModel.allTotal) : 0
        cellPlacements.append(HiveCellPlacement(
            id: "_coordinator_hub",
            cell: nil,
            cx: ox,
            cy: oy,
            fractionDone: hubFraction,
            isSelected: false
        ))

        let queenW: CGFloat = r >= 82 ? 56 : (r >= 52 ? 42 : 26)
        let queenH: CGFloat = queenW * 18.0 / 22.0
        beePlacements.append(HiveBeePlacement(
            name: "coordinator",
            x: ox,
            y: oy - (r >= 52 ? r * 0.14 : 4),
            spriteW: queenW,
            spriteH: queenH,
            padRadius: queenW * 0.56,
            labelMaxW: queenW,
            showName: false,
            isQueen: true
        ))

        let showNames = r >= 56
        let workerW: CGFloat = r >= 82 ? 38 : (r >= 56 ? 28 : 20)
        let workerH: CGFloat = workerW * 18.0 / 22.0

        for cell in hiveModel.cells {
            let cx = ox + ux(cell.q) * stepG
            let cy = oy + uy(cell.q, cell.r) * stepG
            let frac = cell.countOnly != nil ? (cell.done > 0 ? 1.0 : 0.0) : (cell.total > 0 ? Double(cell.done) / Double(cell.total) : 0.0)
            cellPlacements.append(HiveCellPlacement(
                id: cell.id,
                cell: cell,
                cx: cx,
                cy: cy,
                fractionDone: frac,
                isSelected: selectedCellId == cell.id
            ))

            let bornBees = cell.bees.filter { (hiveModel.beeStates[$0] ?? .idle) != .unborn }
                .sorted { a, b in
                    func rank(_ s: SwarmHiveBeeState?) -> Int {
                        switch s {
                        case .issue: return 0
                        case .wait: return 1
                        case .working: return 2
                        default: return 3
                        }
                    }
                    let ra = rank(hiveModel.beeStates[a]), rb = rank(hiveModel.beeStates[b])
                    return ra != rb ? ra < rb : a < b
                }

            guard !bornBees.isEmpty else { continue }

            let perRow = r >= 56 ? min(3, max(2, Int(floor(1.7 * r / 38.0)))) : 2
            let maxShown = perRow * 2
            let shown = Array(bornBees.prefix(maxShown))
            let rows: [[String]] = shown.count > perRow
                ? [Array(shown.prefix(perRow)), Array(shown.dropFirst(perRow))]
                : [shown]

            let rowGap: CGFloat = showNames ? (r >= 82 ? 44 : 33) : 24
            let topLimit = cy - k * r + (r >= 82 ? 46 : 36)
            let botLimit = cy + k * r - (showNames ? 16 : 8)
            let baseY = (topLimit + botLimit) / 2.0 - CGFloat(rows.count - 1) * rowGap / 2.0

            for (ri, row) in rows.enumerated() {
                let by = baseY + CGFloat(ri) * rowGap
                let dyFromCenter = abs((by + (r >= 82 ? 14.0 : 12.0)) - cy)
                let innerR = max(12.0, r - 4.0)
                let hexHalfW = max(workerW * 0.6, innerR * (1.0 - 0.5 * min(1.0, dyFromCenter / max(1.0, k * innerR))))
                let availRowW = hexHalfW * 2.0

                let rowHorizStep: CGFloat
                let labelMaxW: CGFloat
                if row.count <= 1 {
                    rowHorizStep = 0
                    labelMaxW = min(r * 1.15, max(workerW + 8.0, availRowW - 6.0))
                } else {
                    let maxStepByHex = (availRowW - 4.0) / CGFloat(row.count)
                    rowHorizStep = min(r >= 82 ? 62.0 : 52.0, max(workerW + 2.0, maxStepByHex))
                    labelMaxW = max(workerW, rowHorizStep - 4.0)
                }

                for (ci, name) in row.enumerated() {
                    let bx = cx + (CGFloat(ci) - CGFloat(row.count - 1) / 2.0) * rowHorizStep
                    beePlacements.append(HiveBeePlacement(
                        name: name,
                        x: bx,
                        y: by,
                        spriteW: workerW,
                        spriteH: workerH,
                        padRadius: workerW * 0.56,
                        labelMaxW: labelMaxW,
                        showName: showNames,
                        isQueen: false
                    ))
                }
            }
        }

        let beeMap = Dictionary(uniqueKeysWithValues: beePlacements.map { ($0.name, $0) })
        return ComputedHiveLayout(
            baseRadius: rFit,
            radius: r,
            maxZoom: maxZ,
            canvasW: canvasW,
            canvasH: canvasH,
            cells: cellPlacements,
            bees: beePlacements,
            beeMap: beeMap
        )
    }

    private func flatTopHexagonPath(center: CGPoint, radius: CGFloat, startAngleDeg: Double = 0) -> Path {
        var path = Path()
        for k in 0..<6 {
            let angle = (startAngleDeg + Double(k) * 60.0) * .pi / 180.0
            let pt = CGPoint(
                x: center.x + radius * CGFloat(cos(angle)),
                y: center.y + radius * CGFloat(sin(angle))
            )
            if k == 0 {
                path.move(to: pt)
            } else {
                path.addLine(to: pt)
            }
        }
        path.closeSubpath()
        return path
    }

    private func hiveCurveGeometry(
        from a: HiveBeePlacement,
        to b: HiveBeePlacement,
        cutEnd: CGFloat
    ) -> (start: CGPoint, control: CGPoint, end: CGPoint, rawEnd: CGPoint)? {
        let dx = b.x - a.x
        let dy = b.y - a.y
        let len = hypot(dx, dy)
        guard len >= a.padRadius + b.padRadius + 6 else { return nil }
        let vx = dx / len
        let vy = dy / len
        let x1 = a.x + vx * a.padRadius
        let y1 = a.y + vy * a.padRadius
        let x2 = b.x - vx * b.padRadius
        let y2 = b.y - vy * b.padRadius
        let bend = min(24.0, (len - a.padRadius - b.padRadius) * 0.2)
        let mx = (x1 + x2) / 2.0 - vy * bend
        let my = (y1 + y2) / 2.0 + vx * bend
        let hl = max(1.0, hypot(x2 - mx, y2 - my))
        let ex = x2 - ((x2 - mx) / hl) * cutEnd
        let ey = y2 - ((y2 - my) / hl) * cutEnd
        return (
            start: CGPoint(x: x1, y: y1),
            control: CGPoint(x: mx, y: my),
            end: CGPoint(x: ex, y: ey),
            rawEnd: CGPoint(x: x2, y: y2)
        )
    }

    private func quadPoint(start: CGPoint, control: CGPoint, end: CGPoint, t: CGFloat) -> CGPoint {
        let u = 1.0 - t
        let x = u * u * start.x + 2.0 * u * t * control.x + t * t * end.x
        let y = u * u * start.y + 2.0 * u * t * control.y + t * t * end.y
        return CGPoint(x: x, y: y)
    }

    private var hiveLegendRow: some View {
        HStack(spacing: 14) {
            HStack(spacing: 6) {
                Capsule()
                    .fill(SwarmTheme.liveGreen)
                    .frame(width: 16, height: 2)
                Text("Tasks done")
            }
            HStack(spacing: 6) {
                Capsule()
                    .fill(SwarmTheme.accent.opacity(0.75))
                    .frame(width: 16, height: 2)
                Text("Message just sent")
            }
            HStack(spacing: 5) {
                HStack(spacing: 2) {
                    Rectangle().fill(SwarmTheme.warnAmber).frame(width: 4, height: 2)
                    Rectangle().fill(SwarmTheme.warnAmber).frame(width: 4, height: 2)
                    Image(systemName: "arrowtriangle.right.fill")
                        .font(.system(size: 6.5))
                        .foregroundColor(SwarmTheme.warnAmber)
                }
                Text("Waiting on")
            }
        }
        .font(.system(size: 11.5))
        .foregroundColor(SwarmTheme.muted)
    }

    private func selectedHiveCellCard(cell: SwarmHiveCell, hiveModel: SwarmHiveMapModel, synth: SwarmSynthesizedModel) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .top, spacing: 8) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(cell.title)
                        .font(.system(size: 15, weight: .semibold))
                        .foregroundColor(SwarmTheme.ink)
                    if !cell.goal.isEmpty {
                        Text(cell.goal)
                            .font(.system(size: 12.5))
                            .foregroundColor(SwarmTheme.muted)
                    }
                }
                Spacer()
                Text(cell.countOnly == nil && cell.total > 0 ? "\(cell.done) of \(cell.total) done" : "\(cell.bees.count) \(cell.bees.count == 1 ? "bee" : "bees")")
                    .font(.system(size: 11.5, design: .monospaced))
                    .foregroundColor(SwarmTheme.muted)
                    .padding(.top, 2)

                Button(action: {
                    withAnimation(.easeInOut(duration: 0.16)) { selectedCellId = nil }
                }) {
                    Image(systemName: "xmark")
                        .font(.system(size: 11.5, weight: .semibold))
                        .foregroundColor(SwarmTheme.muted)
                        .frame(width: 24, height: 24)
                        .background(SwarmTheme.fieldBg)
                        .clipShape(Circle())
                }
                .buttonStyle(.plain)
            }

            if cell.bees.isEmpty {
                Text("No one is on this thread yet.")
                    .font(.system(size: 12.5))
                    .foregroundColor(SwarmTheme.muted)
            } else {
                VStack(spacing: 6) {
                    ForEach(cell.bees, id: \.self) { name in
                        let sm = synth.members[name]
                        let st = hiveModel.beeStates[name] ?? .idle
                        let statusText: String = {
                            switch st {
                            case .working: return "Working"
                            case .wait:
                                if let w = hiveModel.beeWaitingOn[name] { return "Waiting on @\(w)" }
                                return "Waiting"
                            case .issue: return "Stuck"
                            case .done: return "Done"
                            case .retired: return "Retired"
                            case .unborn: return "Not started"
                            default: return "Resting"
                            }
                        }()
                        let statusColor: Color = {
                            switch st {
                            case .working: return SwarmTheme.accent
                            case .wait, .needs: return SwarmTheme.warnAmber
                            case .issue: return SwarmTheme.errorRed
                            case .done: return SwarmTheme.liveGreen
                            default: return SwarmTheme.muted
                            }
                        }()

                        Button(action: {
                            if let convId = snapshot.members.first(where: { $0.name == name })?.conversationId,
                               !convId.isEmpty, onSelectConversation != nil {
                                onDismissSheet()
                                onSelectConversation?(convId)
                            }
                        }) {
                            HStack(spacing: 10) {
                                SwarmPixelBeeView(
                                    name: name,
                                    pose: st == .working ? .working : .landed,
                                    size: 36
                                )

                                VStack(alignment: .leading, spacing: 2) {
                                    HStack(spacing: 6) {
                                        Text("@\(name)")
                                            .font(.system(size: 13, weight: .semibold))
                                            .foregroundColor(SwarmTheme.ink)
                                        if let title = sm?.title, !title.isEmpty {
                                            Text(title)
                                                .font(.system(size: 11.5))
                                                .foregroundColor(SwarmTheme.muted)
                                                .lineLimit(1)
                                        }
                                    }
                                    if let now = sm?.now, !now.isEmpty, st != .unborn {
                                        Text(now)
                                            .font(.system(size: 11.5))
                                            .foregroundColor(SwarmTheme.faint)
                                            .lineLimit(1)
                                    }
                                }

                                Spacer(minLength: 4)

                                Text(statusText)
                                    .font(.system(size: 11.5, weight: .medium))
                                    .foregroundColor(statusColor)
                            }
                            .padding(.vertical, 4)
                            .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
        }
        .padding(14)
        .background(SwarmTheme.surfaceBg)
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(SwarmTheme.border, lineWidth: 0.8))
    }

    // MARK: - Mentions Graph Canvas & Pixel-Bee Pill Nodes

    private struct GraphEdgeKey: Hashable {
        let from: String
        let to: String
    }

    private struct GraphEdgeStat {
        var direct: Int = 0
        var viaAll: Int = 0
        var total: Int { direct + viaAll }
    }

    private struct NodePos {
        let id: String
        let x: CGFloat
        let y: CGFloat
        let w: CGFloat
        let h: CGFloat
        let member: SwarmMemberInfo?
    }

    private var graphCanvasCard: some View {
        let msgs = visibleMessages
        let members = taskMembers
        let memberSet = Set(members.map(\.name))

        var activeSet: Set<String> = ["coordinator"]
        var edges: [GraphEdgeKey: GraphEdgeStat] = [:]
        var broadcasts: [String: Int] = [:]
        var unaddressed: [String: Int] = [:]

        let allNamesForBroadcast = members.filter { !$0.killed }.map(\.name)

        for m in msgs {
            if SwarmHelpers.isRoleWorker(name: m.agent) { continue }
            activeSet.insert(m.agent)

            let rawTargets = SwarmHelpers.targets(of: m)
            let isAll = SwarmHelpers.isBroadcast(m)
            if isAll {
                broadcasts[m.agent, default: 0] += 1
            }

            let directTargets = rawTargets.filter { $0.caseInsensitiveCompare("all") != .orderedSame && !SwarmHelpers.isRoleWorker(name: $0) }
            if directTargets.isEmpty && !isAll {
                unaddressed[m.agent, default: 0] += 1
            }

            for t in directTargets {
                activeSet.insert(t)
                let key = GraphEdgeKey(from: m.agent, to: t)
                edges[key, default: GraphEdgeStat()].direct += 1
            }

            if isAll && expandAll {
                for t in allNamesForBroadcast where t != m.agent && !directTargets.contains(t) {
                    if !hideInactive { activeSet.insert(t) }
                    let key = GraphEdgeKey(from: m.agent, to: t)
                    edges[key, default: GraphEdgeStat()].viaAll += 1
                }
            }
        }

        let outsiders = Array(Set(allMessages.map(\.agent)).filter { !memberSet.contains($0) && !SwarmHelpers.isRoleWorker(name: $0) }).sorted()
        var allNodeIds = members.map(\.name) + outsiders

        if hideInactive {
            var curDirect = Set<String>()
            if let cur = currentReplayMessage, !SwarmHelpers.isRoleWorker(name: cur.agent) {
                curDirect.insert(cur.agent)
                for t in SwarmHelpers.targets(of: cur) where t.caseInsensitiveCompare("all") != .orderedSame {
                    curDirect.insert(t)
                }
            }
            allNodeIds = allNodeIds.filter { id in
                let isKilled = memberByName[id]?.killed == true
                return curDirect.contains(id) || (activeSet.contains(id) && !isKilled)
            }
        }

        let currentTargets: Set<String> = {
            guard let cur = currentReplayMessage else { return [] }
            var set = Set(SwarmHelpers.targets(of: cur).filter { $0.caseInsensitiveCompare("all") != .orderedSame })
            if expandAll && SwarmHelpers.isBroadcast(cur) {
                for name in allNamesForBroadcast where name != cur.agent {
                    set.insert(name)
                }
            }
            return set
        }()

        let canvasHeight: CGFloat = allNodeIds.count <= 2 ? 190 : (allNodeIds.count <= 10 ? 290 : 340)

        return GeometryReader { geo in
            let width = max(geo.size.width, 300)
            let positions = computeLayout(nodeIds: allNodeIds, width: width, height: canvasHeight)
            let posMap = Dictionary(uniqueKeysWithValues: positions.map { ($0.id, $0) })

            let maxEdgeTotal = max(1, edges.compactMap { k, stat in
                (posMap[k.from] != nil && posMap[k.to] != nil) ? stat.total : nil
            }.max() ?? 1)

            ZStack {
                Canvas { ctx, _ in
                    for (key, stat) in edges {
                        guard let a = posMap[key.from], let b = posMap[key.to] else { continue }
                        let geom = edgeGeometry(from: a, to: b)
                        let isCurEdge = currentReplayMessage.map { $0.agent == key.from && currentTargets.contains(key.to) } ?? false
                        let isSelectedEdge = selectedNodeId.map { $0 == key.from || $0 == key.to } ?? false
                        let isDimmed = (replayIndex != nil && !isCurEdge) || (replayIndex == nil && selectedNodeId != nil && !isSelectedEdge)

                        let strokeWidth: CGFloat = (isCurEdge || isSelectedEdge)
                            ? max(2.2, 1.2 + 2.8 * CGFloat(stat.total) / CGFloat(maxEdgeTotal))
                            : (1.0 + 2.4 * CGFloat(stat.total) / CGFloat(maxEdgeTotal))

                        let edgeColor: Color = {
                            if isCurEdge { return SwarmTheme.ink }
                            if isSelectedEdge { return SwarmTheme.accent }
                            if isDimmed { return SwarmTheme.faint.opacity(0.28) }
                            return SwarmTheme.muted.opacity(0.45)
                        }()

                        var path = Path()
                        path.move(to: geom.start)
                        path.addQuadCurve(to: geom.end, control: geom.control)

                        let style = stat.direct > 0
                            ? StrokeStyle(lineWidth: strokeWidth, lineCap: .round)
                            : StrokeStyle(lineWidth: strokeWidth, lineCap: .round, dash: [4, 3])
                        ctx.stroke(path, with: .color(edgeColor), style: style)

                        let arrowPath = arrowheadPath(tip: geom.end, from: geom.control, size: isCurEdge ? 7.5 : 6.0)
                        ctx.fill(arrowPath, with: .color(edgeColor))

                        if isCurEdge {
                            let dotRect = CGRect(x: geom.end.x - 4, y: geom.end.y - 4, width: 8, height: 8)
                            ctx.fill(Path(ellipseIn: dotRect), with: .color(SwarmTheme.liveGreen))
                            ctx.stroke(Path(ellipseIn: dotRect), with: .color(SwarmTheme.bg), lineWidth: 1.2)
                        }

                        if stat.total > 1 {
                            let midPt = quadMidpoint(start: geom.start, control: geom.control, end: geom.end)
                            let text = Text("\(stat.total)")
                                .font(.system(size: 10, weight: .semibold, design: .monospaced))
                                .foregroundColor(isCurEdge ? SwarmTheme.ink : (isDimmed ? SwarmTheme.faint.opacity(0.5) : SwarmTheme.muted))
                            ctx.draw(text, at: CGPoint(x: midPt.x, y: midPt.y - 5), anchor: .center)
                        }
                    }
                }

                // Tappable node pills with Pixel Bee sprites
                ForEach(positions, id: \.id) { node in
                    let member = node.member
                    let status = member?.status ?? ""
                    let isHot = currentReplayMessage.map { $0.agent == node.id || currentTargets.contains(node.id) } ?? false
                    let isSelected = selectedNodeId == node.id
                    let isActiveNode = replayIndex == nil || activeSet.contains(node.id)
                    let bcastCount = expandAll ? 0 : (broadcasts[node.id] ?? 0)
                    let hasUnaddressed = (unaddressed[node.id] ?? 0) > 0
                    let dotCol: Color? = {
                        switch status.lowercased() {
                        case "working": return SwarmTheme.liveGreen
                        case "error", "stuck": return SwarmTheme.errorRed
                        default: return nil
                        }
                    }()

                    Button(action: {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        selectedNodeId = (selectedNodeId == node.id) ? nil : node.id
                    }) {
                        ZStack(alignment: .topTrailing) {
                            HStack(spacing: 4) {
                                SwarmPixelBeeSpriteView(
                                    name: node.id,
                                    pose: status.lowercased() == "working" ? .working : .landed,
                                    width: 20,
                                    height: 16.5,
                                    statusDotColor: dotCol,
                                    dotBorderColor: isHot || isSelected ? SwarmTheme.surfaceBg : SwarmTheme.cardBg
                                )
                                Text(node.id)
                                    .font(.system(size: 11.5, weight: (isHot || isSelected || node.id == "coordinator") ? .semibold : .medium))
                                    .foregroundColor(isActiveNode ? SwarmTheme.ink : SwarmTheme.faint)
                                    .lineLimit(1)
                            }
                            .padding(.horizontal, 8)
                            .frame(width: node.w, height: node.h)
                            .background(
                                Capsule()
                                    .fill(isHot || isSelected ? SwarmTheme.surfaceBg : SwarmTheme.cardBg)
                            )
                            .overlay(
                                Capsule()
                                    .stroke(
                                        isSelected ? SwarmTheme.accent : (isHot ? SwarmTheme.ink : (isActiveNode ? SwarmTheme.nodeStroke.opacity(0.65) : SwarmTheme.border)),
                                        lineWidth: (isSelected || isHot) ? 1.6 : 0.9
                                    )
                            )
                            .overlay(
                                Group {
                                    if hasUnaddressed {
                                        Capsule()
                                            .stroke(SwarmTheme.border, style: StrokeStyle(lineWidth: 0.9, dash: [3, 2]))
                                            .padding(-3.5)
                                    }
                                }
                            )
                            .opacity(isActiveNode ? 1.0 : 0.45)

                            if bcastCount > 0 {
                                Text("@all \(bcastCount)")
                                    .font(.system(size: 8.5, weight: .semibold, design: .monospaced))
                                    .foregroundColor(SwarmTheme.ink)
                                    .padding(.horizontal, 5)
                                    .padding(.vertical, 1.5)
                                    .background(SwarmTheme.codeBg)
                                    .clipShape(Capsule())
                                    .overlay(Capsule().stroke(SwarmTheme.border, lineWidth: 0.6))
                                    .offset(x: 4, y: -7)
                            }
                        }
                    }
                    .buttonStyle(.plain)
                    .position(x: node.x, y: node.y)
                }
            }
        }
        .frame(height: canvasHeight)
        .background(SwarmTheme.bg)
    }

    // MARK: - Mentions Geometry Helpers

    private func pillWidth(for id: String, maxW: CGFloat) -> CGFloat {
        let est = 42.0 + ceil(CGFloat(id.count) * 6.5)
        return min(maxW, max(82, est))
    }

    private func computeLayout(nodeIds: [String], width: CGFloat, height: CGFloat) -> [NodePos] {
        let n = nodeIds.count
        guard n > 0 else { return [] }
        let nodeH: CGFloat = 29

        if n <= 16 {
            let maxPillW = min(140, max(104, width / max(3.2, CGFloat(ceil(Double(n) / 2.0)) + 0.6) - 8))
            let rx = max(56, width / 2 - maxPillW / 2 - 10)
            let ry = max(54, height / 2 - nodeH / 2 - 18)
            let cx = width / 2
            let cy = height / 2

            return nodeIds.enumerated().map { idx, id in
                let angle = -Double.pi / 2.0 + (2.0 * Double.pi * Double(idx)) / Double(n)
                return NodePos(
                    id: id,
                    x: cx + rx * CGFloat(cos(angle)),
                    y: cy + ry * CGFloat(sin(angle)),
                    w: pillWidth(for: id, maxW: maxPillW),
                    h: nodeH,
                    member: memberByName[id]
                )
            }
        }

        let perRow = max(2, Int(floor(width / 112)))
        var result: [NodePos] = []
        let rows = Int(ceil(Double(n) / Double(perRow)))
        let rowStep = max(44, (height - 40) / CGFloat(max(1, rows)))
        for (idx, id) in nodeIds.enumerated() {
            let row = idx / perRow
            let col = idx % perRow
            let countInRow = min(perRow, n - row * perRow)
            let gap = width / CGFloat(countInRow + 1)
            result.append(NodePos(
                id: id,
                x: gap * CGFloat(col + 1),
                y: 26 + CGFloat(row) * rowStep,
                w: pillWidth(for: id, maxW: max(86, gap - 8)),
                h: nodeH,
                member: memberByName[id]
            ))
        }
        return result
    }

    private func edgeGeometry(from a: NodePos, to b: NodePos) -> (start: CGPoint, control: CGPoint, end: CGPoint) {
        let dx = b.x - a.x
        let dy = b.y - a.y
        let len = max(1, hypot(dx, dy))
        let nx = -dy / len
        let ny = dx / len
        let bend: CGFloat = 24
        let mx = (a.x + b.x) / 2 + nx * bend
        let my = (a.y + b.y) / 2 + ny * bend
        let ctrl = CGPoint(x: mx, y: my)

        func trim(node: NodePos, toward target: CGPoint) -> CGPoint {
            let tx = target.x - node.x
            let ty = target.y - node.y
            let l = max(1, hypot(tx, ty))
            let ux = tx / l
            let uy = ty / l
            let bx = abs(ux) > 0.001 ? (node.w / 2) / abs(ux) : .infinity
            let by = abs(uy) > 0.001 ? (node.h / 2) / abs(uy) : .infinity
            let t = min(0.45 * l, min(bx, by) + 4)
            return CGPoint(x: node.x + ux * t, y: node.y + uy * t)
        }

        return (
            start: trim(node: a, toward: ctrl),
            control: ctrl,
            end: trim(node: b, toward: ctrl)
        )
    }

    private func quadMidpoint(start: CGPoint, control: CGPoint, end: CGPoint) -> CGPoint {
        quadPoint(start: start, control: control, end: end, t: 0.5)
    }

    private func arrowheadPath(tip: CGPoint, from ctrl: CGPoint, size: CGFloat) -> Path {
        let angle = atan2(tip.y - ctrl.y, tip.x - ctrl.x)
        let spread: CGFloat = .pi / 6.5
        let p1 = CGPoint(x: tip.x - size * cos(angle - spread), y: tip.y - size * sin(angle - spread))
        let p2 = CGPoint(x: tip.x - size * cos(angle + spread), y: tip.y - size * sin(angle + spread))
        var path = Path()
        path.move(to: tip)
        path.addLine(to: p1)
        path.addLine(to: p2)
        path.closeSubpath()
        return path
    }

    // MARK: - Mentions Legend & Selected Message Detail Card

    private var mentionsLegendRow: some View {
        HStack(spacing: 12) {
            HStack(spacing: 4) {
                Text("→")
                    .font(.system(size: 11, weight: .bold))
                Text("@提及 · 线宽 = 次数")
            }
            HStack(spacing: 4) {
                Text("⇢")
                    .font(.system(size: 11, weight: .bold))
                Text("经 @all")
            }
            HStack(spacing: 4) {
                Text("@all")
                    .font(.system(size: 9.5, weight: .semibold, design: .monospaced))
                    .padding(.horizontal, 4)
                    .padding(.vertical, 1)
                    .background(SwarmTheme.codeBg)
                    .clipShape(Capsule())
                Text("广播")
            }
        }
        .font(.system(size: 11))
        .foregroundColor(SwarmTheme.muted)
    }

    private func detailCard(_ msg: SwarmBoardMessage) -> some View {
        let targets = SwarmHelpers.targets(of: msg)
        let cleanBody = targets.isEmpty ? msg.text : SwarmHelpers.stripLeadingMentions(msg.text)
        let selectedMember = selectedNodeId.flatMap { memberByName[$0] }

        return VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 6) {
                Text(msg.agent)
                    .font(.system(size: 13, weight: .bold))
                    .foregroundColor(SwarmTheme.ink)

                if !targets.isEmpty {
                    Text("→")
                        .font(.system(size: 12))
                        .foregroundColor(SwarmTheme.faint)
                    Text(targets.map { "@\($0)" }.joined(separator: " "))
                        .font(.system(size: 12.5, weight: .medium))
                        .foregroundColor(SwarmTheme.muted)
                        .lineLimit(1)
                }

                Spacer()

                if let sel = selectedMember, !sel.conversationId.isEmpty, !sel.isCurrent, onSelectConversation != nil {
                    Button(action: {
                        onDismissSheet()
                        onSelectConversation?(sel.conversationId)
                    }) {
                        HStack(spacing: 3) {
                            Text("进入子会话")
                            Image(systemName: "arrow.up.right")
                                .font(.system(size: 9.5, weight: .semibold))
                        }
                        .font(.system(size: 11, weight: .semibold))
                        .foregroundColor(SwarmTheme.accent)
                        .padding(.horizontal, 8)
                        .padding(.vertical, 3)
                        .background(SwarmTheme.accent.opacity(0.12))
                        .clipShape(Capsule())
                    }
                    .buttonStyle(.plain)
                }

                Text("#\(msg.seq) · \(SwarmHelpers.formatClock(msg.time))")
                    .font(.system(size: 11, design: .monospaced))
                    .foregroundColor(SwarmTheme.faint)
            }

            Text(SwarmHelpers.inlineAttributed(cleanBody, baseSize: 12.5))
                .lineSpacing(3)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(SwarmTheme.cardBg)
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .stroke(SwarmTheme.border, lineWidth: 0.8)
        )
    }
}
