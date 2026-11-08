import Foundation
import PDFKit

guard CommandLine.arguments.count == 2,
      let document = PDFDocument(url: URL(fileURLWithPath: CommandLine.arguments[1])),
      !document.isLocked else {
    exit(1)
}

var remaining = 50_000
for index in 0..<min(document.pageCount, 100) {
    if remaining <= 0 { break }
    guard let text = document.page(at: index)?.string, !text.isEmpty else { continue }
    let segment = text.unicodeScalars.prefix(remaining)
    print(String(segment))
    remaining -= segment.count + 1
}
