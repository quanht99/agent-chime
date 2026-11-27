import XCTest

final class DeployTests: IsolatedTestCase {
    var helper: URL!
    var sounds: URL!
    var dest: URL!

    override func setUp() {
        super.setUp()
        let fm = FileManager.default
        helper = home.appendingPathComponent("src-helper")
        sounds = home.appendingPathComponent("src-sounds", isDirectory: true)
        dest = home.appendingPathComponent("dest", isDirectory: true)
        do {
            try Data("helper-v1".utf8).write(to: helper)
            try fm.createDirectory(at: sounds, withIntermediateDirectories: true)
            try Data("a".utf8).write(to: sounds.appendingPathComponent("a.wav"))
            try Data("b".utf8).write(to: sounds.appendingPathComponent("b.wav"))
        } catch {
            XCTFail("fixture setup failed: \(error)")
        }
    }

    private func inode(_ url: URL) throws -> NSNumber {
        try XCTUnwrap(FileManager.default.attributesOfItem(atPath: url.path)[.systemFileNumber] as? NSNumber)
    }

    func testDeployCopiesWhenMissing() throws {
        let changed = try Deployer.sync(helper: helper, sounds: sounds, into: dest)
        XCTAssertTrue(changed.contains("agentchime-hook"))
        let target = dest.appendingPathComponent("agentchime-hook")
        let perms = try FileManager.default.attributesOfItem(atPath: target.path)[.posixPermissions] as? NSNumber
        XCTAssertEqual(perms?.intValue, 0o755)
        XCTAssertEqual(try Data(contentsOf: target), Data("helper-v1".utf8))
        XCTAssertEqual(try Data(contentsOf: dest.appendingPathComponent("Sounds/a.wav")), Data("a".utf8))
        XCTAssertTrue(FileManager.default.fileExists(atPath: dest.appendingPathComponent("Sounds/b.wav").path))
    }

    func testDeploySkipsIdentical() throws {
        _ = try Deployer.sync(helper: helper, sounds: sounds, into: dest)
        XCTAssertEqual(try Deployer.sync(helper: helper, sounds: sounds, into: dest), [])
    }

    func testDeployReplacesChanged() throws {
        _ = try Deployer.sync(helper: helper, sounds: sounds, into: dest)
        let target = dest.appendingPathComponent("agentchime-hook")
        let before = try inode(target)
        try Data("helper-v2".utf8).write(to: helper)
        XCTAssertEqual(try Deployer.sync(helper: helper, sounds: sounds, into: dest), ["agentchime-hook"])
        XCTAssertNotEqual(try inode(target), before)
        XCTAssertEqual(try Data(contentsOf: target), Data("helper-v2".utf8))
    }

    func testDeployRemovesStaleSounds() throws {
        _ = try Deployer.sync(helper: helper, sounds: sounds, into: dest)
        let note = dest.appendingPathComponent("Sounds/notes.txt")
        try Data("keep".utf8).write(to: note)
        try FileManager.default.removeItem(at: sounds.appendingPathComponent("b.wav"))
        _ = try Deployer.sync(helper: helper, sounds: sounds, into: dest)
        XCTAssertFalse(FileManager.default.fileExists(atPath: dest.appendingPathComponent("Sounds/b.wav").path))
        XCTAssertTrue(FileManager.default.fileExists(atPath: dest.appendingPathComponent("Sounds/a.wav").path))
        XCTAssertTrue(FileManager.default.fileExists(atPath: note.path))
    }

    func testDeployCreatesOnlyHelperAndSounds() throws {
        _ = try Deployer.sync(helper: helper, sounds: sounds, into: dest)
        let entries = try FileManager.default.contentsOfDirectory(atPath: dest.path).sorted()
        XCTAssertEqual(entries, ["Sounds", "agentchime-hook"])
    }
}
