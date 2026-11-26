import XCTest

final class PathsLogTests: IsolatedTestCase {
    func testHomeOverride() {
        XCTAssertEqual(Paths.home.path, home.path)
        XCTAssertEqual(Paths.claudeSettings.path, home.appendingPathComponent(".claude/settings.json").path)
        XCTAssertTrue(Paths.appSupport.path.hasSuffix("Library/Application Support/AgentChime"))
    }

    private func logLines() -> [String] {
        let text = (try? String(contentsOf: Paths.logFile, encoding: .utf8)) ?? ""
        return text.split(separator: "\n").map(String.init)
    }

    func testLogAppendsLines() {
        Log.write("a")
        Log.write("b")
        let lines = logLines()
        XCTAssertEqual(lines.count, 2)
        XCTAssertTrue(lines[0].hasSuffix(" a"))
        XCTAssertTrue(lines[1].hasSuffix(" b"))
        XCTAssertTrue(lines[0].hasPrefix("20"))
    }

    func testLogRotatesPastMaxBytes() throws {
        Paths.ensureDirectory(Paths.logFile.deletingLastPathComponent())
        try Data(repeating: 0x61, count: Log.maxBytes + 1).write(to: Paths.logFile)
        Log.write("x")
        let rotated = Paths.logFile.deletingLastPathComponent().appendingPathComponent("agentchime.log.1")
        XCTAssertTrue(FileManager.default.fileExists(atPath: rotated.path))
        let lines = logLines()
        XCTAssertEqual(lines.count, 1)
        XCTAssertTrue(lines[0].hasSuffix(" x"))
    }
}
