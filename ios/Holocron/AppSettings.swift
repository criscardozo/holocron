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
        // Left over from the Cloudflare Access setup, removed with it. Kept
        // only so init can clear them off devices that still have them.
        static let legacyAccessClientID = "accessClientID"
        static let legacyAccessClientSecret = "accessClientSecret"
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
        // The public tunnel and its Access service token are gone: Holocron
        // is reached over the LAN or Tailscale, behind Caddy. A secret nothing
        // uses any more should not stay in the Keychain.
        UserDefaults.standard.removeObject(forKey: Keys.legacyAccessClientID)
        Keychain.set("", for: Keys.legacyAccessClientSecret)
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
