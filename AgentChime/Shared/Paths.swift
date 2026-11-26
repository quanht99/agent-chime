import Foundation

enum Paths {
    static var home: URL {
        if let raw = getenv("AGENTCHIME_HOME"), raw.pointee != 0 {
            return URL(fileURLWithPath: String(cString: raw), isDirectory: true)
        }
        return FileManager.default.homeDirectoryForCurrentUser
    }

    static var appSupport: URL {
        home.appendingPathComponent("Library/Application Support/AgentChime", isDirectory: true)
    }
    static var config: URL { appSupport.appendingPathComponent("config.json") }
    static var state: URL { appSupport.appendingPathComponent("state.json") }
    static var stateLock: URL { appSupport.appendingPathComponent("state.lock") }
    static var installedHelper: URL { appSupport.appendingPathComponent("agentchime-hook") }
    static var soundsDir: URL { appSupport.appendingPathComponent("Sounds", isDirectory: true) }
    static var backupsDir: URL { appSupport.appendingPathComponent("Backups", isDirectory: true) }
    static var debugEnvFlag: URL { appSupport.appendingPathComponent("debug-env") }
    static var logFile: URL {
        home.appendingPathComponent("Library/Logs/AgentChime/agentchime.log")
    }
    static var claudeSettings: URL { home.appendingPathComponent(".claude/settings.json") }
    static var codexHooks: URL { home.appendingPathComponent(".codex/hooks.json") }
    static var codexConfig: URL { home.appendingPathComponent(".codex/config.toml") }
    static let systemSoundsDir = URL(fileURLWithPath: "/System/Library/Sounds")

    static func ensureDirectory(_ url: URL) {
        try? FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
    }
}
