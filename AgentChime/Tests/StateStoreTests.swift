import XCTest

final class StateStoreTests: IsolatedTestCase {
    let t0 = Date(timeIntervalSince1970: 1_800_000_000)

    func testTurnStartRoundTrip() {
        StateStore.recordTurnStart(.claude, session: "s1", now: t0)
        XCTAssertEqual(StateStore.takeTurnStart(.claude, session: "s1", now: t0.addingTimeInterval(90)), t0)
        XCTAssertNil(StateStore.takeTurnStart(.claude, session: "s1", now: t0.addingTimeInterval(91)))
    }

    func testKeyedByAgentAndSession() {
        StateStore.recordTurnStart(.claude, session: "s1", now: t0)
        XCTAssertNil(StateStore.takeTurnStart(.codex, session: "s1", now: t0))
        XCTAssertNil(StateStore.takeTurnStart(.claude, session: "s2", now: t0))
        XCTAssertEqual(StateStore.takeTurnStart(.claude, session: "s1", now: t0), t0)
    }

    func testStaleTurnStartDropped() {
        StateStore.recordTurnStart(.claude, session: "s1", now: t0)
        XCTAssertNil(StateStore.takeTurnStart(.claude, session: "s1", now: t0.addingTimeInterval(24 * 3600 + 1)))
        XCTAssertTrue(StateStore.load().turnStarts.isEmpty)
    }

    func testClearTurnStart() {
        StateStore.recordTurnStart(.claude, session: "s1", now: t0)
        StateStore.clearTurnStart(.claude, session: "s1")
        XCTAssertNil(StateStore.takeTurnStart(.claude, session: "s1", now: t0))
    }

    func testRecordPrunesOldEntries() {
        StateStore.recordTurnStart(.claude, session: "old", now: t0)
        _ = StateStore.shouldFire(AgentEvent(agent: .claude, kind: .done, sessionID: "old"), window: 60, now: t0)
        StateStore.recordTurnStart(.claude, session: "new", now: t0.addingTimeInterval(24 * 3600 + 1))
        let s = StateStore.load()
        XCTAssertEqual(Array(s.turnStarts.keys), ["claude|new"])
        XCTAssertTrue(s.lastFired.isEmpty)
    }

    func testAttentionDedupeWindow() {
        let e = AgentEvent(agent: .claude, kind: .attention, sessionID: "s1")
        XCTAssertTrue(StateStore.shouldFire(e, window: 3, now: t0))
        XCTAssertFalse(StateStore.shouldFire(e, window: 3, now: t0.addingTimeInterval(2)))
        XCTAssertTrue(StateStore.shouldFire(e, window: 3, now: t0.addingTimeInterval(3.5)))
        var other = e; other.sessionID = "s2"
        XCTAssertTrue(StateStore.shouldFire(other, window: 3, now: t0.addingTimeInterval(2)))
    }

    func testDoneDedupeWindow() {
        let e = AgentEvent(agent: .codex, kind: .done, sessionID: nil)
        XCTAssertTrue(StateStore.shouldFire(e, window: 60, now: t0))
        XCTAssertFalse(StateStore.shouldFire(e, window: 60, now: t0.addingTimeInterval(59)))
        XCTAssertTrue(StateStore.shouldFire(e, window: 60, now: t0.addingTimeInterval(61)))
        XCTAssertNotNil(StateStore.load().lastFired["codex|-|done"])
    }

    func testConcurrentWritersLoseNothing() {
        let t = t0
        DispatchQueue.concurrentPerform(iterations: 64) { i in
            StateStore.recordTurnStart(.codex, session: "s\(i)", now: t)
        }
        XCTAssertEqual(StateStore.load().turnStarts.count, 64)
    }

    func testCorruptStateFileIsReplaced() throws {
        Paths.ensureDirectory(Paths.appSupport)
        try "not json".write(to: Paths.state, atomically: true, encoding: .utf8)
        XCTAssertEqual(StateStore.load(), AgentChimeState())
        StateStore.recordTurnStart(.claude, session: "s1", now: t0)
        XCTAssertEqual(StateStore.load().turnStarts.count, 1)
        let data = try Data(contentsOf: Paths.state)
        XCTAssertNoThrow(try JSONCoding.decoder().decode(AgentChimeState.self, from: data))
    }

    func testMissingStateFileLoadsDefaults() {
        XCTAssertEqual(StateStore.load(), AgentChimeState())
    }

    func testRecordsAlertSkipAndWarning() {
        let alert = AlertRecord(agent: .claude, kind: .done, duration: 75, date: t0)
        let skip = SkipRecord(agent: .codex, kind: .attention, reason: .headless, date: t0)
        let w = SoundWarning(kind: .done, path: "/Users/example/x.wav")
        StateStore.recordAlert(alert)
        StateStore.recordSkip(skip)
        StateStore.setSoundWarning(w)
        var s = StateStore.load()
        XCTAssertEqual(s.lastAlert, alert)
        XCTAssertEqual(s.lastSkip, skip)
        XCTAssertEqual(s.soundWarning, w)
        StateStore.setSoundWarning(nil)
        s = StateStore.load()
        XCTAssertNil(s.soundWarning)
        XCTAssertEqual(s.lastAlert, alert)
    }

    // MARK: held turns (background work still running when the turn ended)

