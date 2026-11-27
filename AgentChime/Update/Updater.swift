import AppKit
import CryptoKit
import Foundation

enum UpdatePhase: Equatable {
    case idle, checking, upToDate, available, downloading, checkFailed, unverified
    case failed, badBuild, restartNeeded, updated, sourceBuild
}

struct UpdateStatus: Equatable {
    var phase: UpdatePhase
    var current: String
    var latest = ""
    var releasePage = ""
    var lastSuccess: Date?
    var detail = ""

    /// A strictly newer version is known and nothing blocks a new install.
    var canInstall: Bool {
        guard compareVersions(latest, current) == 1 else { return false }
        switch phase {
        case .downloading, .restartNeeded, .updated, .sourceBuild, .checking: return false
        default: return true
        }
    }
}

/// Refuses redirects to hosts the build does not allow.
private final class RedirectGuard: NSObject, URLSessionTaskDelegate {
    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) {
        if let u = request.url, UpdateKeys.allowed(u) { completionHandler(request) } else { completionHandler(nil) }
    }
}

@MainActor
final class Updater: ObservableObject {
    @Published private(set) var status: UpdateStatus

    let isRelease: Bool
    private var configProvider: (() -> AgentChimeConfig)?
    private var loop: Task<Void, Never>?
    private var busy = false
    /// The manifest the last successful check verified.
    private var lastManifest: UpdateManifest?
    private let redirectGuard = RedirectGuard()
    private lazy var session: URLSession = {
        let c = URLSessionConfiguration.ephemeral
        c.httpCookieStorage = nil
        c.httpShouldSetCookies = false
        // A fixed neutral language: the user's locale is not ours to send.
        c.httpAdditionalHeaders = ["User-Agent": "AgentChime", "Accept-Language": "en"]
        c.timeoutIntervalForRequest = 60
        c.timeoutIntervalForResource = 300
        return URLSession(configuration: c, delegate: redirectGuard, delegateQueue: nil)
    }()

    nonisolated static var bundleIsRelease: Bool {
        Bundle.main.object(forInfoDictionaryKey: "AgentChimeRelease") as? String == "YES"
    }

    init(isRelease: Bool = Updater.bundleIsRelease, afterUpdate: Bool = false) {
        self.isRelease = isRelease
        let current = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? ""
        status = UpdateStatus(phase: isRelease ? (afterUpdate ? .updated : .idle) : .sourceBuild, current: current)
    }

    // MARK: - Schedule

    /// First check 60 s after start, then every 24 h; only when the switch is on and this is a release.
    func start(config: @escaping () -> AgentChimeConfig) {
        configProvider = config
        guard isRelease, loop == nil else { return }
        var first: UInt64 = 60
        #if AGENTCHIME_UPDATE_TEST
        if let s = ProcessInfo.processInfo.environment["AGENTCHIME_TEST_UPDATE_DELAY"], let n = UInt64(s) { first = n }
        #endif
        loop = Task { [weak self] in
            try? await Task.sleep(nanoseconds: first * 1_000_000_000)
            while !Task.isCancelled {
                if let self, self.configProvider?().checkForUpdates == true { await self.checkThenMaybeInstall() }
                try? await Task.sleep(nanoseconds: 24 * 3600 * 1_000_000_000)
            }
        }
    }

    func checkNow() {
        guard isRelease else { return }
        Task { await checkThenMaybeInstall() }
    }

    /// `check()`, then (test builds only) the automatic install the e2e script asks for. The install
    /// starts after `check()` returned, so its `busy` flag is not cleared by check's `defer`.
    private func checkThenMaybeInstall() async {
        await check()
        #if AGENTCHIME_UPDATE_TEST
        if status.phase == .available, ProcessInfo.processInfo.environment["AGENTCHIME_TEST_UPDATE_AUTOINSTALL"] == "1" {
            install()
        }
        #endif
    }

    // MARK: - Check

    private func manifestURL(_ name: String) -> URL { URL(string: UpdateKeys.base.absoluteString + "latest/download/" + name)! }

    private func fetchLimited(_ url: URL, max: Int) async throws -> Data {
        guard UpdateKeys.allowed(url) else { throw InstallFailure("url not allowed") }
        let (bytes, resp) = try await session.bytes(from: url)
        guard (resp as? HTTPURLResponse)?.statusCode == 200 else {
            throw InstallFailure("HTTP \((resp as? HTTPURLResponse)?.statusCode ?? 0)")
        }
        var data = Data()
        for try await b in bytes {
            data.append(b)
            if data.count > max { throw UpdateError.unverified }
        }
        return data
    }

