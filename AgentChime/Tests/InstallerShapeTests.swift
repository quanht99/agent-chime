import XCTest

final class InstallerShapeTests: IsolatedTestCase {
    private var settings: URL { Paths.claudeSettings }
    private let helper = "/Users/example/Library/Application Support/AgentChime/agentchime-hook"

    private let shapes: [(name: String, json: String)] = [
        ("root array", "[1, 2]"),
        ("root string", "\"hello\""),
        ("hooks array", "{\"hooks\": []}"),
        ("hooks string", "{\"hooks\": \"x\"}"),
        ("event object", "{\"hooks\": {\"Stop\": {\"a\": 1}}}"),
        ("event string", "{\"hooks\": {\"Notification\": \"x\"}}"),
    ]

    private func seed(_ json: String) throws -> Data {
        try FileManager.default.createDirectory(at: settings.deletingLastPathComponent(), withIntermediateDirectories: true)
        let data = Data(json.utf8)
        try data.write(to: settings)
        return data
    }

    func testInstallRefusesUnexpectedShapes() throws {
        for shape in shapes {
            let original = try seed(shape.json)
            XCTAssertThrowsError(try HookInstaller.install(specs: AgentHooks.claude, fileURL: settings, helperPath: helper), shape.name) { error in
                XCTAssertEqual(error as? InstallError, InstallError(path: settings.path), shape.name)
            }
            XCTAssertEqual(try Data(contentsOf: settings), original, shape.name)
        }
    }

    func testUninstallRefusesUnexpectedShapes() throws {
        for shape in shapes {
            let original = try seed(shape.json)
            XCTAssertThrowsError(try HookInstaller.uninstall(fileURL: settings), shape.name) { error in
                XCTAssertEqual(error as? InstallError, InstallError(path: settings.path), shape.name)
            }
            XCTAssertEqual(try Data(contentsOf: settings), original, shape.name)
        }
    }

    func testStatusReportsUnreadableForUnexpectedShapes() throws {
        for shape in shapes {
            _ = try seed(shape.json)
            XCTAssertEqual(HookInstaller.status(specs: AgentHooks.claude, fileURL: settings, helperPath: helper),
                           .unreadable(path: settings.path), shape.name)
        }
    }

    func testBackupsInSameSecondDoNotOverwriteEachOther() throws {
        let original = try seed("{\"a\": 1}")
        try HookInstaller.writeJSONFile(try JSONValue.parse(Data("{\"a\": 2}".utf8)), to: settings)
        try HookInstaller.writeJSONFile(try JSONValue.parse(Data("{\"a\": 3}".utf8)), to: settings)
        try HookInstaller.writeJSONFile(try JSONValue.parse(Data("{\"a\": 4}".utf8)), to: settings)
        let files = try FileManager.default.contentsOfDirectory(atPath: Paths.backupsDir.path)
        XCTAssertEqual(files.count, 3)
        let contents = try files.map { try Data(contentsOf: Paths.backupsDir.appendingPathComponent($0)) }
        XCTAssertTrue(contents.contains(original), "pristine backup must survive")
    }
}