    func testHeldTurnKeepsTheStartAcrossHandBacks() {
        StateStore.recordTurnStart(.claude, session: "s1", now: t0)
        StateStore.holdTurn(.claude, session: "s1", now: t0.addingTimeInterval(5))
        XCTAssertTrue(StateStore.continueTurn(.claude, session: "s1", now: t0.addingTimeInterval(28)))
        StateStore.holdTurn(.claude, session: "s1", now: t0.addingTimeInterval(30))
        XCTAssertTrue(StateStore.continueTurn(.claude, session: "s1", now: t0.addingTimeInterval(37)))
        XCTAssertEqual(StateStore.takeTurnStart(.claude, session: "s1", now: t0.addingTimeInterval(78)), t0)
        XCTAssertTrue(StateStore.load().heldTurns.isEmpty)
    }

    func testHandBackWithoutHoldStartsFresh() {
        // e.g. the user pressed Esc (no Stop, so no hold) and a background task reported later
        StateStore.recordTurnStart(.claude, session: "s1", now: t0)
        let later = t0.addingTimeInterval(28)
        XCTAssertFalse(StateStore.continueTurn(.claude, session: "s1", now: later))
        XCTAssertEqual(StateStore.takeTurnStart(.claude, session: "s1", now: later), later)
    }

    func testHoldIsUsedOnce() {
        StateStore.recordTurnStart(.claude, session: "s1", now: t0)
        StateStore.holdTurn(.claude, session: "s1", now: t0.addingTimeInterval(5))
        XCTAssertTrue(StateStore.continueTurn(.claude, session: "s1", now: t0.addingTimeInterval(10)))
        let t3 = t0.addingTimeInterval(20)
        XCTAssertFalse(StateStore.continueTurn(.claude, session: "s1", now: t3))
        XCTAssertEqual(StateStore.takeTurnStart(.claude, session: "s1", now: t3), t3)
    }

    func testTypedPromptReplacesAHeldStart() {
        StateStore.recordTurnStart(.claude, session: "s1", now: t0)
        StateStore.holdTurn(.claude, session: "s1", now: t0.addingTimeInterval(5))
        let t1 = t0.addingTimeInterval(600)
        StateStore.recordTurnStart(.claude, session: "s1", now: t1)
        XCTAssertTrue(StateStore.load().heldTurns.isEmpty)
        XCTAssertFalse(StateStore.continueTurn(.claude, session: "s1", now: t1.addingTimeInterval(9)))
        XCTAssertEqual(StateStore.takeTurnStart(.claude, session: "s1", now: t1.addingTimeInterval(9)),
                       t1.addingTimeInterval(9))
    }

    func testHoldWithoutStartContinuesFresh() {
        StateStore.holdTurn(.claude, session: "s1", now: t0)
        let later = t0.addingTimeInterval(28)
        XCTAssertFalse(StateStore.continueTurn(.claude, session: "s1", now: later))
        XCTAssertEqual(StateStore.takeTurnStart(.claude, session: "s1", now: later), later)
    }

    func testHoldsAreKeyedByAgentAndSession() {
        StateStore.recordTurnStart(.claude, session: "s1", now: t0)
        StateStore.recordTurnStart(.claude, session: "s2", now: t0)
        StateStore.holdTurn(.claude, session: "s1", now: t0)
        XCTAssertFalse(StateStore.continueTurn(.claude, session: "s2", now: t0.addingTimeInterval(9)))
        XCTAssertFalse(StateStore.continueTurn(.codex, session: "s1", now: t0.addingTimeInterval(9)))
        XCTAssertTrue(StateStore.continueTurn(.claude, session: "s1", now: t0.addingTimeInterval(9)))
    }

    func testClearTurnStartDropsTheHold() {
        StateStore.recordTurnStart(.claude, session: "s1", now: t0)
        StateStore.holdTurn(.claude, session: "s1", now: t0)
        StateStore.clearTurnStart(.claude, session: "s1")
        XCTAssertTrue(StateStore.load().heldTurns.isEmpty)
    }

    func testStaleHoldsArePruned() {
        StateStore.recordTurnStart(.claude, session: "old", now: t0)
        StateStore.holdTurn(.claude, session: "old", now: t0)
        StateStore.recordTurnStart(.claude, session: "new", now: t0.addingTimeInterval(24 * 3600 + 1))
        XCTAssertTrue(StateStore.load().heldTurns.isEmpty)
    }

    func testHeldStartOlderThanADayIsNotKept() {
        StateStore.recordTurnStart(.claude, session: "s1", now: t0)
        StateStore.holdTurn(.claude, session: "s1", now: t0)
        let later = t0.addingTimeInterval(24 * 3600 + 1)
        XCTAssertFalse(StateStore.continueTurn(.claude, session: "s1", now: later))
        XCTAssertEqual(StateStore.takeTurnStart(.claude, session: "s1", now: later), later)
    }

    func testStateFileFromBeforeHeldTurnsStillLoads() throws {
        Paths.ensureDirectory(Paths.appSupport)
        StateStore.recordTurnStart(.claude, session: "s1", now: t0)
        // Re-write the file without the heldTurns key, as an older helper wrote it.
        var object = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(contentsOf: Paths.state)) as? [String: Any])
        object.removeValue(forKey: "heldTurns")
        try JSONSerialization.data(withJSONObject: object).write(to: Paths.state)
        XCTAssertEqual(StateStore.load().turnStarts.count, 1)
    }
}
