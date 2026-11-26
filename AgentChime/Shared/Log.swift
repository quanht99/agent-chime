import Foundation

enum Log {
    static let maxBytes = 1_000_000

    static func write(_ message: String, now: Date = Date()) {
        let file = Paths.logFile
        Paths.ensureDirectory(file.deletingLastPathComponent())
        rotateIfNeeded(file)

        let stamp = ISO8601DateFormatter().string(from: now)
        guard let data = "\(stamp) \(message)\n".data(using: .utf8) else { return }
        let fd = open(file.path, O_WRONLY | O_APPEND | O_CREAT, 0o644)
        guard fd >= 0 else { return }
        defer { close(fd) }
        data.withUnsafeBytes { buf in
            guard let base = buf.baseAddress else { return }
            _ = Foundation.write(fd, base, buf.count)
        }
    }

    private static func rotateIfNeeded(_ file: URL) {
        let attrs = try? FileManager.default.attributesOfItem(atPath: file.path)
        guard let size = attrs?[.size] as? NSNumber, size.intValue > maxBytes else { return }
        let rotated = file.deletingLastPathComponent().appendingPathComponent("agentchime.log.1")
        try? FileManager.default.removeItem(at: rotated)
        try? FileManager.default.moveItem(at: file, to: rotated)
    }
}
