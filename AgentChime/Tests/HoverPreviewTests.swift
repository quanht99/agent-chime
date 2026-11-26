import XCTest

final class HoverPreviewTests: XCTestCase {
    private let glass = SoundChoice(source: .system, value: "Glass")
    private let ping = SoundChoice(source: .system, value: "Ping")

    /// A fake clock: scheduled actions run only when the test fires them.
    private var scheduled: [(delay: TimeInterval, action: () -> Void, cancelled: Bool)] = []
    private var played: [SoundChoice] = []
    private var stops = 0

    private func makePreview() -> HoverPreview {
        HoverPreview(
            schedule: { [unowned self] delay, action in
                let index = self.scheduled.count
                self.scheduled.append((delay, action, false))
                return { self.scheduled[index].cancelled = true }
            },
            play: { [unowned self] in self.played.append($0) },
            stop: { [unowned self] in self.stops += 1 }
        )
    }

    /// Runs every action that is still waiting; each runs at most once.
    private func fireLive() {
        for i in scheduled.indices where !scheduled[i].cancelled {
            scheduled[i].cancelled = true
            scheduled[i].action()
        }
    }

    func testDelayIsOneSecond() {
        XCTAssertEqual(HoverPreview.delay, 1.0)
    }

    func testPlaysAfterResting() {
        let p = makePreview()
        p.hover(glass)
        XCTAssertEqual(scheduled.map(\.delay), [HoverPreview.delay])
        XCTAssertTrue(played.isEmpty, "nothing plays before the delay")
        fireLive()
        XCTAssertEqual(played, [glass])
    }

    func testMovingOnCancelsThePendingSound() {
        let p = makePreview()
        p.hover(glass)
        p.hover(ping)
        fireLive()
        XCTAssertEqual(played, [ping])
    }

    func testSameItemAgainDoesNotRestartTheTimer() {
        let p = makePreview()
        p.hover(glass)
        p.hover(glass)
        XCTAssertEqual(scheduled.count, 1)
    }

    func testNonSoundItemCancelsAndStops() {
        let p = makePreview()
        p.hover(glass)
        let stopsBefore = stops
        p.hover(nil) // "Choose file…" or a header
        fireLive()
        XCTAssertTrue(played.isEmpty)
        XCTAssertGreaterThan(stops, stopsBefore)
    }

    func testMovingOnStopsTheSoundThatIsPlaying() {
        let p = makePreview()
        p.hover(glass)
        fireLive()
        let stopsBefore = stops
        p.hover(ping)
        XCTAssertGreaterThan(stops, stopsBefore)
    }

    func testEndCancelsAndStops() {
        let p = makePreview()
        p.hover(glass)
        p.end()
        fireLive()
        XCTAssertTrue(played.isEmpty)
        XCTAssertGreaterThan(stops, 0)
    }

    func testHoverAfterEndPlaysAgain() {
        let p = makePreview()
        p.hover(glass)
        fireLive()
        p.end()
        p.hover(glass)
        fireLive()
        XCTAssertEqual(played, [glass, glass])
    }
}
