import XCTest

final class SoundPlayerTests: IsolatedTestCase {
    private let glass = "/System/Library/Sounds/Glass.aiff"
    private let sosumi = "/System/Library/Sounds/Sosumi.aiff"

    func testAfplayArgumentsClampVolume() {
        let u = URL(fileURLWithPath: "/tmp/p.aiff")
        XCTAssertEqual(SoundPlayer.afplayArguments(url: u, volume: 1.7), ["-v", "1.00", "/tmp/p.aiff"])
        XCTAssertEqual(SoundPlayer.afplayArguments(url: u, volume: -1), ["-v", "0.00", "/tmp/p.aiff"])
        XCTAssertEqual(SoundPlayer.afplayArguments(url: u, volume: 0.5), ["-v", "0.50", "/tmp/p.aiff"])
    }

    func testSystemChoiceResolves() {
        let r = SoundPlayer.resolvePlayable(SoundChoice(source: .system, value: "Glass"), kind: .done)
        XCTAssertEqual(r.url.path, glass)
        XCTAssertNil(r.warning)
    }

    func testBundledChoiceResolves() throws {
        Paths.ensureDirectory(Paths.soundsDir)
        let dest = Paths.soundsDir.appendingPathComponent("Test.aiff")
        try FileManager.default.copyItem(at: URL(fileURLWithPath: glass), to: dest)
        let r = SoundPlayer.resolvePlayable(SoundChoice(source: .bundled, value: "Test.aiff"), kind: .done)
        XCTAssertEqual(r.url, dest)
        XCTAssertNil(r.warning)
    }

    func testMissingFileFallsBack() {
        let r = SoundPlayer.resolvePlayable(SoundChoice(source: .file, value: "/nonexistent.mp3"), kind: .done)
        XCTAssertEqual(r.url.path, glass)
        XCTAssertEqual(r.warning, SoundWarning(kind: .done, path: "/nonexistent.mp3"))
    }

    func testCorruptAudioFallsBack() throws {
        let bad = home.appendingPathComponent("x.mp3")
        try "not audio".write(to: bad, atomically: true, encoding: .utf8)
        let r = SoundPlayer.resolvePlayable(SoundChoice(source: .file, value: bad.path), kind: .attention)
        XCTAssertEqual(r.url.path, sosumi)
        XCTAssertEqual(r.warning, SoundWarning(kind: .attention, path: bad.path))
    }

    func testRemovedBundledSoundFallsBack() {
        let r = SoundPlayer.resolvePlayable(SoundChoice(source: .bundled, value: "Gone.wav"), kind: .done)
        XCTAssertEqual(r.url.path, glass)
        XCTAssertEqual(r.warning, SoundWarning(kind: .done, path: Paths.soundsDir.appendingPathComponent("Gone.wav").path))
    }
}
