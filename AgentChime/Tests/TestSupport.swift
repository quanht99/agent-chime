import XCTest

class IsolatedTestCase: XCTestCase {
    var home: URL!

    override func setUp() {
        super.setUp()
        home = FileManager.default.temporaryDirectory
            .appendingPathComponent("agentchime-test-\(UUID().uuidString)", isDirectory: true)
        try? FileManager.default.createDirectory(at: home, withIntermediateDirectories: true)
        setenv("AGENTCHIME_HOME", home.path, 1)
    }

    override func tearDown() {
        unsetenv("AGENTCHIME_HOME")
        try? FileManager.default.removeItem(at: home)
        home = nil
        super.tearDown()
    }
}

func fixtureURL(_ name: String) -> URL {
    URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()
        .appendingPathComponent("Fixtures")
        .appendingPathComponent(name)
}

func fixtureData(_ name: String) -> Data {
    (try? Data(contentsOf: fixtureURL(name))) ?? Data()
}
