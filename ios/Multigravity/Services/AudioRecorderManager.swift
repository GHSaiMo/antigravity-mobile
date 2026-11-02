import Foundation
import Combine
import AVFoundation

extension Notification.Name {
    public static let audioRecordingDidFinishFromLiveActivity = Notification.Name("audioRecordingDidFinishFromLiveActivity")
}

@MainActor
public final class AudioRecorderManager: NSObject, ObservableObject, AVAudioRecorderDelegate {
    public static let shared = AudioRecorderManager()
    
    @Published public var isRecording: Bool = false
    @Published public var isPaused: Bool = false
    @Published public var duration: TimeInterval = 0
    @Published public var soundLevels: [CGFloat] = Array(repeating: 0.12, count: 16)
    
    public var currentCascadeId: String = ""
    public var currentConversationTitle: String = ""
    public var recordingStartDate: Date?
    
    public struct PendingLiveRecording: Sendable {
        public let url: URL
        public let sendImmediately: Bool
        public let cascadeId: String
    }
    @Published public var pendingLiveRecording: PendingLiveRecording?
    
    private var audioRecorder: AVAudioRecorder?
    private var timer: Timer?
    private var currentFileURL: URL?
    
    public override init() {
        super.init()
    }
    
    public func startRecording(
        cascadeId: String = "",
        conversationTitle: String = "",
        completion: @escaping (Bool) -> Void
    ) {
        self.currentCascadeId = cascadeId
        self.currentConversationTitle = conversationTitle
        
        let session = AVAudioSession.sharedInstance()
        if #available(iOS 17.0, *) {
            AVAudioApplication.requestRecordPermission { [weak self] allowed in
                DispatchQueue.main.async {
                    if allowed {
                        self?.beginRecording()
                        completion(true)
                    } else {
                        completion(false)
                    }
                }
            }
        } else {
            session.requestRecordPermission { [weak self] allowed in
                DispatchQueue.main.async {
                    if allowed {
                        self?.beginRecording()
                        completion(true)
                    } else {
                        completion(false)
                    }
                }
            }
        }
    }
    
    private func beginRecording() {
        let session = AVAudioSession.sharedInstance()
        do {
            try session.setCategory(.playAndRecord, mode: .default, options: [.defaultToSpeaker, .allowBluetoothHFP])
            try session.setActive(true)
            
            let tempDir = FileManager.default.temporaryDirectory
            let formatter = DateFormatter()
            formatter.dateFormat = "yyyyMMdd_HHmmss"
            let fileName = "录音_\(formatter.string(from: Date())).m4a"
            let fileURL = tempDir.appendingPathComponent(fileName)
            self.currentFileURL = fileURL
            
            // AAC-LC 压缩格式配置，单声道，64kbps，体积仅约 300KB/分钟
            let settings: [String: Any] = [
                AVFormatIDKey: Int(kAudioFormatMPEG4AAC),
                AVSampleRateKey: 44100.0,
                AVNumberOfChannelsKey: 1,
                AVEncoderAudioQualityKey: AVAudioQuality.high.rawValue,
                AVEncoderBitRateKey: 64000
            ]
            
            let recorder = try AVAudioRecorder(url: fileURL, settings: settings)
            recorder.delegate = self
            recorder.isMeteringEnabled = true
            if recorder.record() {
                self.audioRecorder = recorder
                self.isRecording = true
                self.isPaused = false
                self.duration = 0
                self.soundLevels = Array(repeating: 0.12, count: 16)
                self.recordingStartDate = Date()
                
                // 启动灵动岛与锁屏实时活动
                ActivityManager.shared.startRecordingActivity(
                    cascadeId: self.currentCascadeId,
                    title: self.currentConversationTitle,
                    startDate: self.recordingStartDate ?? Date()
                )
                
                NotificationCenter.default.post(name: .conversationDraftChanged, object: self.currentCascadeId)
                
                startMonitoring()
            }
        } catch {
            print("Failed to record audio: \(error)")
        }
    }
    
    private func startMonitoring() {
        timer?.invalidate()
        timer = Timer.scheduledTimer(withTimeInterval: 0.05, repeats: true) { [weak self] _ in
            guard let self = self, let recorder = self.audioRecorder, recorder.isRecording else { return }
            self.duration = recorder.currentTime
            recorder.updateMeters()
            let power = recorder.averagePower(forChannel: 0) // range -160 to 0 dB
            let clamped = max(-50.0, min(0.0, Double(power)))
            let normalized = CGFloat((clamped + 50.0) / 50.0) // 0.0 ~ 1.0
            let target = max(0.12, normalized)
            
            var levels = self.soundLevels
            levels.removeFirst()
            levels.append(target)
            self.soundLevels = levels
        }
    }
    
    public func togglePause() {
        guard let recorder = audioRecorder else { return }
        if recorder.isRecording {
            recorder.pause()
            isPaused = true
            ActivityManager.shared.updateRecordingActivity(isPaused: true, pausedDuration: duration)
        } else {
            recorder.record()
            isPaused = false
            ActivityManager.shared.updateRecordingActivity(isPaused: false, pausedDuration: duration)
        }
    }
    
    public func stopRecording() -> URL? {
        let finishedCascadeId = self.currentCascadeId
        timer?.invalidate()
        timer = nil
        audioRecorder?.stop()
        audioRecorder = nil
        isRecording = false
        isPaused = false
        ActivityManager.shared.endRecordingActivity()
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        NotificationCenter.default.post(name: .conversationDraftChanged, object: finishedCascadeId)
        return currentFileURL
    }
    
    public func cancelRecording() {
        let canceledCascadeId = self.currentCascadeId
        timer?.invalidate()
        timer = nil
        audioRecorder?.stop()
        audioRecorder = nil
        isRecording = false
        isPaused = false
        ActivityManager.shared.endRecordingActivity()
        if let url = currentFileURL {
            try? FileManager.default.removeItem(at: url)
        }
        currentFileURL = nil
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        NotificationCenter.default.post(name: .conversationDraftChanged, object: canceledCascadeId)
    }
    
    public func finishRecordingFromLiveActivity(sendImmediately: Bool, targetCascadeId: String) -> URL? {
        guard isRecording else { return nil }
        let cascadeId = !targetCascadeId.isEmpty ? targetCascadeId : self.currentCascadeId
        guard let fileURL = stopRecording() else { return nil }
        
        self.pendingLiveRecording = PendingLiveRecording(
            url: fileURL,
            sendImmediately: sendImmediately,
            cascadeId: cascadeId
        )
        
        NotificationCenter.default.post(
            name: .audioRecordingDidFinishFromLiveActivity,
            object: nil,
            userInfo: [
                "url": fileURL,
                "sendImmediately": sendImmediately,
                "cascadeId": cascadeId
            ]
        )
        return fileURL
    }
}
