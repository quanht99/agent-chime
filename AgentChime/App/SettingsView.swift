import AppKit
import SwiftUI

struct SettingsView: View {
    @ObservedObject var state: AppState
    @ObservedObject var updater: Updater
    /// The window measures the unscrolled content to size itself, then shows it inside a ScrollView.
    var scrolling = true

    private var lang: Language { state.lang }

    private var thresholdMinutes: Binding<Int> {
        Binding(
            get: { Int((state.config.doneThresholdSeconds / 60).rounded()) },
            set: { state.config.doneThresholdSeconds = Double($0 * 60) }
        )
    }

    var body: some View {
        Group {
            if scrolling { ScrollView(.vertical) { content } } else { content }
        }
        .frame(width: 520)
        .onAppear { state.refresh() }
    }

    private var content: some View {
        VStack(alignment: .leading, spacing: 14) {
            SoundRow(state: state, kind: .done)
            SoundRow(state: state, kind: .attention)

            HStack {
                Text(L.t(.thresholdPrefix, lang))
                Stepper(value: thresholdMinutes, in: 0...120) {
                    Text("\(thresholdMinutes.wrappedValue)").monospacedDigit()
                }
                Text(L.t(.thresholdSuffix, lang))
            }

            Divider()
            hookRow(.claude)
            hookRow(.codex)
            if let err = state.errorText {
                Text(err).foregroundColor(.red).fixedSize(horizontal: false, vertical: true)
            }

            Divider()
            Toggle(L.t(.alertHeadless, lang), isOn: $state.config.alertHeadless)
            Toggle(L.t(.openAtLogin, lang), isOn: Binding(
                get: { state.launchAtLogin },
                set: { state.launchAtLogin = $0 }
            ))
            Picker(L.t(.language, lang), selection: $state.config.language) {
                ForEach(Language.allCases, id: \.self) { Text($0.nativeName).tag($0) }
            }
            .fixedSize()

            Divider()
            updatesSection

            Divider()
            statusSection
        }
        .padding(20)
        .frame(width: 520)
    }

    @ViewBuilder
    private func hookRow(_ agent: Agent) -> some View {
        let status = state.status(of: agent)
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text(agent.displayName).font(.headline).frame(width: 100, alignment: .leading)
                Text(statusText(agent, status)).fixedSize(horizontal: false, vertical: true)
                Spacer()
                switch status {
                case .installed:
                    removeButton(agent)
                case .partial:
                    installButton(agent)
                    removeButton(agent)
                case .notInstalled:
                    installButton(agent)
                case .unreadable:
                    EmptyView()
                }
            }
            if agent == .codex && state.codexNeedsRetrust {
                Text(L.t(.codexRetrust, lang)).font(.callout).foregroundColor(.secondary)
            }
        }
    }

    private func statusText(_ agent: Agent, _ status: InstallStatus) -> String {
        switch status {
        case .installed:
            guard agent == .codex else { return L.t(.hooksInstalled, lang) }
            switch state.codexTrust {
            case .trusted: return L.t(.codexTrusted, lang)
            case .disabled: return L.t(.codexDisabled, lang)
            case .untrusted, .notInstalled: return L.t(.codexUntrusted, lang)
            }
        case .partial: return L.t(.hooksPartial, lang)
        case .notInstalled: return L.t(.hooksNotInstalled, lang)
        case .unreadable(let path): return L.t(.unreadableConfig, lang, path)
        }
    }

    private func installButton(_ agent: Agent) -> some View {
        Button(L.t(.install, lang)) { state.install(agent) }
    }

    private func removeButton(_ agent: Agent) -> some View {
        Button(L.t(.remove, lang)) { state.uninstall(agent) }
    }

    @ViewBuilder
    private var updatesSection: some View {
        let st = updater.status
        let busy = st.phase == .checking || st.phase == .downloading
        VStack(alignment: .leading, spacing: 6) {
            Text(L.t(.updatesTitle, lang)).font(.headline)
            Text(L.t(.currentVersion, lang, st.current))
            Toggle(L.t(.checkAutomatically, lang), isOn: $state.config.checkForUpdates)
            HStack {
                Button(L.t(.checkNow, lang)) { updater.checkNow() }
                    .disabled(!updater.isRelease || busy)
                let text = updater.statusText(lang)
                if !text.isEmpty { Text(text).fixedSize(horizontal: false, vertical: true) }
            }
            if let last = st.lastSuccess {
                Text(L.t(.lastChecked, lang, TimeFormat.hhmm(last))).font(.callout).foregroundColor(.secondary)
            }
            if st.canInstall {
                if let block = updater.blockText(lang) {
                    Text(block).fixedSize(horizontal: false, vertical: true)
                    Button(L.t(.whatsNew, lang)) { updater.openReleasePage() }
                } else {
                    HStack {
                        Button(L.t(.updateButton, lang)) { updater.install() }
                        Button(L.t(.whatsNew, lang)) { updater.openReleasePage() }
                    }
                }
            }
        }
    }

    @ViewBuilder
    private var statusSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(L.t(.statusTitle, lang)).font(.headline)
            if let a = state.state.lastAlert {
                Text(L.t(.statusLastAlert, lang, a.summary(lang), TimeFormat.hhmm(a.date)))
            } else {
                Text(L.t(.menuNoAlert, lang))
            }
            if let s = state.state.lastSkip {
                Text(L.t(.statusLastSkip, lang, s.summary(lang), s.reason.text(lang), TimeFormat.hhmm(s.date)))
            }
            if let w = state.state.soundWarning {
                Text(w.text(lang))
            }
            Button(L.t(.openLog, lang)) { openLog() }
        }
    }

    private func openLog() {
        let file = Paths.logFile
        if FileManager.default.fileExists(atPath: file.path) {
            NSWorkspace.shared.open(file)
        } else {
            NSWorkspace.shared.open(file.deletingLastPathComponent())
        }
    }
}
