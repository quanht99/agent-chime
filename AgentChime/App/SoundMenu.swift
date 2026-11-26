import AppKit
import SwiftUI

/// The sound picker as a native pop-up button. SwiftUI's Picker does not say which item the
/// pointer is on; NSMenu's delegate does, which lets the app play a sound the pointer rests on.
struct SoundMenu: NSViewRepresentable {
    struct Group: Equatable {
        let title: String
        let items: [SoundChoice]
    }

    let groups: [Group]
    let selection: SoundChoice
    let chooseFileTitle: String
    let onSelect: (SoundChoice) -> Void
    let onChooseFile: () -> Void
    /// The pointer moved onto a sound, or onto something that is not a sound (nil).
    let onHover: (SoundChoice?) -> Void
    let onClose: () -> Void

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    func makeNSView(context: Context) -> NSPopUpButton {
        let button = NSPopUpButton(frame: .zero, pullsDown: false)
        button.autoenablesItems = false
        button.target = context.coordinator
        button.action = #selector(Coordinator.picked(_:))
        button.menu?.delegate = context.coordinator
        button.setContentHuggingPriority(.defaultHigh, for: .horizontal)
        context.coordinator.rebuild(button)
        return button
    }

    func updateNSView(_ button: NSPopUpButton, context: Context) {
        context.coordinator.parent = self
        context.coordinator.rebuild(button)
    }

    final class Coordinator: NSObject, NSMenuDelegate {
        private static let chooseFileTag = 1

        var parent: SoundMenu
        private var builtFrom: (groups: [Group], selection: SoundChoice, chooseFileTitle: String)?

        init(_ parent: SoundMenu) { self.parent = parent }

        func rebuild(_ button: NSPopUpButton) {
            // SwiftUI calls update often (the settings view ticks); rebuilding an open menu
            // would end its tracking, so rebuild only when the content really changed.
            if let built = builtFrom, built.groups == parent.groups, built.selection == parent.selection,
               built.chooseFileTitle == parent.chooseFileTitle {
                return
            }
            builtFrom = (parent.groups, parent.selection, parent.chooseFileTitle)

            button.removeAllItems()
            guard let menu = button.menu else { return }
            var selected: NSMenuItem?
            for group in parent.groups where !group.items.isEmpty {
                if !menu.items.isEmpty { menu.addItem(.separator()) }
                menu.addItem(Self.header(group.title))
                for choice in group.items {
                    let item = NSMenuItem(title: choice.displayName, action: nil, keyEquivalent: "")
                    item.representedObject = choice
                    menu.addItem(item)
                    if choice == parent.selection, selected == nil { selected = item }
                }
            }
            menu.addItem(.separator())
            let choose = NSMenuItem(title: parent.chooseFileTitle, action: nil, keyEquivalent: "")
            choose.tag = Self.chooseFileTag
            menu.addItem(choose)
            button.select(selected)
        }

        private static func header(_ title: String) -> NSMenuItem {
            if #available(macOS 14.0, *) { return NSMenuItem.sectionHeader(title: title) }
            let item = NSMenuItem(title: title, action: nil, keyEquivalent: "")
            item.isEnabled = false
            return item
        }

        @objc func picked(_ sender: NSPopUpButton) {
            guard let item = sender.selectedItem else { return }
            if item.tag == Self.chooseFileTag {
                // Show the real selection again; the panel opens after this event finishes.
                let current = sender.itemArray.first { ($0.representedObject as? SoundChoice) == parent.selection }
                sender.select(current)
                let open = parent.onChooseFile
                DispatchQueue.main.async { open() }
                return
            }
            if let choice = item.representedObject as? SoundChoice, choice != parent.selection {
                parent.onSelect(choice)
            }
        }

        func menu(_ menu: NSMenu, willHighlight item: NSMenuItem?) {
            parent.onHover(item?.representedObject as? SoundChoice)
        }

        func menuDidClose(_ menu: NSMenu) {
            parent.onClose()
        }
    }
}
