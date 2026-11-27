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

    func testNamesWithPathPartsFallBackLikeGo() throws {
        // A real WAV in the sounds folder, so the traversal below would be playable if it were let through.
        try makeWAV(at: Paths.soundsDir.appendingPathComponent("P.wav"))
        let bad: [SoundChoice] = [
            SoundChoice(source: .bundled, value: "../Sounds/P.wav"),
            SoundChoice(source: .bundled, value: "sub/P.wav"),
            SoundChoice(source: .bundled, value: "..\\P.wav"),
            SoundChoice(source: .system, value: "../../Library/Sounds/Glass"),
            SoundChoice(source: .bundled, value: ""),
            SoundChoice(source: .bundled, value: ".."),
            SoundChoice(source: .bundled, value: "."),
        ]
        for c in bad {
            XCTAssertFalse(SoundPlayer.isValidName(c), c.value)
            let r = SoundPlayer.resolvePlayable(c, kind: .attention)
            XCTAssertEqual(r.url.path, sosumi, c.value)
            XCTAssertEqual(r.warning, SoundWarning(kind: .attention, path: c.value), c.value)
        }
    }

    func testPlainNamesAndFilesAreValid() throws {
        XCTAssertTrue(SoundPlayer.isValidName(SoundChoice(source: .system, value: "Glass")))
        XCTAssertTrue(SoundPlayer.isValidName(SoundChoice(source: .bundled, value: "Two..Dots.wav")))
        XCTAssertTrue(SoundPlayer.isValidName(SoundChoice(source: .file, value: "/Users/example/My Sounds/ding.mp3")))
        try makeWAV(at: Paths.soundsDir.appendingPathComponent("Two..Dots.wav"))
        let r = SoundPlayer.resolvePlayable(SoundChoice(source: .bundled, value: "Two..Dots.wav"), kind: .done)
        XCTAssertNil(r.warning)
    }
}
