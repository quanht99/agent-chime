import Foundation

/// Reports whether Codex trusts our hooks. Reads ~/.codex/config.toml and Codex profile files
/// (~/.codex/<profile>.config.toml); never writes any of them.
enum CodexTrust {
    enum State: Equatable { case notInstalled, untrusted(pending: Int), trusted, disabled }

    /// "UserPromptSubmit" -> "user_prompt_submit"
    static func snakeCase(_ event: String) -> String {
        var out = ""
        for (i, ch) in event.enumerated() {
            if ch.isUppercase {
                if i > 0 { out.append("_") }
                out.append(contentsOf: ch.lowercased())
            } else {
                out.append(ch)
            }
        }
        return out
    }

    /// "<hooksPath>:<snake_event>:<groupIdx>:<handlerIdx>" for every handler in our groups.
    static func ourHandlerKeys(hooksPath: String, hooks: JSONValue) -> [String] {
        var keys: [String] = []
        guard let root = hooks["hooks"] else { return keys }
        for event in root.objectKeys ?? [] {
            for (g, group) in (root[event]?.arrayValue ?? []).enumerated() where HookInstaller.isOurGroup(group) {
                for h in 0..<(group["hooks"]?.arrayValue?.count ?? 0) {
                    keys.append("\(hooksPath):\(snakeCase(event)):\(g):\(h)")
                }
            }
        }
        return keys
    }

    static func state(hooksPath: String, hooks: JSONValue?, configTOML: String?) -> State {
        guard let hooks = hooks else { return .notInstalled }
        let keys = ourHandlerKeys(hooksPath: hooksPath, hooks: hooks)
        if keys.isEmpty { return .notInstalled }

        var entries: [String: (hash: Bool, disabled: Bool)] = [:]
        var current: String?
        let prefix = "[hooks.state.\""
        for rawLine in (configTOML ?? "").split(whereSeparator: \.isNewline) {
            let line = rawLine.trimmingCharacters(in: .whitespaces)
            if line.hasPrefix("[") {
                current = nil
                if line.hasPrefix(prefix), let end = line.range(of: "\"]", options: .backwards) {
                    let key = String(line[line.index(line.startIndex, offsetBy: prefix.count)..<end.lowerBound])
                    current = key
                    entries[key] = entries[key] ?? (false, false)
                }
                continue
            }
            guard let key = current else { continue }
            let compact = line.replacingOccurrences(of: " ", with: "")
            if compact.hasPrefix("trusted_hash=\"") { entries[key]?.hash = true }
            if compact.hasPrefix("enabled=false") { entries[key]?.disabled = true }
        }

        if keys.contains(where: { entries[$0]?.disabled == true }) { return .disabled }
        let pending = keys.filter { entries[$0]?.hash != true }.count
        return pending == 0 ? .trusted : .untrusted(pending: pending)
    }

    /// Codex 0.160+ keeps a profile's settings, hook trust included, in its own file:
    /// `codex --profile work` writes ~/.codex/work.config.toml, not config.toml.
    /// Returns config.toml first, then every `*.config.toml` beside it, sorted by name.
    static func configFiles() -> [URL] {
        let dir = Paths.codexConfig.deletingLastPathComponent()
        let profiles = ((try? FileManager.default.contentsOfDirectory(at: dir, includingPropertiesForKeys: nil)) ?? [])
            .filter { $0.lastPathComponent.hasSuffix(".config.toml") }
            .sorted { $0.lastPathComponent < $1.lastPathComponent }
        return [Paths.codexConfig] + profiles
    }

    /// Our hooks run in any profile whose file trusts all of them, so the best state across files wins:
    /// trusted, then disabled, then the fewest pending.
    static func combined(_ states: [State]) -> State {
        if states.contains(.trusted) { return .trusted }
        if states.contains(.disabled) { return .disabled }
        let pending = states.compactMap { state -> Int? in
            if case .untrusted(let count) = state { return count }
            return nil
        }
        if let fewest = pending.min() { return .untrusted(pending: fewest) }
        return .notInstalled
    }

    static func currentState() -> State {
        let hooks = (try? HookInstaller.readJSONFile(Paths.codexHooks)) ?? nil
        let states = configFiles().map { file in
            state(hooksPath: Paths.codexHooks.path, hooks: hooks,
                  configTOML: try? String(contentsOf: file, encoding: .utf8))
        }
        return combined(states)
    }
}
