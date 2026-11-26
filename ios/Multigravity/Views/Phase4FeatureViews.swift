import SwiftUI
import UIKit

// MARK: - 1. BrowserViewerSheet (Remote Live Browser Frame & Touch Takeover)

public struct BrowserViewerSheet: View {
    @Environment(\.dismiss) private var dismiss
    public let cascadeId: String?
    public let initialNavigateURL: String?

    @State private var tabs: [BrowserViewerTab] = []
    @State private var selectedTabId: String? = nil
    @State private var sessionId: String? = nil
    @State private var controlToken: String? = nil
    @State private var isControlling: Bool = false
    @State private var frameImage: UIImage? = nil
    @State private var frameWidthPx: Int = 1280
    @State private var frameHeightPx: Int = 800
    @State private var addressText: String = ""
    @State private var inputTextToSend: String = ""
    @State private var isLoading: Bool = true
    @State private var isBusyAction: Bool = false
    @State private var errorMessage: String? = nil
    @State private var statusBanner: String? = nil
    @State private var pollTask: Task<Void, Never>? = nil
    @State private var lastRenewAt: Date = .distantPast
    @State private var didInitialNavigate: Bool = false

    public init(cascadeId: String? = nil, initialNavigateURL: String? = nil) {
        self.cascadeId = cascadeId
        self.initialNavigateURL = initialNavigateURL
    }

    private var gatewayURL: URL? { AppSettings.shared.gatewayURL ?? AppSettings.shared.serverURL }

    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                // Address & Navigation Bar
                HStack(spacing: 6) {
                    Button { sendNavAction("back") } label: {
                        Image(systemName: "chevron.left").font(.system(size: 14, weight: .semibold))
                    }
                    .disabled(!isControlling || isBusyAction)

                    Button { sendNavAction("forward") } label: {
                        Image(systemName: "chevron.right").font(.system(size: 14, weight: .semibold))
                    }
                    .disabled(!isControlling || isBusyAction)

                    Button { sendNavAction("reload") } label: {
                        Image(systemName: "arrow.clockwise").font(.system(size: 13, weight: .semibold))
                    }
                    .disabled(isBusyAction)

                    TextField("https://...", text: $addressText)
                        .font(.system(size: 12.5, design: .monospaced))
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                        .submitLabel(.go)
                        .padding(.horizontal, 8)
                        .padding(.vertical, 6)
                        .background(Color(uiColor: .secondarySystemBackground))
                        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                        .onSubmit {
                            let target = addressText.trimmingCharacters(in: .whitespacesAndNewlines)
                            if !target.isEmpty { navigateTo(target) }
                        }
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 8)

