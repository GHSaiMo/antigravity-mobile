import Foundation
import AVFoundation
import Network
import Combine
import UIKit

@MainActor
public final class VoiceCallManager: ObservableObject {
    public static let shared = VoiceCallManager()

    public enum CallState: Equatable {
        case idle
        case connecting
        case listening
        case reconnecting
        case failed(String)
    }

    public enum TranscriptRole: Equatable {
        case user
        case ai
    }

    public static let supportedSpeeds: [Double] = [0.75, 1.0, 1.25, 1.5, 1.75, 2.0]

    @Published public private(set) var isInCall: Bool = false
    @Published public private(set) var callState: CallState = .idle
    @Published public private(set) var activeCascadeId: String = ""
    @Published public private(set) var duration: TimeInterval = 0
    @Published public private(set) var isMicMuted: Bool = false
    @Published public private(set) var isSpeakerMuted: Bool = false
    @Published public private(set) var playbackSpeed: Double = 1.0
    @Published public private(set) var isAgentSpeaking: Bool = false
    @Published public private(set) var soundLevels: [CGFloat] = Array(repeating: 0.12, count: 18)
    @Published public private(set) var liveTranscriptText: String = ""
    @Published public private(set) var liveTranscriptRole: TranscriptRole = .user

    public var onCascadeBound: ((String) -> Void)?

    private var connection: NWConnection?
    private var isSocketReady: Bool = false
    private var isIntentionallyClosed: Bool = false

    private var audioEngine: AVAudioEngine?
    private var playerNode: AVAudioPlayerNode?
    private var playbackFormat: AVAudioFormat?
    private var pendingPlaybackBuffers: Int = 0

    private var durationTimer: Timer?
    private var callStartedAt: Date?
    private var transcriptClearTask: Task<Void, Never>?
    private var agentSpeakingDecayTask: Task<Void, Never>?
    private var connectionWatchdogTask: Task<Void, Never>?

    private let audioQueue = DispatchQueue(label: "com.antigravity.mobile.voice.audio", qos: .userInteractive)

    private init() {}

    // MARK: - Public Controls

    public func startCall(
        cascadeId: String,
        modelIdOrEnum: String?,
        baseURL: URL? = nil
    ) {
        if isInCall {
            endCall()
        }

        // Stop offline .m4a recorder if active in same session
        if AudioRecorderManager.shared.isRecording {
            AudioRecorderManager.shared.cancelRecording()
        }

        let resolvedURL = baseURL ?? AppSettings.shared.serverURL
        guard let serverURL = resolvedURL else {
            callState = .failed("未配置服务器地址")
            return
        }

        isIntentionallyClosed = false
        isSocketReady = false
        activeCascadeId = cascadeId
        isInCall = true
        callState = .connecting
        duration = 0
        isMicMuted = false
        isSpeakerMuted = false
        isAgentSpeaking = false
        liveTranscriptText = ""
        soundLevels = Array(repeating: 0.12, count: 18)
        callStartedAt = Date()

        UIImpactFeedbackGenerator(style: .medium).impactOccurred()

        do {
            try setupAudioEngine()
        } catch {
            print("[VoiceCall] AudioEngine setup failed: \(error)")
            callState = .failed("麦克风或音频引擎初始化失败")
            endCall(preserveFailure: true)
            return
        }

        connectWebSocket(baseURL: serverURL, cascadeId: cascadeId, modelIdOrEnum: modelIdOrEnum)
        startDurationTimer()
    }

    public func endCall(preserveFailure: Bool = false) {
        isIntentionallyClosed = true
        isSocketReady = false

        connectionWatchdogTask?.cancel()
        connectionWatchdogTask = nil
        transcriptClearTask?.cancel()
        transcriptClearTask = nil
        agentSpeakingDecayTask?.cancel()
        agentSpeakingDecayTask = nil

        durationTimer?.invalidate()
        durationTimer = nil

        if let conn = connection {
            conn.stateUpdateHandler = nil
            conn.cancel()
            connection = nil
        }

        teardownAudioEngine()

        isInCall = false
        isAgentSpeaking = false
        liveTranscriptText = ""
        soundLevels = Array(repeating: 0.12, count: 18)
        if !preserveFailure {
            callState = .idle
            activeCascadeId = ""
        }
    }

