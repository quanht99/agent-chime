import Foundation

struct AgentChimeState: Codable, Equatable {
    var turnStarts: [String: Date] = [:]   // key "agent|session"
    /// Turns that ended while background work was still running: key "agent|session", value when
    /// that Stop came. The next hand-back prompt keeps the turn's start instead of replacing it.
    var heldTurns: [String: Date] = [:]
    var lastFired: [String: Date] = [:]    // key "agent|session|kind"; missing session uses "-"
    var lastAlert: AlertRecord? = nil
    var lastSkip: SkipRecord? = nil
    var soundWarning: SoundWarning? = nil

    init() {}

    /// Missing keys take their defaults, so a file written by an older helper still loads.
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        turnStarts = try c.decodeIfPresent([String: Date].self, forKey: .turnStarts) ?? [:]
        heldTurns = try c.decodeIfPresent([String: Date].self, forKey: .heldTurns) ?? [:]
        lastFired = try c.decodeIfPresent([String: Date].self, forKey: .lastFired) ?? [:]
        lastAlert = try c.decodeIfPresent(AlertRecord.self, forKey: .lastAlert)
        lastSkip = try c.decodeIfPresent(SkipRecord.self, forKey: .lastSkip)
        soundWarning = try c.decodeIfPresent(SoundWarning.self, forKey: .soundWarning)
    }
}

enum StateStore {
    static let turnStartMaxAge: TimeInterval = 24 * 3600

    /// Lock-free read. A missing or unreadable file yields a default state.
    static func load() -> AgentChimeState {
        guard let data = try? Data(contentsOf: Paths.state) else { return AgentChimeState() }
        do {
            return try JSONCoding.decoder().decode(AgentChimeState.self, from: data)
        } catch {
            Log.write("state.json is unreadable, using defaults: \(error.localizedDescription)")
            return AgentChimeState()
        }
    }

    /// Runs `body` on the current state under an exclusive flock, then writes atomically.
    /// If the lock file cannot be opened, it still runs and writes (fail open).
    static func update<T>(_ body: (inout AgentChimeState) -> T) -> T {
        Paths.ensureDirectory(Paths.appSupport)
        let fd = open(Paths.stateLock.path, O_CREAT | O_RDWR, 0o600)
        if fd >= 0 {
            flock(fd, LOCK_EX)
        } else {
            Log.write("cannot open state.lock (errno \(errno)), writing state without a lock")
        }
        defer {
            if fd >= 0 {
                flock(fd, LOCK_UN)
                close(fd)
            }
        }

        var state = load()
        let result = body(&state)
        do {
            let data = try JSONCoding.encoder().encode(state)
            try data.write(to: Paths.state, options: .atomic)
        } catch {
            Log.write("cannot write state.json: \(error.localizedDescription)")
        }
        return result
    }

    private static func turnKey(_ agent: Agent, _ session: String) -> String {
        "\(agent.rawValue)|\(session)"
    }

    private static func firedKey(_ event: AgentEvent) -> String {
        "\(event.agent.rawValue)|\(event.sessionID ?? "-")|\(event.kind.rawValue)"
    }

    private static func prune(_ state: inout AgentChimeState, now: Date) {
        let cutoff = now.addingTimeInterval(-turnStartMaxAge)
        state.turnStarts = state.turnStarts.filter { $0.value >= cutoff }
        state.heldTurns = state.heldTurns.filter { $0.value >= cutoff }
        state.lastFired = state.lastFired.filter { $0.value >= cutoff }
    }

    static func recordTurnStart(_ agent: Agent, session: String, now: Date) {
        update { state in
            prune(&state, now: now)
            let key = turnKey(agent, session)
            state.turnStarts[key] = now
            state.heldTurns.removeValue(forKey: key)
        }
    }

    /// The turn ended while background work runs: keep its start for the hand-back that follows.
    static func holdTurn(_ agent: Agent, session: String, now: Date) {
        update { state in
            prune(&state, now: now)
            state.heldTurns[turnKey(agent, session)] = now
        }
    }

    /// A hand-back prompt. Right after a hold it keeps the turn's start (and uses up the hold);
    /// otherwise it starts the turn now, like a typed prompt. Returns true when the start was kept.
    static func continueTurn(_ agent: Agent, session: String, now: Date) -> Bool {
        update { state in
            prune(&state, now: now)
            let key = turnKey(agent, session)
            let held = state.heldTurns.removeValue(forKey: key) != nil
            if held, state.turnStarts[key] != nil { return true }
            state.turnStarts[key] = now
            return false
        }
    }

    static func takeTurnStart(_ agent: Agent, session: String, now: Date) -> Date? {
        update { state in
            let key = turnKey(agent, session)
            state.heldTurns.removeValue(forKey: key)
            guard let start = state.turnStarts.removeValue(forKey: key) else { return nil }
            return now.timeIntervalSince(start) > turnStartMaxAge ? nil : start
        }
    }

    static func clearTurnStart(_ agent: Agent, session: String) {
        update { state in
            let key = turnKey(agent, session)
            state.turnStarts.removeValue(forKey: key)
            state.heldTurns.removeValue(forKey: key)
        }
    }

    static func shouldFire(_ event: AgentEvent, window: TimeInterval, now: Date) -> Bool {
        update { state in
            let key = firedKey(event)
            if let last = state.lastFired[key], now.timeIntervalSince(last) < window { return false }
            state.lastFired[key] = now
            return true
        }
    }

    static func recordAlert(_ record: AlertRecord) {
        update { $0.lastAlert = record }
    }

    static func recordSkip(_ record: SkipRecord) {
        update { $0.lastSkip = record }
    }

    static func setSoundWarning(_ warning: SoundWarning?) {
        update { $0.soundWarning = warning }
    }
}
