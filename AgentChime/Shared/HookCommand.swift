import Foundation

enum HookCommand: String, CaseIterable {
    case claudePrompt = "claude-prompt", claudeStop = "claude-stop", claudeStopFailure = "claude-stop-failure",
         claudePermission = "claude-permission", claudeNotification = "claude-notification", claudeAsk = "claude-ask",
         codexPrompt = "codex-prompt", codexStop = "codex-stop", codexPermission = "codex-permission",
         codexInterrupt = "codex-interrupt"

    var agent: Agent {
        switch self {
        case .claudePrompt, .claudeStop, .claudeStopFailure, .claudePermission, .claudeNotification, .claudeAsk:
            return .claude
        case .codexPrompt, .codexStop, .codexPermission, .codexInterrupt:
            return .codex
        }
    }
}

enum HookAction: Equatable {
    case turnStarted(session: String)
    /// Claude Code's own prompt handing background work back (not something the user typed).
    case turnContinued(session: String)
    /// The turn ended but background shell tasks or subagents are still running.
    case turnHeld(session: String, running: Int)
    case turnCancelled(session: String)
    case alert(AgentEvent)
    case ignore(reason: String)
}

enum HookParser {
    static func action(for command: HookCommand, payload: Data) -> HookAction {
        let agent = command.agent
        let isStop = command == .claudeStop || command == .claudeStopFailure || command == .codexStop

        guard let root = try? JSONValue.parse(payload), case .object = root else {
            if isStop { return .alert(AgentEvent(agent: agent, kind: .done, sessionID: nil)) }
            return .ignore(reason: "invalid payload")
        }

        var session = root["session_id"]?.stringValue
        if session == nil && agent == .codex { session = root["turn_id"]?.stringValue }

        switch command {
        case .claudePrompt, .codexPrompt:
            guard let s = session else { return .ignore(reason: "no session id") }
            if command == .claudePrompt, isHandBack(root["prompt"]?.stringValue) { return .turnContinued(session: s) }
            return .turnStarted(session: s)
        case .codexInterrupt:
            guard let s = session else { return .ignore(reason: "no session id") }
            return .turnCancelled(session: s)
        case .claudeStop, .claudeStopFailure, .codexStop:
            if root["stop_hook_active"]?.boolValue == true { return .ignore(reason: "stop_hook_active") }
            let running = runningBackgroundTasks(root)
            if running > 0, let s = session { return .turnHeld(session: s, running: running) }
            return .alert(AgentEvent(agent: agent, kind: .done, sessionID: session))
        case .claudePermission, .codexPermission:
            return .alert(AgentEvent(agent: agent, kind: .attention, sessionID: session,
                                     detail: root["tool_name"]?.stringValue))
        case .claudeNotification:
            let type = root["notification_type"]?.stringValue
            guard type == "elicitation_dialog" else { return .ignore(reason: "notification \(type ?? "none")") }
            return .alert(AgentEvent(agent: agent, kind: .attention, sessionID: session,
                                     detail: root["message"]?.stringValue))
        case .claudeAsk:
            return .alert(AgentEvent(agent: agent, kind: .attention, sessionID: session, detail: "AskUserQuestion"))
        }
    }

    /// Prompts Claude Code injects when background work finishes. Observed in Claude Code 2.1.
    private static let handBackPrefixes = ["<task-notification>", "<agent-message "]

    private static func isHandBack(_ prompt: String?) -> Bool {
        guard let prompt = prompt else { return false }
        let text = prompt.drop { $0.isWhitespace }
        return handBackPrefixes.contains { text.hasPrefix($0) }
    }

    /// Entries of the Stop payload's `background_tasks` whose status is "running".
    /// A missing or malformed field counts as none.
    private static func runningBackgroundTasks(_ root: JSONValue) -> Int {
        guard let tasks = root["background_tasks"]?.arrayValue else { return 0 }
        return tasks.filter { $0["status"]?.stringValue == "running" }.count
    }
}