    public func toggleMicMute() {
        isMicMuted.toggle()
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
    }

    public func toggleSpeakerMute() {
        isSpeakerMuted.toggle()
        playerNode?.volume = isSpeakerMuted ? 0.0 : 1.0
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
    }

    public func cyclePlaybackSpeed() {
        let speeds = Self.supportedSpeeds
        if let idx = speeds.firstIndex(of: playbackSpeed) {
            playbackSpeed = speeds[(idx + 1) % speeds.count]
        } else {
            playbackSpeed = 1.0
        }
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        sendSettingsUpdate(planModel: nil, playbackSpeed: playbackSpeed)
    }

    public func updatePlanModel(_ modelIdOrEnum: String?) {
        guard isInCall, let modelInt = Self.resolvePlanModelInt(modelIdOrEnum), modelInt > 0 else { return }
        sendSettingsUpdate(planModel: modelInt, playbackSpeed: nil)
    }

    // MARK: - WebSocket Connection (jetski-voice-proto)

    private func connectWebSocket(baseURL: URL, cascadeId: String, modelIdOrEnum: String?) {
        guard var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: true) else {
            callState = .failed("无效的网关地址")
            endCall(preserveFailure: true)
            return
        }

        let scheme = components.scheme?.lowercased()
        components.scheme = (scheme == "https") ? "wss" : "ws"

        var path = components.path
        if !path.hasSuffix("/") {
            path += "/"
        }
        path += "voice/ws"
        components.path = path

        let planModelInt = Self.resolvePlanModelInt(modelIdOrEnum) ?? 0
        components.queryItems = [
            URLQueryItem(name: "conversation_id", value: cascadeId),
            URLQueryItem(name: "plan_model", value: String(planModelInt)),
            URLQueryItem(name: "sample_rate", value: "16000")
        ]

        guard let wsURL = components.url else {
            callState = .failed("无法构建语音连接地址")
            endCall(preserveFailure: true)
            return
        }

        let hostStr = (wsURL.host ?? "").lowercased()
        let isCloudflare = CloudflareAnycastAccelerator.isCloudflareTunnelHost(hostStr)

        let endpoint = NWEndpoint.url(wsURL)
        let wsOptions = NWProtocolWebSocket.Options()
        wsOptions.autoReplyPing = true
        wsOptions.setSubprotocols(["jetski-voice-proto"])

        var extraHeaders: [(String, String)] = []
        if isCloudflare {
            extraHeaders.append(("Host", hostStr))
        }
        if let token = KeychainHelper.shared.read(key: .deviceToken) ?? AppSettings.shared.deviceToken, !token.isEmpty {
            extraHeaders.append(("Authorization", "Bearer \(token)"))
            extraHeaders.append(("x-device-token", token))
        }
        if !extraHeaders.isEmpty {
            wsOptions.setAdditionalHeaders(extraHeaders)
        }

        let parameters: NWParameters
        if wsURL.scheme?.lowercased() == "wss" {
            let tlsOptions = NWProtocolTLS.Options()
            let isLoopback = hostStr == "127.0.0.1" || hostStr == "::1" || hostStr == "localhost"
            if isLoopback {
                sec_protocol_options_set_verify_block(tlsOptions.securityProtocolOptions, { _, _, completion in
                    completion(true)
                }, .global())
            }
            if isCloudflare {
                sec_protocol_options_set_tls_server_name(tlsOptions.securityProtocolOptions, hostStr)
            }
            parameters = NWParameters(tls: tlsOptions)
        } else {
            parameters = NWParameters.tcp
        }
        parameters.defaultProtocolStack.applicationProtocols.insert(wsOptions, at: 0)

        let conn = NWConnection(to: endpoint, using: parameters)
        self.connection = conn

        conn.stateUpdateHandler = { [weak self] state in
            guard let self else { return }
            Task { @MainActor in
                self.handleConnectionState(state)
            }
        }

        conn.start(queue: .main)

