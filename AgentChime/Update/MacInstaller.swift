import CryptoKit
import Foundation

enum BlockReason: Equatable {
    case moveToApplications
    case cantInstallHere
}

enum InstallOutcome: Equatable {
    /// The new app said `ready`; the old bundle is trashed and `ready()` was called.
    case relaunched
    /// The new app exited before `ready`; the old bundle is back in place.
    case badBuild
    /// Neither `ready` nor an exit within 20 s; the old bundle is trashed.
    case restartNeeded
}

struct InstallFailure: LocalizedError {
    let message: String
    init(_ m: String) { message = m }
    var errorDescription: String? { message }
}

/// Download, verify, unpack, swap, relaunch and roll back. Never call `install` from a unit test.
final class MacInstaller {
    private let session: URLSession
    private let bundleURL: URL
    /// Fetches the manifest again (verified); used once when the archive does not match.
    private let refetch: () async throws -> UpdateManifest
    private let readyTimeout: TimeInterval = 20
    /// The running version: whatever a refetch returns must be strictly newer.
    private let currentVersion: String

    init(session: URLSession, bundleURL: URL = Bundle.main.bundleURL, currentVersion: String? = nil,
         refetch: @escaping () async throws -> UpdateManifest) {
        self.session = session
        self.bundleURL = bundleURL
        self.currentVersion = currentVersion
            ?? Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? ""
        self.refetch = refetch
    }

    // MARK: - Pure helpers (unit tested)

    static func installBlockReason(bundleURL: URL, downloads: URL, isWritable: (String) -> Bool) -> BlockReason? {
        let path = bundleURL.standardizedFileURL.path
        let dl = downloads.standardizedFileURL.path
        if path.contains("/AppTranslocation/") || path.hasPrefix(dl.hasSuffix("/") ? dl : dl + "/") {
            return .moveToApplications
        }
        let parent = bundleURL.standardizedFileURL.deletingLastPathComponent().path
        return isWritable(parent) ? nil : .cantInstallHere
    }

    /// The block reason for the running app.
    static func currentBlockReason() -> BlockReason? {
        let downloads = FileManager.default.urls(for: .downloadsDirectory, in: .userDomainMask).first
            ?? URL(fileURLWithPath: "/nonexistent-downloads")
        return installBlockReason(bundleURL: Bundle.main.bundleURL, downloads: downloads) {
            FileManager.default.isWritableFile(atPath: $0)
        }
    }

    /// True when `buffer` holds a complete line that is exactly `ready` (a trailing CR is ignored).
    /// The last, unterminated piece never counts: it may be a half-written line.
    static func containsReadyLine(_ buffer: Data) -> Bool {
        // Bytes, not Characters: Swift treats "\r\n" as one Character.
        var lines = buffer.split(separator: 0x0A, omittingEmptySubsequences: false)
        lines.removeLast()
        return lines.contains { line in
            var l = line
            if l.last == 0x0D { l = l.dropLast() }
            return l.elementsEqual("ready".utf8)
        }
    }

    /// Exactly one top-level `*.app`, and it must be named `AgentChime.app`.
    static func findSingleApp(in dir: URL) throws -> URL {
        let items = try FileManager.default.contentsOfDirectory(at: dir, includingPropertiesForKeys: nil)
        let apps = items.filter { $0.pathExtension == "app" }
        guard apps.count == 1, apps[0].lastPathComponent == "AgentChime.app" else {
            throw InstallFailure("the archive does not hold exactly one AgentChime.app")
        }
        return apps[0]
    }

    // MARK: - Install

