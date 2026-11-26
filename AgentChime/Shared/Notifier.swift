import Foundation

enum FireOutcome: Equatable {
    case played(URL)
    case skipped(SkipReason)
}

enum Notifier {
    static func fire(_ event: AgentEvent, config: AgentChimeConfig, now: Date, isHeadless: Bool,
                     player: (URL, Double) -> Bool = SoundPlayer.playDetached) -> FireOutcome {
        if let reason = AlertPolicy.skipReason(event: event, config: config, now: now, isHeadless: isHeadless) {
            return skip(event, reason: reason, now: now)
        }
        if !StateStore.shouldFire(event, window: AlertPolicy.dedupeWindow(for: event.kind), now: now) {
            return skip(event, reason: .duplicate, now: now)
        }

        let resolved = SoundPlayer.resolvePlayable(config.sound(for: event.kind), kind: event.kind)
        StateStore.setSoundWarning(resolved.warning)

        let record = AlertRecord(agent: event.agent, kind: event.kind, duration: event.duration, date: now)
        if !player(resolved.url, config.volume(for: event.kind)) {
            Log.write("afplay spawn failed")
        }
        StateStore.recordAlert(record)
        Log.write("alert played: \(record.summary(.en)) sound=\(resolved.url.path)")
        return .played(resolved.url)
    }

    private static func skip(_ event: AgentEvent, reason: SkipReason, now: Date) -> FireOutcome {
        let record = SkipRecord(agent: event.agent, kind: event.kind, reason: reason, date: now)
        StateStore.recordSkip(record)
        Log.write("alert skipped: \(record.summary(.en)) reason=\(reason.text(.en))")
        return .skipped(reason)
    }
}
