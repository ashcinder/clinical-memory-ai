import Foundation
import Vision
import ImageIO

// macOS Vision performs OCR on this computer; no cloud service is used.
guard CommandLine.arguments.count == 2 else { exit(2) }
let url = URL(fileURLWithPath: CommandLine.arguments[1])
let request = VNRecognizeTextRequest()
request.recognitionLevel = .accurate
request.recognitionLanguages = ["zh-Hans", "en-US"]
request.usesLanguageCorrection = true
let handler = VNImageRequestHandler(url: url, options: [:])
do {
    try handler.perform([request])
    let text = (request.results ?? []).compactMap { $0.topCandidates(1).first?.string }.joined(separator: "\n")
    print(text)
} catch {
    fputs("Local OCR failed\n", stderr)
    exit(1)
}
