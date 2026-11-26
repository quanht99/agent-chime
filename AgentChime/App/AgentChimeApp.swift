import SwiftUI

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    let appState = AppState()
    private var timer: Timer?

    func applicationDidFinishLaunching(_ notification: Notification) {
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
        if !appState.config.settingsShownOnce {
            SettingsWindow.show(appState)
            appState.config.settingsShownOnce = true
        }
    }
}

@main
struct AgentChimeApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate

    var body: some Scene {
        MenuBarExtra {
            MenuContent(state: delegate.appState)
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
