import Foundation

/// Plays a sound once the pointer has rested on it in the sound menu for `delay` seconds.
/// Moving to another item, to a non-sound item, or closing the menu cancels the wait and
/// stops the sound that is playing.
final class HoverPreview {
    static let delay: TimeInterval = 1.0

    typealias Cancel = () -> Void

    private let schedule: (TimeInterval, @escaping () -> Void) -> Cancel
    private let play: (SoundChoice) -> Void
    private let stop: () -> Void
    private var cancelPending: Cancel?
    private var hovered: SoundChoice?

    init(schedule: @escaping (TimeInterval, @escaping () -> Void) -> Cancel,
         play: @escaping (SoundChoice) -> Void,
         stop: @escaping () -> Void) {
        self.schedule = schedule
        self.play = play
        self.stop = stop
    }

    /// `nil` means the pointer is on something that is not a sound.
    func hover(_ choice: SoundChoice?) {
        if choice == hovered { return }
        reset()
        hovered = choice
        guard let choice = choice else { return }
        cancelPending = schedule(Self.delay) { [weak self] in
            self?.cancelPending = nil
            self?.play(choice)
        }
    }

    /// The menu closed.
    func end() {
        reset()
        hovered = nil
    }

    private func reset() {
        cancelPending?()
        cancelPending = nil
        stop()
    }
}
