import AVFoundation
import Foundation
import Observation

/// 原生流式语音听写管理器 (/gateway/audio/dictate)
@MainActor
@Observable
public final class AudioDictationManager {
    public private(set) var isDictating = false

    private var audioEngine: AVAudioEngine?
    private var webSocketTask: GatewayWebSocketTask?
    private var isReceiving = false
    private var onTranscriptUpdate: ((String) -> Void)?
    private var onError: ((String) -> Void)?

    // 句段多句累加状态机（防止停顿后新一句话冲掉前面已定稿的句子）
    private var committedText: String = ""
    private var currentInterim: String = ""

    public init() {}

    public func start(
        cascadeId: String,
        baseURL: URL,
        initialText: String = "",
        onTranscriptUpdate: @escaping (String) -> Void,
        onError: @escaping (String) -> Void
    ) {
        if isDictating { return }

        self.committedText = initialText.trimmingCharacters(in: .whitespacesAndNewlines)
        self.currentInterim = ""

        let cid = cascadeId.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? cascadeId
        let mime = "audio/pcm;rate=16000".addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? "audio/pcm%3Brate%3D16000"
        guard let request = APIClient.shared.authorizedWebSocketRequest(
            pathAndQuery: "gateway/audio/dictate?cascadeId=\(cid)&mimeType=\(mime)",
            baseURL: baseURL
        ) else {
            onError(String(localized: "无法构建语音识别连接"))
            return
        }

        self.onTranscriptUpdate = onTranscriptUpdate
        self.onError = onError

        let session = AVAudioSession.sharedInstance()
        do {
            try session.setCategory(.playAndRecord, mode: .measurement, options: [.defaultToSpeaker, .allowBluetoothHFP])
            try session.setActive(true)
        } catch {
            onError(String(localized: "无法配置音频会话: \(error.localizedDescription)"))
            return
        }

        let engine = AVAudioEngine()
        let inputNode = engine.inputNode
        let hwFormat = inputNode.outputFormat(forBus: 0)
        guard let targetFormat = AVAudioFormat(
            commonFormat: .pcmFormatInt16,
            sampleRate: 16000,
            channels: 1,
            interleaved: true
        ) else {
            onError(String(localized: "不支持 16kHz PCM 音频格式"))
            return
        }
        guard let converter = AVAudioConverter(from: hwFormat, to: targetFormat) else {
            onError(String(localized: "音频采样率转换初始化失败"))
            return
        }

        let ws = GatewayWebSocketTask(
            request: request,
            requestTimeout: 15,
            resourceTimeout: 600
        )

        self.webSocketTask = ws
        self.audioEngine = engine
        self.isDictating = true
        self.isReceiving = true

        ws.resume()
        receiveNextMessage()

        let wsRef = ws
        inputNode.installTap(onBus: 0, bufferSize: 2048, format: hwFormat) { buffer, _ in
            let ratio = targetFormat.sampleRate / max(1.0, hwFormat.sampleRate)
            let cap = AVAudioFrameCount(max(256, Int(Double(buffer.frameLength) * ratio) + 64))
            guard let outBuf = AVAudioPCMBuffer(pcmFormat: targetFormat, frameCapacity: cap) else { return }
            var err: NSError?
            var consumed = false
            let status = converter.convert(to: outBuf, error: &err) { _, outStatus in
                if consumed {
                    outStatus.pointee = .noDataNow
                    return nil
                }
                consumed = true
                outStatus.pointee = .haveData
                return buffer
            }
            guard status != .error, err == nil, outBuf.frameLength > 0,
                  let ch = outBuf.int16ChannelData?[0] else { return }
            let byteCount = Int(outBuf.frameLength) * MemoryLayout<Int16>.size
            let data = Data(bytes: ch, count: byteCount)
            wsRef.send(.data(data)) { _ in }
        }

        do {
            try engine.start()
        } catch {
            onError(String(localized: "启动麦克风失败: \(error.localizedDescription)"))
            stop()
        }
    }

    public func stop() {
        guard isDictating || isReceiving || audioEngine != nil || webSocketTask != nil else { return }
        isDictating = false
        if let engine = audioEngine {
            engine.inputNode.removeTap(onBus: 0)
            engine.stop()
        }
        audioEngine = nil
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)

