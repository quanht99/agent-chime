import SwiftUI

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    let appState = AppState()
    let afterUpdate = CommandLine.arguments.contains("--after-update")
    lazy var updater = Updater(afterUpdate: afterUpdate)
    private var timer: Timer?

    func applicationDidFinishLaunching(_ notification: Notification) {
        let version = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "?"
        Log.write("started \(version)")
        // 1. Copy the helper and built-in sounds into Application Support.
        let bundle = Bundle.main
        let helper = bundle.bundleURL.appendingPathComponent("Contents/MacOS/agentchime-hook")
        let sounds = bundle.bundleURL.appendingPathComponent("Contents/Resources/Sounds")
        do {
            _ = try Deployer.sync(helper: helper, sounds: sounds)
        } catch {
            Log.write("deploy failed: \(error.localizedDescription)")
        }

        // 2-3. State exists already; read everything again now that the files are in place.
        appState.refresh()
        timer = Timer.scheduledTimer(withTimeInterval: 5, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.appState.refresh() }
        }

        // 4. First launch: show Settings once.
        let firstLaunch = !appState.config.settingsShownOnce
        if firstLaunch || afterUpdate {
            SettingsWindow.show(appState, updater: updater)
            appState.config.settingsShownOnce = true
        }

        updater.start { [weak self] in self?.appState.config ?? ConfigStore.load() }

        // The old app waits for this line. Afterwards stdout goes to /dev/null (SIGPIPE is already
        // ignored in main.swift).
        if afterUpdate {
            print("ready")
            fflush(stdout)
            let null = open("/dev/null", O_WRONLY)
            if null >= 0 { dup2(null, 1); close(null) }
        }
    }
}

struct AgentChimeApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate

    var body: some Scene {
        MenuBarExtra {
            MenuContent(state: delegate.appState, updater: delegate.updater)
        } label: {
            MenuBarIcon(state: delegate.appState)
        }
        .menuBarExtraStyle(.menu)
    }
}

private struct MenuBarIcon: View {
    @ObservedObject var state: AppState

    var body: some View {
        Image(systemName: state.config.isMuted(at: state.now) ? "bell.slash" : "bell")
    }
}
