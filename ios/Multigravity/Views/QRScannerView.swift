import SwiftUI
import AVFoundation

public struct QRScannerView: View {
    @Environment(\.dismiss) private var dismiss
    
    public var onScanDetected: ((PairingInfo) -> Void)?
    
    @State private var hasCameraPermission = false
    @State private var isCheckingPermission = true
    @State private var permissionDenied = false
    @State private var cameraRestricted = false
    @State private var showManualInput = false
    @State private var errorMessage: String? = nil
    
    public init(onScanDetected: ((PairingInfo) -> Void)? = nil) {
        self.onScanDetected = onScanDetected
    }
    
    public var body: some View {
        NavigationStack {
            ZStack {
                Color.black.ignoresSafeArea()
                
                if isCheckingPermission {
                    ProgressView()
                        .tint(.white)
                } else if permissionDenied {
                    VStack(spacing: 16) {
                        Image(systemName: cameraRestricted ? "video.slash.fill" : "camera.fill")
                            .font(.system(size: 48))
                            .foregroundColor(.gray)
                        Text(cameraRestricted
                             ? String(localized: "相机已被设备策略限制")
                             : String(localized: "需要相机权限以扫描配对二维码"))
                            .font(.headline)
                            .foregroundColor(.white)
                            .multilineTextAlignment(.center)
                            .padding(.horizontal, 24)
                        Text(cameraRestricted
                             ? String(localized: "无法使用相机扫码，请直接粘贴电脑终端输出的配对链接完成配对")
                             : String(localized: "请在系统设置中允许 Multigravity 访问相机，或直接使用配对链接配对"))
                            .font(.subheadline)
                            .foregroundColor(.gray)
                            .multilineTextAlignment(.center)
                            .padding(.horizontal, 32)
                        
                        // Quick native PasteButton card: zero permission alert, one tap to pair
                        HStack(spacing: 12) {
                            Image(systemName: "doc.on.clipboard.fill")
                                .font(.system(size: 16))
                                .foregroundColor(.indigo)
                            VStack(alignment: .leading, spacing: 2) {
                                Text("粘贴配对链接")
                                    .font(.system(size: 14, weight: .semibold))
                                    .foregroundColor(.white)
                                Text("一键导入终端输出的 agy://pair 链接")
                                    .font(.system(size: 11.5))
                                    .foregroundColor(.white.opacity(0.72))
                                    .lineLimit(2)
                                    .fixedSize(horizontal: false, vertical: true)
                            }
                            Spacer(minLength: 4)
                            PasteButton(payloadType: String.self) { strings in
                                handlePastedPayloads(strings)
                            }
                            .buttonBorderShape(.capsule)
                            .controlSize(.small)
                            .tint(.indigo)
                        }
                        .padding(.horizontal, 14)
                        .padding(.vertical, 12)
                        .background(
                            RoundedRectangle(cornerRadius: 14, style: .continuous)
                                .fill(Color.white.opacity(0.12))
                        )
                        .overlay(
                            RoundedRectangle(cornerRadius: 14, style: .continuous)
                                .stroke(Color.white.opacity(0.18), lineWidth: 1)
                        )
                        .padding(.horizontal, 28)
                        .padding(.top, 8)
                        
                        Button(action: { showManualInput = true }) {
                            HStack(spacing: 6) {
                                Image(systemName: "keyboard")
                                Text("手动输入或粘贴配对链接")
                            }
                            .font(.system(size: 14.5, weight: .semibold))
                            .foregroundColor(.white)
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 12)
                            .background(cameraRestricted ? Color.indigo : Color.white.opacity(0.16))
                            .cornerRadius(12)
                            .overlay(
                                RoundedRectangle(cornerRadius: 12)
                                    .stroke(Color.white.opacity(cameraRestricted ? 0 : 0.22), lineWidth: 1)
                            )
                        }
                        .buttonStyle(.plain)
                        .padding(.horizontal, 28)
                        
                        if !cameraRestricted {
                            Button("打开系统设置") {
                                if let url = URL(string: UIApplication.openSettingsURLString) {
                                    UIApplication.shared.open(url)
                                }
                            }
                            .font(.system(size: 14, weight: .medium))
                            .foregroundColor(.white.opacity(0.85))
                            .padding(.top, 4)
                        }
                    }
                } else {
                    // Camera live viewfinder
                    QRCameraRepresentable { code in
                        handleScannedCode(code)
                    }
                    .ignoresSafeArea()
                    
                    // Visual scanning overlay
                    scannerOverlayView
                }
            }
            .navigationTitle("扫码配对")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("取消") {
                        dismiss()
                    }
                    .foregroundColor(.white)
                }
                
