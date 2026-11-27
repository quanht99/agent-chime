import CryptoKit
import XCTest

final class UpdateCoreTests: XCTestCase {
    private func vec(_ name: String) -> Data { fixtureData("update/\(name)") }

    /// The TEST-ONLY public key, read from the fixtures. Never the release key.
    private func testKey() throws -> Curve25519.Signing.PublicKey {
        let hex = String(decoding: vec("test-public-key.hex"), as: UTF8.self)
            .trimmingCharacters(in: .whitespacesAndNewlines)
        var bytes = [UInt8]()
        var i = hex.startIndex
        while i < hex.endIndex {
            let j = hex.index(i, offsetBy: 2)
            bytes.append(UInt8(hex[i..<j], radix: 16)!)
            i = j
        }
        return try Curve25519.Signing.PublicKey(rawRepresentation: bytes)
    }

    func testGoodVectorVerifies() throws {
        let m = try parseVerified(body: vec("good.json"), sig: vec("good.json.sig"), key: try testKey())
        XCTAssertEqual(m.version, "0.9.1")
        XCTAssertEqual(m.files.count, 5)
        XCTAssertEqual(try m.macFile().name, "AgentChime-0.9.1-macos.zip")
    }

    func testTamperedFails() throws {
        XCTAssertThrowsError(try parseVerified(body: vec("tampered.json"), sig: vec("good.json.sig"), key: try testKey())) {
            XCTAssertEqual($0 as? UpdateError, .unverified)
        }
    }

    func testWrongKeyFails() throws {
        XCTAssertThrowsError(try parseVerified(body: vec("good.json"), sig: vec("wrongkey.json.sig"), key: try testKey())) {
            XCTAssertEqual($0 as? UpdateError, .unverified)
        }
    }

    func testFormat2Fails() throws {
        XCTAssertThrowsError(try parseVerified(body: vec("format2.json"), sig: vec("format2.json.sig"), key: try testKey())) {
            XCTAssertEqual($0 as? UpdateError, .badManifest)
        }
    }

    func testNoMacFile() throws {
        let m = try parseVerified(body: vec("nomac.json"), sig: vec("nomac.json.sig"), key: try testKey())
        XCTAssertThrowsError(try m.macFile()) { XCTAssertEqual($0 as? UpdateError, .noFile) }
    }

    func testOversizedInputsAreUnverified() throws {
        let big = Data(count: 64 * 1024 + 1)
        XCTAssertThrowsError(try parseVerified(body: big, sig: vec("good.json.sig"), key: try testKey())) {
            XCTAssertEqual($0 as? UpdateError, .unverified)
        }
        XCTAssertThrowsError(try parseVerified(body: vec("good.json"), sig: Data(count: 201), key: try testKey())) {
            XCTAssertEqual($0 as? UpdateError, .unverified)
        }
    }

    func testCompareVersions() {
        let cases: [(String, String, Int?)] = [
            ("0.3.0", "0.3.1", -1), ("0.3.1", "0.3.1", 0), ("v0.3.1", "0.3.1", 0),
            ("0.10.0", "0.9.0", 1), ("1.0.0", "0.99.99", 1),
            ("dev", "0.3.0", nil), ("0.3", "0.3.0", nil), ("0.3.0-rc1", "0.3.0", nil), ("e2e-new", "0.3.0", nil),
        ]
        for (a, b, want) in cases {
            XCTAssertEqual(compareVersions(a, b), want, "\(a) vs \(b)")
        }
    }

    func testAllowedURL() {
        let yes = [
            "https://github.com/quanht99/agent-chime/releases/latest/download/latest.json",
            "https://objects.githubusercontent.com/x",
            "https://release-assets.githubusercontent.com/x",
        ]
        let no = [
            "http://github.com/x", "https://githubusercontent.com.evil.com/x", "https://github.com.evil.com/x",
            "https://evilgithubusercontent.com/x", "https://user@github.com/x", "ftp://github.com/x",
            "http://127.0.0.1:1/x",
        ]
        for s in yes { XCTAssertTrue(isAllowedURL(URL(string: s)!), s) }
        for s in no { XCTAssertFalse(isAllowedURL(URL(string: s)!), s) }
    }

    #if !AGENTCHIME_UPDATE_TEST
    func testReleaseKeyIsNotTestKey() throws {
        XCTAssertFalse(UpdateKeys.testBuild)
        let test = try testKey()
        if let k = UpdateKeys.publicKey {
            XCTAssertNotEqual(k.rawRepresentation, test.rawRepresentation)
        }
        XCTAssertFalse(UpdateKeys.allowed(URL(string: "http://127.0.0.1:1/x")!))
        XCTAssertEqual(UpdateKeys.base.absoluteString, "https://github.com/quanht99/agent-chime/releases/")
    }
    #endif
}
