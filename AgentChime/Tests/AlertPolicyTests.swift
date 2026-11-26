import XCTest

final class AlertPolicyTests: XCTestCase {
    let t0 = Date(timeIntervalSince1970: 1_800_000_000)

    private func event(_ kind: AlertKind, duration: TimeInterval?) -> AgentEvent {
        AgentEvent(agent: .claude, kind: kind, sessionID: "s1", duration: duration)
    }

    func testDoneShorterThanThresholdIsSkipped() {
        let r = AlertPolicy.skipReason(event: event(.done, duration: 59), config: AgentChimeConfig(), now: t0, isHeadless: false)
        XCTAssertEqual(r, .shortTurn(duration: 59, threshold: 60))
    }

    func testDoneAtExactlyThresholdPlays() {
        XCTAssertNil(AlertPolicy.skipReason(event: event(.done, duration: 60), config: AgentChimeConfig(), now: t0, isHeadless: false))
    }

    func testThresholdZeroAlwaysPlays() {
        var cfg = AgentChimeConfig()
        cfg.doneThresholdSeconds = 0
        XCTAssertNil(AlertPolicy.skipReason(event: event(.done, duration: 1), config: cfg, now: t0, isHeadless: false))
    }

    func testDoneWithoutStartTimePlays() {
        XCTAssertNil(AlertPolicy.skipReason(event: event(.done, duration: nil), config: AgentChimeConfig(), now: t0, isHeadless: false))
    }

    func testAttentionIgnoresThreshold() {
        var cfg = AgentChimeConfig()
        cfg.doneThresholdSeconds = 600
        XCTAssertNil(AlertPolicy.skipReason(event: event(.attention, duration: 1), config: cfg, now: t0, isHeadless: false))
    }

    func testMuteFuturePastAndForever() {
        var cfg = AgentChimeConfig()
        let e = event(.attention, duration: nil)
        cfg.mutedUntil = t0.addingTimeInterval(60)
        XCTAssertEqual(AlertPolicy.skipReason(event: e, config: cfg, now: t0, isHeadless: false), .muted(until: t0.addingTimeInterval(60)))
        cfg.mutedUntil = t0.addingTimeInterval(-1)
        XCTAssertNil(AlertPolicy.skipReason(event: e, config: cfg, now: t0, isHeadless: false))
        cfg.mutedUntil = Date.distantFuture
        XCTAssertEqual(AlertPolicy.skipReason(event: e, config: cfg, now: t0, isHeadless: false), .muted(until: Date.distantFuture))
    }

    func testHeadlessSkippedUnlessEnabled() {
        var cfg = AgentChimeConfig()
        let e = event(.attention, duration: nil)
        XCTAssertEqual(AlertPolicy.skipReason(event: e, config: cfg, now: t0, isHeadless: true), .headless)
        cfg.alertHeadless = true
        XCTAssertNil(AlertPolicy.skipReason(event: e, config: cfg, now: t0, isHeadless: true))
    }

    func testMutedWinsOverShortTurn() {
        var cfg = AgentChimeConfig()
        cfg.mutedUntil = t0.addingTimeInterval(60)
        let r = AlertPolicy.skipReason(event: event(.done, duration: 1), config: cfg, now: t0, isHeadless: true)
        XCTAssertEqual(r, .muted(until: t0.addingTimeInterval(60)))
    }

    func testDedupeWindows() {
        XCTAssertEqual(AlertPolicy.dedupeWindow(for: .attention), 3)
        XCTAssertEqual(AlertPolicy.dedupeWindow(for: .done), 60)
    }
}
