import Foundation

enum Deployer {
    /// Copies the helper binary and bundled sounds into `appSupport`.
    /// Returns the names of files that changed ("agentchime-hook", "Sounds/<file>").
    static func sync(helper: URL, sounds: URL, into appSupport: URL = Paths.appSupport) throws -> [String] {
        let fm = FileManager.default
        try fm.createDirectory(at: appSupport, withIntermediateDirectories: true)
        var changed: [String] = []

        let helperName = "agentchime-hook"
        let helperDest = appSupport.appendingPathComponent(helperName)
        let helperData = try Data(contentsOf: helper)
        if (try? Data(contentsOf: helperDest)) != helperData {
            try replaceAtomically(helperDest, with: helperData, permissions: 0o755)
            changed.append(helperName)
        }

        let soundsDest = appSupport.appendingPathComponent("Sounds", isDirectory: true)
        try fm.createDirectory(at: soundsDest, withIntermediateDirectories: true)
        let wanted = try fm.contentsOfDirectory(atPath: sounds.path).filter(SoundLibrary.isAudioFile)
        for name in wanted {
            let data = try Data(contentsOf: sounds.appendingPathComponent(name))
            let target = soundsDest.appendingPathComponent(name)
            if (try? Data(contentsOf: target)) != data {
                try replaceAtomically(target, with: data, permissions: 0o644)
                changed.append("Sounds/\(name)")
            }
        }
        let existing = (try? fm.contentsOfDirectory(atPath: soundsDest.path)) ?? []
        for name in existing where SoundLibrary.isAudioFile(name) && !wanted.contains(name) {
            try fm.removeItem(at: soundsDest.appendingPathComponent(name))
            changed.append("Sounds/\(name)")
        }
        return changed
    }

    /// Writes to a temp file in the same directory, then renames over the target so the
    /// target gets a fresh inode (overwriting a signed binary in place can get it killed).
    private static func replaceAtomically(_ target: URL, with data: Data, permissions: Int) throws {
        let fm = FileManager.default
        let tmp = target.deletingLastPathComponent()
            .appendingPathComponent(".\(target.lastPathComponent).\(UUID().uuidString).tmp")
        try data.write(to: tmp)
        do {
            try fm.setAttributes([.posixPermissions: permissions], ofItemAtPath: tmp.path)
            if rename(tmp.path, target.path) != 0 {
                throw NSError(domain: NSPOSIXErrorDomain, code: Int(errno))
            }
        } catch {
            try? fm.removeItem(at: tmp)
            throw error
        }
    }
}
