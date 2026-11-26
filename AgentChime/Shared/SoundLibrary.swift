import Foundation

enum SoundLibrary {
    static let audioExtensions: Set<String> = ["wav", "aiff", "aif", "caf", "mp3", "m4a"]

    static func isAudioFile(_ name: String) -> Bool {
        audioExtensions.contains((name as NSString).pathExtension.lowercased())
    }

    static func systemSounds(in dir: URL = Paths.systemSoundsDir) -> [SoundChoice] {
        let names = (try? FileManager.default.contentsOfDirectory(atPath: dir.path)) ?? []
        return names
            .filter { ($0 as NSString).pathExtension.lowercased() == "aiff" }
            .map { ($0 as NSString).deletingPathExtension }
            .sorted()
            .map { SoundChoice(source: .system, value: $0) }
    }

    static func bundledSounds(in dir: URL = Paths.soundsDir) -> [SoundChoice] {
        let names = (try? FileManager.default.contentsOfDirectory(atPath: dir.path)) ?? []
        return names
            .filter(isAudioFile)
            .sorted { $0.localizedCaseInsensitiveCompare($1) == .orderedAscending }
            .map { SoundChoice(source: .bundled, value: $0) }
    }
}
