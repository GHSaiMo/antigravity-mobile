import SwiftUI
import AVFoundation

public struct QRScannerView: View {
    @Environment(\.dismiss) private var dismiss
    
    public var onPairSuccess: ((PairingInfo) -> Void)?
    
    @State private var hasCameraPermission = false
    @State private var isCheckingPermission = true
    @State private var permissionDenied = false
    @State private var showManualInput = false
    @State private var manualURI = ""
    @State private var errorMessage: String? = nil
    @State private var isPairing = false
    @State private var pairingSuccess = false
    
    public init(onPairSuccess: ((PairingInfo) -> Void)? = nil) {
        self.onPairSuccess = onPairSuccess
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
                        Image(systemName: "camera.fill")
                            .font(.system(size: 48))
                            .foregroundColor(.gray)
                        Text("需要相机权限以扫描配对二维码")
                            .font(.headline)
                            .foregroundColor(.white)
                        Text("请在系统设置中允许 Antigravity 访问相机")
                            .font(.subheadline)
                            .foregroundColor(.gray)
                            .multilineTextAlignment(.center)
                            .padding(.horizontal, 32)
                        
                        Button("打开系统设置") {
                            if let url = URL(string: UIApplication.openSettingsURLString) {
                                UIApplication.shared.open(url)
                            }
                        }
                        .buttonStyle(.borderedProminent)
                        .padding(.top, 8)
                        
                        Button("手动输入配对链接") {
                            showManualInput = true
                        }
                        .foregroundColor(.indigo)
                        .padding(.top, 4)
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
                
                // Loading overlay when pairing in progress
                if isPairing {
                    Color.black.opacity(0.6).ignoresSafeArea()
                    VStack(spacing: 14) {
                        ProgressView()
                            .scaleEffect(1.3)
                            .tint(.white)
                        Text("正在与 Mac 网关配对...")
                            .font(.system(size: 15, weight: .medium))
                            .foregroundColor(.white)
                    }
                    .padding(24)
                    .background(.ultraThinMaterial)
                    .cornerRadius(16)
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
                manualInputSheet
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
        VStack {
            Spacer()
            
            // Viewfinder square
            ZStack {
                RoundedRectangle(cornerRadius: 16)
                    .stroke(Color.white.opacity(0.8), lineWidth: 3)
                    .frame(width: 250, height: 250)
                
                // Corner accents
                RoundedRectangle(cornerRadius: 16)
                    .stroke(Color.indigo, lineWidth: 5)
                    .frame(width: 250, height: 250)
            }
            
            Text("对准 Mac 终端中的配对二维码")
                .font(.system(size: 14, weight: .medium))
                .foregroundColor(.white)
                .padding(.top, 24)
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
            .padding(.bottom, 32)
        }
    }
    
    private var manualInputSheet: some View {
        NavigationStack {
            Form {
                Section(header: Text("配对链接 (URI)"), footer: Text("可在 Mac 终端中直接复制以 agy://pair 开头的配对链接并粘贴于此。")) {
                    TextField("agy://pair?host=...&port=...", text: $manualURI)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                    
                    if let clipboard = UIPasteboard.general.string, clipboard.hasPrefix("agy://pair") {
                        Button("粘贴剪贴板内容") {
                            manualURI = clipboard
                        }
                    }
                }
            }
            .navigationTitle("手动输入配对链接")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("取消") { showManualInput = false }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("确认配对") {
                        showManualInput = false
                        handleScannedCode(manualURI)
                    }
                    .disabled(manualURI.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }
            }
        }
        .presentationDetents([.medium])
    }
    
    private func checkCameraPermission() async {
        let status = AVCaptureDevice.authorizationStatus(for: .video)
        switch status {
        case .authorized:
            hasCameraPermission = true
            isCheckingPermission = false
        case .notDetermined:
            let granted = await AVCaptureDevice.requestAccess(for: .video)
            hasCameraPermission = granted
            permissionDenied = !granted
            isCheckingPermission = false
        case .denied, .restricted:
            permissionDenied = true
            isCheckingPermission = false
        @unknown default:
            permissionDenied = true
            isCheckingPermission = false
        }
    }
    
    private func handleScannedCode(_ rawCode: String) {
        guard !isPairing else { return }
        
        switch PairingService.shared.parsePairingURI(rawCode) {
        case .failure(let err):
            errorMessage = err.localizedDescription
            UINotificationFeedbackGenerator().notificationOccurred(.error)
        case .success(let info):
            UINotificationFeedbackGenerator().notificationOccurred(.success)
            isPairing = true
            
            Task {
                do {
                    _ = try await PairingService.shared.pair(with: info)
                    await MainActor.run {
                        isPairing = false
                        pairingSuccess = true
                        onPairSuccess?(info)
                        dismiss()
                    }
                } catch {
                    await MainActor.run {
                        isPairing = false
                        errorMessage = error.localizedDescription
                        UINotificationFeedbackGenerator().notificationOccurred(.error)
                    }
                }
            }
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
    private var hasDetectedCode = false
    
    override func viewDidLoad() {
        super.viewDidLoad()
        setupCaptureSession()
    }
    
    override func viewDidLayoutSubviews() {
        super.viewDidLayoutSubviews()
        previewLayer?.frame = view.bounds
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
        
        DispatchQueue.global(qos: .userInitiated).async {
            session.startRunning()
        }
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
