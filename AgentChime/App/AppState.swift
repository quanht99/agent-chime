import AppKit
import Foundation
import ServiceManagement

@MainActor
final class AppState: ObservableObject {
    @Published var config: AgentChimeConfig {
        didSet {
            guard config != oldValue else { return }
            do { try ConfigStore.save(config) } catch {
                Log.write("cannot save config.json: \(error.localizedDescription)")
            }
        }
    }
    @Published private(set) var state: AgentChimeState
    @Published private(set) var claudeStatus: InstallStatus
    @Published private(set) var codexStatus: InstallStatus
    @Published private(set) var codexTrust: CodexTrust.State
    @Published var errorText: String?
    @Published private(set) var codexNeedsRetrust = false
    /// Refreshed every tick so views re-evaluate mute expiry.
    @Published private(set) var now = Date()

    init() {
        config = ConfigStore.load()
        state = StateStore.load()
        claudeStatus = HookInstaller.status(specs: AgentHooks.claude, fileURL: Paths.claudeSettings)
        codexStatus = HookInstaller.status(specs: AgentHooks.codex, fileURL: Paths.codexHooks)
        codexTrust = CodexTrust.currentState()
    }

    var lang: Language { config.language }

    func refresh() {
        now = Date()
        state = StateStore.load()
        claudeStatus = HookInstaller.status(specs: AgentHooks.claude, fileURL: Paths.claudeSettings)
        codexStatus = HookInstaller.status(specs: AgentHooks.codex, fileURL: Paths.codexHooks)
        codexTrust = CodexTrust.currentState()
        objectWillChange.send()
    }

    func status(of agent: Agent) -> InstallStatus {
        agent == .claude ? claudeStatus : codexStatus
    }

    private func target(_ agent: Agent) -> (specs: [HookSpec], file: URL) {
        agent == .claude ? (AgentHooks.claude, Paths.claudeSettings) : (AgentHooks.codex, Paths.codexHooks)
    }

    func install(_ agent: Agent) {
        guard FileManager.default.isExecutableFile(atPath: Paths.installedHelper.path) else {
            errorText = L.t(.helperMissing, lang)
            return
        }
        let t = target(agent)
        do {
            let replaced = try HookInstaller.install(specs: t.specs, fileURL: t.file)
            errorText = nil
            if agent == .codex { codexNeedsRetrust = replaced }
        } catch let e as InstallError {
            errorText = L.t(.unreadableConfig, lang, e.path)
        } catch {
            Log.write("install \(agent.rawValue) failed: \(error.localizedDescription)")
            errorText = error.localizedDescription
        }
        refresh()
    }

    func uninstall(_ agent: Agent) {
        let t = target(agent)
        do {
            try HookInstaller.uninstall(fileURL: t.file)
            errorText = nil
            if agent == .codex { codexNeedsRetrust = false }
        } catch let e as InstallError {
            errorText = L.t(.unreadableConfig, lang, e.path)
        } catch {
            Log.write("uninstall \(agent.rawValue) failed: \(error.localizedDescription)")
            errorText = error.localizedDescription
        }
        refresh()
    }

    /// Sets the sound choice for an alert kind.
    func choose(_ c: SoundChoice, for kind: AlertKind) {
        if kind == .done { config.soundDone = c } else { config.soundAttention = c }
    }

    func mute(for seconds: TimeInterval?) {
        config.mute(for: seconds, now: Date())
        refresh()
    }

    func unmute() {
        config.unmute()
        refresh()
    }

    // Hover preview in the sound menu: plays in-process (NSSound) so it can be stopped
    // the moment the pointer moves on.
    private var hoverSound: NSSound?
    private var hoverKind: AlertKind = .done
    private lazy var hoverPreviewer = HoverPreview(
        schedule: { delay, action in
            let timer = Timer(timeInterval: delay, repeats: false) { _ in action() }
            RunLoop.main.add(timer, forMode: .common) // an open menu runs the loop in event-tracking mode
            return { timer.invalidate() }
        },
        play: { [weak self] choice in self?.playHover(choice) },
        stop: { [weak self] in self?.stopHover() }
    )

    func hoverPreview(_ choice: SoundChoice?, kind: AlertKind) {
        hoverKind = kind
        hoverPreviewer.hover(choice)
    }

    func endHoverPreview() {
        hoverPreviewer.end()
    }

    private func playHover(_ choice: SoundChoice) {
        stopHover()
        guard SoundPlayer.isValidName(choice),
              let sound = NSSound(contentsOf: SoundPlayer.url(for: choice), byReference: true) else { return }
        sound.volume = Float(min(1, max(0, config.volume(for: hoverKind))))
        sound.play()
        hoverSound = sound
    }

    private func stopHover() {
        hoverSound?.stop()
        hoverSound = nil
    }

    func preview(_ kind: AlertKind) {
        let resolved = SoundPlayer.resolvePlayable(config.sound(for: kind), kind: kind)
        _ = SoundPlayer.playDetached(url: resolved.url, volume: config.volume(for: kind))
    }

    var launchAtLogin: Bool {
        get { SMAppService.mainApp.status == .enabled }
        set {
            do {
                if newValue { try SMAppService.mainApp.register() } else { try SMAppService.mainApp.unregister() }
                errorText = nil
            } catch {
                Log.write("login item change failed: \(error.localizedDescription)")
                errorText = L.t(.loginItemError, lang, error.localizedDescription)
            }
            objectWillChange.send()
        }
    }

    var needsSetup: Bool {
        for s in [claudeStatus, codexStatus] {
            switch s {
            case .partial, .unreadable: return true
            default: break
            }
        }
        if codexStatus == .installed {
            switch codexTrust {
            case .untrusted, .disabled: return true
            default: break
            }
        }
        return claudeStatus == .notInstalled && codexStatus == .notInstalled
    }
}