                // Tab Switcher (only if multiple tabs exist)
                if tabs.count > 1 {
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: 6) {
                            ForEach(tabs) { tab in
                                let active = tab.tabId == selectedTabId
                                Button {
                                    Task { await switchToTab(tab.tabId) }
                                } label: {
                                    Text(tab.displayTitle)
                                        .font(.system(size: 11.5, weight: active ? .semibold : .regular))
                                        .lineLimit(1)
                                        .frame(maxWidth: 140)
                                        .padding(.horizontal, 10)
                                        .padding(.vertical, 5)
                                        .background(active ? Color.indigo.opacity(0.18) : Color(uiColor: .secondarySystemBackground))
                                        .foregroundColor(active ? .indigo : .primary)
                                        .clipShape(Capsule())
                                }
                                .buttonStyle(.plain)
                            }
                        }
                        .padding(.horizontal, 12)
                        .padding(.bottom, 6)
                    }
                }

                Divider()

                // Live Frame Canvas
                ZStack {
                    Color(uiColor: .systemGroupedBackground)
                    if let img = frameImage {
                        GeometryReader { geo in
                            let aspect = CGFloat(max(frameWidthPx, 1)) / CGFloat(max(frameHeightPx, 1))
                            let fitSize = fittedSize(container: geo.size, aspect: aspect)
                            let originX = (geo.size.width - fitSize.width) / 2
                            let originY = (geo.size.height - fitSize.height) / 2

                            Image(uiImage: img)
                                .resizable()
                                .aspectRatio(contentMode: .fit)
                                .frame(width: fitSize.width, height: fitSize.height)
                                .position(x: geo.size.width / 2, y: geo.size.height / 2)
                                .contentShape(Rectangle())
                                .gesture(
                                    SpatialTapGesture()
                                        .onEnded { value in
                                            guard isControlling else {
                                                statusBanner = String(localized: "请先点击右上角「接管」获取控制权")
                                                return
                                            }
                                            let relX = (value.location.x - originX) / max(fitSize.width, 1)
                                            let relY = (value.location.y - originY) / max(fitSize.height, 1)
                                            guard relX >= 0, relX <= 1, relY >= 0, relY <= 1 else { return }
                                            let px = Int((relX * CGFloat(frameWidthPx)).rounded())
                                            let py = Int((relY * CGFloat(frameHeightPx)).rounded())
                                            sendTap(x: px, y: py)
                                        }
                                )
                        }
                    } else if isLoading {
                        VStack(spacing: 10) {
                            ProgressView()
                            Text("正在连接宿主机浏览器画面...")
                                .font(.system(size: 13))
                                .foregroundColor(.secondary)
                        }
                    } else if let err = errorMessage {
                        VStack(spacing: 10) {
                            Image(systemName: "safari")
                                .font(.system(size: 30))
                                .foregroundColor(.orange)
                            Text(err)
                                .font(.system(size: 13))
                                .foregroundColor(.secondary)
                                .multilineTextAlignment(.center)
                                .padding(.horizontal, 24)
                            Button("重试连接") {
                                Task { await bootstrapViewer() }
                            }
                            .buttonStyle(.bordered)
                        }
                    } else {
                        Text("暂无可用浏览器标签页")
                            .font(.system(size: 13.5))
                            .foregroundColor(.secondary)
                    }

                    if let banner = statusBanner {
                        VStack {
                            Spacer()
                            Text(banner)
                                .font(.system(size: 12, weight: .medium))
                                .foregroundColor(.white)
                                .padding(.horizontal, 12)
                                .padding(.vertical, 6)
                                .background(Color.black.opacity(0.72))
                                .clipShape(Capsule())
                                .padding(.bottom, 10)
                        }
                        .allowsHitTesting(false)
                    }
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)

                // Interactive Control Bar (Scroll + Paste/Type + Enter)
                if isControlling {
                    Divider()
                    VStack(spacing: 8) {
                        HStack(spacing: 8) {
                            TextField("输入文本 / 密码 / 验证码并发送...", text: $inputTextToSend)
                                .font(.system(size: 13))
                                .textInputAutocapitalization(.never)
                                .autocorrectionDisabled()
                                .padding(.horizontal, 10)
                                .padding(.vertical, 7)
                                .background(Color(uiColor: .secondarySystemBackground))
                                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                                .submitLabel(.send)
                                .onSubmit { sendTypedText() }

                            Button("输入") { sendTypedText() }
                                .buttonStyle(.borderedProminent)
                                .controlSize(.small)
                                .tint(.indigo)
                                .disabled(inputTextToSend.isEmpty || isBusyAction)
                        }
                        HStack(spacing: 8) {
                            Button { sendWheel(deltaY: -360) } label: {
                                Label("上滑", systemImage: "arrow.up")
                                    .font(.system(size: 11.5, weight: .medium))
                            }
                            .buttonStyle(.bordered)
                            .controlSize(.mini)

                            Button { sendWheel(deltaY: 360) } label: {
                                Label("下滑", systemImage: "arrow.down")
                                    .font(.system(size: 11.5, weight: .medium))
                            }
                            .buttonStyle(.bordered)
                            .controlSize(.mini)

                            Button { sendKey(key: "Enter", code: "Enter", keyCode: 13) } label: {
                                Text("回车")
                                    .font(.system(size: 11.5, weight: .medium))
                            }
                            .buttonStyle(.bordered)
                            .controlSize(.mini)

                            Button { sendKey(key: "Backspace", code: "Backspace", keyCode: 8) } label: {
                                Image(systemName: "delete.left")
                                    .font(.system(size: 11.5))
                            }
                            .buttonStyle(.bordered)
                            .controlSize(.mini)

                            Spacer()

                            Button { Task { await fetchFrameOnce() } } label: {
                                Label("刷新", systemImage: "camera.viewfinder")
                                    .font(.system(size: 11.5, weight: .medium))
                            }
                            .buttonStyle(.bordered)
                            .controlSize(.mini)
                        }
                    }
                    .padding(.horizontal, 12)
                    .padding(.vertical, 8)
                    .background(Color(uiColor: .systemBackground))
                }
            }
            .navigationTitle("浏览器实时接管")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("关闭") { dismiss() }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        Task {
                            if isControlling {
                                await releaseControl()
                            } else {
                                await acquireControl()
                            }
                        }
                    } label: {
                        Text(isControlling ? String(localized: "释放") : String(localized: "接管"))
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(isControlling ? .orange : .indigo)
                    }
                    .disabled(isBusyAction || sessionId == nil)
                }
            }
        }
        .task {
            await bootstrapViewer()
        }
        .onDisappear {
            pollTask?.cancel()
            pollTask = nil
            let sid = sessionId
            let tok = controlToken
            let cid = cascadeId
            let tid = selectedTabId
            if let gw = gatewayURL {
                Task.detached {
                    if let tok, !tok.isEmpty {
                        try? await APIClient.shared.releaseBrowserControl(cascadeId: cid, sessionId: sid, tabId: tid, controlToken: tok, baseURL: gw)
                    }
                    if let sid, !sid.isEmpty {
                        try? await APIClient.shared.closeBrowserView(sessionId: sid, baseURL: gw)
                    }
                }
            }
        }
    }

    private func fittedSize(container: CGSize, aspect: CGFloat) -> CGSize {
        guard container.width > 0, container.height > 0, aspect > 0 else { return container }
        let containerAspect = container.width / container.height
        if aspect > containerAspect {
            return CGSize(width: container.width, height: container.width / aspect)
        } else {
            return CGSize(width: container.height * aspect, height: container.height)
        }
    }

    @MainActor
    private func bootstrapViewer() async {
        guard let gw = gatewayURL else {
            errorMessage = String(localized: "未连接到网关")
            isLoading = false
            return
        }
        isLoading = true
        errorMessage = nil
        do {
            let tabsResp = try await APIClient.shared.fetchBrowserTabs(cascadeId: cascadeId, baseURL: gw)
            let list = tabsResp.tabs ?? []
            tabs = list
            let chosen = list.first(where: { $0.active == true }) ?? list.first
            selectedTabId = chosen?.tabId
            if let u = chosen?.url { addressText = u }
            if let w = chosen?.widthPx, w > 0 { frameWidthPx = w }
            if let h = chosen?.heightPx, h > 0 { frameHeightPx = h }

            let openResp = try await APIClient.shared.openBrowserView(cascadeId: cascadeId, tabId: selectedTabId, baseURL: gw)
            sessionId = openResp.sessionId
            if let tid = openResp.tab?.tabId, !tid.isEmpty { selectedTabId = tid }
            if let u = openResp.tab?.url, !u.isEmpty { addressText = u }
            lastRenewAt = Date()

            // If launched from OAuth hybrid flow with a target URL, take control and navigate automatically.
            if let initURL = initialNavigateURL, !initURL.isEmpty, !didInitialNavigate {
                didInitialNavigate = true
                await acquireControl()
                navigateTo(initURL)
            } else {
                await fetchFrameOnce()
            }
            isLoading = false
            startPolling()
        } catch {
            errorMessage = error.localizedDescription
            isLoading = false
        }
    }

    @MainActor
    private func switchToTab(_ tabId: String) async {
        selectedTabId = tabId
        await fetchFrameOnce()
    }

    @MainActor
    private func startPolling() {
        pollTask?.cancel()
        pollTask = Task {
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 750_000_000)
                if Task.isCancelled { break }
                if let sid = sessionId, let gw = gatewayURL, Date().timeIntervalSince(lastRenewAt) > 25 {
                    try? await APIClient.shared.renewBrowserView(sessionId: sid, baseURL: gw)
                    lastRenewAt = Date()
                }
                await fetchFrameOnce()
            }
        }
    }

    @MainActor
    private func fetchFrameOnce() async {
        guard let gw = gatewayURL else { return }
        do {
            let resp = try await APIClient.shared.readBrowserFrame(
                cascadeId: cascadeId,
                sessionId: sessionId,
                tabId: selectedTabId,
                maxWidthPx: 960,
                maxHeightPx: 1440,
                baseURL: gw
            )
            if let w = resp.tab?.widthPx ?? resp.widthPx, w > 0 { frameWidthPx = w }
            if let h = resp.tab?.heightPx ?? resp.heightPx, h > 0 { frameHeightPx = h }
            if let u = resp.tab?.url, !u.isEmpty { addressText = u }
            if let b64 = resp.imageData, !b64.isEmpty,
               let data = Data(base64Encoded: b64),
               let img = UIImage(data: data) {
                frameImage = img
                errorMessage = nil
            }
        } catch {
            if frameImage == nil {
                errorMessage = error.localizedDescription
            }
        }
    }

    @MainActor
    private func acquireControl() async {
        guard let gw = gatewayURL else { return }
        isBusyAction = true
        defer { isBusyAction = false }
        do {
            let resp = try await APIClient.shared.takeBrowserControl(
                cascadeId: cascadeId,
                sessionId: sessionId,
                tabId: selectedTabId,
                baseURL: gw
            )
            controlToken = resp.controlToken
            isControlling = true
            flashStatus(String(localized: "已接管控制权，轻触画面即可点击"))
        } catch {
            flashStatus(error.localizedDescription)
        }
    }

    @MainActor
    private func releaseControl() async {
        guard let gw = gatewayURL else { return }
        isBusyAction = true
        defer { isBusyAction = false }
        try? await APIClient.shared.releaseBrowserControl(
            cascadeId: cascadeId,
            sessionId: sessionId,
            tabId: selectedTabId,
            controlToken: controlToken,
            baseURL: gw
        )
        controlToken = nil
        isControlling = false
        flashStatus(String(localized: "已交还浏览器控制权"))
    }

    private func sendNavAction(_ action: String) {
        guard let gw = gatewayURL else { return }
        isBusyAction = true
        Task { @MainActor in
            defer { isBusyAction = false }
            do {
                _ = try await APIClient.shared.controlBrowserTab(
                    cascadeId: cascadeId,
                    sessionId: sessionId,
                    tabId: selectedTabId,
                    controlToken: controlToken,
                    action: action,
                    baseURL: gw
                )
                try? await Task.sleep(nanoseconds: 250_000_000)
                await fetchFrameOnce()
            } catch {
                flashStatus(error.localizedDescription)
            }
        }
    }

    private func navigateTo(_ rawURL: String) {
        guard let gw = gatewayURL else { return }
        let target = rawURL.contains("://") ? rawURL : "https://\(rawURL)"
        addressText = target
        isBusyAction = true
        Task { @MainActor in
            defer { isBusyAction = false }
            do {
                _ = try await APIClient.shared.controlBrowserTab(
                    cascadeId: cascadeId,
                    sessionId: sessionId,
                    tabId: selectedTabId,
                    controlToken: controlToken,
                    action: "navigate",
                    url: target,
                    baseURL: gw
                )
                try? await Task.sleep(nanoseconds: 350_000_000)
                await fetchFrameOnce()
            } catch {
                flashStatus(error.localizedDescription)
            }
        }
    }

    private func sendTap(x: Int, y: Int) {
        guard let gw = gatewayURL else { return }
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        let events: [[String: JSONValue]] = [
            ["kind": .string("pointer_down"), "x": .int(x), "y": .int(y), "buttons": .int(1), "clickCount": .int(1)],
            ["kind": .string("pointer_up"), "x": .int(x), "y": .int(y), "buttons": .int(0), "clickCount": .int(1)]
        ]
        Task { @MainActor in
            do {
                _ = try await APIClient.shared.controlBrowserTab(
                    cascadeId: cascadeId,
                    sessionId: sessionId,
                    tabId: selectedTabId,
                    controlToken: controlToken,
                    action: "input_event",
                    inputEvents: events,
                    baseURL: gw
                )
                try? await Task.sleep(nanoseconds: 200_000_000)
                await fetchFrameOnce()
            } catch {
                flashStatus(error.localizedDescription)
            }
        }
    }

    private func sendWheel(deltaY: Int) {
        guard let gw = gatewayURL else { return }
        let ev: [String: JSONValue] = [
            "kind": .string("wheel"),
            "x": .int(frameWidthPx / 2),
            "y": .int(frameHeightPx / 2),
            "deltaX": .int(0),
            "deltaY": .int(deltaY)
        ]
        Task { @MainActor in
            _ = try? await APIClient.shared.controlBrowserTab(
                cascadeId: cascadeId,
                sessionId: sessionId,
                tabId: selectedTabId,
                controlToken: controlToken,
                action: "input_event",
                inputEvent: ev,
                baseURL: gw
            )
            try? await Task.sleep(nanoseconds: 180_000_000)
            await fetchFrameOnce()
        }
    }

    private func sendTypedText() {
        guard let gw = gatewayURL else { return }
        let text = inputTextToSend
        guard !text.isEmpty else { return }
        inputTextToSend = ""
        let ev: [String: JSONValue] = [
            "kind": .string("paste"),
            "text": .string(text)
        ]
        Task { @MainActor in
            _ = try? await APIClient.shared.controlBrowserTab(
                cascadeId: cascadeId,
                sessionId: sessionId,
                tabId: selectedTabId,
                controlToken: controlToken,
                action: "input_event",
                inputEvent: ev,
                baseURL: gw
            )
            try? await Task.sleep(nanoseconds: 200_000_000)
            await fetchFrameOnce()
        }
    }

    private func sendKey(key: String, code: String, keyCode: Int) {
        guard let gw = gatewayURL else { return }
        let events: [[String: JSONValue]] = [
            ["kind": .string("key_down"), "key": .string(key), "code": .string(code), "keyCode": .int(keyCode)],
            ["kind": .string("key_up"), "key": .string(key), "code": .string(code), "keyCode": .int(keyCode)]
        ]
        Task { @MainActor in
            _ = try? await APIClient.shared.controlBrowserTab(
                cascadeId: cascadeId,
                sessionId: sessionId,
                tabId: selectedTabId,
                controlToken: controlToken,
                action: "input_event",
                inputEvents: events,
                baseURL: gw
            )
            try? await Task.sleep(nanoseconds: 200_000_000)
            await fetchFrameOnce()
        }
    }

    @MainActor
    private func flashStatus(_ msg: String) {
        statusBanner = msg
        Task { @MainActor in
            try? await Task.sleep(nanoseconds: 2_500_000_000)
            if statusBanner == msg { statusBanner = nil }
        }
    }
}

