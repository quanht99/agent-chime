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

/// Writes a short 16-bit mono 8 kHz PCM WAV (silence) so tests need no fixture audio files.
func makeWAV(at url: URL, frames: Int = 800) throws {
    func le32(_ v: UInt32) -> [UInt8] { (0..<4).map { UInt8((v >> (8 * UInt32($0))) & 0xff) } }
    func le16(_ v: UInt16) -> [UInt8] { [UInt8(v & 0xff), UInt8(v >> 8)] }
    let rate: UInt32 = 8000
    let dataSize = UInt32(frames * 2)
    var b: [UInt8] = []
    b += Array("RIFF".utf8) + le32(36 + dataSize) + Array("WAVE".utf8)
    b += Array("fmt ".utf8) + le32(16) + le16(1) + le16(1) + le32(rate) + le32(rate * 2) + le16(2) + le16(16)
    b += Array("data".utf8) + le32(dataSize) + [UInt8](repeating: 0, count: Int(dataSize))
    try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
    try Data(b).write(to: url)
}
