import Foundation

struct HookSpec: Equatable {
    let event: String
    let matcher: String?
    let command: HookCommand
}

enum AgentHooks {
    static let claude: [HookSpec] = [
        .init(event: "UserPromptSubmit", matcher: nil, command: .claudePrompt),
        .init(event: "Stop", matcher: nil, command: .claudeStop),
        .init(event: "StopFailure", matcher: nil, command: .claudeStopFailure),
        .init(event: "PermissionRequest", matcher: nil, command: .claudePermission),
        .init(event: "Notification", matcher: "elicitation_dialog", command: .claudeNotification),
        .init(event: "PreToolUse", matcher: "AskUserQuestion", command: .claudeAsk),
    ]
}

extension AgentHooks {
    static let codex: [HookSpec] = [
        .init(event: "UserPromptSubmit", matcher: nil, command: .codexPrompt),
        .init(event: "Stop", matcher: nil, command: .codexStop),
        .init(event: "PermissionRequest", matcher: nil, command: .codexPermission),
        .init(event: "Interrupt", matcher: nil, command: .codexInterrupt),
    ]
}
