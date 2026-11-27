import CryptoKit
import Foundation

// The signed-manifest core of the self-update. Mirrors go/internal/update
// (ParseVerified, Compare, AllowedURL). Foundation and CryptoKit only.

struct UpdateFile: Decodable, Equatable {
    let os: String
    let arch: String
    let name: String
    let size: Int64
    let sha256: String
}

struct UpdateManifest: Decodable {
    let format: Int
    let version: String
    let releasePage: String
    let files: [UpdateFile]
}

enum UpdateError: Error, Equatable {
    /// The signature or a checksum failed.
    case unverified
    /// The signature is fine but the content is invalid.
    case badManifest
    /// The manifest has no archive for this platform.
    case noFile
}

enum UpdateLimits {
    static let maxManifest = 64 << 10
    static let maxSig = 200
    static let maxArchive: Int64 = 100 << 20
    /// The only place release pages may live, in every build.
    static let releaseBase = "https://github.com/quanht99/agent-chime/releases/"
}

/// Verifies `sig` (std base64 of an Ed25519 signature) over `body` before parsing it, then validates the content.
func parseVerified(body: Data, sig: Data, key: Curve25519.Signing.PublicKey) throws -> UpdateManifest {
    if body.count > UpdateLimits.maxManifest || sig.count > UpdateLimits.maxSig { throw UpdateError.unverified }
    let text = String(decoding: sig, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
    guard let raw = Data(base64Encoded: text), raw.count == 64 else { throw UpdateError.unverified }
    guard key.isValidSignature(raw, for: body) else { throw UpdateError.unverified }

    guard let m = try? JSONDecoder().decode(UpdateManifest.self, from: body) else { throw UpdateError.badManifest }
    guard m.format == 1, parseVersion(m.version) != nil, m.releasePage.hasPrefix(UpdateLimits.releaseBase) else {
        throw UpdateError.badManifest
    }
    let prefix = "AgentChime-\(m.version)-"
    for f in m.files {
        if !f.name.hasPrefix(prefix) || f.name.contains("/") || f.name.contains("\\") || f.name.contains("..") {
            throw UpdateError.badManifest
        }
        if f.size < 1 || f.size > UpdateLimits.maxArchive || !isLowerHex64(f.sha256) { throw UpdateError.badManifest }
    }
    return m
}

private func isLowerHex64(_ s: String) -> Bool {
    let u = Array(s.utf8)
    return u.count == 64 && u.allSatisfy { ($0 >= 0x30 && $0 <= 0x39) || ($0 >= 0x61 && $0 <= 0x66) }
}

/// Strict X.Y.Z with an optional leading "v"; each part 1-9 ASCII digits.
private func parseVersion(_ s: String) -> [Int]? {
    let t = s.hasPrefix("v") ? String(s.dropFirst()) : s
    let parts = t.split(separator: ".", omittingEmptySubsequences: false)
    guard parts.count == 3 else { return nil }
    var out: [Int] = []
    for p in parts {
        let u = Array(p.utf8)
        guard !u.isEmpty, u.count <= 9, u.allSatisfy({ $0 >= 0x30 && $0 <= 0x39 }), let n = Int(p) else { return nil }
        out.append(n)
    }
    return out
}

/// -1, 0 or 1; nil when either version is unparsable.
func compareVersions(_ a: String, _ b: String) -> Int? {
    guard let va = parseVersion(a), let vb = parseVersion(b) else { return nil }
    for i in 0..<3 {
        if va[i] < vb[i] { return -1 }
        if va[i] > vb[i] { return 1 }
    }
    return 0
}

/// https, no userinfo, host github.com or a subdomain of githubusercontent.com.
func isAllowedURL(_ u: URL) -> Bool {
    guard u.scheme?.lowercased() == "https", u.user == nil, u.password == nil, let host = u.host?.lowercased() else {
        return false
    }
    return host == "github.com" || host.hasSuffix(".githubusercontent.com")
}

extension UpdateManifest {
    func macFile() throws -> UpdateFile {
        guard let f = files.first(where: { $0.os == "macos" && $0.arch == "universal" }) else { throw UpdateError.noFile }
        return f
    }
}
