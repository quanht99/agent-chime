import XCTest

final class NotifierTests: IsolatedTestCase {
    let t0 = Date(timeIntervalSince1970: 1_800_000_000)
    private var calls: [(URL, Double)] = []

    private func player() -> (URL, Double) -> Bool {
        return { [unowned self] url, vol in
            self.calls.append((url, vol))
            return true
        }
    }

    private func event(_ kind: AlertKind, session: String? = "s1", duration: TimeInterval? = nil) -> AgentEvent {
        AgentEvent(agent: .claude, kind: kind, sessionID: session, duration: duration)
    }

    func testFireTwiceWithin3sPlaysOnce() {
        let cfg = AgentChimeConfig()
        let first = Notifier.fire(event(.attention), config: cfg, now: t0, isHeadless: false, player: player())
        let second = Notifier.fire(event(.attention), config: cfg, now: t0.addingTimeInterval(1), isHeadless: false, player: player())
        guard case .played = first else { return XCTFail("first should play") }
        XCTAssertEqual(second, .skipped(.duplicate))
        XCTAssertEqual(calls.count, 1)
    }

    func testDoneAndAttentionDoNotDedupeEachOther() {
        let cfg = AgentChimeConfig()
        _ = Notifier.fire(event(.done, duration: 100), config: cfg, now: t0, isHeadless: false, player: player())
        _ = Notifier.fire(event(.attention), config: cfg, now: t0.addingTimeInterval(1), isHeadless: false, player: player())
        XCTAssertEqual(calls.count, 2)
    }

    func testSkipIsRecordedAndNothingPlays() {
        var cfg = AgentChimeConfig()
        cfg.mutedUntil = Date.distantFuture
        let out = Notifier.fire(event(.attention), config: cfg, now: t0, isHeadless: false, player: player())
        XCTAssertEqual(out, .skipped(.muted(until: Date.distantFuture)))
        XCTAssertEqual(StateStore.load().lastSkip?.reason, .muted(until: Date.distantFuture))
        XCTAssertTrue(calls.isEmpty)
    }

    func testPlayRecordsLastAlert() {
        let out = Notifier.fire(event(.done, duration: 90), config: AgentChimeConfig(), now: t0, isHeadless: false, player: player())
        guard case .played = out else { return XCTFail("should play") }
        XCTAssertEqual(StateStore.load().lastAlert, AlertRecord(agent: .claude, kind: .done, duration: 90, date: t0))
    }

    func testWarningClearsAfterGoodSound() {
        var bad = AgentChimeConfig()
        bad.soundAttention = SoundChoice(source: .file, value: "/nonexistent.mp3")
        _ = Notifier.fire(event(.attention, session: "a"), config: bad, now: t0, isHeadless: false, player: player())
        XCTAssertEqual(StateStore.load().soundWarning, SoundWarning(kind: .attention, path: "/nonexistent.mp3"))
        _ = Notifier.fire(event(.attention, session: "b"), config: AgentChimeConfig(), now: t0.addingTimeInterval(10), isHeadless: false, player: player())
        XCTAssertNil(StateStore.load().soundWarning)
    }

    func testUnknownSourceInStoredConfigFallsBack() throws {
        let json = #"{"soundDone":{"source":"cloud","value":"Aurora.wav"},"volumeDone":0.4}"#
        Paths.ensureDirectory(Paths.config.deletingLastPathComponent())
        try Data(json.utf8).write(to: Paths.config)
        let cfg = ConfigStore.load()
        XCTAssertEqual(cfg.soundDone, SoundChoice.defaultDone)
        XCTAssertEqual(cfg.volumeDone, 0.4)
        let out = Notifier.fire(event(.done, duration: 100), config: cfg, now: t0, isHeadless: false, player: player())
        XCTAssertEqual(out, .played(SoundPlayer.url(for: .defaultDone)))
        XCTAssertEqual(calls.last?.1, 0.4)
    }

    func testFileChoicePlaysThatFile() throws {
        let wav = home.appendingPathComponent("mine.wav")
        try makeWAV(at: wav)
        var cfg = AgentChimeConfig()
        cfg.soundDone = SoundChoice(source: .file, value: wav.path)
        let out = Notifier.fire(event(.done, duration: 100), config: cfg, now: t0, isHeadless: false, player: player())
        XCTAssertEqual(out, .played(wav))
        XCTAssertNil(StateStore.load().soundWarning)
    }
}
