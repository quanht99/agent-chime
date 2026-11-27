import AppKit
import Combine
import SwiftUI

enum SettingsWindow {
    private static var window: NSWindow?
    private static var languageSub: AnyCancellable?

    @MainActor static func show(_ state: AppState, updater: Updater) {
        state.refresh()
        if window == nil {
            let w = NSWindow(
                contentRect: NSRect(x: 0, y: 0, width: 520, height: 400),
                styleMask: [.titled, .closable],
                backing: .buffered,
                defer: false
            )
            w.isReleasedWhenClosed = false
            w.contentView = NSHostingView(rootView: SettingsView(state: state, updater: updater))
            window = w
            languageSub = state.$config
                .map { $0.language }
                .removeDuplicates()
                .receive(on: DispatchQueue.main)
                .sink { lang in window?.title = L.t(.settingsTitle, lang) }
        }
        if let w = window {
            w.title = L.t(.settingsTitle, state.lang)
            // Size from the SwiftUI content; a ScrollView takes over if it would not fit on screen.
            let measure = NSHostingView(rootView: SettingsView(state: state, updater: updater, scrolling: false))
            let wanted = measure.fittingSize
            let maxHeight = (w.screen ?? NSScreen.main)?.visibleFrame.height ?? 800
            w.setContentSize(NSSize(width: 520, height: min(wanted.height, maxHeight - 80)))
            w.center()
            w.makeKeyAndOrderFront(nil)
        }
        NSApp.activate(ignoringOtherApps: true)
    }
}
