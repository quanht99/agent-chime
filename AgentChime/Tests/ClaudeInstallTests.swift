import XCTest

final class ClaudeInstallTests: IsolatedTestCase {
    private var settings: URL { Paths.claudeSettings }
    private let helper = "/Users/example/Library/Application Support/AgentChime/agentchime-hook"

    private func seedFixture() throws -> Data {
        try FileManager.default.createDirectory(at: settings.deletingLastPathComponent(), withIntermediateDirectories: true)
        let data = fixtureData("claude-settings.json")
        XCTAssertFalse(data.isEmpty)
        try data.write(to: settings)
        return data
    }

    private func install() throws -> Bool {
        try HookInstaller.install(specs: AgentHooks.claude, fileURL: settings, helperPath: helper)
    }

    private func parsed(_ url: URL? = nil) throws -> JSONValue {
        try JSONValue.parse(Data(contentsOf: url ?? settings))
    }

    func testInstallAppendsSixGroupsAndKeepsEverythingElse() throws {
        let original = try seedFixture()
        let before = try JSONValue.parse(original)
        XCTAssertFalse(try install())
        let after = try parsed()

        XCTAssertEqual(after.objectKeys, before.objectKeys)
        let beforeHooks = before["hooks"]!
        let afterHooks = after["hooks"]!
        for key in beforeHooks.objectKeys! {
            let old = beforeHooks[key]!.arrayValue!
            let new = afterHooks[key]!.arrayValue!
            XCTAssertEqual(Array(new.prefix(old.count)), old, key)
            for g in new.dropFirst(old.count) { XCTAssertTrue(HookInstaller.isOurGroup(g)) }
        }
        XCTAssertEqual(afterHooks.objectKeys!.last, "Notification")
        XCTAssertFalse(beforeHooks.objectKeys!.contains("Notification"))
        let ours = afterHooks.objectKeys!.flatMap { k in
            afterHooks[k]!.arrayValue!.filter { HookInstaller.isOurGroup($0) }
        }
        XCTAssertEqual(ours.count, 6)
        // other top-level keys untouched
        for key in before.objectKeys! where key != "hooks" {
            XCTAssertEqual(after[key], before[key])
        }
    }

    func testGroupsAreAsyncWithExpectedMatchers() throws {
        _ = try seedFixture()
        _ = try install()
        let hooks = try parsed()["hooks"]!
        func ours(_ event: String) -> JSONValue {
            hooks[event]!.arrayValue!.last { HookInstaller.isOurGroup($0) }!
        }
        for spec in AgentHooks.claude {
            let g = ours(spec.event)
            XCTAssertEqual(g["matcher"]?.stringValue, spec.matcher, spec.event)
            let h = g["hooks"]!.arrayValue![0]
            XCTAssertEqual(h["async"]?.boolValue, true)
            XCTAssertEqual(h["type"]?.stringValue, "command")
            XCTAssertEqual(h["command"]?.stringValue,
                           HookInstaller.command(helperPath: helper, subcommand: spec.command))
        }
        XCTAssertEqual(ours("Notification")["matcher"]?.stringValue, "elicitation_dialog")
        XCTAssertEqual(ours("PreToolUse")["matcher"]?.stringValue, "AskUserQuestion")
        XCTAssertNil(ours("Stop")["matcher"])
    }

    func testInstallIsIdempotent() throws {
        _ = try seedFixture()
        XCTAssertFalse(try install())
        let first = try Data(contentsOf: settings)
        XCTAssertFalse(try install())
        XCTAssertEqual(try Data(contentsOf: settings), first)
    }

    func testReinstallReplacesOldCommandInPlace() throws {
        _ = try seedFixture()
        _ = try install()
        var root = try parsed()
        var stop = root["hooks"]!["Stop"]!.arrayValue!
        let idx = stop.firstIndex { HookInstaller.isOurGroup($0) }!
        stop[idx] = HookInstaller.group(for: AgentHooks.claude[1], helperPath: "/old/agentchime-hook")
        stop.append(.object([JSONMember(key: "hooks", value: .array([.object([
            JSONMember(key: "type", value: .string("command")),
            JSONMember(key: "command", value: .string("/Users/example/bin/stranger"))])]))]))
        var hooks = root["hooks"]!
        hooks.set("Stop", .array(stop))
        root.set("hooks", hooks)
        try root.serialized().appending("\n").write(to: settings, atomically: true, encoding: .utf8)

        XCTAssertTrue(try install())
        let newStop = try parsed()["hooks"]!["Stop"]!.arrayValue!
        XCTAssertEqual(newStop.count, stop.count)
        XCTAssertEqual(newStop[idx], HookInstaller.group(for: AgentHooks.claude[1], helperPath: helper))
        XCTAssertEqual(newStop.last, stop.last)
    }

    func testInstallThenUninstallIsByteIdentical() throws {
        let original = try seedFixture()
        _ = try install()
        XCTAssertNotEqual(try Data(contentsOf: settings), original)
        try HookInstaller.uninstall(fileURL: settings)
        XCTAssertEqual(try Data(contentsOf: settings), original)
    }

    func testInstallIntoMissingFileCreatesMinimalSettings() throws {
        XCTAssertFalse(FileManager.default.fileExists(atPath: settings.path))
        _ = try install()
        let root = try parsed()
        XCTAssertEqual(root.objectKeys, ["hooks"])
        XCTAssertEqual(HookInstaller.status(specs: AgentHooks.claude, fileURL: settings, helperPath: helper), .installed)
    }

