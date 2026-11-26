import Foundation

signal(SIGPIPE, SIG_IGN)

let arguments = CommandLine.arguments
guard arguments.count > 1, let command = HookCommand(rawValue: arguments[1]) else {
    Log.write("unknown subcommand")
    exit(0)
}

/// Reads stdin to EOF, but gives up after `timeout` seconds (a launcher may leave the pipe open forever).
/// On timeout the payload is treated as empty, which takes the normal invalid-payload path.
func readStdin(timeout: TimeInterval) -> Data {
    let deadline = Date().addingTimeInterval(timeout)
    var data = Data()
    var buffer = [UInt8](repeating: 0, count: 65536)
    while true {
        let remaining = Int(deadline.timeIntervalSinceNow * 1000)
        if remaining <= 0 { return Data() }
        var pfd = pollfd(fd: 0, events: Int16(POLLIN), revents: 0)
        let ready = poll(&pfd, 1, Int32(remaining))
        if ready < 0 {
            if errno == EINTR { continue }
            return Data()
        }
        if ready == 0 { return Data() }
        let n = read(0, &buffer, buffer.count)
        if n > 0 {
            data.append(contentsOf: buffer[0..<n])
        } else if n == 0 {
            return data
        } else if errno != EINTR && errno != EAGAIN {
            return Data()
        }
    }
}

let payload: Data = isatty(0) != 0 ? Data() : readStdin(timeout: 0.3)
let env = ProcessInfo.processInfo.environment

if FileManager.default.fileExists(atPath: Paths.debugEnvFlag.path) {
    Log.write("env-probe " + RunContext.probeLine(command: command, env: env,
                                                  ancestors: RunContext.ancestorCommandLines(), payload: payload))
}

let agent = command.agent
let action = HookParser.action(for: command, payload: payload)
let now = Date()

if RunContext.isNested(agent, env: env) {
    if case .alert(let event) = action {
        StateStore.recordSkip(SkipRecord(agent: event.agent, kind: event.kind, reason: .nested, date: now))
    }
    Log.write("hook \(command.rawValue) skipped: nested")
    exit(0)
}

switch action {
case .turnStarted(let session):
    StateStore.recordTurnStart(agent, session: session, now: now)
case .turnContinued(let session):
    let kept = StateStore.continueTurn(agent, session: session, now: now)
    Log.write("hook \(command.rawValue): background work handed back, " + (kept ? "turn continues" : "new turn"))
case .turnHeld(let session, let running):
    StateStore.holdTurn(agent, session: session, now: now)
    Log.write("hook \(command.rawValue) held: \(running) background task(s) still running")
case .turnCancelled(let session):
    StateStore.clearTurnStart(agent, session: session)
case .ignore(let reason):
    Log.write("hook \(command.rawValue) ignored: \(reason)")
case .alert(var event):
    if event.kind == .done, let session = event.sessionID {
        if let start = StateStore.takeTurnStart(agent, session: session, now: now) {
            event.duration = now.timeIntervalSince(start)
        } else {
            Log.write("no start time")
        }
    }
    let config = ConfigStore.load()
    let ancestors = (agent == .codex && !config.alertHeadless) ? RunContext.ancestorCommandLines() : []
    let isHeadless = RunContext.isHeadless(agent, env: env, ancestors: ancestors)
    _ = Notifier.fire(event, config: config, now: now, isHeadless: isHeadless)
}

exit(0)