// MARK: - 2. OAuthHybridSheet (Plugin Auth & MCP OAuth Loopback/External Closure)

public struct OAuthHybridSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.openURL) private var openURL

    public let title: String
    public let authUrl: String
    public let isLoopbackCallback: Bool
    public let mcpServerName: String?
    public let onCompleted: () -> Void

    @State private var callbackURLInput: String = ""
    @State private var authCodeInput: String = ""
    @State private var isSubmitting: Bool = false
    @State private var feedbackMessage: String? = nil
    @State private var isErrorFeedback: Bool = false
    @State private var showBrowserViewer: Bool = false

    public init(
        title: String,
        authUrl: String,
        isLoopbackCallback: Bool,
        mcpServerName: String? = nil,
        onCompleted: @escaping () -> Void
    ) {
        self.title = title
        self.authUrl = authUrl
        self.isLoopbackCallback = isLoopbackCallback
        self.mcpServerName = mcpServerName
        self.onCompleted = onCompleted
    }

    private var gatewayURL: URL? { AppSettings.shared.gatewayURL ?? AppSettings.shared.serverURL }

    public var body: some View {
        NavigationStack {
            List {
                Section(header: Text("授权对象")) {
                    HStack {
                        Text(title).font(.system(size: 15, weight: .semibold))
                        Spacer()
                        if isLoopbackCallback {
                            Text("本机回环回调")
                                .font(.system(size: 11, weight: .medium))
                                .foregroundColor(.orange)
                                .padding(.horizontal, 7)
                                .padding(.vertical, 2)
                                .background(Color.orange.opacity(0.14))
                                .clipShape(Capsule())
                        }
                    }
                    Text(authUrl)
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundColor(.secondary)
                        .lineLimit(3)
                        .textSelection(.enabled)
                }

                Section(
                    header: Text("推荐授权方式"),
                    footer: Text(isLoopbackCallback
                        ? "此授权回调指向电脑端 127.0.0.1，推荐直接在电脑内置浏览器中接管登录；也可先在手机浏览器完成登录，再将最后跳转失败的 http://127.0.0.1 链接粘贴到下方转发回电脑端。"
                        : "点击下方按钮在系统浏览器完成授权后返回刷新状态即可。")
                ) {
                    if isLoopbackCallback {
                        Button {
                            showBrowserViewer = true
                        } label: {
                            Label("在电脑浏览器中接管登录（推荐）", systemImage: "safari.fill")
                                .font(.system(size: 14.5, weight: .semibold))
                                .foregroundColor(.indigo)
                        }
                    }

                    Button {
                        if let u = URL(string: authUrl) {
                            openURL(u)
                        }
                    } label: {
                        Label("在手机浏览器中打开授权页", systemImage: "arrow.up.right.square")
                            .font(.system(size: 14))
                    }

                    Button {
                        UIPasteboard.general.string = authUrl
                        feedbackMessage = String(localized: "已复制授权链接")
                        isErrorFeedback = false
                    } label: {
                        Label("复制授权链接", systemImage: "doc.on.doc")
                            .font(.system(size: 14))
                    }
                }

                if isLoopbackCallback {
                    Section(header: Text("兜底：粘贴本机回调链接转发")) {
                        TextField("粘贴 http://127.0.0.1:... 完整回调链接", text: $callbackURLInput)
                            .font(.system(size: 12.5, design: .monospaced))
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                        Button {
                            submitCallbackForward()
                        } label: {
                            if isSubmitting {
                                ProgressView()
                            } else {
                                Text("转发至电脑端完成授权")
                                    .font(.system(size: 14, weight: .semibold))
                            }
                        }
                        .disabled(isSubmitting || callbackURLInput.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                    }
                }

                if let srv = mcpServerName, !srv.isEmpty {
                    Section(header: Text("或提交授权码 (Authorization Code)")) {
                        TextField("粘贴授权码...", text: $authCodeInput)
                            .font(.system(size: 12.5, design: .monospaced))
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                        Button {
                            submitMcpAuthCode(serverName: srv)
                        } label: {
                            Text("提交授权码")
                                .font(.system(size: 14, weight: .semibold))
                        }
                        .disabled(isSubmitting || authCodeInput.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                    }
                }

                if let fb = feedbackMessage {
                    Section {
                        Text(fb)
                            .font(.system(size: 13))
                            .foregroundColor(isErrorFeedback ? .red : .green)
                    }
                }
            }
            .navigationTitle("账号授权")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("完成") {
                        onCompleted()
                        dismiss()
                    }
                }
            }
        }
        .sheet(isPresented: $showBrowserViewer, onDismiss: { onCompleted() }) {
            BrowserViewerSheet(cascadeId: nil, initialNavigateURL: authUrl)
        }
    }

    private func submitCallbackForward() {
        guard let gw = gatewayURL else { return }
        let raw = callbackURLInput.trimmingCharacters(in: .whitespacesAndNewlines)
        isSubmitting = true
        feedbackMessage = nil
        Task { @MainActor in
            defer { isSubmitting = false }
            do {
                let resp = try await APIClient.shared.forwardLoopbackOAuthCallback(callbackUrl: raw, baseURL: gw)
                feedbackMessage = resp.message ?? String(localized: "已转发回调至电脑端")
                isErrorFeedback = false
                onCompleted()
            } catch {
                feedbackMessage = error.localizedDescription
                isErrorFeedback = true
            }
        }
    }

    private func submitMcpAuthCode(serverName: String) {
        guard let gw = gatewayURL else { return }
        let code = authCodeInput.trimmingCharacters(in: .whitespacesAndNewlines)
        isSubmitting = true
        feedbackMessage = nil
        Task { @MainActor in
            defer { isSubmitting = false }
            do {
                try await APIClient.shared.completeMcpOAuth(serverName: serverName, authorizationCode: code, baseURL: gw)
                feedbackMessage = String(localized: "MCP OAuth 授权已完成")
                isErrorFeedback = false
                onCompleted()
            } catch {
                feedbackMessage = error.localizedDescription
                isErrorFeedback = true
            }
        }
    }
}

