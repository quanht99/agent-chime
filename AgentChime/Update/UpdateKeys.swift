import CryptoKit
import Foundation

/// The compiled-in trust anchor. A release build has no way to change the key or the base URL.
enum UpdateKeys {
    /// The owner's release public key (hex). Empty until the owner provides it.
    /// go/internal/update/keys_swift_test.go keeps this equal to Go's releasePublicKeyHex.
    static let releasePublicKeyHex = ""

    #if AGENTCHIME_UPDATE_TEST
    /// TEST ONLY. Equals go/internal/update/testdata/TEST-ONLY-public-key.hex.
    static let testPublicKeyHex = "5d4b0ea694e2958ee299813abf5eecc6de5c41a32580efa54358f391fcd49441"
    static let testBuild = true
    static var publicKey: Curve25519.Signing.PublicKey? { key(fromHex: testPublicKeyHex) }
    static var base: URL {
        if var s = ProcessInfo.processInfo.environment["AGENTCHIME_TEST_UPDATE_URL"], !s.isEmpty {
            if !s.hasSuffix("/") { s += "/" }
            if let u = URL(string: s) { return u }
        }
        return URL(string: UpdateLimits.releaseBase)!
    }
    static func allowed(_ u: URL) -> Bool {
        if isAllowedURL(u) { return true }
        return u.scheme?.lowercased() == "http" && u.user == nil && u.host == "127.0.0.1"
    }
    #else
    static let testBuild = false
    static var publicKey: Curve25519.Signing.PublicKey? { key(fromHex: releasePublicKeyHex) }
    static var base: URL { URL(string: UpdateLimits.releaseBase)! }
    static func allowed(_ u: URL) -> Bool { isAllowedURL(u) }
    #endif

    private static func key(fromHex hex: String) -> Curve25519.Signing.PublicKey? {
        let u = Array(hex.utf8)
        guard u.count == 64 else { return nil }
        var bytes = [UInt8]()
        for i in stride(from: 0, to: 64, by: 2) {
            guard let b = UInt8(String(decoding: u[i..<i + 2], as: UTF8.self), radix: 16) else { return nil }
            bytes.append(b)
        }
        return try? Curve25519.Signing.PublicKey(rawRepresentation: bytes)
    }
}
