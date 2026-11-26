import XCTest

final class RunContextTests: IsolatedTestCase {
    func testNestedDetection() {
        XCTAssertTrue(RunContext.isNested(.codex, env: ["CLAUDECODE": "1"]))
        XCTAssertTrue(RunContext.isNested(.claude, env: ["CODEX_THREAD_ID": "x"]))
        XCTAssertFalse(RunContext.isNested(.claude, env: ["CLAUDECODE": "1"]))
        XCTAssertFalse(RunContext.isNested(.codex, env: ["CLAUDECODE": ""]))
    }

    func testClaudeHeadlessByEntrypoint() {
        XCTAssertTrue(RunContext.isHeadless(.claude, env: ["CLAUDE_CODE_ENTRYPOINT": "sdk-cli"], ancestors: []))
        XCTAssertFalse(RunContext.isHeadless(.claude, env: ["CLAUDE_CODE_ENTRYPOINT": "cli"], ancestors: []))
        XCTAssertFalse(RunContext.isHeadless(.claude, env: ["CLAUDE_CODE_ENTRYPOINT": "claude-desktop"], ancestors: []))
        XCTAssertFalse(RunContext.isHeadless(.claude, env: [:], ancestors: []))
    }

    func testCodexHeadlessByExecAncestor() {
        XCTAssertTrue(RunContext.isHeadless(.codex, env: [:], ancestors: [["/bin/zsh", "-lc", "x"],
                                                                       ["/Users/example/.local/bin/codex", "exec", "hi"]]))
        XCTAssertTrue(RunContext.isHeadless(.codex, env: [:], ancestors: [["node", "/a/bin/codex.js", "e", "hi"]]))
        XCTAssertFalse(RunContext.isHeadless(.codex, env: [:], ancestors: [["codex"]]))
        XCTAssertFalse(RunContext.isHeadless(.codex, env: [:], ancestors: [["codex", "resume"]]))
    }

    func testProbeLineRedactsAndOmitsPrompt() {
        let env = ["CLAUDE_CODE_OAUTH_TOKEN": "tok123", "CLAUDE_CODE_ENTRYPOINT": "cli", "HOME": "/Users/example"]
        let payload = Data(#"{"prompt":"secret plan","hook_event_name":"Stop"}"#.utf8)
        let line = RunContext.probeLine(command: .claudeStop, env: env, ancestors: [["/usr/bin/claude", "--resume-long-argument-value"]],
                                        payload: payload)
        XCTAssertFalse(line.contains("CLAUDE_CODE_OAUTH_TOKEN"))
        XCTAssertFalse(line.contains("tok123"))
        XCTAssertTrue(line.contains("CLAUDE_CODE_ENTRYPOINT=cli"))
        XCTAssertFalse(line.contains("secret plan"))
        XCTAssertTrue(line.contains("prompt"))
        XCTAssertTrue(line.contains("hook_event_name=Stop"))
        XCTAssertFalse(line.contains("/Users/example"))
        XCTAssertFalse(line.contains("--resume-long-argument-value"))
    }

    func testAncestorCommandLinesWorks() {
        let lines = RunContext.ancestorCommandLines()
        XCTAssertFalse(lines.isEmpty)
        XCTAssertFalse(lines[0].isEmpty)
        XCTAssertFalse(lines[0][0].isEmpty)
    }
}
