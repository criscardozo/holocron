import Foundation
import Observation

/// Where the server lives and how to authenticate against it. The address is a
/// plain preference; the token is a credential and lives in the Keychain.
@MainActor
@Observable
final class AppSettings {
    private enum Keys {
        static let serverURL = "serverURL"
        static let token = "apiToken"
    }

    var serverURL: String {
        didSet { UserDefaults.standard.set(serverURL, forKey: Keys.serverURL) }
    }

    var token: String {
        didSet { Keychain.set(token, for: Keys.token) }
    }

    init() {
        serverURL = UserDefaults.standard.string(forKey: Keys.serverURL) ?? ""
        token = Keychain.get(Keys.token) ?? ""
    }

    /// A client for the configured server, or nil when setup is incomplete.
    var client: APIClient? {
        guard let url = Self.normalisedURL(serverURL), !token.isEmpty else { return nil }
        return APIClient(baseURL: url, token: token)
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