// MARK: - 3. DesktopUpdateSheet (Antigravity Desktop Self-Update & Changelog)

public struct DesktopUpdateSheet: View {
    @Environment(\.colorScheme) private var colorScheme
    public let initialStatus: DesktopUpdateStatus?

    @State private var status: DesktopUpdateStatus?
    @State private var changelogMarkdown: String = ""
    @State private var isLoading: Bool = true
    @State private var isApplying: Bool = false
    @State private var showConfirmApply: Bool = false
    @State private var messageText: String? = nil
    @State private var errorText: String? = nil

    public init(initialStatus: DesktopUpdateStatus? = nil) {
        self.initialStatus = initialStatus
        _status = State(initialValue: initialStatus)
    }

    private var gatewayURL: URL? { AppSettings.shared.gatewayURL ?? AppSettings.shared.serverURL }

    private var currentVersionDisplay: String {
        if let cur = status?.currentVersion, !cur.isEmpty {
            return "v\(cur)"
        }
        return String(localized: "未知")
    }

    private var latestVersionDisplay: String? {
        guard let latest = status?.latestVersion, !latest.isEmpty else { return nil }
        return "v\(latest)"
    }

    private var heroAccentColor: Color {
        if isLoading && status == nil { return .secondary }
        if errorText != nil && status == nil { return .orange }
        if status?.hasUpdate == true { return .indigo }
        return .green
    }

    private var heroBadgeSymbol: String? {
        if isLoading && status == nil { return nil }
        if errorText != nil && status == nil { return "exclamationmark.circle.fill" }
        if status?.hasUpdate == true { return "arrow.down.circle.fill" }
        if status != nil { return "checkmark.circle.fill" }
        return nil
    }

    private var heroTitle: String {
        if isLoading && status == nil {
            return String(localized: "正在检查更新...")
        }
        if errorText != nil && status == nil {
            return String(localized: "检查更新失败")
        }
        if status?.hasUpdate == true {
            return String(localized: "发现新版本")
        }
        return String(localized: "已是最新版本")
    }

    private var heroSubtitle: String? {
        if isLoading && status == nil {
            return nil
        }
        if errorText != nil && status == nil {
            return nil
        }
        if status?.hasUpdate == true {
            if let latest = latestVersionDisplay {
                return String(localized: "桌面端可升级至 \(latest)")
            }
            return String(localized: "检测到可用的桌面端新版本")
        }
        if status != nil {
            return String(localized: "宿主机 Antigravity 已是最新版本")
        }
        return nil
    }

    private var cardBorderColor: Color {
        Color.primary.opacity(colorScheme == .dark ? 0.08 : 0.05)
    }

    private var shouldCenterVertically: Bool {
        changelogMarkdown.isEmpty
    }

