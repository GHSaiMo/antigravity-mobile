import Foundation
import Combine
import AVFoundation

public final class AudioRecorderManager: NSObject, ObservableObject, AVAudioRecorderDelegate {
    @Published public var isRecording: Bool = false
    @Published public var isPaused: Bool = false
    @Published public var duration: TimeInterval = 0
    @Published public var soundLevels: [CGFloat] = Array(repeating: 0.12, count: 16)
    
    private var audioRecorder: AVAudioRecorder?
    private var timer: Timer?
    private var currentFileURL: URL?
    
    public override init() {
        super.init()
    }
    
    public func startRecording(completion: @escaping (Bool) -> Void) {
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
        } else {
            recorder.record()
            isPaused = false
        }
    }
    
    public func stopRecording() -> URL? {
        timer?.invalidate()
        timer = nil
        audioRecorder?.stop()
        audioRecorder = nil
        isRecording = false
        isPaused = false
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        return currentFileURL
    }
    
    public func cancelRecording() {
        timer?.invalidate()
        timer = nil
        audioRecorder?.stop()
        audioRecorder = nil
        isRecording = false
        isPaused = false
        if let url = currentFileURL {
            try? FileManager.default.removeItem(at: url)
        }
        currentFileURL = nil
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
    }
}