    func testInvalidJSONIsNeverOverwritten() throws {
        try FileManager.default.createDirectory(at: settings.deletingLastPathComponent(), withIntermediateDirectories: true)
        let broken = Data("{ broken".utf8)
        try broken.write(to: settings)
        XCTAssertThrowsError(try install()) { XCTAssertEqual($0 as? InstallError, InstallError(path: settings.path)) }
        XCTAssertThrowsError(try HookInstaller.uninstall(fileURL: settings)) {
            XCTAssertEqual($0 as? InstallError, InstallError(path: settings.path))
        }
        XCTAssertEqual(try Data(contentsOf: settings), broken)
        XCTAssertEqual(HookInstaller.status(specs: AgentHooks.claude, fileURL: settings, helperPath: helper),
                       .unreadable(path: settings.path))
    }

    func testFilePermissionsPreserved() throws {
        _ = try seedFixture()
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: settings.path)
        _ = try install()
        let perms = try FileManager.default.attributesOfItem(atPath: settings.path)[.posixPermissions] as? NSNumber
        XCTAssertEqual(perms?.intValue, 0o600)
    }

    func testSymlinkStaysSymlink() throws {
        let dotfiles = home.appendingPathComponent("dotfiles", isDirectory: true)
        try FileManager.default.createDirectory(at: dotfiles, withIntermediateDirectories: true)
        let target = dotfiles.appendingPathComponent("settings.json")
        try fixtureData("claude-settings.json").write(to: target)
        try FileManager.default.createDirectory(at: settings.deletingLastPathComponent(), withIntermediateDirectories: true)
        try FileManager.default.createSymbolicLink(at: settings, withDestinationURL: target)

        _ = try install()
        XCTAssertNotNil(try? FileManager.default.destinationOfSymbolicLink(atPath: settings.path))
        let root = try parsed(target)
        XCTAssertEqual(HookInstaller.status(of: root, specs: AgentHooks.claude, helperPath: helper), .installed)
    }

    func testBackupWritten() throws {
        let original = try seedFixture()
        _ = try install()
        let files = try FileManager.default.contentsOfDirectory(atPath: Paths.backupsDir.path)
        XCTAssertEqual(files.count, 1)
        XCTAssertTrue(files[0].hasPrefix("settings.json."))
        XCTAssertEqual(try Data(contentsOf: Paths.backupsDir.appendingPathComponent(files[0])), original)
    }

    func testBackupsPrunedToTen() throws {
        try FileManager.default.createDirectory(at: Paths.backupsDir, withIntermediateDirectories: true)
        for i in 0..<12 {
            let name = String(format: "settings.json.20200101-0000%02d", i)
            try Data("x".utf8).write(to: Paths.backupsDir.appendingPathComponent(name))
        }
        _ = try seedFixture()
        _ = try install()
        let files = try FileManager.default.contentsOfDirectory(atPath: Paths.backupsDir.path)
        XCTAssertEqual(files.count, 10)
        XCTAssertFalse(files.contains("settings.json.20200101-000000"))
    }

    func testStatusTransitions() throws {
        _ = try seedFixture()
        XCTAssertEqual(HookInstaller.status(specs: AgentHooks.claude, fileURL: settings, helperPath: helper), .notInstalled)
        _ = try install()
        XCTAssertEqual(HookInstaller.status(specs: AgentHooks.claude, fileURL: settings, helperPath: helper), .installed)

        var root = try parsed()
        var hooks = root["hooks"]!
        let stop = hooks["Stop"]!.arrayValue!.filter { !HookInstaller.isOurGroup($0) }
        hooks.set("Stop", .array(stop))
        root.set("hooks", hooks)
        XCTAssertEqual(HookInstaller.status(of: root, specs: AgentHooks.claude, helperPath: helper),
                       .partial(installed: 5, total: 6))
    }

    func testCommandRunsThroughShell() throws {
        let dir = home.appendingPathComponent("a b", isDirectory: true)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let script = dir.appendingPathComponent("agentchime-hook")
        let out = home.appendingPathComponent("out.txt")
        try "#!/bin/sh\nprintf '%s' \"$1\" > '\(out.path)'\n".write(to: script, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: script.path)

        let p = Process()
        p.executableURL = URL(fileURLWithPath: "/bin/sh")
        p.arguments = ["-c", HookInstaller.command(helperPath: script.path, subcommand: .claudeStop)]
        try p.run()
        p.waitUntilExit()
        XCTAssertEqual(try String(contentsOf: out), "claude-stop")
    }

    func testCommandEscapesSpecialCharacters() {
        XCTAssertEqual(HookInstaller.command(helperPath: #"/a"b$c`d\e"#, subcommand: .claudeStop),
                       #""/a\"b\$c\`d\\e" claude-stop"#)
    }

    func testIsOurGroupRequiresAllHandlersToMatch() {
        let ours = HookInstaller.group(for: AgentHooks.claude[0], helperPath: helper)
        XCTAssertTrue(HookInstaller.isOurGroup(ours))
        XCTAssertFalse(HookInstaller.isOurGroup(.object([JSONMember(key: "hooks", value: .array([]))])))
        XCTAssertFalse(HookInstaller.isOurGroup(.string("x")))
    }
}
