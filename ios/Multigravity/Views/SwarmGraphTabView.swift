import SwiftUI

struct SwarmGraphTabView: View {
    let snapshot: SwarmSnapshot
    let onSelectConversation: ((String) -> Void)?
    let onDismissSheet: () -> Void

    @State private var replayIndex: Int? = nil // nil == Live (all messages)
    @State private var isPlaying: Bool = false
    @State private var hideInactive: Bool = false
    @State private var expandAll: Bool = false
    @State private var selectedNodeId: String? = nil

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
        if let sel = selectedNodeId {
            return allMessages.last(where: { $0.agent == sel }) ?? allMessages.last(where: {
                SwarmHelpers.targets(of: $0).contains(where: { $0.caseInsensitiveCompare(sel) == .orderedSame })
            })
        }
        return allMessages.last
    }

    private var memberByName: [String: SwarmMemberInfo] {
        Dictionary(uniqueKeysWithValues: snapshot.members.map { ($0.name, $0) })
    }

    private var taskMembers: [SwarmMemberInfo] {
        let filtered = snapshot.members.filter { !SwarmHelpers.isRoleWorker(name: $0.name, role: $0.role) }
        return filtered.sorted { a, b in
            if a.name == "coordinator" { return true }
            if b.name == "coordinator" { return false }
            return a.name < b.name
        }
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 12) {
                replayControlsBar
                    .padding(.horizontal, 14)
                    .padding(.top, 8)

                graphCanvasCard
                    .padding(.horizontal, 12)

                legendRow
                    .padding(.horizontal, 16)

                if let detailMsg = displayDetailMessage {
                    detailCard(detailMsg)
                        .padding(.horizontal, 14)
                        .padding(.bottom, 20)
                }
            }
        }
        .background(SwarmTheme.bg)
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
                try? await Task.sleep(nanoseconds: 950_000_000)
                if Task.isCancelled || !isPlaying { break }
                let cur = replayIndex ?? total
                if cur < allMessages.count {
                    replayIndex = cur + 1
                } else {
                    isPlaying = false
                }
            }
        }
    }

    // MARK: - Replay Controls Bar

    private var replayControlsBar: some View {
        let total = allMessages.count
        let currentPos = replayIndex ?? total
        let isLive = replayIndex == nil

        return VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Button(action: {
                    isPlaying = false
                    replayIndex = total > 0 ? 1 : 0
                }) {
                    Image(systemName: "backward.end.fill")
                        .font(.system(size: 12))
                        .foregroundColor(total > 0 ? SwarmTheme.ink : SwarmTheme.faint)
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
                        .font(.system(size: 13))
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
            }

            HStack(spacing: 8) {
                if let cur = currentReplayMessage {
                    let clock = SwarmHelpers.formatClock(cur.time)
                    let delta = SwarmHelpers.formatDelta(from: allMessages.first?.time, to: cur.time)
                    Text("\(currentPos) / \(total) · #\(cur.seq) · \(clock)\(delta.map { " (\($0))" } ?? "")")
                        .font(.system(size: 11.5, design: .monospaced))
                        .foregroundColor(SwarmTheme.muted)
                        .lineLimit(1)
                } else {
                    Text("Live · 共 \(total) 条消息")
                        .font(.system(size: 11.5))
                        .foregroundColor(SwarmTheme.muted)
                }

                Spacer()

                toggleChip(title: "隐藏未活跃", isOn: $hideInactive)
                toggleChip(title: "展开 @all", isOn: $expandAll)
            }
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

    // MARK: - Graph Canvas & Nodes

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

        // Build active set & edges
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

        // Outsiders that posted directly
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
                // Curved edges & arrowheads
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

                        // Arrowhead at end
                        let arrowPath = arrowheadPath(tip: geom.end, from: geom.control, size: isCurEdge ? 7.5 : 6.0)
                        ctx.fill(arrowPath, with: .color(edgeColor))

                        // Travelling/active dot at end of current replay edge
                        if isCurEdge {
                            let dotRect = CGRect(x: geom.end.x - 4, y: geom.end.y - 4, width: 8, height: 8)
                            ctx.fill(Path(ellipseIn: dotRect), with: .color(SwarmTheme.liveGreen))
                            ctx.stroke(Path(ellipseIn: dotRect), with: .color(SwarmTheme.bg), lineWidth: 1.2)
                        }

                        // Count label at Bezier midpoint when count > 1
                        if stat.total > 1 {
                            let midPt = quadMidpoint(start: geom.start, control: geom.control, end: geom.end)
                            let text = Text("\(stat.total)")
                                .font(.system(size: 10, weight: .semibold, design: .monospaced))
                                .foregroundColor(isCurEdge ? SwarmTheme.ink : (isDimmed ? SwarmTheme.faint.opacity(0.5) : SwarmTheme.muted))
                            ctx.draw(text, at: CGPoint(x: midPt.x, y: midPt.y - 5), anchor: .center)
                        }
                    }
                }

                // Tappable node pills
                ForEach(positions, id: \.id) { node in
                    let member = node.member
                    let status = member?.status ?? ""
                    let isHot = currentReplayMessage.map { $0.agent == node.id || currentTargets.contains(node.id) } ?? false
                    let isSelected = selectedNodeId == node.id
                    let isActiveNode = replayIndex == nil || activeSet.contains(node.id)
                    let bcastCount = expandAll ? 0 : (broadcasts[node.id] ?? 0)
                    let hasUnaddressed = (unaddressed[node.id] ?? 0) > 0

                    Button(action: {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        selectedNodeId = (selectedNodeId == node.id) ? nil : node.id
                    }) {
                        ZStack(alignment: .topTrailing) {
                            HStack(spacing: 5) {
                                nodeStatusDot(status: status, isDimmed: !isActiveNode)
                                Text(node.id)
                                    .font(.system(size: 11.5, weight: (isHot || isSelected || node.id == "coordinator") ? .semibold : .medium))
                                    .foregroundColor(isActiveNode ? SwarmTheme.ink : SwarmTheme.faint)
                                    .lineLimit(1)
                            }
                            .padding(.horizontal, 10)
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

    @ViewBuilder
    private func nodeStatusDot(status: String, isDimmed: Bool) -> some View {
        switch status.lowercased() {
        case "working":
            Circle()
                .fill(isDimmed ? SwarmTheme.faint : SwarmTheme.liveGreen)
                .frame(width: 6.5, height: 6.5)
        case "error", "stuck":
            Circle()
                .fill(SwarmTheme.errorRed)
                .frame(width: 6.5, height: 6.5)
        case "killed":
            Circle()
                .stroke(SwarmTheme.faint, lineWidth: 1.1)
                .frame(width: 6.5, height: 6.5)
        default:
            Circle()
                .stroke(isDimmed ? SwarmTheme.faint.opacity(0.5) : SwarmTheme.muted, lineWidth: 1.2)
                .frame(width: 6.5, height: 6.5)
        }
    }

    // MARK: - Geometry Helpers (Ported from swarm_board/app.js)

    private func pillWidth(for id: String, maxW: CGFloat) -> CGFloat {
        let est = 32.0 + ceil(CGFloat(id.count) * 6.5)
        return min(maxW, max(74, est))
    }

    private func computeLayout(nodeIds: [String], width: CGFloat, height: CGFloat) -> [NodePos] {
        let n = nodeIds.count
        guard n > 0 else { return [] }
        let nodeH: CGFloat = 28

        if n <= 16 {
            let maxPillW = min(136, max(100, width / max(3.2, CGFloat(ceil(Double(n) / 2.0)) + 0.6) - 8))
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

        // Fallback rows for very large swarms (>16)
        let perRow = max(2, Int(floor(width / 110)))
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
                w: pillWidth(for: id, maxW: max(84, gap - 8)),
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
        let t: CGFloat = 0.5
        let x = (1 - t) * (1 - t) * start.x + 2 * (1 - t) * t * control.x + t * t * end.x
        let y = (1 - t) * (1 - t) * start.y + 2 * (1 - t) * t * control.y + t * t * end.y
        return CGPoint(x: x, y: y)
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

    // MARK: - Legend & Selected Message Detail Card

    private var legendRow: some View {
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