    func install(manifest: UpdateManifest, file: UpdateFile, ready: @escaping () -> Void) async throws -> InstallOutcome {
        let fm = FileManager.default
        let work = try fm.url(for: .itemReplacementDirectory, in: .userDomainMask, appropriateFor: bundleURL, create: true)
        var keepWork = false
        defer { if !keepWork { try? fm.removeItem(at: work) } }

        // 1. Download and check size and SHA-256 (one manifest refetch on a mismatch).
        let zip = work.appendingPathComponent("update.zip")
        var m = manifest
        var f = file
        do {
            try await download(m, f, to: zip)
        } catch UpdateError.unverified {
            try await Task.sleep(nanoseconds: 5_000_000_000)
            m = try await refetch()
            // The release may have been pulled back in between: never install anything not newer.
            guard compareVersions(m.version, currentVersion) == 1 else { throw UpdateError.unverified }
            f = try m.macFile()
            try? fm.removeItem(at: zip)
            try await download(m, f, to: zip)
        }

        // 2-3. Unpack and find the app.
        let unpacked = work.appendingPathComponent("unpacked", isDirectory: true)
        try fm.createDirectory(at: unpacked, withIntermediateDirectories: true)
        let ditto = try await Self.run("/usr/bin/ditto", ["-x", "-k", zip.path, unpacked.path], timeout: 120)
        guard ditto.status == 0 else { throw InstallFailure("could not unpack the update") }
        try? fm.removeItem(at: zip)
        let newApp = try Self.findSingleApp(in: unpacked)

        // 4. The version in the bundle must be the manifest's.
        let plist = NSDictionary(contentsOf: newApp.appendingPathComponent("Contents/Info.plist"))
        guard plist?["CFBundleShortVersionString"] as? String == m.version else {
            throw InstallFailure("the update has the wrong version")
        }

        // 5. Code signature.
        let cs = try await Self.run("/usr/bin/codesign", ["--verify", "--deep", "--strict", newApp.path], timeout: 60)
        guard cs.status == 0 else { throw InstallFailure("the update's signature check failed") }

        // 6. It must run and print its version.
        let exe = newApp.appendingPathComponent("Contents/MacOS/AgentChime").path
        let ver = try await Self.run(exe, ["--version"], timeout: 10)
        guard ver.status == 0, ver.output.trimmingCharacters(in: .whitespacesAndNewlines) == m.version else {
            throw InstallFailure("the new version did not report its version")
        }

        // 7. No quarantine flag (exit code ignored).
        _ = try? await Self.run("/usr/bin/xattr", ["-dr", "com.apple.quarantine", newApp.path], timeout: 30)

        // 8. Swap. The old bundle now sits at newApp's path; keep it there until the relaunch ends.
        guard renamex_np(newApp.path, bundleURL.path, UInt32(RENAME_SWAP)) == 0 else {
            throw InstallFailure("could not replace the app (\(String(cString: strerror(errno))))")
        }
        let oldApp = newApp
        Log.write("update: swapped in \(m.version)")

        // 9. Relaunch and wait for `ready`.
        let result = await relaunch(timeout: readyTimeout)
        switch result {
        case .ready:
            Self.trash(oldApp)
            ready()
            return .relaunched
        case .exited:
            if renamex_np(oldApp.path, bundleURL.path, UInt32(RENAME_SWAP)) == 0 {
                try? fm.removeItem(at: oldApp)  // now holds the new bundle
                Log.write("update: new version exited before ready, rolled back")
                return .badBuild
            }
            // Could not swap back: keep the old bundle where it is so nothing is lost.
            keepWork = true
            throw InstallFailure("rollback failed; the previous version is in \(oldApp.deletingLastPathComponent().path)")
        case .timeout:
            Self.trash(oldApp)
            return .restartNeeded
        }
    }

    private static func trash(_ url: URL) {
        do { try FileManager.default.trashItem(at: url, resultingItemURL: nil) } catch {
            try? FileManager.default.removeItem(at: url)
        }
    }

    // MARK: - Download

    /// Streams the archive to `dest`, hashing as it goes. It stops with `.unverified` as soon as more
    /// bytes arrive than the manifest's size, and removes the partial file on any failure.
    func download(_ m: UpdateManifest, _ f: UpdateFile, to dest: URL) async throws {
        guard f.name == (f.name as NSString).lastPathComponent, !f.name.contains("/"), f.name != "..", f.name != ".",
              f.size > 0, f.size <= UpdateLimits.maxArchive else { throw UpdateError.unverified }
        guard let url = URL(string: UpdateKeys.base.absoluteString + "download/v\(m.version)/\(f.name)"),
              UpdateKeys.allowed(url) else { throw InstallFailure("download URL not allowed") }
        let (bytes, resp) = try await session.bytes(from: url)
        guard (resp as? HTTPURLResponse)?.statusCode == 200 else {
            throw InstallFailure("download failed (HTTP \((resp as? HTTPURLResponse)?.statusCode ?? 0))")
        }
        if resp.expectedContentLength > f.size { throw UpdateError.unverified }
        let fm = FileManager.default
        try? fm.removeItem(at: dest)
        guard fm.createFile(atPath: dest.path, contents: nil) else { throw InstallFailure("could not write the download") }
        let fh = try FileHandle(forWritingTo: dest)
        var ok = false
        defer {
            try? fh.close()
            if !ok { try? fm.removeItem(at: dest) }
        }
        var hasher = SHA256()
        var received: Int64 = 0
        var chunk = Data()
        chunk.reserveCapacity(1 << 16)
        for try await byte in bytes {
            received += 1
            if received > f.size { throw UpdateError.unverified }
            chunk.append(byte)
            if chunk.count >= 1 << 16 {
                try fh.write(contentsOf: chunk)
                hasher.update(data: chunk)
                chunk.removeAll(keepingCapacity: true)
            }
        }
        if !chunk.isEmpty {
            try fh.write(contentsOf: chunk)
            hasher.update(data: chunk)
        }
        guard received == f.size,
              hasher.finalize().map({ String(format: "%02x", $0) }).joined() == f.sha256 else { throw UpdateError.unverified }
        ok = true
    }