    private func latestOnce(_ key: Curve25519.Signing.PublicKey) async throws -> UpdateManifest {
        let body = try await fetchLimited(manifestURL("latest.json"), max: UpdateLimits.maxManifest)
        let sig = try await fetchLimited(manifestURL("latest.json.sig"), max: UpdateLimits.maxSig)
        return try parseVerified(body: body, sig: sig, key: key)
    }

    /// Fetch and verify the manifest, retrying once after 5 s (a release may have been published mid-check).
    private func fetchLatest(_ key: Curve25519.Signing.PublicKey) async throws -> UpdateManifest {
        do { return try await latestOnce(key) } catch {
            if Task.isCancelled { throw error }
            try await Task.sleep(nanoseconds: 5_000_000_000)
            do { return try await latestOnce(key) } catch let e as UpdateError where e == .unverified || e == .badManifest {
                throw UpdateError.unverified
            }
        }
    }

    private func check() async {
        guard isRelease, !busy else { return }
        // No compiled-in key (release build before the owner provides it): nothing can be verified, so no network.
        guard let key = UpdateKeys.publicKey else {
            status.phase = .unverified
            Log.write("update: no release key compiled in; cannot verify updates")
            return
        }
        busy = true
        defer { busy = false }
        status.phase = .checking
        do {
            let m = try await fetchLatest(key)
            lastManifest = m
            status.lastSuccess = Date()
            status.latest = m.version
            status.releasePage = m.releasePage
            let newer = compareVersions(m.version, status.current) == 1
            status.phase = newer ? .available : .upToDate
            if newer { Log.write("update: \(m.version) is available") }
        } catch UpdateError.unverified {
            status.phase = .unverified
            Log.write("update: manifest could not be verified")
        } catch {
            status.phase = .checkFailed
            Log.write("update: check failed: \(error.localizedDescription)")
        }
    }

    // MARK: - Install

    var blockReason: BlockReason? { MacInstaller.currentBlockReason() }

    func install() {
        guard isRelease, !busy, status.canInstall, blockReason == nil, let key = UpdateKeys.publicKey else { return }
        guard let manifest = lastManifest, manifest.version == status.latest else { return }
        busy = true
        status.phase = .downloading
        let installer = MacInstaller(session: session, currentVersion: status.current) { [weak self] in
            guard let self else { throw UpdateError.unverified }
            return try await self.fetchLatest(key)
        }
        Task {
            defer { busy = false }
            do {
                let file = try manifest.macFile()
                let out = try await installer.install(manifest: manifest, file: file) {
                    DispatchQueue.main.async { NSApp.terminate(nil) }
                }
                switch out {
                case .relaunched: status.phase = .updated
                case .badBuild: status.phase = .badBuild
                case .restartNeeded: status.phase = .restartNeeded
                }
            } catch UpdateError.unverified {
                status.phase = .unverified
                Log.write("update: download could not be verified")
            } catch {
                status.phase = .failed
                status.detail = error.localizedDescription
                Log.write("update failed: \(error.localizedDescription)")
            }
        }
    }

    func openReleasePage() {
        let s = status.releasePage.hasPrefix(UpdateLimits.releaseBase) ? status.releasePage : UpdateLimits.releaseBase + "latest"
        if let u = URL(string: s) { NSWorkspace.shared.open(u) }
    }

    // MARK: - Text

    private var lang: Language { configProvider?().language ?? .en }

    /// The menu item, only when an update can be installed from here.
    var menuLabel: String? {
        guard status.canInstall, blockReason == nil else { return nil }
        return L.t(.updateMenuItem, lang, status.latest)
    }

    func statusText(_ lang: Language) -> String {
        switch status.phase {
        case .idle: return ""
        case .checking: return L.t(.checking, lang)
        case .upToDate: return L.t(.upToDate, lang)
        case .available: return L.t(.updateAvailable, lang, status.latest)
        case .downloading: return L.t(.downloading, lang)
        case .checkFailed: return L.t(.checkFailed, lang)
        case .unverified: return L.t(.updateUnverified, lang)
        case .failed: return L.t(.updateFailed, lang, status.detail)
        case .badBuild: return L.t(.newVersionFailed, lang)
        case .restartNeeded: return L.t(.restartToFinish, lang)
        case .updated: return L.t(.updatedTo, lang, status.current)
        case .sourceBuild: return L.t(.sourceBuild, lang)
        }
    }

    func blockText(_ lang: Language) -> String? {
        switch blockReason {
        case .moveToApplications: return L.t(.moveToApplications, lang)
        case .cantInstallHere: return L.t(.cantInstallHere, lang)
        case nil: return nil
        }
    }
}
