import XCTest

final class ConfigTests: IsolatedTestCase {
    let t0 = Date(timeIntervalSince1970: 1_800_000_000)

    func testDefaults() {
        let c = AgentChimeConfig()
        XCTAssertEqual(c.soundDone, SoundChoice(source: .system, value: "Glass"))
        XCTAssertEqual(c.soundAttention, SoundChoice(source: .system, value: "Sosumi"))
        XCTAssertEqual(c.volumeDone, 1.0)
        XCTAssertEqual(c.volumeAttention, 1.0)
        XCTAssertEqual(c.doneThresholdSeconds, 60)
        XCTAssertNil(c.mutedUntil)
        XCTAssertFalse(c.alertHeadless)
        XCTAssertEqual(c.language, .en)
        XCTAssertFalse(c.settingsShownOnce)
    }

    func testEmptyJSONGivesDefaults() throws {
        let c = try JSONCoding.decoder().decode(AgentChimeConfig.self, from: Data("{}".utf8))
        XCTAssertEqual(c, AgentChimeConfig())
    }

    func testBadFieldKeepsOthers() throws {
        let json = #"{"language":"fr","doneThresholdSeconds":30,"volumeDone":"x","volumeAttention":7}"#
        let c = try JSONCoding.decoder().decode(AgentChimeConfig.self, from: Data(json.utf8))
        XCTAssertEqual(c.language, .en)
        XCTAssertEqual(c.doneThresholdSeconds, 30)
        XCTAssertEqual(c.volumeDone, 1.0)
        XCTAssertEqual(c.volumeAttention, 1.0)
    }

    func testCorruptConfigFileGivesDefaults() throws {
        Paths.ensureDirectory(Paths.appSupport)
        try "not json".write(to: Paths.config, atomically: true, encoding: .utf8)
        XCTAssertEqual(ConfigStore.load(), AgentChimeConfig())
    }

    func testMissingConfigFileGivesDefaults() {
        XCTAssertEqual(ConfigStore.load(), AgentChimeConfig())
    }

    func testSaveLoadRoundTrip() throws {
        var c = AgentChimeConfig()
        c.soundDone = SoundChoice(source: .file, value: "/Users/example/x.mp3")
        c.soundAttention = SoundChoice(source: .bundled, value: "Chime.wav")
        c.volumeDone = 0.4
        c.volumeAttention = 0.7
        c.doneThresholdSeconds = 120
        c.mutedUntil = t0
        c.alertHeadless = true
        c.language = .vi
        c.settingsShownOnce = true
        try ConfigStore.save(c)
        XCTAssertEqual(ConfigStore.load(), c)
    }

    func testCheckForUpdatesDefaultsOn() throws {
        for json in ["{}", #"{"language":"vi"}"#] {
            let c = try JSONCoding.decoder().decode(AgentChimeConfig.self, from: Data(json.utf8))
            XCTAssertTrue(c.checkForUpdates)
        }
    }

    func testCheckForUpdatesRoundTrip() throws {
        var c = AgentChimeConfig()
        c.checkForUpdates = false
        try ConfigStore.save(c)
        XCTAssertFalse(ConfigStore.load().checkForUpdates)
    }

    func testMuteAndUnmute() {
        var c = AgentChimeConfig()
        c.mute(for: 1800, now: t0)
        XCTAssertTrue(c.isMuted(at: t0.addingTimeInterval(1799)))
        XCTAssertFalse(c.isMuted(at: t0.addingTimeInterval(1800)))
        c.mute(for: nil, now: t0)
        XCTAssertEqual(c.mutedUntil, .distantFuture)
        c.unmute()
        XCTAssertNil(c.mutedUntil)
    }

    func testSoundChoiceDisplayName() {
        XCTAssertEqual(SoundChoice(source: .bundled, value: "Soft Chime.wav").displayName, "Soft Chime")
        XCTAssertEqual(SoundChoice(source: .file, value: "/a/b/x.mp3").displayName, "x")
        XCTAssertEqual(SoundChoice(source: .system, value: "Glass").displayName, "Glass")
    }

    func testDurationFormat() {
        XCTAssertEqual(DurationFormat.short(42), "42s")
        XCTAssertEqual(DurationFormat.short(60), "1m00s")
        XCTAssertEqual(DurationFormat.short(192), "3m12s")
        XCTAssertEqual(DurationFormat.short(3900), "1h05m")
    }

    func testUnknownSourceFallsBack() throws {
        let json = #"{"soundDone":{"source":"cloud","value":"x"},"soundAttention":{"source":"bundled","value":"B.wav"},"doneThresholdSeconds":30}"#
        let c = try JSONCoding.decoder().decode(AgentChimeConfig.self, from: Data(json.utf8))
        XCTAssertEqual(c.soundDone, SoundChoice.defaultDone)
        XCTAssertEqual(c.soundAttention, SoundChoice(source: .bundled, value: "B.wav"))
        XCTAssertEqual(c.doneThresholdSeconds, 30)
        let bad = #"{"soundAttention":{"source":"cloud","value":"x"}}"#
        let c2 = try JSONCoding.decoder().decode(AgentChimeConfig.self, from: Data(bad.utf8))
        XCTAssertEqual(c2.soundAttention, SoundChoice.defaultAttention)
    }
}
