import Foundation

enum AlertPolicy {
    static let attentionDedupeWindow: TimeInterval = 3
    static let doneDedupeWindow: TimeInterval = 60

    static func dedupeWindow(for kind: AlertKind) -> TimeInterval {
        kind == .attention ? attentionDedupeWindow : doneDedupeWindow
    }

    /// Returns why the alert should not sound, or nil when it should.
    static func skipReason(event: AgentEvent, config: AgentChimeConfig, now: Date, isHeadless: Bool) -> SkipReason? {
        if config.isMuted(at: now), let until = config.mutedUntil {
            return .muted(until: until)
        }
        if isHeadless && !config.alertHeadless {
            return .headless
        }
        if event.kind == .done,
           let duration = event.duration,
           config.doneThresholdSeconds > 0,
           duration < config.doneThresholdSeconds {
            return .shortTurn(duration: duration, threshold: config.doneThresholdSeconds)
        }
        return nil
    }
}