    public var body: some View {
        NavigationStack {
            GeometryReader { proxy in
                ScrollView {
                    VStack(spacing: 0) {
                        if shouldCenterVertically {
                            Spacer(minLength: 28)
                        }

                        VStack(spacing: 24) {
                            statusHeroSection

                            if status != nil || !isLoading {
                                versionCard
                            }

                            if let running = status?.runningCount, running > 0, status?.hasUpdate == true {
                                HStack(alignment: .top, spacing: 12) {
                                    Image(systemName: "exclamationmark.triangle.fill")
                                        .font(.system(size: 16))
                                        .foregroundColor(.orange)
                                        .padding(.top, 2)
                                    Text("当前有 \(running) 个活跃会话正在运行。Antigravity 2.35.1+ 重启后会自动恢复会话，但仍建议等关键操作完成后更新。")
                                        .font(.system(size: 13.5))
                                        .foregroundColor(.secondary)
                                        .lineSpacing(3.5)
                                        .fixedSize(horizontal: false, vertical: true)
                                }
                                .padding(16)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .background(Color.orange.opacity(colorScheme == .dark ? 0.14 : 0.10))
                                .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                            }

                            if let msg = messageText {
                                HStack(alignment: .top, spacing: 10) {
                                    Image(systemName: "checkmark.circle.fill")
                                        .font(.system(size: 15))
                                        .foregroundColor(.green)
                                        .padding(.top, 1)
                                    Text(msg)
                                        .font(.system(size: 13.5, weight: .medium))
                                        .foregroundColor(.green)
                                        .lineSpacing(3)
                                        .fixedSize(horizontal: false, vertical: true)
                                }
                                .padding(.horizontal, 16)
                                .padding(.vertical, 13)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .background(Color.green.opacity(colorScheme == .dark ? 0.14 : 0.10))
                                .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                            }

                            if let err = errorText {
                                HStack(alignment: .top, spacing: 10) {
                                    Image(systemName: "exclamationmark.circle.fill")
                                        .font(.system(size: 15))
                                        .foregroundColor(.red)
                                        .padding(.top, 1)
                                    Text(err)
                                        .font(.system(size: 13.5))
                                        .foregroundColor(.red)
                                        .lineSpacing(3)
                                        .fixedSize(horizontal: false, vertical: true)
                                }
                                .padding(.horizontal, 16)
                                .padding(.vertical, 13)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .background(Color.red.opacity(colorScheme == .dark ? 0.14 : 0.10))
                                .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                            }

                            if status?.hasUpdate == true {
                                Button {
                                    showConfirmApply = true
                                } label: {
                                    HStack(spacing: 8) {
                                        if isApplying {
                                            ProgressView().tint(.white)
                                        }
                                        Text(isApplying ? String(localized: "正在通知桌面端...") : String(localized: "立即应用更新并重启桌面端"))
                                            .font(.system(size: 16, weight: .semibold))
                                    }
                                    .frame(maxWidth: .infinity)
                                    .padding(.vertical, 6)
                                }
                                .buttonStyle(.borderedProminent)
                                .controlSize(.large)
                                .tint(.indigo)
                                .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                                .disabled(isApplying)
                            }

                            if !changelogMarkdown.isEmpty {
                                VStack(alignment: .leading, spacing: 10) {
                                    Text("更新日志")
                                        .font(.system(size: 13.5, weight: .semibold))
                                        .foregroundColor(.secondary)
                                        .padding(.horizontal, 4)
                                    MarkdownContentView(content: changelogMarkdown)
                                        .padding(16)
                                        .frame(maxWidth: .infinity, alignment: .leading)
                                        .background(Color(uiColor: .secondarySystemGroupedBackground))
                                        .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
                                        .overlay(
                                            RoundedRectangle(cornerRadius: 16, style: .continuous)
                                                .stroke(cardBorderColor, lineWidth: 0.5)
                                        )
                                }
                            }
                        }
                        .frame(maxWidth: 520)

                        if shouldCenterVertically {
                            Spacer(minLength: 36)
                            Spacer(minLength: 28)
                        }
                    }
                    .padding(.horizontal, 20)
                    .padding(.top, shouldCenterVertically ? 12 : 28)
                    .padding(.bottom, 36)
                    .frame(maxWidth: .infinity)
                    .frame(minHeight: proxy.size.height)
                    .animation(.easeInOut(duration: 0.22), value: shouldCenterVertically)
                }
                .refreshable {
                    await loadUpdateAndChangelog()
                }
            }
            .background(Color(uiColor: .systemGroupedBackground).ignoresSafeArea())
            .navigationTitle("Antigravity 桌面端更新")
            .navigationBarTitleDisplayMode(.inline)
            .presentationDragIndicator(.visible)
            .confirmationDialog("确认更新并重启电脑端 Antigravity？", isPresented: $showConfirmApply, titleVisibility: .visible) {
                Button("确认更新并重启") {
                    Task { await triggerApplyUpdate() }
                }
                Button("取消", role: .cancel) {}
            } message: {
                Text("电脑端将安装新版本并短暂停机重启，网关会自动重连。")
            }
        }
        .presentationDragIndicator(.visible)
        .task {
            await loadUpdateAndChangelog()
        }
    }

    private var statusHeroSection: some View {
        VStack(spacing: 16) {
            ZStack(alignment: .bottomTrailing) {
                ZStack {
                    Circle()
                        .fill(heroAccentColor.opacity(colorScheme == .dark ? 0.16 : 0.11))
                        .frame(width: 76, height: 76)
                    Circle()
                        .stroke(heroAccentColor.opacity(colorScheme == .dark ? 0.24 : 0.16), lineWidth: 1)
                        .frame(width: 76, height: 76)

                    if isLoading && status == nil {
                        ProgressView()
                            .controlSize(.regular)
                    } else {
                        Image(systemName: "desktopcomputer")
                            .font(.system(size: 30, weight: .medium))
                            .foregroundStyle(status?.hasUpdate == true ? Color.indigo : Color.primary.opacity(0.88))
                    }
                }

                if let badgeSymbol = heroBadgeSymbol {
                    Image(systemName: badgeSymbol)
                        .font(.system(size: 21, weight: .semibold))
                        .symbolRenderingMode(.palette)
                        .foregroundStyle(.white, heroAccentColor)
                        .background(
                            Circle()
                                .fill(Color(uiColor: .systemGroupedBackground))
                                .frame(width: 25, height: 25)
                        )
                        .offset(x: 2, y: 2)
                }
            }

            VStack(spacing: 8) {
                Text(heroTitle)
                    .font(.system(size: 20, weight: .semibold))
                    .foregroundColor(.primary)
                    .multilineTextAlignment(.center)

                if let subtitle = heroSubtitle {
                    Text(subtitle)
                        .font(.system(size: 14))
                        .foregroundColor(.secondary)
                        .multilineTextAlignment(.center)
                        .lineSpacing(3.5)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            .padding(.horizontal, 12)
        }
        .frame(maxWidth: .infinity)
        .padding(.bottom, 4)
    }

    private var versionCard: some View {
        VStack(spacing: 0) {
            versionRow(
                title: String(localized: "当前桌面端版本"),
                value: currentVersionDisplay,
                valueColor: .primary,
                valueWeight: .semibold
            )
            if let latest = latestVersionDisplay {
                Divider()
                    .padding(.leading, 18)
                versionRow(
                    title: String(localized: "最新可用版本"),
                    value: latest,
                    valueColor: status?.hasUpdate == true ? .indigo : .secondary,
                    valueWeight: status?.hasUpdate == true ? .bold : .medium
                )
            }
        }
        .background(Color(uiColor: .secondarySystemGroupedBackground))
        .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 16, style: .continuous)
                .stroke(cardBorderColor, lineWidth: 0.5)
        )
    }

    private func versionRow(
        title: String,
        value: String,
        valueColor: Color,
        valueWeight: Font.Weight
    ) -> some View {
        HStack(spacing: 12) {
            Text(title)
                .font(.system(size: 15))
                .foregroundColor(.secondary)
            Spacer(minLength: 12)
            Text(value)
                .font(.system(size: 15.5, weight: valueWeight, design: .monospaced))
                .foregroundColor(valueColor)
        }
        .padding(.horizontal, 18)
        .padding(.vertical, 16)
    }

