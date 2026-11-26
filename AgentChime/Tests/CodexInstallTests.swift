import XCTest

final class CodexInstallTests: IsolatedTestCase {
    private var hooksFile: URL { Paths.codexHooks }
    private let helper = "/Users/example/Library/Application Support/AgentChime/agentchime-hook"

    private func seedFixture() throws -> Data {
        try FileManager.default.createDirectory(at: hooksFile.deletingLastPathComponent(), withIntermediateDirectories: true)
        let data = fixtureData("codex-hooks.json")
        XCTAssertFalse(data.isEmpty)
        try data.write(to: hooksFile)
        return data
    }

    @discardableResult
    private func install() throws -> Bool {
        try HookInstaller.install(specs: AgentHooks.codex, fileURL: hooksFile, helperPath: helper)
    }

    func testInstallAppendsAfterExistingGroups() throws {
        let original = try seedFixture()
        let before = try JSONValue.parse(original)["hooks"]!
        try install()
        let after = try JSONValue.parse(Data(contentsOf: hooksFile))["hooks"]!
        XCTAssertEqual(after.objectKeys, before.objectKeys)
        let touched: Set<String> = ["UserPromptSubmit", "Stop", "PermissionRequest", "Interrupt"]
        for key in before.objectKeys! {
            let old = before[key]!.arrayValue!
            let new = after[key]!.arrayValue!
            if touched.contains(key) {
                XCTAssertEqual(new.count, 2, key)
                XCTAssertEqual(new[0], old[0], key)
                XCTAssertTrue(HookInstaller.isOurGroup(new[1]), key)
                XCTAssertEqual(new[1]["hooks"]?.arrayValue?.first?["async"]?.boolValue, true)
            } else {
                XCTAssertEqual(new, old, key)
            }
        }
    }

    func testInstallIsIdempotent() throws {
        _ = try seedFixture()
        try install()
        let once = try Data(contentsOf: hooksFile)
        XCTAssertFalse(try HookInstaller.install(specs: AgentHooks.codex, fileURL: hooksFile, helperPath: helper))
        XCTAssertEqual(try Data(contentsOf: hooksFile), once)
        XCTAssertEqual(HookInstaller.status(specs: AgentHooks.codex, fileURL: hooksFile, helperPath: helper), .installed)
    }

    func testInstallThenUninstallIsByteIdentical() throws {
        let original = try seedFixture()
        try install()
        XCTAssertNotEqual(try Data(contentsOf: hooksFile), original)
        try HookInstaller.uninstall(fileURL: hooksFile)
        XCTAssertEqual(try Data(contentsOf: hooksFile), original)
    }

    func testInvalidHooksJSONIsNeverOverwritten() throws {
        try FileManager.default.createDirectory(at: hooksFile.deletingLastPathComponent(), withIntermediateDirectories: true)
        let bad = Data("{ not json".utf8)
        try bad.write(to: hooksFile)
        XCTAssertThrowsError(try install())
        XCTAssertThrowsError(try HookInstaller.uninstall(fileURL: hooksFile))
        XCTAssertEqual(try Data(contentsOf: hooksFile), bad)
        XCTAssertEqual(HookInstaller.status(specs: AgentHooks.codex, fileURL: hooksFile, helperPath: helper),
                       .unreadable(path: hooksFile.path))
    }

    func testMissingHooksFileCreatesMinimal() throws {
        XCTAssertFalse(FileManager.default.fileExists(atPath: hooksFile.deletingLastPathComponent().path))
        try install()
        let root = try JSONValue.parse(Data(contentsOf: hooksFile))
        XCTAssertEqual(root.objectKeys, ["hooks"])
        XCTAssertEqual(root["hooks"]?.objectKeys, ["UserPromptSubmit", "Stop", "PermissionRequest", "Interrupt"])
    }
}
