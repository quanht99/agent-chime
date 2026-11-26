import AVFoundation
import Foundation

enum SoundPlayer {
    static let afplayPath = "/usr/bin/afplay"

    static func url(for choice: SoundChoice) -> URL {
        switch choice.source {
        case .system:
            return Paths.systemSoundsDir.appendingPathComponent("\(choice.value).aiff")
        case .bundled:
            return Paths.soundsDir.appendingPathComponent(choice.value)
        case .file:
            return URL(fileURLWithPath: choice.value)
        }
    }

    static func isPlayable(_ url: URL) -> Bool {
        guard FileManager.default.fileExists(atPath: url.path) else { return false }
        return (try? AVAudioFile(forReading: url)) != nil
    }

    static func resolvePlayable(_ choice: SoundChoice, kind: AlertKind) -> (url: URL, warning: SoundWarning?) {
        let wanted = url(for: choice)
        if isPlayable(wanted) { return (wanted, nil) }
        let fallback = url(for: SoundChoice.defaultChoice(for: kind))
        return (fallback, SoundWarning(kind: kind, path: wanted.path))
    }

    static func afplayArguments(url: URL, volume: Double) -> [String] {
        let clamped = min(1, max(0, volume.isNaN ? 1 : volume))
        let v = String(format: "%.2f", locale: Locale(identifier: "en_US_POSIX"), clamped)
        return ["-v", v, url.path]
    }

    /// Spawns afplay in its own session and returns without waiting for it.
    static func playDetached(url: URL, volume: Double) -> Bool {
        var attr: posix_spawnattr_t?
        guard posix_spawnattr_init(&attr) == 0 else { return false }
        defer { posix_spawnattr_destroy(&attr) }
        guard posix_spawnattr_setflags(&attr, Int16(POSIX_SPAWN_SETSID | POSIX_SPAWN_CLOEXEC_DEFAULT)) == 0 else { return false }

        var actions: posix_spawn_file_actions_t?
        guard posix_spawn_file_actions_init(&actions) == 0 else { return false }
        defer { posix_spawn_file_actions_destroy(&actions) }
        _ = posix_spawn_file_actions_addopen(&actions, 0, "/dev/null", O_RDONLY, 0)
        _ = posix_spawn_file_actions_addopen(&actions, 1, "/dev/null", O_WRONLY, 0)
        _ = posix_spawn_file_actions_addopen(&actions, 2, "/dev/null", O_WRONLY, 0)

        let argv = [afplayPath] + afplayArguments(url: url, volume: volume)
        var cArgs: [UnsafeMutablePointer<CChar>?] = argv.map { strdup($0) }
        cArgs.append(nil)
        defer { cArgs.forEach { free($0) } }

        var pid: pid_t = 0
        let rc = posix_spawn(&pid, afplayPath, &actions, &attr, cArgs, environ)
        return rc == 0
    }
}
