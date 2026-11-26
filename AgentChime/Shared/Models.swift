import Foundation

enum Agent: String, Codable, CaseIterable {
    case claude, codex

    var displayName: String {
        switch self {
        case .claude: return "Claude Code"
        case .codex: return "Codex"
        }
    }
}

enum AlertKind: String, Codable {
    case done, attention
}

struct AgentEvent: Equatable {
    var agent: Agent
    var kind: AlertKind
    var sessionID: String?
    var detail: String? = nil
    var duration: TimeInterval? = nil
}

enum SkipReason: Codable, Equatable {
    case muted(until: Date)
    case headless
    case duplicate
    case nested
    case shortTurn(duration: TimeInterval, threshold: TimeInterval)

    func text(_ lang: Language) -> String {
        switch self {
        case .muted(let until):
            if until == Date.distantFuture { return L.t(.reasonMuted, lang) }
            return L.t(.reasonMutedUntil, lang, TimeFormat.hhmm(until))
        case .headless:
            return L.t(.reasonHeadless, lang)
        case .duplicate:
            return L.t(.reasonDuplicate, lang)
        case .nested:
            return L.t(.reasonNested, lang)
        case .shortTurn(let duration, let threshold):
            return L.t(.reasonShortTurn, lang, DurationFormat.short(duration), DurationFormat.short(threshold))
        }
    }
}

struct AlertRecord: Codable, Equatable {
    var agent: Agent
    var kind: AlertKind
    var duration: TimeInterval?
    var date: Date

    func summary(_ lang: Language) -> String {
        switch kind {
        case .attention:
            return L.t(.summaryAttention, lang, agent.displayName)
        case .done:
            if let duration = duration {
                return L.t(.summaryDoneDuration, lang, agent.displayName, DurationFormat.short(duration))
            }
            return L.t(.summaryDone, lang, agent.displayName)
        }
    }
}

struct SkipRecord: Codable, Equatable {
    var agent: Agent
    var kind: AlertKind
    var reason: SkipReason
    var date: Date

    func summary(_ lang: Language) -> String {
        L.t(kind == .done ? .summaryDone : .summaryAttention, lang, agent.displayName)
    }
}

struct SoundWarning: Codable, Equatable {
    var kind: AlertKind
    var path: String
}

enum DurationFormat {
    static func short(_ s: TimeInterval) -> String {
        let total = max(0, Int(s.rounded()))
        if total < 60 { return "\(total)s" }
        if total < 3600 { return String(format: "%dm%02ds", total / 60, total % 60) }
        return String(format: "%dh%02dm", total / 3600, (total % 3600) / 60)
    }
}

enum TimeFormat {
    static func hhmm(_ d: Date) -> String {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "HH:mm"
        return f.string(from: d)
    }
}
