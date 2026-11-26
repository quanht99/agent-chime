import Foundation

enum L {
    enum Key: String, CaseIterable {
        case statusOn
        case statusMuted
        case statusMutedUntil
        case menuMute
        case mute30
        case mute60
        case muteForever
        case menuUnmute
        case menuLast
        case menuNoAlert
        case menuNeedsSetup
        case menuSettings
        case menuQuit
        case summaryDone
        case summaryDoneDuration
        case summaryAttention
        case settingsTitle
        case soundDone
        case soundAttention
        case preview
        case volume
        case thresholdPrefix
        case thresholdSuffix
        case groupSystem
        case groupBuiltIn
        case groupFile
        case chooseFile
        case hooksInstalled
        case hooksPartial
        case hooksNotInstalled
        case codexUntrusted
        case codexTrusted
        case codexDisabled
        case codexRetrust
        case unreadableConfig
        case helperMissing
        case install
        case remove
        case alertHeadless
        case openAtLogin
        case loginItemError
        case language
        case statusTitle
        case statusLastAlert
        case statusLastSkip
        case soundWarning
        case openLog
        case reasonMuted
        case reasonMutedUntil
        case reasonHeadless
        case reasonShortTurn
        case reasonDuplicate
        case reasonNested
    }

    static func t(_ key: Key, _ lang: Language, _ args: String...) -> String {
        let pair = table(key)
        var s = lang == .vi ? pair.vi : pair.en
        for arg in args {
            guard let r = s.range(of: "%@") else { break }
            s.replaceSubrange(r, with: arg)
        }
        return s
    }

    private static func table(_ key: Key) -> (en: String, vi: String) {
        switch key {
        case .statusOn: return ("● On", "● Đang bật")
        case .statusMuted: return ("○ Muted", "○ Đã tắt tiếng")
        case .statusMutedUntil: return ("○ Muted until %@", "○ Tắt tiếng đến %@")
        case .menuMute: return ("Mute", "Tắt tiếng")
        case .mute30: return ("30 minutes", "30 phút")
        case .mute60: return ("1 hour", "1 giờ")
        case .muteForever: return ("Until I turn it back on", "Đến khi bật lại")
        case .menuUnmute: return ("Unmute", "Bật tiếng lại")
        case .menuLast: return ("Last: %@ — %@", "Gần nhất: %@ — %@")
        case .menuNoAlert: return ("No alerts yet", "Chưa có âm nào")
        case .menuNeedsSetup: return ("⚠ Setup needed — open Settings", "⚠ Cần cài đặt — mở Cài đặt")
        case .menuSettings: return ("Settings…", "Cài đặt…")
        case .menuQuit: return ("Quit", "Thoát")
        case .summaryDone: return ("%@ done", "%@ xong")
        case .summaryDoneDuration: return ("%@ done (%@)", "%@ xong (%@)")
        case .summaryAttention: return ("%@ needs you", "%@ cần duyệt")
        case .settingsTitle: return ("AgentChime Settings", "Cài đặt AgentChime")
        case .soundDone: return ("\"Done\" sound", "Âm \"Xong việc\"")
        case .soundAttention: return ("\"Needs you\" sound", "Âm \"Cần duyệt\"")
        case .preview: return ("▶ Preview", "▶ Nghe thử")
        case .volume: return ("Volume", "Âm lượng")
        case .thresholdPrefix: return ("Only when the turn ran longer than", "Chỉ kêu khi lượt chạy lâu hơn")
        case .thresholdSuffix: return ("min   (0 = always)", "phút   (0 = luôn kêu)")
        case .groupSystem: return ("System sounds", "Âm hệ thống")
        case .groupBuiltIn: return ("Built-in sounds", "Âm có sẵn")
        case .groupFile: return ("Your file", "File của bạn")
        case .chooseFile: return ("Choose file…", "Chọn file…")
        case .hooksInstalled: return ("✔ Hooks installed", "✔ Đã cài hook")
        case .hooksPartial: return ("⚠ Some hooks are missing", "⚠ Thiếu một số hook")
        case .hooksNotInstalled: return ("✖ Not installed", "✖ Chưa cài")
        case .codexUntrusted: return ("⚠ Installed, not trusted — open Codex, type /hooks", "⚠ Đã cài, chưa tin cậy — mở Codex, gõ /hooks")
        case .codexTrusted: return ("✔ Installed and trusted", "✔ Đã cài và đã tin cậy")
        case .codexDisabled: return ("⚠ Hooks are disabled in Codex — open Codex, type /hooks", "⚠ Hook đang bị tắt trong Codex — mở Codex, gõ /hooks")
        case .codexRetrust: return ("Hooks were updated. Open Codex and type /hooks to trust them again.", "Hook vừa được cập nhật. Mở Codex, gõ /hooks để tin cậy lại.")
        case .unreadableConfig: return ("Can't read %@ (invalid JSON). AgentChime won't modify this file.", "Không đọc được %@ (JSON lỗi). AgentChime không sửa file này.")
        case .helperMissing: return ("AgentChime's helper isn't installed yet. Quit and reopen AgentChime, then try again.", "Chưa cài được helper của AgentChime. Hãy thoát và mở lại AgentChime rồi thử lại.")
        case .install: return ("Install", "Cài")
        case .remove: return ("Remove", "Gỡ")
        case .alertHeadless: return ("Also alert for non-interactive runs (claude -p, codex exec)", "Kêu cả với lượt chạy không tương tác (claude -p, codex exec)")
        case .openAtLogin: return ("Open AgentChime at login", "Mở AgentChime khi đăng nhập")
        case .loginItemError: return ("Couldn't change the login item: %@", "Không đổi được mục mở khi đăng nhập: %@")
        case .language: return ("Language", "Ngôn ngữ")
        case .statusTitle: return ("Status", "Trạng thái")
        case .statusLastAlert: return ("Last alert: %@ — %@", "Âm gần nhất: %@ — %@")
        case .statusLastSkip: return ("Last skipped: %@ — %@ — %@", "Bỏ qua gần nhất: %@ — %@ — %@")
        case .soundWarning: return ("⚠ Couldn't open %@. Played the default sound instead.", "⚠ Không mở được %@. Đã phát âm mặc định thay thế.")
        case .openLog: return ("Open log file", "Mở file log")
        case .reasonMuted: return ("muted", "đang tắt tiếng")
        case .reasonMutedUntil: return ("muted until %@", "đang tắt tiếng đến %@")
        case .reasonHeadless: return ("non-interactive run", "lượt chạy không tương tác")
        case .reasonShortTurn: return ("short turn (%@ < %@)", "lượt ngắn (%@ < %@)")
        case .reasonDuplicate: return ("same as the previous alert", "trùng với lần báo trước")
        case .reasonNested: return ("agent running inside another agent", "agent chạy lồng trong agent khác")
        }
    }
}
