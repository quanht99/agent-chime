import Foundation

enum InstallStatus: Equatable {
    case notInstalled
    case partial(installed: Int, total: Int)
    case installed
    case unreadable(path: String)
}

/// The file holds invalid JSON or an unexpected shape; nothing was written.
struct InstallError: Error, Equatable {
    let path: String
}

enum HookInstaller {
    static let marker = "agentchime-hook"
    static let maxBackups = 10

    // MARK: Pure JSON transforms

    /// Codex derives its trust hash from the hook command string and the group's position, so changing this
    /// format (or a HookCommand raw value) makes Codex ask the user to trust the hooks again.
    static func command(helperPath: String, subcommand: HookCommand) -> String {
        var escaped = ""
        for ch in helperPath {
            if ch == "\\" || ch == "\"" || ch == "$" || ch == "`" { escaped.append("\\") }
            escaped.append(ch)
        }
        return "\"\(escaped)\" \(subcommand.rawValue)"
    }

    static func group(for spec: HookSpec, helperPath: String) -> JSONValue {
        var members: [JSONMember] = []
        if let matcher = spec.matcher { members.append(JSONMember(key: "matcher", value: .string(matcher))) }
        let handler = JSONValue.object([
            JSONMember(key: "type", value: .string("command")),
            JSONMember(key: "command", value: .string(command(helperPath: helperPath, subcommand: spec.command))),
            JSONMember(key: "async", value: .bool(true)),
        ])
        members.append(JSONMember(key: "hooks", value: .array([handler])))
        return .object(members)
    }

    static func isOurGroup(_ group: JSONValue) -> Bool {
        guard let handlers = group["hooks"]?.arrayValue, !handlers.isEmpty else { return false }
        return handlers.allSatisfy { ($0["command"]?.stringValue ?? "").contains(marker) }
    }

    private static func matcher(of group: JSONValue) -> String? { group["matcher"]?.stringValue }

    static func merged(into root: JSONValue?, specs: [HookSpec], helperPath: String) -> (root: JSONValue, replacedExisting: Bool) {
        var root = root ?? .object([])
        var hooks = root["hooks"] ?? .object([])
        var replaced = false

        for spec in specs {
            let wanted = group(for: spec, helperPath: helperPath)
            var groups = hooks[spec.event]?.arrayValue ?? []
            if let i = groups.firstIndex(where: { isOurGroup($0) && matcher(of: $0) == spec.matcher }) {
                if groups[i] != wanted {
                    groups[i] = wanted
                    replaced = true
                }
            } else {
                groups.append(wanted)
            }
            hooks.set(spec.event, .array(groups))
        }
        root.set("hooks", hooks)
        return (root, replaced)
    }

    static func removingOurGroups(from root: JSONValue) -> JSONValue {
        guard var hooks = root["hooks"], let events = hooks.objectKeys else { return root }
        var removedAny = false
        for event in events {
            guard let groups = hooks[event]?.arrayValue else { continue }
            let kept = groups.filter { !isOurGroup($0) }
            if kept.count == groups.count { continue }
            removedAny = true
            if kept.isEmpty { hooks.removeKey(event) } else { hooks.set(event, .array(kept)) }
        }
        guard removedAny else { return root }
        var result = root
        if hooks.objectKeys?.isEmpty == true { result.removeKey("hooks") } else { result.set("hooks", hooks) }
        return result
    }

    static func status(of root: JSONValue?, specs: [HookSpec], helperPath: String) -> InstallStatus {
        guard let hooks = root?["hooks"] else { return .notInstalled }
        var matching = 0
        var anyOurs = false
        for event in hooks.objectKeys ?? [] {
            if (hooks[event]?.arrayValue ?? []).contains(where: { isOurGroup($0) }) { anyOurs = true }
        }
        for spec in specs {
            let wanted = group(for: spec, helperPath: helperPath)
            if (hooks[spec.event]?.arrayValue ?? []).contains(wanted) { matching += 1 }
        }
        if matching == specs.count && !specs.isEmpty { return .installed }
        if matching == 0 && !anyOurs { return .notInstalled }
        return .partial(installed: matching, total: specs.count)
    }

