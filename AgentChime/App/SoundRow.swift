import AppKit
import SwiftUI
import UniformTypeIdentifiers

struct SoundRow: View {
    @ObservedObject var state: AppState
    let kind: AlertKind

    private var lang: Language { state.lang }
    private var current: SoundChoice { state.config.sound(for: kind) }

    private var system: [SoundChoice] { SoundLibrary.systemSounds() }
    private var builtIn: [SoundChoice] { SoundLibrary.bundledSounds() }

    private func setSound(_ choice: SoundChoice) {
        if kind == .done { state.config.soundDone = choice } else { state.config.soundAttention = choice }
    }

    private var volume: Binding<Double> {
        Binding(
            get: { state.config.volume(for: kind) },
            set: { v in
                if kind == .done { state.config.volumeDone = v } else { state.config.volumeAttention = v }
            }
        )
    }

    private func chooseFile() {
        let panel = NSOpenPanel()
        panel.allowedContentTypes = [.audio]
        panel.allowsMultipleSelection = false
        panel.canChooseDirectories = false
        if panel.runModal() == .OK, let url = panel.url {
            setSound(SoundChoice(source: .file, value: url.path))
        }
    }

    private var groups: [SoundMenu.Group] {
        // The current choice always appears, even if it vanished from the lists.
        let missing = !system.contains(current) && !builtIn.contains(current) && current.source != .file
        var groups = [
            SoundMenu.Group(title: L.t(.groupSystem, lang), items: system),
            SoundMenu.Group(title: L.t(.groupBuiltIn, lang), items: builtIn + (missing ? [current] : [])),
        ]
        if current.source == .file {
            groups.append(SoundMenu.Group(title: L.t(.groupFile, lang), items: [current]))
        }
        return groups
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(L.t(kind == .done ? .soundDone : .soundAttention, lang)).font(.headline)
            HStack {
                SoundMenu(
                    groups: groups,
                    selection: current,
                    chooseFileTitle: L.t(.chooseFile, lang),
                    onSelect: { setSound($0) },
                    onChooseFile: { chooseFile() },
                    onHover: { state.hoverPreview($0, kind: kind) },
                    onClose: { state.endHoverPreview() }
                )
                .fixedSize()
                Button(L.t(.preview, lang)) { state.preview(kind) }
            }
            HStack {
                Text(L.t(.volume, lang))
                Slider(value: volume, in: 0...1)
            }
        }
    }
}
