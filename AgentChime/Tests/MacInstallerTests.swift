import CryptoKit
import XCTest

final class MacInstallerTests: IsolatedTestCase {
    private func block(_ path: String, writable: Bool) -> BlockReason? {
        MacInstaller.installBlockReason(
            bundleURL: URL(fileURLWithPath: path),
            downloads: URL(fileURLWithPath: "/Users/example/Downloads"),
            isWritable: { _ in writable })
    }

    func testInstallBlockReason() {
        XCTAssertEqual(block("/Users/example/Downloads/AgentChime.app", writable: true), .moveToApplications)
        XCTAssertEqual(block("/private/var/folders/x/AppTranslocation/y/d/AgentChime.app", writable: true), .moveToApplications)
        XCTAssertEqual(block("/Applications/AgentChime.app", writable: false), .cantInstallHere)
        XCTAssertNil(block("/Applications/AgentChime.app", writable: true))
    }

    private func makeDir(_ names: [String]) throws -> URL {
        let d = home.appendingPathComponent("unpack-\(UUID().uuidString)")
        for n in names {
            try FileManager.default.createDirectory(at: d.appendingPathComponent(n), withIntermediateDirectories: true)
        }
        return d
    }

    func testFindSingleApp() throws {
        let one = try makeDir(["AgentChime.app"])
        XCTAssertEqual(try MacInstaller.findSingleApp(in: one).lastPathComponent, "AgentChime.app")
        XCTAssertThrowsError(try MacInstaller.findSingleApp(in: try makeDir(["AgentChime.app", "Extra.app"])))
        XCTAssertThrowsError(try MacInstaller.findSingleApp(in: try makeDir(["Other.app"])))
        XCTAssertThrowsError(try MacInstaller.findSingleApp(in: try makeDir([])))
    }

    // MARK: - ready line

    func testReadyLineParsing() {
        func ready(_ s: String) -> Bool { MacInstaller.containsReadyLine(Data(s.utf8)) }
        XCTAssertTrue(ready("ready\n"))
        XCTAssertTrue(ready("ready\r\n"))
        XCTAssertTrue(ready("hello\nready\nlater"))
        XCTAssertTrue(ready("hello\r\nready\r\n"))
        XCTAssertFalse(ready("ready"), "an unterminated line may be half written")
        XCTAssertFalse(ready("rea"))
        XCTAssertFalse(ready("readyish\n"))
        XCTAssertFalse(ready("xready\n"))
        XCTAssertFalse(ready(" ready\n"))
        XCTAssertFalse(ready(""))
    }

    // MARK: - streaming download (URLProtocol stub, no network)

    private final class StubProtocol: URLProtocol {
        static var body = Data()
        static var declaredLength: Int?
        override class func canInit(with request: URLRequest) -> Bool { true }
        override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
        override func startLoading() {
            var headers = ["Content-Type": "application/zip"]
            if let n = Self.declaredLength { headers["Content-Length"] = String(n) }
            let resp = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: "HTTP/1.1", headerFields: headers)!
            client?.urlProtocol(self, didReceive: resp, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: Self.body)
            client?.urlProtocolDidFinishLoading(self)
        }
        override func stopLoading() {}
    }

    private func installer() -> MacInstaller {
        let c = URLSessionConfiguration.ephemeral
        c.protocolClasses = [StubProtocol.self]
        return MacInstaller(session: URLSession(configuration: c), currentVersion: "0.0.1") { throw UpdateError.unverified }
    }

    private func manifest(for body: Data, size: Int64? = nil) throws -> (UpdateManifest, UpdateFile) {
        let sum = SHA256.hash(data: body).map { String(format: "%02x", $0) }.joined()
        let f = UpdateFile(os: "macos", arch: "universal", name: "AgentChime-9.9.9-macos.zip", size: size ?? Int64(body.count), sha256: sum)
        return (UpdateManifest(format: 1, version: "9.9.9", releasePage: UpdateLimits.releaseBase + "tag/v9.9.9", files: [f]), f)
    }

    func testDownloadKeepsAMatchingArchive() async throws {
        let body = Data((0..<200_000).map { UInt8($0 % 251) })
        StubProtocol.body = body
        StubProtocol.declaredLength = body.count
        let (m, f) = try manifest(for: body)
        let dest = home.appendingPathComponent("ok.zip")
        try await installer().download(m, f, to: dest)
        XCTAssertEqual(try Data(contentsOf: dest), body)
    }

    func testDownloadStopsWhenMoreBytesArriveThanTheManifestSays() async throws {
        let body = Data(repeating: 7, count: 100_000)
        StubProtocol.body = body
        StubProtocol.declaredLength = nil  // no honest Content-Length: only the byte count can stop it
        let (m, f) = try manifest(for: body, size: 1_000)
        let dest = home.appendingPathComponent("big.zip")
        do {
            try await installer().download(m, f, to: dest)
            XCTFail("expected .unverified")
        } catch let e as UpdateError {
            XCTAssertEqual(e, .unverified)
        }
        XCTAssertFalse(FileManager.default.fileExists(atPath: dest.path), "the partial file must be removed")
    }

    func testDownloadRejectsAWrongHashAndRemovesTheFile() async throws {
        let body = Data(repeating: 1, count: 5_000)
        StubProtocol.body = Data(repeating: 2, count: 5_000)
        StubProtocol.declaredLength = 5_000
        let (m, f) = try manifest(for: body)
        let dest = home.appendingPathComponent("bad.zip")
        do {
            try await installer().download(m, f, to: dest)
            XCTFail("expected .unverified")
        } catch let e as UpdateError {
            XCTAssertEqual(e, .unverified)
        }
        XCTAssertFalse(FileManager.default.fileExists(atPath: dest.path))
    }
}