    @MainActor
    private func loadUpdateAndChangelog() async {
        guard let gw = gatewayURL else {
            isLoading = false
            return
        }
        isLoading = true
        errorText = nil
        defer { isLoading = false }
        do {
            let st = try await APIClient.shared.fetchDesktopUpdateStatus(baseURL: gw)
            status = st
            let targetVer = (st.latestVersion?.isEmpty == false ? st.latestVersion : st.currentVersion)
            if let cl = try? await APIClient.shared.fetchDesktopChangelog(version: targetVer, baseURL: gw),
               let md = cl.markdown, !md.isEmpty {
                changelogMarkdown = md
            }
        } catch {
            errorText = error.localizedDescription
        }
    }

    @MainActor
    private func triggerApplyUpdate() async {
        guard let gw = gatewayURL else { return }
        isApplying = true
        errorText = nil
        defer { isApplying = false }
        do {
            let resp = try await APIClient.shared.applyDesktopUpdate(baseURL: gw)
            messageText = resp.message ?? String(localized: "已通知桌面端应用更新并重启")
        } catch {
            errorText = error.localizedDescription
        }
    }
}

// MARK: - 4. WorkspaceSearchTabView (Embedded inside CascadeChangesSheet)

public struct WorkspaceSearchTabView: View {
    public enum Mode: String, CaseIterable {
        case code = "代码"
        case files = "文件"
    }

    public let cascadeId: String
    public let mode: Mode

    @State private var query: String = ""
    @State private var isRegex: Bool = false
    @State private var caseSensitive: Bool = false
    @State private var isSearching: Bool = false
    @State private var codeHits: [CodeSearchHit] = []
    @State private var fileHits: [FileSearchHit] = []
    @State private var errorMessage: String? = nil
    @State private var previewData: PreviewFilePayload? = nil

    private struct PreviewFilePayload: Identifiable {
        let id = UUID()
        let title: String
        let uri: String
        let highlightLine: Int?
        var content: String
        var isLoading: Bool
        var error: String?
    }

    public init(cascadeId: String, mode: Mode) {
        self.cascadeId = cascadeId
        self.mode = mode
    }

    private var gatewayURL: URL? { AppSettings.shared.gatewayURL ?? AppSettings.shared.serverURL }

