import SwiftUI
import VisionKit

/// Apple 原生智能文稿扫描器（VisionKit 驱动，支持自动透视矫正、四角边缘吸附与去阴影黑白增强）
public struct DocumentScannerView: UIViewControllerRepresentable {
    public let onScanned: ([UIImage]) -> Void
    public let onCancel: () -> Void
    
    public init(onScanned: @escaping ([UIImage]) -> Void, onCancel: @escaping () -> Void) {
        self.onScanned = onScanned
        self.onCancel = onCancel
    }
    
    public func makeUIViewController(context: Context) -> VNDocumentCameraViewController {
        let scanner = VNDocumentCameraViewController()
        scanner.delegate = context.coordinator
        return scanner
    }
    
    public func updateUIViewController(_ uiViewController: VNDocumentCameraViewController, context: Context) {}
    
    public func makeCoordinator() -> Coordinator {
        Coordinator(onScanned: onScanned, onCancel: onCancel)
    }
    
    public final class Coordinator: NSObject, VNDocumentCameraViewControllerDelegate {
        private let onScanned: ([UIImage]) -> Void
        private let onCancel: () -> Void
        
        public init(onScanned: @escaping ([UIImage]) -> Void, onCancel: @escaping () -> Void) {
            self.onScanned = onScanned
            self.onCancel = onCancel
        }
        
        public func documentCameraViewController(_ controller: VNDocumentCameraViewController, didFinishWith scan: VNDocumentCameraScan) {
            var images: [UIImage] = []
            for index in 0..<scan.pageCount {
                images.append(scan.imageOfPage(at: index))
            }
            controller.dismiss(animated: true) {
                self.onScanned(images)
            }
        }
        
        public func documentCameraViewControllerDidCancel(_ controller: VNDocumentCameraViewController) {
            controller.dismiss(animated: true) {
                self.onCancel()
            }
        }
        
        public func documentCameraViewController(_ controller: VNDocumentCameraViewController, didFailWithError error: Error) {
            controller.dismiss(animated: true) {
                self.onCancel()
            }
        }
    }
}
