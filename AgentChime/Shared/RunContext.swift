import Foundation

enum RunContext {
    static let claudeMarkers = ["CLAUDECODE", "CLAUDE_CODE_SESSION_ID"]
    static let codexMarkers = ["CODEX_THREAD_ID", "CODEX_SANDBOX"]
    static let claudeHeadlessEntrypoints: Set<String> = ["sdk-cli", "sdk-ts", "sdk-py"]

    private static func anySet(_ names: [String], in env: [String: String]) -> Bool {
        names.contains { !(env[$0] ?? "").isEmpty }
    }

    /// A Claude hook that sees Codex markers, or a Codex hook that sees Claude markers, runs inside the other agent.
    static func isNested(_ agent: Agent, env: [String: String]) -> Bool {
        switch agent {
        case .claude: return anySet(codexMarkers, in: env)
        case .codex: return anySet(claudeMarkers, in: env)
        }
    }

    static func isHeadless(_ agent: Agent, env: [String: String], ancestors: [[String]]) -> Bool {
        switch agent {
        case .claude:
            return claudeHeadlessEntrypoints.contains(env["CLAUDE_CODE_ENTRYPOINT"] ?? "")
        case .codex:
            return ancestors.contains { argv in
                for i in argv.indices where i + 1 < argv.count {
                    let name = (argv[i] as NSString).lastPathComponent
                    if (name == "codex" || name == "codex.js") && (argv[i + 1] == "exec" || argv[i + 1] == "e") {
                        return true
                    }
                }
                return false
            }
        }
    }

    // MARK: - Ancestor scan

    private static func parentPID(of pid: pid_t) -> pid_t? {
        var mib: [Int32] = [CTL_KERN, KERN_PROC, KERN_PROC_PID, pid]
        var info = kinfo_proc()
        var size = MemoryLayout<kinfo_proc>.stride
        guard sysctl(&mib, UInt32(mib.count), &info, &size, nil, 0) == 0, size > 0 else { return nil }
        return info.kp_eproc.e_ppid
    }

    private static func argv(of pid: pid_t) -> [String]? {
        var mib: [Int32] = [CTL_KERN, KERN_PROCARGS2, pid]
        var size = 0
        guard sysctl(&mib, UInt32(mib.count), nil, &size, nil, 0) == 0, size > MemoryLayout<Int32>.size else { return nil }
        var buf = [UInt8](repeating: 0, count: size)
        guard sysctl(&mib, UInt32(mib.count), &buf, &size, nil, 0) == 0, size > MemoryLayout<Int32>.size else { return nil }

        let argc = buf.withUnsafeBytes { $0.load(as: Int32.self) }
        var i = MemoryLayout<Int32>.size
        while i < size && buf[i] != 0 { i += 1 }      // executable path
        while i < size && buf[i] == 0 { i += 1 }      // padding
        var args: [String] = []
        while i < size && args.count < Int(argc) {
            var j = i
            while j < size && buf[j] != 0 { j += 1 }
            args.append(String(decoding: buf[i..<j], as: UTF8.self))
            i = j + 1
        }
        return args
    }

    /// argv of each ancestor, nearest first, from the parent up to (excluding) pid 1.
    static func ancestorCommandLines(maxDepth: Int = 8) -> [[String]] {
        var result: [[String]] = []
        var pid = getppid()
        while pid > 1 && result.count < maxDepth {
            if let args = argv(of: pid), !args.isEmpty { result.append(args) }
            guard let next = parentPID(of: pid), next != pid else { break }
            pid = next
        }
        return result
    }

    // MARK: - Env probe

    private static let probeEnvPrefixes = ["CLAUDE", "CODEX", "TERM_PROGRAM", "__CFBundleIdentifier", "AI_AGENT"]
    private static let probeSecretWords = ["TOKEN", "KEY", "SECRET", "AUTH"]
    private static let probePayloadFields = ["hook_event_name", "permission_mode", "tool_name", "notification_type",
                                             "stop_hook_active", "agent_id", "agent_type"]

    private static func clip(_ s: String, _ n: Int) -> String { String(s.prefix(n)) }

    static func probeLine(command: HookCommand, env: [String: String], ancestors: [[String]], payload: Data) -> String {
        let envPart = env.keys.sorted().filter { name in
            let upper = name.uppercased()
            return probeEnvPrefixes.contains { name.hasPrefix($0) } && !probeSecretWords.contains { upper.contains($0) }
        }.map { "\($0)=\(clip(env[$0] ?? "", 120))" }.joined(separator: " ")

        let ancestorPart = ancestors.map { argv -> String in
            let name = (argv.first.map { ($0 as NSString).lastPathComponent }) ?? ""
            return argv.count > 1 ? "\(name) \(clip(argv[1], 20))" : name
        }.joined(separator: " | ")

        var payloadPart = "invalid"
        if let root = try? JSONValue.parse(payload), let keys = root.objectKeys {
            var parts = ["keys=[" + keys.sorted().joined(separator: ",") + "]"]
            for field in probePayloadFields {
                guard let v = root[field] else { continue }
                switch v {
                case .string(let s): parts.append("\(field)=\(clip(s, 120))")
                case .bool(let b): parts.append("\(field)=\(b)")
                case .number(let n): parts.append("\(field)=\(clip(n, 120))")
                default: break
                }
            }
            payloadPart = parts.joined(separator: " ")
        }
        return "cmd=\(command.rawValue) env{\(envPart)} ancestors[\(ancestorPart)] payload{\(payloadPart)}"
    }
}
