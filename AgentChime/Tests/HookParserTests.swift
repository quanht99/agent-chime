import XCTest

final class HookParserTests: XCTestCase {
    private func act(_ c: HookCommand, _ json: String) -> HookAction {
        HookParser.action(for: c, payload: Data(json.utf8))
    }

    func testPromptStartsTurn() {
        XCTAssertEqual(act(.claudePrompt, #"{"session_id":"s1"}"#), .turnStarted(session: "s1"))
        XCTAssertEqual(act(.claudePrompt, "{}"), .ignore(reason: "no session id"))
    }

    func testStopAndStopFailureAreDone() {
        let done = HookAction.alert(AgentEvent(agent: .claude, kind: .done, sessionID: "s1"))
        XCTAssertEqual(act(.claudeStop, #"{"session_id":"s1"}"#), done)
        XCTAssertEqual(act(.claudeStopFailure, #"{"session_id":"s1"}"#), done)
    }

    func testStopHookActiveIgnored() {
        XCTAssertEqual(act(.claudeStop, #"{"session_id":"s1","stop_hook_active":true}"#),
                       .ignore(reason: "stop_hook_active"))
    }

    func testPermissionIsAttentionWithToolName() {
        XCTAssertEqual(act(.claudePermission, #"{"session_id":"s1","tool_name":"Bash"}"#),
                       .alert(AgentEvent(agent: .claude, kind: .attention, sessionID: "s1", detail: "Bash")))
    }

    func testElicitationIsAttention() {
        XCTAssertEqual(act(.claudeNotification, #"{"session_id":"s1","notification_type":"elicitation_dialog","message":"Pick"}"#),
                       .alert(AgentEvent(agent: .claude, kind: .attention, sessionID: "s1", detail: "Pick")))
    }

    func testIdlePromptIgnored() {
        XCTAssertEqual(act(.claudeNotification, #"{"session_id":"s1","notification_type":"idle_prompt"}"#),
                       .ignore(reason: "notification idle_prompt"))
        XCTAssertEqual(act(.claudeNotification, #"{"session_id":"s1"}"#),
                       .ignore(reason: "notification none"))
    }

    func testPermissionPromptNotificationIgnored() {
        XCTAssertEqual(act(.claudeNotification, #"{"session_id":"s1","notification_type":"permission_prompt"}"#),
                       .ignore(reason: "notification permission_prompt"))
    }

    func testAskIsAttention() {
        XCTAssertEqual(act(.claudeAsk, #"{"session_id":"s1"}"#),
                       .alert(AgentEvent(agent: .claude, kind: .attention, sessionID: "s1", detail: "AskUserQuestion")))
    }

    func testGarbageStopStillAlerts() {
        let done = HookAction.alert(AgentEvent(agent: .claude, kind: .done, sessionID: nil))
        XCTAssertEqual(act(.claudeStop, "not json"), done)
        XCTAssertEqual(act(.claudeStopFailure, ""), done)
        XCTAssertEqual(act(.codexStop, "not json"),
                       .alert(AgentEvent(agent: .codex, kind: .done, sessionID: nil)))
    }

    func testGarbagePermissionIsIgnored() {
        XCTAssertEqual(act(.claudePermission, "not json"), .ignore(reason: "invalid payload"))
        XCTAssertEqual(act(.claudePrompt, ""), .ignore(reason: "invalid payload"))
    }

    func testCodexSessionFallsBackToTurnID() {
        XCTAssertEqual(act(.codexPrompt, #"{"turn_id":"t1"}"#), .turnStarted(session: "t1"))
        XCTAssertEqual(act(.codexPrompt, #"{"session_id":"s","turn_id":"t1"}"#), .turnStarted(session: "s"))
    }

    func testCodexStopPermissionInterrupt() {
        XCTAssertEqual(act(.codexStop, #"{"session_id":"s1"}"#),
                       .alert(AgentEvent(agent: .codex, kind: .done, sessionID: "s1")))
        XCTAssertEqual(act(.codexStop, #"{"session_id":"s1","stop_hook_active":true}"#),
                       .ignore(reason: "stop_hook_active"))
        XCTAssertEqual(act(.codexPermission, #"{"session_id":"s1","tool_name":"shell"}"#),
                       .alert(AgentEvent(agent: .codex, kind: .attention, sessionID: "s1", detail: "shell")))
        XCTAssertEqual(act(.codexInterrupt, #"{"session_id":"s1"}"#), .turnCancelled(session: "s1"))
        XCTAssertEqual(act(.codexInterrupt, "{}"), .ignore(reason: "no session id"))
    }

    // MARK: background work (Claude Code ends a turn while shell tasks or subagents keep running)

    func testStopWithRunningBackgroundWorkHoldsTheTurn() {
        let one = #"{"session_id":"s1","background_tasks":[{"id":"b1","type":"shell","status":"running"}]}"#
        XCTAssertEqual(act(.claudeStop, one), .turnHeld(session: "s1", running: 1))
        let two = #"{"session_id":"s1","background_tasks":[{"type":"shell","status":"running"},{"type":"subagent","status":"running"}]}"#
        XCTAssertEqual(act(.claudeStop, two), .turnHeld(session: "s1", running: 2))
    }

    func testStopWithoutRunningBackgroundWorkIsDone() {
        let done = HookAction.alert(AgentEvent(agent: .claude, kind: .done, sessionID: "s1"))
        XCTAssertEqual(act(.claudeStop, #"{"session_id":"s1","background_tasks":[]}"#), done)
        XCTAssertEqual(act(.claudeStop, #"{"session_id":"s1","background_tasks":"x"}"#), done)
        XCTAssertEqual(act(.claudeStop, #"{"session_id":"s1","background_tasks":[42,"y",null]}"#), done)
        XCTAssertEqual(act(.claudeStop, #"{"session_id":"s1","background_tasks":[{"status":"completed"},{"type":"shell"}]}"#), done)
    }

    func testRunningBackgroundWorkWithoutSessionStillAlerts() {
        XCTAssertEqual(act(.claudeStop, #"{"background_tasks":[{"status":"running"}]}"#),
                       .alert(AgentEvent(agent: .claude, kind: .done, sessionID: nil)))
    }

    func testStopHookActiveWinsOverBackgroundWork() {
        XCTAssertEqual(act(.claudeStop, #"{"session_id":"s1","stop_hook_active":true,"background_tasks":[{"status":"running"}]}"#),
                       .ignore(reason: "stop_hook_active"))
    }

    func testBackgroundHandBackPromptsContinueTheTurn() {
        XCTAssertEqual(act(.claudePrompt, #"{"session_id":"s1","prompt":"<task-notification>\n<task-id>b1</task-id>"}"#),
                       .turnContinued(session: "s1"))
        XCTAssertEqual(act(.claudePrompt, #"{"session_id":"s1","prompt":"<agent-message from=\"a1\">\n[Subagent hand-back]"}"#),
                       .turnContinued(session: "s1"))
        XCTAssertEqual(act(.claudePrompt, #"{"session_id":"s1","prompt":"  \n<task-notification>x"}"#),
                       .turnContinued(session: "s1"))
    }

    func testTypedPromptsStartATurn() {
        XCTAssertEqual(act(.claudePrompt, #"{"session_id":"s1","prompt":"fix the <task-notification> parser"}"#),
                       .turnStarted(session: "s1"))
        XCTAssertEqual(act(.claudePrompt, #"{"session_id":"s1","prompt":"<agent-messages are odd"}"#),
                       .turnStarted(session: "s1"))
        XCTAssertEqual(act(.claudePrompt, #"{"session_id":"s1","prompt":42}"#), .turnStarted(session: "s1"))
        XCTAssertEqual(act(.codexPrompt, #"{"session_id":"s1","prompt":"<task-notification>x"}"#),
                       .turnStarted(session: "s1"))
    }
}