    static func sha256Hex(of url: URL) throws -> String {
        let fh = try FileHandle(forReadingFrom: url)
        defer { try? fh.close() }
        var hasher = SHA256()
        while let chunk = try fh.read(upToCount: 1 << 20), !chunk.isEmpty { hasher.update(data: chunk) }
        return hasher.finalize().map { String(format: "%02x", $0) }.joined()
    }

    // MARK: - Processes

    private struct RunResult { let status: Int32; let output: String }

    /// Runs a tool off the main thread; a timeout kills it and throws.
    private static func run(_ path: String, _ args: [String], timeout: TimeInterval) async throws -> RunResult {
        try await withCheckedThrowingContinuation { cont in
            DispatchQueue.global().async {
                let p = Process()
                p.executableURL = URL(fileURLWithPath: path)
                p.arguments = args
                let out = Pipe()
                p.standardOutput = out
                p.standardError = FileHandle.nullDevice
                p.standardInput = FileHandle.nullDevice
                let done = DispatchSemaphore(value: 0)
                p.terminationHandler = { _ in done.signal() }
                do { try p.run() } catch { cont.resume(throwing: error); return }
                var data = Data()
                let reader = DispatchGroup()
                reader.enter()
                DispatchQueue.global().async {
                    data = out.fileHandleForReading.readDataToEndOfFile()
                    reader.leave()
                }
                if done.wait(timeout: .now() + timeout) == .timedOut {
                    p.terminate()
                    _ = done.wait(timeout: .now() + 2)
                    cont.resume(throwing: InstallFailure("\((path as NSString).lastPathComponent) timed out"))
                    return
                }
                reader.wait()
                cont.resume(returning: RunResult(status: p.terminationStatus, output: String(decoding: data, as: UTF8.self)))
            }
        }
    }

    private enum Relaunch { case ready, exited, timeout }

    /// Starts the new app with --after-update and waits for the exact line `ready`.
    private func relaunch(timeout: TimeInterval) async -> Relaunch {
        await withCheckedContinuation { cont in
            let lock = NSLock()
            var finished = false
            func finish(_ r: Relaunch) {
                lock.lock(); defer { lock.unlock() }
                guard !finished else { return }
                finished = true
                cont.resume(returning: r)
            }
            let p = Process()
            p.executableURL = bundleURL.appendingPathComponent("Contents/MacOS/AgentChime")
            p.arguments = ["--after-update"]
            let pipe = Pipe()
            p.standardOutput = pipe
            p.standardError = FileHandle.nullDevice
            p.standardInput = FileHandle.nullDevice
            var buffer = Data()
            pipe.fileHandleForReading.readabilityHandler = { h in
                let d = h.availableData
                if d.isEmpty { h.readabilityHandler = nil; return }
                lock.lock()
                buffer.append(d)
                let sawReady = Self.containsReadyLine(buffer)
                lock.unlock()
                if sawReady { finish(.ready) }
            }
            p.terminationHandler = { _ in
                // Give the read handler a moment to deliver a final `ready` line.
                DispatchQueue.global().asyncAfter(deadline: .now() + 0.3) { finish(.exited) }
            }
            do { try p.run() } catch {
                Log.write("update: cannot start the new version: \(error.localizedDescription)")
                finish(.exited)
                return
            }
            DispatchQueue.global().asyncAfter(deadline: .now() + timeout) { finish(.timeout) }
        }
    }
}
