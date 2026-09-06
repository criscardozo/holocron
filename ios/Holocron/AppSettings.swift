import Foundation
import Observation

/// Where the server lives and how to authenticate against it. Addresses and
/// identifiers are plain preferences; the two secrets live in the Keychain.
@MainActor
@Observable
final class AppSettings {
    private enum Keys {
        static let serverURL = "serverURL"
        static let token = "apiToken"
        static let accessClientID = "accessClientID"
        static let accessClientSecret = "accessClientSecret"
    }

    var serverURL: String {
        didSet { UserDefaults.standard.set(serverURL, forKey: Keys.serverURL) }
    }

    var token: String {
        didSet { Keychain.set(token, for: Keys.token) }
    }

    /// Cloudflare Access service token. Only needed when the dashboard is
    /// published through a tunnel with Access in front; on a LAN install both
    /// stay empty and nothing changes. The client id is an identifier, not a
    /// secret, so it sits with the other preferences.
    var accessClientID: String {
        didSet { UserDefaults.standard.set(accessClientID, forKey: Keys.accessClientID) }
    }

    var accessClientSecret: String {
        didSet { Keychain.set(accessClientSecret, for: Keys.accessClientSecret) }
    }

    init() {
        serverURL = UserDefaults.standard.string(forKey: Keys.serverURL) ?? ""
        token = Keychain.get(Keys.token) ?? ""
        accessClientID = UserDefaults.standard.string(forKey: Keys.accessClientID) ?? ""
        accessClientSecret = Keychain.get(Keys.accessClientSecret) ?? ""
    }

    /// A client for the configured server, or nil when setup is incomplete.
    var client: APIClient? {
        guard let url = Self.normalisedURL(serverURL), !token.isEmpty else { return nil }
        return APIClient(baseURL: url, token: token,
                         accessClientID: accessClientID,
                         accessClientSecret: accessClientSecret)
    }

    var isConfigured: Bool { client != nil }

    /// Whether the configured server address is a home-network one.
    ///
    /// Used to withhold the power-off button when it is not. A Raspberry Pi 4
    /// cannot be woken remotely, so pressing it from outside the house means no
    /// Jellyfin until somebody gets home — and that is a mistake the app can
    /// see coming, unlike a confirmation dialog, which only works if it is read.
    ///
    /// Deliberately conservative: anything it cannot recognise as local counts
    /// as away. Being wrong in that direction withholds a button; being wrong
    /// the other way loses the server.
    var isOnHomeNetwork: Bool {
        guard let host = Self.normalisedURL(serverURL)?.host() else { return false }
        return Self.isPrivateHost(host)
    }

    /// Recognises the addresses that only resolve inside a home network.
    nonisolated static func isPrivateHost(_ host: String) -> Bool {
        let lower = host.lowercased()
        if lower == "localhost" || lower.hasSuffix(".local") { return true }

        let parts = lower.split(separator: ".")
        guard parts.count == 4, let first = Int(parts[0]), let second = Int(parts[1]),
              parts.allSatisfy({ Int($0) != nil }) else { return false }

        switch first {
        case 10: return true
        case 192: return second == 168
        // 172.16.0.0/12, which is the range people forget is private.
        case 172: return (16...31).contains(second)
        default: return false
        }
    }

    /// Accepts "192.168.1.10:8090" as readily as a full URL, since that is what
    /// someone reads off their router. Pure parsing, so it is not tied to the
    /// main actor.
    nonisolated static func normalisedURL(_ raw: String) -> URL? {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }
        let withScheme = trimmed.contains("://") ? trimmed : "http://\(trimmed)"
        guard let url = URL(string: withScheme), url.host() != nil else { return nil }
        return url
    }
}