    public var body: some View {
        VStack(spacing: 0) {
            VStack(spacing: 8) {
                HStack(spacing: 8) {
                    Image(systemName: "magnifyingglass")
                        .foregroundColor(.secondary)
                    TextField(mode == .code ? "搜索项目代码关键词..." : "按路径或文件名搜索...", text: $query)
                        .font(.system(size: 14))
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .submitLabel(.search)
                        .onSubmit { runSearch() }
                    if !query.isEmpty {
                        Button {
                            query = ""
                            codeHits = []
                            fileHits = []
                        } label: {
                            Image(systemName: "xmark.circle.fill").foregroundColor(.secondary)
                        }
                    }
                    Button("搜索") { runSearch() }
                        .buttonStyle(.borderedProminent)
                        .controlSize(.small)
                        .tint(.indigo)
                        .disabled(isSearching || (mode == .code && query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty))
                }
                .padding(.horizontal, 10)
                .padding(.vertical, 7)
                .background(Color(uiColor: .secondarySystemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))

                if mode == .code {
                    HStack(spacing: 12) {
                        Toggle("正则", isOn: $isRegex)
                            .toggleStyle(.button)
                            .controlSize(.mini)
                        Toggle("区分大小写", isOn: $caseSensitive)
                            .toggleStyle(.button)
                            .controlSize(.mini)
                        Spacer()
                        if !codeHits.isEmpty {
                            Text("\(codeHits.count) 条命中")
                                .font(.system(size: 11.5))
                                .foregroundColor(.secondary)
                        }
                    }
                }
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 10)

            Divider()

            if isSearching {
                VStack(spacing: 10) {
                    ProgressView()
                    Text("正在检索工作区...")
                        .font(.system(size: 13))
                        .foregroundColor(.secondary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if let err = errorMessage {
                VStack(spacing: 8) {
                    Image(systemName: "exclamationmark.triangle.fill").foregroundColor(.orange)
                    Text(err).font(.system(size: 13)).foregroundColor(.secondary).multilineTextAlignment(.center)
                }
                .padding(24)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if mode == .code {
                if codeHits.isEmpty {
                    emptyHint(query.isEmpty ? String(localized: "输入关键词检索工作区代码") : String(localized: "未找到匹配代码"))
                } else {
                    List(codeHits) { hit in
                        Button {
                            openFilePreview(title: hit.relativePath, uri: hit.uri, line: hit.lineNumber)
                        } label: {
                            VStack(alignment: .leading, spacing: 4) {
                                HStack {
                                    Text(hit.relativePath)
                                        .font(.system(size: 12.5, weight: .semibold, design: .monospaced))
                                        .foregroundColor(.indigo)
                                        .lineLimit(1)
                                        .truncationMode(.middle)
                                    Spacer()
                                    Text("L\(hit.lineNumber)")
                                        .font(.system(size: 11, weight: .bold, design: .monospaced))
                                        .foregroundColor(.secondary)
                                }
                                Text(hit.snippet.trimmingCharacters(in: .whitespaces))
                                    .font(.system(size: 12, design: .monospaced))
                                    .foregroundColor(.primary)
                                    .lineLimit(2)
                            }
                            .padding(.vertical, 2)
                        }
                        .buttonStyle(.plain)
                    }
                    .listStyle(.plain)
                }
            } else {
                if fileHits.isEmpty {
                    emptyHint(query.isEmpty ? String(localized: "点击搜索浏览或按文件名定位") : String(localized: "未找到匹配文件"))
                } else {
                    List(fileHits) { hit in
                        Button {
                            if hit.isDirectory != true {
                                openFilePreview(title: hit.relativePath, uri: hit.uri, line: nil)
                            }
                        } label: {
                            HStack(spacing: 8) {
                                Image(systemName: hit.isDirectory == true ? "folder.fill" : "doc.text")
                                    .foregroundColor(hit.isDirectory == true ? .blue : .indigo)
                                Text(hit.relativePath)
                                    .font(.system(size: 13, design: .monospaced))
                                    .foregroundColor(.primary)
                                    .lineLimit(1)
                                    .truncationMode(.middle)
                            }
                        }
                        .buttonStyle(.plain)
                    }
                    .listStyle(.plain)
                }
            }
        }
        .sheet(item: $previewData) { item in
            NavigationStack {
                Group {
                    if item.isLoading {
                        ProgressView("正在读取文件...")
                    } else if let err = item.error {
                        Text(err).font(.system(size: 13)).foregroundColor(.secondary).padding()
                    } else {
                        ScrollView([.vertical, .horizontal]) {
                            VStack(alignment: .leading, spacing: 2) {
                                let lines = item.content.components(separatedBy: .newlines)
                                ForEach(Array(lines.enumerated()), id: \.offset) { idx, line in
                                    let lineNo = idx + 1
                                    let isTarget = item.highlightLine == lineNo
                                    HStack(alignment: .top, spacing: 10) {
                                        Text("\(lineNo)")
                                            .font(.system(size: 11, design: .monospaced))
                                            .foregroundColor(isTarget ? .orange : .secondary)
                                            .frame(width: 36, alignment: .trailing)
                                        Text(line.isEmpty ? " " : line)
                                            .font(.system(size: 12, design: .monospaced))
                                            .foregroundColor(.primary)
                                            .textSelection(.enabled)
                                    }
                                    .padding(.horizontal, 8)
                                    .padding(.vertical, 1)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                                    .background(isTarget ? Color.orange.opacity(0.16) : Color.clear)
                                }
                            }
                            .padding(.vertical, 10)
                        }
                    }
                }
                .navigationTitle((item.title as NSString).lastPathComponent)
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .topBarTrailing) {
                        Button("关闭") { previewData = nil }
                    }
                }
            }
        }
    }

    private func emptyHint(_ text: String) -> some View {
        VStack(spacing: 8) {
            Image(systemName: mode == .code ? "doc.text.magnifyingglass" : "folder.badge.questionmark")
                .font(.system(size: 28))
                .foregroundColor(.secondary)
            Text(text).font(.system(size: 13.5)).foregroundColor(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private func runSearch() {
        guard let gw = gatewayURL, !cascadeId.isEmpty else { return }
        isSearching = true
        errorMessage = nil
        Task { @MainActor in
            defer { isSearching = false }
            do {
                if mode == .code {
                    codeHits = try await APIClient.shared.searchWorkspaceCode(
                        cascadeId: cascadeId,
                        query: query,
                        isRegex: isRegex,
                        caseSensitive: caseSensitive,
                        baseURL: gw
                    )
                } else {
                    fileHits = try await APIClient.shared.searchWorkspaceFiles(
                        cascadeId: cascadeId,
                        query: query,
                        baseURL: gw
                    )
                }
            } catch {
                errorMessage = error.localizedDescription
            }
        }
    }

    private func openFilePreview(title: String, uri: String, line: Int?) {
        guard let gw = gatewayURL else { return }
        let payload = PreviewFilePayload(title: title, uri: uri, highlightLine: line, content: "", isLoading: true, error: nil)
        previewData = payload
        Task { @MainActor in
            do {
                let resp = try await APIClient.shared.fetchFileContent(uri: uri, baseURL: gw)
                if previewData?.id == payload.id {
                    previewData = PreviewFilePayload(title: title, uri: uri, highlightLine: line, content: resp.content, isLoading: false, error: nil)
                }
            } catch {
                if previewData?.id == payload.id {
                    previewData = PreviewFilePayload(title: title, uri: uri, highlightLine: line, content: "", isLoading: false, error: error.localizedDescription)
                }
            }
        }
    }
}

// MARK: - 5. ArtifactSnapshotsSheet (Artifact Version History)

public struct ArtifactSnapshotsSheet: View {
    @Environment(\.dismiss) private var dismiss
    public let snapshots: [ArtifactSnapshotItem]
    public let onSelectSnapshot: (ArtifactSnapshotItem) -> Void

    @State private var selectedIndex: Int = 0

    public init(snapshots: [ArtifactSnapshotItem], onSelectSnapshot: @escaping (ArtifactSnapshotItem) -> Void) {
        self.snapshots = snapshots
        self.onSelectSnapshot = onSelectSnapshot
        _selectedIndex = State(initialValue: max(0, snapshots.count - 1))
    }

    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 8) {
                        ForEach(Array(snapshots.enumerated()), id: \.element.id) { idx, snap in
                            let isSelected = idx == selectedIndex
                            Button {
                                selectedIndex = idx
                            } label: {
                                VStack(alignment: .leading, spacing: 2) {
                                    Text("v\(snap.index)")
                                        .font(.system(size: 12.5, weight: .bold, design: .monospaced))
                                    if let ts = snap.lastEdited, !ts.isEmpty {
                                        Text(String(ts.prefix(16)).replacingOccurrences(of: "T", with: " "))
                                            .font(.system(size: 10))
                                            .foregroundColor(isSelected ? .white.opacity(0.85) : .secondary)
                                    }
                                }
                                .padding(.horizontal, 10)
                                .padding(.vertical, 6)
                                .background(isSelected ? Color.indigo : Color(uiColor: .secondarySystemBackground))
                                .foregroundColor(isSelected ? .white : .primary)
                                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                            }
                            .buttonStyle(.plain)
                        }
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 10)
                }
                Divider()
                if snapshots.indices.contains(selectedIndex) {
                    let snap = snapshots[selectedIndex]
                    ScrollView {
                        VStack(alignment: .leading, spacing: 10) {
                            if let summary = snap.summary, !summary.isEmpty {
                                Text(summary)
                                    .font(.system(size: 12.5))
                                    .foregroundColor(.secondary)
                                    .padding(10)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                                    .background(Color(uiColor: .secondarySystemBackground))
                                    .clipShape(RoundedRectangle(cornerRadius: 8))
                            }
                            MarkdownContentView(content: snap.content)
                        }
                        .padding(16)
                    }
                }
            }
            .navigationTitle("历史版本 (\(snapshots.count))")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("关闭") { dismiss() }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    if snapshots.indices.contains(selectedIndex) {
                        Button("预览此版本") {
                            onSelectSnapshot(snapshots[selectedIndex])
                            dismiss()
                        }
                        .fontWeight(.semibold)
                    }
                }
            }
        }
    }
}

// MARK: - 6. RemoteSSHSectionView (Embedded inside NewConversationSheet)

public struct RemoteSSHSectionView: View {
    public let onConnectedChanged: () -> Void

    @State private var remotes: [RemoteHostDescriptor] = []
    @State private var connectedIds: Set<String> = []
    @State private var activeJob: RemoteConnectJob? = nil
    @State private var pollTask: Task<Void, Never>? = nil
    @State private var promptInput: String = ""
    @State private var showAddRemoteSheet: Bool = false
    @State private var newName: String = ""
    @State private var newHost: String = ""
    @State private var newUser: String = ""
    @State private var newPort: String = "22"
    @State private var isLoading: Bool = false
    @State private var feedbackError: String? = nil

    public init(onConnectedChanged: @escaping () -> Void) {
        self.onConnectedChanged = onConnectedChanged
    }

