import Foundation

enum Language: String, Codable, CaseIterable {
    case en, vi

    var nativeName: String {
        switch self {
        case .en: return "English"
        case .vi: return "Tiếng Việt"
        }
    }
}

struct SoundChoice: Codable, Hashable {
    enum Source: String, Codable { case system, bundled, file }

    var source: Source
    var value: String

    static let defaultDone = SoundChoice(source: .system, value: "Glass")
    static let defaultAttention = SoundChoice(source: .system, value: "Sosumi")

    static func defaultChoice(for kind: AlertKind) -> SoundChoice {
        kind == .done ? defaultDone : defaultAttention
    }

    var displayName: String {
        switch source {
        case .system: return value
        case .bundled, .file:
            return URL(fileURLWithPath: value).deletingPathExtension().lastPathComponent
        }
    }
}

struct AgentChimeConfig: Codable, Equatable {
    var soundDone = SoundChoice.defaultDone
    var soundAttention = SoundChoice.defaultAttention
    var volumeDone = 1.0
    var volumeAttention = 1.0
    var doneThresholdSeconds: Double = 60
    var mutedUntil: Date? = nil
    var alertHeadless = false
    var language = Language.en
    var settingsShownOnce = false

    init() {}

    enum CodingKeys: String, CodingKey {
        case soundDone, soundAttention, volumeDone, volumeAttention, doneThresholdSeconds
        case mutedUntil, alertHeadless, language, settingsShownOnce
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        let d = AgentChimeConfig()
        soundDone = (try? c.decodeIfPresent(SoundChoice.self, forKey: .soundDone)) ?? d.soundDone
        soundAttention = (try? c.decodeIfPresent(SoundChoice.self, forKey: .soundAttention)) ?? d.soundAttention
        let vd = (try? c.decodeIfPresent(Double.self, forKey: .volumeDone)) ?? d.volumeDone
        let va = (try? c.decodeIfPresent(Double.self, forKey: .volumeAttention)) ?? d.volumeAttention
        volumeDone = min(1, max(0, vd))
        volumeAttention = min(1, max(0, va))
        doneThresholdSeconds = (try? c.decodeIfPresent(Double.self, forKey: .doneThresholdSeconds)) ?? d.doneThresholdSeconds
        mutedUntil = (try? c.decodeIfPresent(Date.self, forKey: .mutedUntil)) ?? nil
        alertHeadless = (try? c.decodeIfPresent(Bool.self, forKey: .alertHeadless)) ?? d.alertHeadless
        language = (try? c.decodeIfPresent(Language.self, forKey: .language)) ?? d.language
        settingsShownOnce = (try? c.decodeIfPresent(Bool.self, forKey: .settingsShownOnce)) ?? d.settingsShownOnce
    }

    func sound(for kind: AlertKind) -> SoundChoice {
        kind == .done ? soundDone : soundAttention
    }

    func volume(for kind: AlertKind) -> Double {
        kind == .done ? volumeDone : volumeAttention
    }

    func isMuted(at now: Date) -> Bool {
        guard let until = mutedUntil else { return false }
        return until > now
    }

    mutating func mute(for seconds: TimeInterval?, now: Date) {
        if let seconds = seconds {
            mutedUntil = now.addingTimeInterval(seconds)
        } else {
            mutedUntil = Date.distantFuture
        }
    }

    mutating func unmute() {
        mutedUntil = nil
    }
}

enum JSONCoding {
    static func encoder() -> JSONEncoder {
        let e = JSONEncoder()
        e.dateEncodingStrategy = .secondsSince1970
        e.outputFormatting = [.prettyPrinted, .sortedKeys]
        return e
    }

    static func decoder() -> JSONDecoder {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .secondsSince1970
        return d
    }
}

enum ConfigStore {
    static func load(from url: URL = Paths.config) -> AgentChimeConfig {
        guard let data = try? Data(contentsOf: url) else { return AgentChimeConfig() }
        do {
            return try JSONCoding.decoder().decode(AgentChimeConfig.self, from: data)
        } catch {
            Log.write("config.json is unreadable, using defaults: \(error.localizedDescription)")
            return AgentChimeConfig()
        }
    }

    static func save(_ config: AgentChimeConfig, to url: URL = Paths.config) throws {
        Paths.ensureDirectory(url.deletingLastPathComponent())
        let data = try JSONCoding.encoder().encode(config)
        try data.write(to: url, options: .atomic)
    }
}
