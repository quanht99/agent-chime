import SwiftUI

struct MenuContent: View {
    @ObservedObject var state: AppState

    var body: some View {
        let lang = state.lang
        let cfg = state.config
        let muted = cfg.isMuted(at: state.now)

        if !muted {
            Text(L.t(.statusOn, lang))
            Menu(L.t(.menuMute, lang)) {
                Button(L.t(.mute30, lang)) { state.mute(for: 30 * 60) }
                Button(L.t(.mute60, lang)) { state.mute(for: 60 * 60) }
                Button(L.t(.muteForever, lang)) { state.mute(for: nil) }
            }
        } else {
            if let until = cfg.mutedUntil, until != Date.distantFuture {
                Text(L.t(.statusMutedUntil, lang, TimeFormat.hhmm(until)))
            } else {
                Text(L.t(.statusMuted, lang))
            }
            Button(L.t(.menuUnmute, lang)) { state.unmute() }
        }

        Divider()
        if let last = state.state.lastAlert {
            Text(L.t(.menuLast, lang, last.summary(lang), TimeFormat.hhmm(last.date)))
        } else {
            Text(L.t(.menuNoAlert, lang))
        }
        if let w = state.state.soundWarning {
            Text(L.t(.soundWarning, lang, w.path))
        }
        if state.needsSetup {
            Button(L.t(.menuNeedsSetup, lang)) { SettingsWindow.show(state) }
        }

        Divider()
        Button(L.t(.menuSettings, lang)) { SettingsWindow.show(state) }
            .keyboardShortcut(",")
        Button(L.t(.menuQuit, lang)) { NSApplication.shared.terminate(nil) }
            .keyboardShortcut("q")
    }
}