    /// Refuses shapes we cannot edit without damaging other data: a non-object root, a non-object `hooks`,
    /// or a non-array value for an event we touch.
    static func validateShape(_ root: JSONValue?, events: [String], path: String) throws {
        guard let root = root else { return }
        guard root.objectKeys != nil else { throw InstallError(path: path) }
        guard let hooks = root["hooks"] else { return }
        guard hooks.objectKeys != nil else { throw InstallError(path: path) }
        for event in events {
            if let value = hooks[event], value.arrayValue == nil { throw InstallError(path: path) }
        }
    }

    private static func events(of specs: [HookSpec]) -> [String] { specs.map { $0.event } }

    // MARK: File access

    static func readJSONFile(_ url: URL) throws -> JSONValue? {
        guard FileManager.default.fileExists(atPath: url.path) else { return nil }
        do {
            return try JSONValue.parse(try Data(contentsOf: url))
        } catch {
            throw InstallError(path: url.path)
        }
    }

    static func writeJSONFile(_ value: JSONValue, to url: URL) throws {
        let fm = FileManager.default
        let target = url.resolvingSymlinksInPath()
        let dir = target.deletingLastPathComponent()
        try fm.createDirectory(at: dir, withIntermediateDirectories: true)

        var permissions: NSNumber = 0o644
        if fm.fileExists(atPath: target.path) {
            if let p = try? fm.attributesOfItem(atPath: target.path)[.posixPermissions] as? NSNumber { permissions = p }
            backUp(target)
        }

        let tmp = dir.appendingPathComponent(".\(target.lastPathComponent).agentchime-\(UUID().uuidString).tmp")
        do {
            try Data((value.serialized() + "\n").utf8).write(to: tmp)
            try fm.setAttributes([.posixPermissions: permissions], ofItemAtPath: tmp.path)
            guard rename(tmp.path, target.path) == 0 else {
                throw NSError(domain: NSPOSIXErrorDomain, code: Int(errno))
            }
        } catch {
            try? fm.removeItem(at: tmp)
            throw error
        }
    }

    private static func backUp(_ file: URL) {
        let fm = FileManager.default
        let dir = Paths.backupsDir
        Paths.ensureDirectory(dir)
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "yyyyMMdd-HHmmss"
        let name = file.lastPathComponent
        let stamp = f.string(from: Date())
        let millis = Int(Date().timeIntervalSince1970 * 1000) % 1000
        let base = String(format: "%@.%@-%03d", name, stamp, millis)
        var dest = dir.appendingPathComponent(base)
        var n = 2
        while fm.fileExists(atPath: dest.path) {
            dest = dir.appendingPathComponent("\(base)-\(n)")
            n += 1
        }
        do {
            try fm.copyItem(at: file, to: dest)
        } catch {
            Log.write("backup of \(name) failed: \(error.localizedDescription)")
            return
        }
        let mine = ((try? fm.contentsOfDirectory(atPath: dir.path)) ?? [])
            .filter { $0.hasPrefix(name + ".") }
            .sorted(by: >)
        for old in mine.dropFirst(maxBackups) {
            try? fm.removeItem(at: dir.appendingPathComponent(old))
        }
    }

    // MARK: Install / uninstall / status

    /// Returns whether an existing AgentChime group was replaced in place.
    @discardableResult
    static func install(specs: [HookSpec], fileURL: URL, helperPath: String = Paths.installedHelper.path) throws -> Bool {
        let existing = try readJSONFile(fileURL)
        try validateShape(existing, events: events(of: specs), path: fileURL.path)
        let result = merged(into: existing, specs: specs, helperPath: helperPath)
        if result.root != existing {
            try writeJSONFile(result.root, to: fileURL)
        }
        return result.replacedExisting
    }

    static func uninstall(fileURL: URL) throws {
        guard let existing = try readJSONFile(fileURL) else { return }
        try validateShape(existing, events: events(of: AgentHooks.claude + AgentHooks.codex), path: fileURL.path)
        let result = removingOurGroups(from: existing)
        if result != existing { try writeJSONFile(result, to: fileURL) }
    }

    static func status(specs: [HookSpec], fileURL: URL, helperPath: String = Paths.installedHelper.path) -> InstallStatus {
        do {
            let root = try readJSONFile(fileURL)
            try validateShape(root, events: events(of: specs), path: fileURL.path)
            return status(of: root, specs: specs, helperPath: helperPath)
        } catch let e as InstallError {
            return .unreadable(path: e.path)
        } catch {
            return .unreadable(path: fileURL.path)
        }
    }
}
