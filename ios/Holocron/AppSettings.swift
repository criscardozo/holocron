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
