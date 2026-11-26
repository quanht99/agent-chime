import XCTest

final class TextTests: IsolatedTestCase {
    let t0 = Date(timeIntervalSince1970: 1_800_000_000)

    func testSummaries() {
        let a = AlertRecord(agent: .claude, kind: .done, duration: 192, date: t0)
        XCTAssertEqual(a.summary(.en), "Claude Code done (3m12s)")
        XCTAssertEqual(a.summary(.vi), "Claude Code xong (3m12s)")
        XCTAssertEqual(AlertRecord(agent: .codex, kind: .attention, duration: nil, date: t0).summary(.en), "Codex needs you")
        XCTAssertEqual(SkipRecord(agent: .codex, kind: .done, reason: .duplicate, date: t0).summary(.vi), "Codex xong")
    }

    func testSkipReasonText() {
        XCTAssertEqual(SkipReason.shortTurn(duration: 42, threshold: 60).text(.en), "short turn (42s < 1m00s)")
        XCTAssertEqual(SkipReason.muted(until: .distantFuture).text(.vi), "đang tắt tiếng")
    }

    func testEveryKeyHasBothLanguages() {
        for key in L.Key.allCases {
            XCTAssertFalse(L.t(key, .en).isEmpty, "\(key) en")
            XCTAssertFalse(L.t(key, .vi).isEmpty, "\(key) vi")
        }
    }

    func testPlaceholderCountsMatch() {
        for key in L.Key.allCases {
            XCTAssertEqual(L.t(key, .en).components(separatedBy: "%@").count,
                           L.t(key, .vi).components(separatedBy: "%@").count, "\(key)")
        }
    }
}