        if let ws = webSocketTask, isReceiving {
            ws.send(.string("{\"type\":\"end\"}")) { [weak self] _ in
                // 保留接收窗口，等待上游返回最终识别结果及 complete 标志
                DispatchQueue.main.asyncAfter(deadline: .now() + 1.2) { [weak self] in
                    self?.cleanupWebSocket()
                }
            }
        } else {
            cleanupWebSocket()
        }
    }

    private func cleanupWebSocket() {
        guard isReceiving || webSocketTask != nil else { return }
        isReceiving = false
        webSocketTask?.cancel()
        webSocketTask = nil

        // 若连接关闭时仍有尚未定稿的临时文本，自动提交定稿
        if !currentInterim.isEmpty {
            committedText = Self.joinText(committedText, currentInterim)
            currentInterim = ""
            onTranscriptUpdate?(committedText)
        }
    }

    private func receiveNextMessage() {
        guard isReceiving, let ws = webSocketTask else { return }
        ws.receive { [weak self] result in
            Task { @MainActor [weak self] in
                guard let s = self, s.isReceiving else { return }
                switch result {
                case .success(let message):
                    if case .string(let text) = message {
                        s.handleServerJSON(text)
                    } else if case .data(let data) = message, let text = String(data: data, encoding: .utf8) {
                        s.handleServerJSON(text)
                    }
                    s.receiveNextMessage()
                case .failure(let error):
                    if s.isDictating {
                        s.onError?(String(localized: "语音识别连接中断: \(error.localizedDescription)"))
                    }
                    s.cleanupWebSocket()
                }
            }
        }
    }

    private func handleServerJSON(_ raw: String) {
        guard let data = raw.data(using: .utf8),
              let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return }
        if let err = obj["error"] as? String, !err.isEmpty {
            onError?(err)
            cleanupWebSocket()
            return
        }
        if let type = obj["type"] as? String, type == "complete" {
            cleanupWebSocket()
            return
        }
        let direct = (obj["text"] as? String) ?? (obj["transcript"] as? String) ?? ""
        let transcriptionNested = ((obj["transcription"] as? [String: Any])?["text"] as? String) ?? ""
        let updateNested = ((obj["update"] as? [String: Any])?["text"] as? String) ?? ""
        let resolved = !direct.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            ? direct
            : (!transcriptionNested.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? transcriptionNested : updateNested)
        
        let isFinalDirect = obj["isFinal"] as? Bool
        let isFinalNested = (obj["transcription"] as? [String: Any])?["isFinal"] as? Bool
        let isFinal = isFinalDirect ?? isFinalNested ?? false

        if !resolved.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            if isFinal {
                // 当前句子已定稿：提交进 committedText，并清空临时区
                committedText = Self.joinText(committedText, resolved)
                currentInterim = ""
                onTranscriptUpdate?(committedText)
            } else {
                // 当前句子正在流式识别中：拼在已定稿文本后面实时预览
                currentInterim = resolved
                let merged = Self.joinText(committedText, currentInterim)
                onTranscriptUpdate?(merged)
            }
        }
    }

    private static func joinText(_ base: String, _ append: String) -> String {
        let b = base.trimmingCharacters(in: .whitespacesAndNewlines)
        let a = append.trimmingCharacters(in: .whitespacesAndNewlines)
        if b.isEmpty { return a }
        if a.isEmpty { return b }
        if base.hasSuffix(" ") || base.hasSuffix("\n") || append.hasPrefix(" ") {
            return base + append
        }
        guard let lastB = b.last, let firstA = a.first else { return "\(b) \(a)" }
        if isCJK(lastB) || isCJK(firstA) {
            return "\(b)\(a)"
        }
        return "\(b) \(a)"
    }

    private static func isCJK(_ c: Character) -> Bool {
        guard let scalar = c.unicodeScalars.first else { return false }
        let v = scalar.value
        return (v >= 0x4E00 && v <= 0x9FFF) ||
               (v >= 0x3400 && v <= 0x4DBF) ||
               (v >= 0x3000 && v <= 0x303F) ||
               (v >= 0xFF00 && v <= 0xFFEF)
    }
}

/// 消息旁白语音播报播放器 (/gateway/narration + 系统 AVSpeechSynthesizer 兜底)
@MainActor
@Observable
public final class NarrationAudioPlayer: NSObject, AVAudioPlayerDelegate, AVSpeechSynthesizerDelegate {
    public private(set) var isPlaying = false
    public private(set) var statusLabel = ""

    private var audioPlayer: AVAudioPlayer?
    private var synthesizer: AVSpeechSynthesizer?
    private var playTask: Task<Void, Never>?

    public override init() {
        super.init()
    }

    public func play(text: String, baseURL: URL?) {
        let cleaned = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cleaned.isEmpty else { return }
        stop()
        isPlaying = true
        statusLabel = String(localized: "正在合成语音…")

        playTask = Task {
            if let baseURL,
               let audioData = try? await APIClient.shared.synthesizeNarrationAudio(text: cleaned, baseURL: baseURL),
               !Task.isCancelled {
                do {
                    try AVAudioSession.sharedInstance().setCategory(.playback, mode: .spokenAudio)
                    try AVAudioSession.sharedInstance().setActive(true)
                    let player = try AVAudioPlayer(data: audioData)
                    player.delegate = self
                    self.audioPlayer = player
                    self.statusLabel = String(localized: "语音播报中")
                    player.play()
                    return
                } catch {
                    // fallback to system TTS
                }
            }
            if !Task.isCancelled {
                speakWithSystemTTS(cleaned)
            }
        }
    }

    public func stop() {
        playTask?.cancel()
        playTask = nil
        audioPlayer?.stop()
        audioPlayer = nil
        synthesizer?.stopSpeaking(at: .immediate)
        synthesizer = nil
        isPlaying = false
        statusLabel = ""
    }

    private static func preferredTTSLanguageCode() -> String {
        let lang = (Bundle.main.preferredLocalizations.first ?? Locale.preferredLanguages.first ?? "en").lowercased()
        if lang.hasPrefix("zh") {
            return "zh-CN"
        }
        if lang.hasPrefix("ja") {
            return "ja-JP"
        }
        return "en-US"
    }

    private func speakWithSystemTTS(_ text: String) {
        statusLabel = String(localized: "系统朗读中")
        try? AVAudioSession.sharedInstance().setCategory(.playback, mode: .spokenAudio)
        try? AVAudioSession.sharedInstance().setActive(true)
        let synth = AVSpeechSynthesizer()
        synth.delegate = self
        let utterance = AVSpeechUtterance(string: text)
        utterance.voice = AVSpeechSynthesisVoice(language: Self.preferredTTSLanguageCode())
        utterance.rate = AVSpeechUtteranceDefaultSpeechRate
        self.synthesizer = synth
        synth.speak(utterance)
    }

    nonisolated public func audioPlayerDidFinishPlaying(_ player: AVAudioPlayer, successfully flag: Bool) {
        Task { @MainActor in
            self.stop()
        }
    }

    nonisolated public func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didFinish utterance: AVSpeechUtterance) {
        Task { @MainActor in
            self.stop()
        }
    }
}
