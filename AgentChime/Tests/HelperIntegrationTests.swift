import XCTest

final class HelperIntegrationTests: IsolatedTestCase {
    /// The spec's budget is 0.5 s; the bound is 1 s because a loaded machine (or macOS checking a
    /// freshly built binary) has pushed one run to 0.93 s. A helper that ignored its 0.3 s stdin
    /// deadline would still fail: the safety net below only kills it after 3 s.
    private static let runtimeBound: TimeInterval = 1.0
    private static var warmedUp = false

    override func setUp() {
        super.setUp() // sets `home`
        // The first exec of a new binary pays a one-off system check; keep it out of the timings.
        if !Self.warmedUp {
            Self.warmedUp = true
            _ = try? run(["no-such-command"], stdin: Data())
        }
    }

    private func helperURL() throws -> URL {
        let url = Bundle(for: Self.self).bundleURL.deletingLastPathComponent().appendingPathComponent("agentchime-hook")
        try XCTSkipUnless(FileManager.default.isExecutableFile(atPath: url.path), "agentchime-hook not built next to the test bundle")
        return url
    }

    private func muteEverything() throws {
        var config = AgentChimeConfig()
        config.mute(for: nil, now: Date())
        try ConfigStore.save(config)
    }

    private struct Result { var status: Int32; var stdout: Data; var seconds: TimeInterval }

    /// `stdin == nil` leaves the pipe open and never closed.
    private func run(_ args: [String], stdin: Data?) throws -> Result {
        let p = Process()
        p.executableURL = try helperURL()
        p.arguments = args
        p.environment = ["AGENTCHIME_HOME": home.path, "PATH": "/usr/bin:/bin", "HOME": home.path]
        let inPipe = Pipe(), outPipe = Pipe()
        p.standardInput = inPipe
        p.standardOutput = outPipe
        p.standardError = FileHandle.nullDevice
        let start = Date()
        try p.run()
        if let stdin = stdin {
            inPipe.fileHandleForWriting.write(stdin)
            try? inPipe.fileHandleForWriting.close()
        }
        // Safety net so a hanging helper fails the test instead of hanging the suite.
        DispatchQueue.global().asyncAfter(deadline: .now() + 3) { if p.isRunning { p.terminate() } }
        p.waitUntilExit()
        let seconds = Date().timeIntervalSince(start)
        let out = outPipe.fileHandleForReading.readDataToEndOfFile()
        if stdin == nil { try? inPipe.fileHandleForWriting.close() }
        return Result(status: p.terminationStatus, stdout: out, seconds: seconds)
    }

    private func assertQuietSuccess(_ r: Result, _ name: String, file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertEqual(r.status, 0, name, file: file, line: line)
        XCTAssertTrue(r.stdout.isEmpty, "\(name) stdout", file: file, line: line)
        XCTAssertLessThan(r.seconds, Self.runtimeBound, "\(name) runtime", file: file, line: line)
    }

    func testPromptAndStopExitQuietly() throws {
        try muteEverything()
        let payload = Data("{\"session_id\":\"s1\",\"hook_event_name\":\"x\"}".utf8)
        assertQuietSuccess(try run(["claude-prompt"], stdin: payload), "claude-prompt")
        assertQuietSuccess(try run(["claude-stop"], stdin: payload), "claude-stop")
    }

    func testUnknownSubcommandExitsQuietly() throws {
        try muteEverything()
        assertQuietSuccess(try run(["no-such-command"], stdin: Data()), "unknown")
    }

    func testGarbagePayloadExitsQuietly() throws {
        try muteEverything()
        assertQuietSuccess(try run(["claude-stop"], stdin: Data("not json \u{0}\u{1}".utf8)), "garbage")
    }

    func testOpenStdinStillExitsWithinDeadline() throws {
        try muteEverything()
        assertQuietSuccess(try run(["claude-stop"], stdin: nil), "open stdin")
    }

    /// The helper wiring for a turn that ends while background work runs: the first Stop must not
    /// alert or drop the start, the hand-back keeps it, and the last Stop (nothing running) alerts.
    func testBackgroundWorkHoldsTheTurnUntilTheLastStop() throws {
        try muteEverything()
        func send(_ command: String, _ json: String) throws {
            assertQuietSuccess(try run([command], stdin: Data(json.utf8)), command)
        }
        try send("claude-prompt", #"{"session_id":"s1","prompt":"start two background jobs"}"#)
        let start = try XCTUnwrap(StateStore.load().turnStarts["claude|s1"])

        try send("claude-stop", #"{"session_id":"s1","background_tasks":[{"type":"shell","status":"running"}]}"#)
        XCTAssertEqual(StateStore.load().turnStarts["claude|s1"], start)
        XCTAssertNil(StateStore.load().lastSkip, "a held Stop does not alert")

        try send("claude-prompt", #"{"session_id":"s1","prompt":"<task-notification>\n<task-id>b1</task-id>"}"#)
        XCTAssertEqual(StateStore.load().turnStarts["claude|s1"], start)

        try send("claude-stop", #"{"session_id":"s1","background_tasks":[]}"#)
        let s = StateStore.load()
        XCTAssertNil(s.turnStarts["claude|s1"])
        XCTAssertEqual(s.lastSkip?.kind, .done) // muted, so the done alert is recorded as a skip
    }
}