                ToolbarItem(placement: .primaryAction) {
                    Button(action: { showManualInput = true }) {
                        Image(systemName: "keyboard")
                            .foregroundColor(.white)
                    }
                }
            }
            .sheet(isPresented: $showManualInput) {
                ManualPairingSheet { info in
                    showManualInput = false
                    dismiss()
                    onScanDetected?(info)
                }
            }
            .alert("配对错误", isPresented: Binding(
                get: { errorMessage != nil },
                set: { if !$0 { errorMessage = nil } }
            )) {
                Button("确定", role: .cancel) {}
            } message: {
                Text(errorMessage ?? "")
            }
        }
        .task {
            await checkCameraPermission()
        }
    }
    
    private var scannerOverlayView: some View {
        GeometryReader { geometry in
            let boxSize = min(250, max(160, min(geometry.size.width - 64, geometry.size.height - 170)))
            VStack {
                Spacer()
                
                // Viewfinder square
                ZStack {
                    RoundedRectangle(cornerRadius: 16)
                        .stroke(Color.white.opacity(0.8), lineWidth: 3)
                        .frame(width: boxSize, height: boxSize)
                    
                    // Corner accents
                    RoundedRectangle(cornerRadius: 16)
                        .stroke(Color.indigo, lineWidth: 5)
                        .frame(width: boxSize, height: boxSize)
                }
                
                Text("对准电脑终端中的配对二维码")
                    .font(.system(size: 14, weight: .medium))
                    .foregroundColor(.white)
                    .padding(.top, 20)
                    .shadow(radius: 4)
                
                Spacer()
                
                Button(action: { showManualInput = true }) {
                    HStack {
                        Image(systemName: "pencil.line")
                        Text("手动输入或粘贴配对链接")
                    }
                    .font(.system(size: 14, weight: .medium))
                    .foregroundColor(.white)
                    .padding(.horizontal, 20)
                    .padding(.vertical, 12)
                    .background(Color.white.opacity(0.2))
                    .cornerRadius(24)
                }
                .padding(.bottom, 24)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }
    
    private func checkCameraPermission() async {
        let status = AVCaptureDevice.authorizationStatus(for: .video)
        switch status {
        case .authorized:
            hasCameraPermission = true
            cameraRestricted = false
            permissionDenied = false
            isCheckingPermission = false
        case .notDetermined:
            let granted = await AVCaptureDevice.requestAccess(for: .video)
            hasCameraPermission = granted
            cameraRestricted = false
            permissionDenied = !granted
            isCheckingPermission = false
        case .restricted:
            cameraRestricted = true
            permissionDenied = true
            isCheckingPermission = false
        case .denied:
            cameraRestricted = false
            permissionDenied = true
            isCheckingPermission = false
        @unknown default:
            cameraRestricted = false
            permissionDenied = true
            isCheckingPermission = false
        }
    }

    private func handlePastedPayloads(_ strings: [String]) {
        let combined = strings.joined(separator: "\n").trimmingCharacters(in: .whitespacesAndNewlines)
        guard !combined.isEmpty,
              let extracted = PairingService.shared.extractPairingURI(from: combined) else {
            errorMessage = String(localized: "剪贴板中未找到有效的配对链接（需包含 agy://pair）")
            UINotificationFeedbackGenerator().notificationOccurred(.error)
            return
        }
        handleScannedCode(extracted)
    }
    
    private func handleScannedCode(_ rawCode: String) {
        switch PairingService.shared.parsePairingURI(rawCode) {
        case .failure(let err):
            errorMessage = err.localizedDescription
            UINotificationFeedbackGenerator().notificationOccurred(.error)
        case .success(let info):
            UINotificationFeedbackGenerator().notificationOccurred(.success)
            dismiss()
            onScanDetected?(info)
        }
    }
}

// MARK: - AVCapture Video Preview Representable

struct QRCameraRepresentable: UIViewControllerRepresentable {
    let onCodeDetected: (String) -> Void
    
    func makeUIViewController(context: Context) -> QRScannerViewController {
        let controller = QRScannerViewController()
        controller.onCodeDetected = onCodeDetected
        return controller
    }
    
    func updateUIViewController(_ uiViewController: QRScannerViewController, context: Context) {}
}

final class QRScannerViewController: UIViewController, AVCaptureMetadataOutputObjectsDelegate {
    var onCodeDetected: ((String) -> Void)?
    
    private var captureSession: AVCaptureSession?
    private var previewLayer: AVCaptureVideoPreviewLayer?
    private var rotationCoordinator: AVCaptureDevice.RotationCoordinator?
    private var rotationObservation: NSKeyValueObservation?
    private var hasDetectedCode = false

    deinit {
        rotationObservation?.invalidate()
    }
    
    override func viewDidLoad() {
        super.viewDidLoad()
        setupCaptureSession()
    }
    
    override func viewDidLayoutSubviews() {
        super.viewDidLayoutSubviews()
        previewLayer?.frame = view.bounds
        if let angle = rotationCoordinator?.videoRotationAngleForHorizonLevelPreview {
            applyPreviewRotation(angle)
        }
    }
    
    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        hasDetectedCode = false
        if let session = captureSession, !session.isRunning {
            DispatchQueue.global(qos: .userInitiated).async {
                session.startRunning()
            }
        }
    }
    
    override func viewWillDisappear(_ animated: Bool) {
        super.viewWillDisappear(animated)
        if let session = captureSession, session.isRunning {
            DispatchQueue.global(qos: .userInitiated).async {
                session.stopRunning()
            }
        }
    }
    
    private func setupCaptureSession() {
        let session = AVCaptureSession()
        captureSession = session
        
        guard let device = AVCaptureDevice.default(for: .video) else { return }
        guard let input = try? AVCaptureDeviceInput(device: device) else { return }
        if session.canAddInput(input) {
            session.addInput(input)
        }
        
        let metadataOutput = AVCaptureMetadataOutput()
        if session.canAddOutput(metadataOutput) {
            session.addOutput(metadataOutput)
            metadataOutput.setMetadataObjectsDelegate(self, queue: DispatchQueue.main)
            metadataOutput.metadataObjectTypes = [.qr]
        }
        
        let preview = AVCaptureVideoPreviewLayer(session: session)
        preview.videoGravity = .resizeAspectFill
        view.layer.addSublayer(preview)
        previewLayer = preview

        let coordinator = AVCaptureDevice.RotationCoordinator(device: device, previewLayer: preview)
        rotationCoordinator = coordinator
        applyPreviewRotation(coordinator.videoRotationAngleForHorizonLevelPreview)
        rotationObservation = coordinator.observe(\.videoRotationAngleForHorizonLevelPreview, options: [.new]) { [weak self] _, change in
            guard let angle = change.newValue else { return }
            DispatchQueue.main.async { [weak self] in
                self?.applyPreviewRotation(angle)
            }
        }
        
        DispatchQueue.global(qos: .userInitiated).async {
            session.startRunning()
        }
    }

    private func applyPreviewRotation(_ angle: CGFloat) {
        guard let connection = previewLayer?.connection,
              connection.isVideoRotationAngleSupported(angle) else {
            return
        }
        connection.videoRotationAngle = angle
    }
    
    func metadataOutput(_ output: AVCaptureMetadataOutput, didOutput metadataObjects: [AVMetadataObject], from connection: AVCaptureConnection) {
        guard !hasDetectedCode,
              let first = metadataObjects.first as? AVMetadataMachineReadableCodeObject,
              let stringVal = first.stringValue else {
            return
        }
        hasDetectedCode = true
        onCodeDetected?(stringVal)
    }
}