        connectionWatchdogTask?.cancel()
        connectionWatchdogTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: 10_000_000_000)
            guard let self, !Task.isCancelled else { return }
            if !self.isSocketReady && !self.isIntentionallyClosed {
                self.callState = .failed("语音连接超时")
                self.endCall(preserveFailure: true)
            }
        }
    }

    private func handleConnectionState(_ state: NWConnection.State) {
        switch state {
        case .ready:
            connectionWatchdogTask?.cancel()
            connectionWatchdogTask = nil
            isSocketReady = true
            if callState == .connecting {
                callState = .listening
            }
            if playbackSpeed != 1.0 {
                sendSettingsUpdate(planModel: nil, playbackSpeed: playbackSpeed)
            }
            receiveNextMessage()

        case .failed(let error):
            print("[VoiceCall] WS failed: \(error)")
            if !isIntentionallyClosed {
                callState = .failed("语音连接中断")
                endCall(preserveFailure: true)
            }

        case .cancelled:
            if !isIntentionallyClosed {
                endCall()
            }

        default:
            break
        }
    }

    private func receiveNextMessage() {
        guard let conn = connection, !isIntentionallyClosed else { return }

        conn.receiveMessage { [weak self] data, context, _, error in
            guard let self else { return }
            Task { @MainActor in
                if let error {
                    if !self.isIntentionallyClosed {
                        print("[VoiceCall] Receive error: \(error)")
                        self.callState = .failed("语音通话已断开")
                        self.endCall(preserveFailure: true)
                    }
                    return
                }

                if let wsMeta = context?.protocolMetadata(definition: NWProtocolWebSocket.definition) as? NWProtocolWebSocket.Metadata {
                    if wsMeta.opcode == .close {
                        if !self.isIntentionallyClosed {
                            self.endCall()
                        }
                        return
                    }
                }

                if let data, !data.isEmpty {
                    self.handleServerProtobufFrame(data)
                }

                self.receiveNextMessage()
            }
        }
    }

    private func sendBinaryFrame(_ data: Data) {
        guard let conn = connection, isSocketReady, !isIntentionallyClosed else { return }
        let metadata = NWProtocolWebSocket.Metadata(opcode: .binary)
        let context = NWConnection.ContentContext(identifier: "voiceBinaryFrame", metadata: [metadata])
        conn.send(content: data, contentContext: context, isComplete: true, completion: .contentProcessed { _ in })
    }

    // MARK: - Audio Engine (16kHz Mic Input -> 640-frame PCM16LE, 24kHz PCM16LE -> Speaker)

    private func setupAudioEngine() throws {
        let session = AVAudioSession.sharedInstance()
        try session.setCategory(
            .playAndRecord,
            mode: .voiceChat,
            options: [.defaultToSpeaker, .allowBluetoothHFP, .allowBluetoothA2DP]
        )
        try session.setActive(true, options: .notifyOthersOnDeactivation)

        let engine = AVAudioEngine()
        let player = AVAudioPlayerNode()
        player.volume = isSpeakerMuted ? 0.0 : 1.0

        guard let outFormat = AVAudioFormat(
            commonFormat: .pcmFormatFloat32,
            sampleRate: 24000,
            channels: 1,
            interleaved: false
        ) else {
            throw NSError(domain: "VoiceCallManager", code: -1, userInfo: [NSLocalizedDescriptionKey: "Invalid playback format"])
        }

        engine.attach(player)
        engine.connect(player, to: engine.mainMixerNode, format: outFormat)

        let inputNode = engine.inputNode
        let hwInputFormat = inputNode.outputFormat(forBus: 0)
        guard hwInputFormat.sampleRate > 0, hwInputFormat.channelCount > 0 else {
            throw NSError(domain: "VoiceCallManager", code: -2, userInfo: [NSLocalizedDescriptionKey: "Invalid microphone format"])
        }

        guard let targetMicFormat = AVAudioFormat(
            commonFormat: .pcmFormatInt16,
            sampleRate: 16000,
            channels: 1,
            interleaved: true
        ), let converter = AVAudioConverter(from: hwInputFormat, to: targetMicFormat) else {
            throw NSError(domain: "VoiceCallManager", code: -3, userInfo: [NSLocalizedDescriptionKey: "Audio converter init failed"])
        }

        var pcmAccumulator = Data()
        pcmAccumulator.reserveCapacity(2560)
        let chunkByteSize = 1280 // 640 samples * 2 bytes = 40ms @ 16kHz

        inputNode.installTap(onBus: 0, bufferSize: 1024, format: hwInputFormat) { [weak self] buffer, _ in
            guard let self else { return }
            let ratio = 16000.0 / hwInputFormat.sampleRate
            let capacity = AVAudioFrameCount(Double(buffer.frameLength) * ratio + 32)
            guard let convertedBuffer = AVAudioPCMBuffer(pcmFormat: targetMicFormat, frameCapacity: capacity) else { return }

            var hasProvidedInput = false
            var convError: NSError?
            converter.convert(to: convertedBuffer, error: &convError) { _, outStatus in
                if hasProvidedInput {
                    outStatus.pointee = .noDataNow
                    return nil
                }
                hasProvidedInput = true
                outStatus.pointee = .haveData
                return buffer
            }

            let frameCount = Int(convertedBuffer.frameLength)
            guard frameCount > 0, let int16Ptr = convertedBuffer.int16ChannelData?[0] else { return }

            // Compute mic RMS for waveform when AI is not speaking
            var sumSquares: Float = 0
            for i in 0..<frameCount {
                let norm = Float(int16Ptr[i]) / 32768.0
                sumSquares += norm * norm
            }
            let rms = sqrt(sumSquares / Float(max(1, frameCount)))

            let rawBytes = Data(bytes: int16Ptr, count: frameCount * 2)
            pcmAccumulator.append(rawBytes)

            while pcmAccumulator.count >= chunkByteSize {
                let chunk = pcmAccumulator.prefix(chunkByteSize)
                pcmAccumulator.removeFirst(chunkByteSize)
                let chunkData = Data(chunk)

                Task { @MainActor [weak self] in
                    guard let self, self.isInCall else { return }
                    if !self.isAgentSpeaking {
                        let level = self.isMicMuted ? CGFloat(0.10) : min(1.0, max(0.12, CGFloat(rms * 5.2)))
                        self.pushSoundLevel(level)
                    }
                    let payload = self.isMicMuted ? Data(count: chunkByteSize) : chunkData
                    let protoFrame = Self.encodeMicAudioFrame(payload)
                    self.sendBinaryFrame(protoFrame)
                }
            }
        }

        engine.prepare()
        try engine.start()
        player.play()

        self.audioEngine = engine
        self.playerNode = player
        self.playbackFormat = outFormat
    }

    private func teardownAudioEngine() {
        if let engine = audioEngine {
            engine.inputNode.removeTap(onBus: 0)
            playerNode?.stop()
            engine.stop()
        }
        playerNode = nil
        audioEngine = nil
        playbackFormat = nil
        pendingPlaybackBuffers = 0
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
    }

    private func enqueuePCM24k(_ pcmData: Data) {
        let sampleCount = pcmData.count / 2
        guard sampleCount > 0,
              let format = playbackFormat,
              let player = playerNode,
              let buffer = AVAudioPCMBuffer(pcmFormat: format, frameCapacity: AVAudioFrameCount(sampleCount)),
              let floatChannel = buffer.floatChannelData?[0] else { return }

        buffer.frameLength = AVAudioFrameCount(sampleCount)

        var sumSquares: Float = 0
        pcmData.withUnsafeBytes { rawBuf in
            guard let base = rawBuf.baseAddress?.assumingMemoryBound(to: Int16.self) else { return }
            for i in 0..<sampleCount {
                let s = Int16(littleEndian: base[i])
                let f = Float(s) / 32768.0
                floatChannel[i] = f
                sumSquares += f * f
            }
        }

        let rms = sqrt(sumSquares / Float(max(1, sampleCount)))
        let visualLevel = min(1.0, max(0.16, CGFloat(rms * 4.5)))
        isAgentSpeaking = true
        pushSoundLevel(visualLevel)

        pendingPlaybackBuffers += 1
        if !player.isPlaying {
            player.play()
        }

        player.scheduleBuffer(buffer) { [weak self] in
            Task { @MainActor [weak self] in
                guard let self else { return }
                self.pendingPlaybackBuffers = max(0, self.pendingPlaybackBuffers - 1)
                if self.pendingPlaybackBuffers == 0 {
                    self.agentSpeakingDecayTask?.cancel()
                    self.agentSpeakingDecayTask = Task { [weak self] in
                        try? await Task.sleep(nanoseconds: 250_000_000)
                        guard let self, !Task.isCancelled else { return }
                        if self.pendingPlaybackBuffers == 0 {
                            self.isAgentSpeaking = false
                        }
                    }
                }
            }
        }
    }

    private func clearPlaybackQueue() {
        guard let player = playerNode else { return }
        player.stop()
        pendingPlaybackBuffers = 0
        isAgentSpeaking = false
        if audioEngine?.isRunning == true {
            player.play()
        }
    }

    private func pushSoundLevel(_ level: CGFloat) {
        var next = soundLevels
        if !next.isEmpty {
            next.removeFirst()
        }
        next.append(level)
        soundLevels = next
    }

    private func startDurationTimer() {
        durationTimer?.invalidate()
        durationTimer = Timer.scheduledTimer(withTimeInterval: 0.5, repeats: true) { [weak self] _ in
            Task { @MainActor [weak self] in
                guard let self, let started = self.callStartedAt, self.isInCall else { return }
                self.duration = Date().timeIntervalSince(started)
            }
        }
    }

    private func updateLiveTranscript(text: String, role: TranscriptRole) {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        liveTranscriptRole = role
        liveTranscriptText = trimmed

        transcriptClearTask?.cancel()
        transcriptClearTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: 3_200_000_000)
            guard let self, !Task.isCancelled else { return }
            self.liveTranscriptText = ""
        }
    }

    // MARK: - Protobuf Wire Encoding / Decoding (exa.remote_agent_proxy_pb)

    public static func resolvePlanModelInt(_ modelIdOrEnum: String?) -> Int? {
        guard let raw = modelIdOrEnum?.trimmingCharacters(in: .whitespacesAndNewlines), !raw.isEmpty else {
            return nil
        }
        if let direct = Int(raw), direct > 0 {
            return direct
        }
        if raw.hasPrefix("MODEL_PLACEHOLDER_M"),
           let num = Int(raw.dropFirst("MODEL_PLACEHOLDER_M".count)), num > 0 {
            return 1000 + num
        }
        let lower = raw.lowercased()
        switch lower {
        case "gemini-3.8-flash-high": return 1318
        case "gemini-3.8-flash-medium": return 1319
        case "gemini-3.8-flash-low": return 1320
        case "gemini-3.1-pro-high": return 1037
        case "gemini-3.1-pro-low": return 1036
        case "claude-opus-4-6-thinking": return 1026
        case "claude-sonnet-4-6": return 1035
        default: return nil
        }
    }

    /// VoiceClientMessage { bytes mic_audio = 2; }
    private static func encodeMicAudioFrame(_ pcmBytes: Data) -> Data {
        var out = Data()
        out.reserveCapacity(pcmBytes.count + 5)
        out.append(0x12) // field 2, wire type 2 (length-delimited)
        appendVarint(UInt64(pcmBytes.count), to: &out)
        out.append(pcmBytes)
        return out
    }

    /// VoiceClientMessage { SettingsUpdate settings_update = 3; }
    /// SettingsUpdate { int32 plan_model = 1; double playback_speed = 2; }
    private func sendSettingsUpdate(planModel: Int?, playbackSpeed: Double?) {
        var inner = Data()
        if let pm = planModel, pm > 0 {
            inner.append(0x08) // field 1, varint
            Self.appendVarint(UInt64(pm), to: &inner)
        }
        if let speed = playbackSpeed {
            inner.append(0x11) // field 2, 64-bit float (wire type 1)
            var bits = speed.bitPattern.littleEndian
            withUnsafeBytes(of: &bits) { inner.append(contentsOf: $0) }
        }
        guard !inner.isEmpty else { return }

        var outer = Data()
        outer.append(0x1A) // field 3, length-delimited
        Self.appendVarint(UInt64(inner.count), to: &outer)
        outer.append(inner)
        sendBinaryFrame(outer)
    }

    private static func appendVarint(_ value: UInt64, to data: inout Data) {
        var v = value
        while v >= 0x80 {
            data.append(UInt8((v & 0x7F) | 0x80))
            v >>= 7
        }
        data.append(UInt8(v))
    }

    private func handleServerProtobufFrame(_ data: Data) {
        var offset = 0
        let bytes = [UInt8](data)
        let count = bytes.count

        while offset < count {
            guard let tag = Self.readVarint(bytes, offset: &offset) else { break }
            let fieldNumber = Int(tag >> 3)
            let wireType = Int(tag & 0x07)

            switch wireType {
            case 0: // Varint
                _ = Self.readVarint(bytes, offset: &offset)
            case 1: // 64-bit
                offset += 8
            case 5: // 32-bit
                offset += 4
            case 2: // Length-delimited
                guard let lenU64 = Self.readVarint(bytes, offset: &offset) else { return }
                let length = Int(lenU64)
                guard length >= 0, offset + length <= count else { return }
                let subSlice = Data(bytes[offset..<(offset + length)])
                offset += length

                switch fieldNumber {
                case 1: // voice_audio (24kHz PCM16LE)
                    enqueuePCM24k(subSlice)
                case 2: // status (SessionStatus)
                    handleSessionStatus(subSlice)
                case 7: // voice_transcript (VoiceTranscript)
                    if let text = Self.extractFirstStringField(subSlice) {
                        updateLiveTranscript(text: text, role: .ai)
                    }
                case 8: // user_speech (UserSpeech)
                    if let text = Self.extractFirstStringField(subSlice) {
                        updateLiveTranscript(text: text, role: .user)
                    }
                case 9: // clear_audio (ClearAudio)
                    clearPlaybackQueue()
                default:
                    break
                }
            default:
                return
            }
        }
    }

    private func handleSessionStatus(_ statusData: Data) {
        let bytes = [UInt8](statusData)
        var offset = 0
        var stateVal: Int = 0
        var messageText: String = ""
        var returnedCascadeId: String = ""

        while offset < bytes.count {
            guard let tag = Self.readVarint(bytes, offset: &offset) else { break }
            let field = Int(tag >> 3)
            let wt = Int(tag & 0x07)
            switch wt {
            case 0:
                if let v = Self.readVarint(bytes, offset: &offset) {
                    if field == 1 { stateVal = Int(v) }
                }
            case 2:
                guard let lenU64 = Self.readVarint(bytes, offset: &offset) else { return }
                let len = Int(lenU64)
                guard len >= 0, offset + len <= bytes.count else { return }
                let slice = Data(bytes[offset..<(offset + len)])
                offset += len
                if field == 3 {
                    messageText = String(data: slice, encoding: .utf8) ?? ""
                } else if field == 6 {
                    returnedCascadeId = String(data: slice, encoding: .utf8) ?? ""
                }
            case 1:
                offset += 8
            case 5:
                offset += 4
            default:
                return
            }
        }

        if !returnedCascadeId.isEmpty && returnedCascadeId != activeCascadeId {
            activeCascadeId = returnedCascadeId
            onCascadeBound?(returnedCascadeId)
        }

        switch stateVal {
        case 1, 2: // STARTING, LISTENING
            callState = .listening
        case 3: // RETRYING
            callState = .reconnecting
        case 4: // STOPPING
            endCall()
        case 5: // FAILED
            let errMsg = messageText.isEmpty ? "语音会话启动失败" : messageText
            callState = .failed(errMsg)
            endCall(preserveFailure: true)
        default:
            break
        }
    }

    private static func extractFirstStringField(_ data: Data) -> String? {
        let bytes = [UInt8](data)
        var offset = 0
        while offset < bytes.count {
            guard let tag = readVarint(bytes, offset: &offset) else { break }
            let field = Int(tag >> 3)
            let wt = Int(tag & 0x07)
            switch wt {
            case 0:
                _ = readVarint(bytes, offset: &offset)
            case 1:
                offset += 8
            case 5:
                offset += 4
            case 2:
                guard let lenU64 = readVarint(bytes, offset: &offset) else { return nil }
                let len = Int(lenU64)
                guard len >= 0, offset + len <= bytes.count else { return nil }
                if field == 1 {
                    return String(data: Data(bytes[offset..<(offset + len)]), encoding: .utf8)
                }
                offset += len
            default:
                return nil
            }
        }
        return nil
    }

    private static func readVarint(_ bytes: [UInt8], offset: inout Int) -> UInt64? {
        var result: UInt64 = 0
        var shift: UInt64 = 0
        while offset < bytes.count {
            let b = bytes[offset]
            offset += 1
            result |= UInt64(b & 0x7F) << shift
            if (b & 0x80) == 0 {
                return result
            }
            shift += 7
            if shift >= 64 { return nil }
        }
        return nil
    }
}
