import XCTest

final class CodexTrustTests: XCTestCase {
    private let path = "/Users/example/.codex/hooks.json"
    private let helper = "/Users/example/Library/Application Support/AgentChime/agentchime-hook"
    private var toml: String { String(decoding: fixtureData("codex-config-hooks-state.toml"), as: UTF8.self) }

    private var installedHooks: JSONValue {
        let original = try! JSONValue.parse(fixtureData("codex-hooks.json"))
        return HookInstaller.merged(into: original, specs: AgentHooks.codex, helperPath: helper).root
    }
    private func entry(_ event: String, hash: Bool = true, enabled: Bool = true) -> String {
        "\n[hooks.state.\"\(path):\(event):1:0\"]\nenabled = \(enabled)\n" + (hash ? "trusted_hash = \"sha256:fake\"\n" : "")
    }
    private let ourEvents = ["user_prompt_submit", "stop", "permission_request", "interrupt"]

    func testSnakeCase() {
        XCTAssertEqual(CodexTrust.snakeCase("UserPromptSubmit"), "user_prompt_submit")
        XCTAssertEqual(CodexTrust.snakeCase("Stop"), "stop")
        XCTAssertEqual(CodexTrust.snakeCase("PermissionRequest"), "permission_request")
        XCTAssertEqual(CodexTrust.snakeCase("Interrupt"), "interrupt")
    }

    func testTrustKeysForOurGroups() {
        let keys = CodexTrust.ourHandlerKeys(hooksPath: path, hooks: installedHooks)
        XCTAssertEqual(Set(keys), Set(ourEvents.map { "\(path):\($0):1:0" }))
        XCTAssertEqual(keys.count, 4)
    }

    func testUntrustedWhenNoEntries() {
        XCTAssertEqual(CodexTrust.state(hooksPath: path, hooks: installedHooks, configTOML: toml), .untrusted(pending: 4))
    }

    func testPartiallyTrusted() {
        let t = toml + entry("user_prompt_submit") + entry("stop")
        XCTAssertEqual(CodexTrust.state(hooksPath: path, hooks: installedHooks, configTOML: t), .untrusted(pending: 2))
    }

    func testTrustedWhenAllHaveHashes() {
        let t = toml + ourEvents.map { entry($0) }.joined()
        XCTAssertEqual(CodexTrust.state(hooksPath: path, hooks: installedHooks, configTOML: t), .trusted)
    }

    func testDisabledWhenAnyEnabledFalse() {
        let t = toml + entry("user_prompt_submit") + entry("stop", enabled: false)
        XCTAssertEqual(CodexTrust.state(hooksPath: path, hooks: installedHooks, configTOML: t), .disabled)
    }

    func testNotInstalledWhenNoOurGroups() {
        let original = try! JSONValue.parse(fixtureData("codex-hooks.json"))
        XCTAssertEqual(CodexTrust.state(hooksPath: path, hooks: original, configTOML: toml), .notInstalled)
        XCTAssertEqual(CodexTrust.state(hooksPath: path, hooks: nil, configTOML: toml), .notInstalled)
    }

    func testMissingTomlMeansUntrusted() {
        XCTAssertEqual(CodexTrust.state(hooksPath: path, hooks: installedHooks, configTOML: nil), .untrusted(pending: 4))
    }

    func testCombinedPrefersTrustedFile() {
        XCTAssertEqual(CodexTrust.combined([.untrusted(pending: 4), .trusted]), .trusted)
        XCTAssertEqual(CodexTrust.combined([.disabled, .trusted]), .trusted)
    }

    func testCombinedDisabledBeatsUntrusted() {
        XCTAssertEqual(CodexTrust.combined([.untrusted(pending: 4), .disabled]), .disabled)
    }

    func testCombinedTakesFewestPending() {
        XCTAssertEqual(CodexTrust.combined([.untrusted(pending: 4), .untrusted(pending: 1)]), .untrusted(pending: 1))
    }

    func testCombinedNotInstalled() {
        XCTAssertEqual(CodexTrust.combined([.notInstalled, .notInstalled]), .notInstalled)
        XCTAssertEqual(CodexTrust.combined([]), .notInstalled)
    }
}

/// Codex 0.160+ stores a profile's hook trust in ~/.codex/<profile>.config.toml (`codex --profile work`).
final class CodexTrustProfileTests: IsolatedTestCase {
    private let helper = "/Users/example/Library/Application Support/AgentChime/agentchime-hook"
    private let ourEvents = ["user_prompt_submit", "stop", "permission_request", "interrupt"]
    private var codexDir: URL { home.appendingPathComponent(".codex", isDirectory: true) }

    override func setUp() {
        super.setUp()
        try! FileManager.default.createDirectory(at: codexDir, withIntermediateDirectories: true)
        let original = try! JSONValue.parse(fixtureData("codex-hooks.json"))
        let installed = HookInstaller.merged(into: original, specs: AgentHooks.codex, helperPath: helper).root
        try! HookInstaller.writeJSONFile(installed, to: Paths.codexHooks)
        try! fixtureData("codex-config-hooks-state.toml").write(to: Paths.codexConfig)
    }

    private func trustTOML(_ events: [String]) -> String {
        events.map { "[hooks.state.\"\(Paths.codexHooks.path):\($0):1:0\"]\ntrusted_hash = \"sha256:fake\"\n" }.joined()
    }

    func testUntrustedWithoutProfileFile() {
        XCTAssertEqual(CodexTrust.currentState(), .untrusted(pending: 4))
    }

    func testTrustInProfileFileCounts() throws {
        try trustTOML(ourEvents).write(to: codexDir.appendingPathComponent("work.config.toml"), atomically: true, encoding: .utf8)
        XCTAssertEqual(CodexTrust.currentState(), .trusted)
    }

    func testPartialProfileTrustReportsPending() throws {
        try trustTOML(["stop"]).write(to: codexDir.appendingPathComponent("work.config.toml"), atomically: true, encoding: .utf8)
        XCTAssertEqual(CodexTrust.currentState(), .untrusted(pending: 3))
    }

    func testOtherTomlFilesIgnored() throws {
        try trustTOML(ourEvents).write(to: codexDir.appendingPathComponent("notes.toml"), atomically: true, encoding: .utf8)
        XCTAssertEqual(CodexTrust.currentState(), .untrusted(pending: 4))
    }

    func testConfigFilesListsMainThenProfiles() throws {
        for name in ["b.config.toml", "a.config.toml", "notes.toml"] {
            try "".write(to: codexDir.appendingPathComponent(name), atomically: true, encoding: .utf8)
        }
        XCTAssertEqual(CodexTrust.configFiles().map(\.lastPathComponent), ["config.toml", "a.config.toml", "b.config.toml"])
    }
}