    private var gatewayURL: URL? { AppSettings.shared.gatewayURL ?? AppSettings.shared.serverURL }

    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Label("SSH 远端主机", systemImage: "server.rack")
                    .font(.system(size: 12.5, weight: .semibold))
                    .foregroundColor(.secondary)
                Spacer()
                Button {
                    showAddRemoteSheet = true
                } label: {
                    Label("添加", systemImage: "plus")
                        .font(.system(size: 12, weight: .medium))
                }
                .buttonStyle(.borderless)
            }
            .padding(.top, 8)

            if let err = feedbackError {
                Text(err)
                    .font(.system(size: 12))
                    .foregroundColor(.orange)
            }

            if let job = activeJob, job.done != true {
                HStack(spacing: 8) {
                    ProgressView().scaleEffect(0.8)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(job.name ?? "SSH").font(.system(size: 13, weight: .semibold))
                        Text(job.message ?? String(localized: "正在连接..."))
                            .font(.system(size: 11.5))
                            .foregroundColor(.secondary)
                    }
                    Spacer()
                }
                .padding(10)
                .background(Color.indigo.opacity(0.08))
                .clipShape(RoundedRectangle(cornerRadius: 10))
            }

            if remotes.isEmpty && !isLoading {
                Text("未发现 ~/.ssh/config 或已保存的远端主机，点击右上角「添加」即可配置。")
                    .font(.system(size: 12))
                    .foregroundColor(.secondary)
                    .padding(.vertical, 4)
            } else {
                ForEach(remotes) { remote in
                    let isConn = isRemoteConnected(remote)
                    HStack(spacing: 10) {
                        Image(systemName: isConn ? "checkmark.circle.fill" : "terminal")
                            .foregroundColor(isConn ? .green : .indigo)
                            .font(.system(size: 15))
                        VStack(alignment: .leading, spacing: 2) {
                            Text(remote.displayName)
                                .font(.system(size: 14, weight: .semibold))
                                .foregroundColor(.primary)
                                .lineLimit(1)
                            if !remote.hostSummary.isEmpty {
                                Text(remote.hostSummary)
                                    .font(.system(size: 11.5, design: .monospaced))
                                    .foregroundColor(.secondary)
                                    .lineLimit(1)
                            }
                        }
                        Spacer()
                        if isConn {
                            Button("断开") {
                                disconnect(remote)
                            }
                            .buttonStyle(.bordered)
                            .controlSize(.mini)
                            .tint(.orange)
                        } else {
                            Button("连接") {
                                connect(remote)
                            }
                            .buttonStyle(.borderedProminent)
                            .controlSize(.mini)
                            .tint(.indigo)
                            .disabled(activeJob?.done == false)
                        }
                    }
                    .padding(10)
                    .background(Color(uiColor: .secondarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 11, style: .continuous))
                }
            }
        }
        .task { await loadRemotes() }
        .onDisappear { pollTask?.cancel() }
        .alert(
            activeJob?.prompt?.message ?? String(localized: "SSH 认证提示"),
            isPresented: Binding(
                get: { activeJob?.status == "prompt" && activeJob?.prompt != nil },
                set: { _ in }
            )
        ) {
            if activeJob?.prompt?.isSecret == true {
                SecureField("输入密码或密钥口令", text: $promptInput)
            } else {
                TextField("输入响应（如 yes）", text: $promptInput)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
            }
            Button("取消", role: .cancel) {
                promptInput = ""
                pollTask?.cancel()
                activeJob = nil
            }
            Button("提交") {
                submitPrompt()
            }
        }
        .sheet(isPresented: $showAddRemoteSheet) {
            NavigationStack {
                Form {
                    TextField("显示名称（如 gpu-box）", text: $newName)
                        .textInputAutocapitalization(.never)
                    TextField("主机地址（IP 或域名）", text: $newHost)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                    TextField("用户名（可选）", text: $newUser)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                    TextField("端口（默认 22）", text: $newPort)
                        .keyboardType(.numberPad)
                }
                .navigationTitle("添加 SSH 主机")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .topBarLeading) {
                        Button("取消") { showAddRemoteSheet = false }
                    }
                    ToolbarItem(placement: .topBarTrailing) {
                        Button("保存") {
                            saveNewRemote()
                        }
                        .fontWeight(.semibold)
                        .disabled(newHost.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                    }
                }
            }
            .presentationDetents([.medium])
        }
    }

    private func isRemoteConnected(_ remote: RemoteHostDescriptor) -> Bool {
        if let rid = remote.remoteId, connectedIds.contains(rid) { return true }
        if let n = remote.name, connectedIds.contains(n) { return true }
        return false
    }

    @MainActor
    private func loadRemotes() async {
        guard let gw = gatewayURL else { return }
        isLoading = true
        defer { isLoading = false }
        if let resp = try? await APIClient.shared.fetchRemotes(baseURL: gw) {
            remotes = resp.remotes ?? []
            var set: Set<String> = []
            for c in resp.connected ?? [] {
                if let rid = c.remoteId, !rid.isEmpty { set.insert(rid) }
                if let n = c.name, !n.isEmpty { set.insert(n) }
            }
            connectedIds = set
        }
    }

    private func connect(_ remote: RemoteHostDescriptor) {
        guard let gw = gatewayURL else { return }
        feedbackError = nil
        let hop = remote.remoteMultiHop?.hops?.first
        let host = hop?.host ?? remote.name ?? ""
        let user = hop?.user ?? ""
        let port = hop?.port ?? 22
        Task { @MainActor in
            do {
                let job = try await APIClient.shared.connectRemote(
                    name: remote.displayName,
                    host: host,
                    user: user,
                    port: port,
                    baseURL: gw
                )
                activeJob = job
                startPollingJob(job.jobId)
            } catch {
                feedbackError = error.localizedDescription
            }
        }
    }

    @MainActor
    private func startPollingJob(_ jobId: String) {
        pollTask?.cancel()
        pollTask = Task {
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 700_000_000)
                if Task.isCancelled { break }
                guard let gw = gatewayURL else { break }
                if let job = try? await APIClient.shared.fetchRemoteConnectStatus(jobId: jobId, baseURL: gw) {
                    activeJob = job
                    if job.done == true {
                        if job.status == "error" {
                            feedbackError = job.error ?? String(localized: "SSH 连接失败")
                        } else {
                            await loadRemotes()
                            onConnectedChanged()
                        }
                        break
                    }
                }
            }
        }
    }

    private func submitPrompt() {
        guard let gw = gatewayURL, let job = activeJob else { return }
        let answer = promptInput
        promptInput = ""
        Task { @MainActor in
            do {
                try await APIClient.shared.respondToRemotePrompt(jobId: job.jobId, remoteId: job.remoteId, response: answer, baseURL: gw)
            } catch {
                feedbackError = error.localizedDescription
            }
        }
    }

    private func disconnect(_ remote: RemoteHostDescriptor) {
        guard let gw = gatewayURL else { return }
        let rid = remote.remoteId ?? remote.name ?? ""
        guard !rid.isEmpty else { return }
        Task { @MainActor in
            do {
                try await APIClient.shared.disconnectRemote(remoteId: rid, baseURL: gw)
                await loadRemotes()
                onConnectedChanged()
            } catch {
                feedbackError = error.localizedDescription
            }
        }
    }

    private func saveNewRemote() {
        guard let gw = gatewayURL else { return }
        let host = newHost.trimmingCharacters(in: .whitespacesAndNewlines)
        let name = newName.trimmingCharacters(in: .whitespacesAndNewlines)
        let user = newUser.trimmingCharacters(in: .whitespacesAndNewlines)
        let port = Int(newPort.trimmingCharacters(in: .whitespacesAndNewlines)) ?? 22
        showAddRemoteSheet = false
        Task { @MainActor in
            do {
                try await APIClient.shared.saveRemote(name: name.isEmpty ? host : name, host: host, user: user, port: port, baseURL: gw)
                newName = ""; newHost = ""; newUser = ""; newPort = "22"
                await loadRemotes()
            } catch {
                feedbackError = error.localizedDescription
            }
        }
    }
}
