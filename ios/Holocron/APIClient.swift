import Foundation

/// Errors surfaced to the UI. The messages are user-facing and in Spanish;
/// technical detail stays in the underlying error.
enum APIError: LocalizedError, Equatable {
    case notConfigured
    case unauthorized
    case noToken
    case notReachable
    case accessDenied
    case server(status: Int, message: String)
    case decoding

    var errorDescription: String? {
        switch self {
        case .notConfigured:
            "Configurá la dirección del servidor y el token en Ajustes."
        case .unauthorized:
            "El token no es válido. Generá uno nuevo en Ajustes de Holocron."
        case .noToken:
            "El servidor todavía no tiene un token. Generalo en Ajustes de la web."
        case .notReachable:
            // No longer "are you on the same network?": the server is also
            // reachable through the public domain, where the answer would be no
            // and the advice wrong.
            "No se pudo conectar con el servidor. Revisá la dirección en Ajustes."
        case .accessDenied:
            "Cloudflare Access rechazó el pedido. Revisá el service token en Ajustes."
        case let .server(status, message):
            message.isEmpty ? "El servidor respondió \(status)." : message
        case .decoding:
            "El servidor respondió algo inesperado."
        }
    }
}

/// Talks to the Holocron JSON API. Values are immutable, so the client is safe
/// to hand to any task.
struct APIClient: Sendable {
    let baseURL: URL
    let token: String
    /// Cloudflare Access service token, for when the server is published
    /// through a tunnel with Access in front. Empty on a LAN install, where
    /// there is nothing in the way.
    let accessClientID: String
    let accessClientSecret: String

    init(baseURL: URL, token: String, accessClientID: String = "", accessClientSecret: String = "") {
        self.baseURL = baseURL
        self.token = token
        self.accessClientID = accessClientID
        self.accessClientSecret = accessClientSecret
    }

    private static let decoder = JSONDecoder()

    // MARK: - System

    func system() async throws -> SystemStats {
        try await get("system")
    }

    // MARK: - Disk

    func diskFolders() async throws -> [DiskFolder] {
        try await get("disk", as: DiskFolders.self).folders
    }

    func diskDetail(id: Int64) async throws -> DiskDetail {
        try await get("disk/\(id)")
    }

    func diskBrowse(id: Int64, path: String) async throws -> DiskListing {
        var items = [URLQueryItem(name: "path", value: path)]
        if path.isEmpty { items = [] }
        return try await get("disk/\(id)/browse", query: items)
    }

    @discardableResult
    func startDiskScan(id: Int64) async throws -> Bool {
        try await send("disk/\(id)/scan", method: "POST")
        return true
    }

    // MARK: - Naming

    func naming() async throws -> NamingReport {
        try await get("naming")
    }

    @discardableResult
    func rescanNaming() async throws -> NamingReport {
        try await request("naming/scan", method: "POST")
    }

    // MARK: - Library quality

    func quality() async throws -> QualityReport {
        try await get("quality")
    }

    @discardableResult
    func startQualityScan() async throws -> Bool {
        try await send("quality/scan", method: "POST")
        return true
    }

    /// Asks Jellyfin to re-read one item. Form-encoded rather than JSON: it is
    /// what the web posts, and the handler reads a form value.
    func refreshQualityItem(_ itemID: String) async throws {
        try await sendForm("quality/refresh", fields: ["item": itemID])
    }

    // MARK: - Jellyfin Quick Connect

    func startJellyfinLink() async throws -> JellyfinLinkStatus {
        try await request("jellyfin/link", method: "POST")
    }

    func jellyfinLinkStatus() async throws -> JellyfinLinkStatus {
        try await get("jellyfin/link")
    }

    // MARK: - Media

    func media() async throws -> MediaLibrary {
        try await get("media")
    }

    func syncMedia() async throws {
        try await send("media/sync", method: "POST")
    }

    // MARK: - Subtitles

    func subtitles() async throws -> SubtitlesReport {
        try await get("subtitles")
    }

    func searchSubtitles(title: String, year: Int) async throws -> [SubtitleResult] {
        var query = [URLQueryItem(name: "title", value: title)]
        if year > 0 {
            query.append(URLQueryItem(name: "year", value: String(year)))
        }
        return try await get("subtitles/search", query: query, as: SubtitleResults.self).results
    }

    func downloadSubtitle(fileID: String, path: String) async throws {
        struct Body: Encodable {
            let fileId: Int
            let path: String
        }
        guard let numeric = Int(fileID) else { throw APIError.decoding }
        try await send("subtitles/download", method: "POST",
                       body: Body(fileId: numeric, path: path))
    }

    // MARK: - Torrents

    func torrents() async throws -> TorrentList {
        try await get("torrents")
    }

    func addMagnet(_ magnet: String, category: String = "") async throws {
        struct Body: Encodable {
            let magnet: String
            let category: String
        }
        try await send("torrents", method: "POST", body: Body(magnet: magnet, category: category))
    }

    func torrentAction(hash: String, action: String) async throws {
        try await send("torrents/\(hash)/\(action)", method: "POST")
    }

    // MARK: - Plumbing

    private func get<T: Decodable>(
        _ path: String,
        query: [URLQueryItem] = [],
        as _: T.Type = T.self
    ) async throws -> T {
        try await request(path, method: "GET", query: query)
    }

    /// Performs a request whose response body is ignored.
    private func send(_ path: String, method: String, body: (some Encodable)? = Optional<Never>.none) async throws {
        _ = try await perform(path, method: method, query: [], body: body)
    }

    private func request<T: Decodable>(
        _ path: String,
        method: String,
        query: [URLQueryItem] = [],
        body: (some Encodable)? = Optional<Never>.none
    ) async throws -> T {
        let data = try await perform(path, method: method, query: query, body: body)
        do {
            return try Self.decoder.decode(T.self, from: data)
        } catch {
            throw APIError.decoding
        }
    }

    /// Builds an authenticated request. Everything the client sends goes
    /// through here, so the credentials are attached in exactly one place.
    func makeRequest(_ path: String, method: String, query: [URLQueryItem] = []) throws -> URLRequest {
        guard var components = URLComponents(url: baseURL.appending(path: "api/v1/\(path)"),
                                             resolvingAgainstBaseURL: false) else {
            throw APIError.notConfigured
        }
        if !query.isEmpty { components.queryItems = query }
        guard let url = components.url else { throw APIError.notConfigured }

        var req = URLRequest(url: url)
        req.httpMethod = method
        req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        // Access checks these before the request ever reaches the Pi. Holocron
        // still checks the bearer token afterwards: two layers, on purpose.
        if !accessClientID.isEmpty, !accessClientSecret.isEmpty {
            req.setValue(accessClientID, forHTTPHeaderField: "CF-Access-Client-Id")
            req.setValue(accessClientSecret, forHTTPHeaderField: "CF-Access-Client-Secret")
        }
        req.timeoutInterval = 20
        return req
    }

    /// Posts a form, which is what the handlers written for the web UI read.
    private func sendForm(_ path: String, fields: [String: String]) async throws {
        var req = try makeRequest(path, method: "POST")
        req.setValue("application/x-www-form-urlencoded", forHTTPHeaderField: "Content-Type")
        var components = URLComponents()
        components.queryItems = fields.map { URLQueryItem(name: $0.key, value: $0.value) }
        req.httpBody = components.percentEncodedQuery?.data(using: .utf8)
        _ = try await run(req)
    }

    private func perform(
        _ path: String,
        method: String,
        query: [URLQueryItem],
        body: (some Encodable)?
    ) async throws -> Data {
        var req = try makeRequest(path, method: method, query: query)
        if let body {
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = try JSONEncoder().encode(body)
        }
        return try await run(req)
    }

    /// Sends a prepared request and maps the response onto APIError.
    private func run(_ req: URLRequest) async throws -> Data {

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await URLSession.shared.data(for: req)
        } catch {
            throw APIError.notReachable
        }

        guard let http = response as? HTTPURLResponse else { throw APIError.decoding }

        // An Access challenge does not look like an API error: URLSession
        // follows the redirect and hands back the login page with status 200,
        // so decoding would fail with "el servidor respondió algo inesperado"
        // and send the user looking in the wrong place.
        if Self.isAccessChallenge(http) {
            throw APIError.accessDenied
        }

        switch http.statusCode {
        case 200..<300:
            return data
        case 401:
            throw APIError.unauthorized
        case 503:
            throw APIError.noToken
        default:
            throw APIError.server(status: http.statusCode, message: Self.serverMessage(data))
        }
    }

    /// Recognises Cloudflare Access getting in the way.
    ///
    /// Access stamps `cf-access-aud` and `cf-access-domain` on its own
    /// responses, and that is the only signal present in every rejection
    /// measured against the real deployment. Everything else varies: the status
    /// is 403 for a Service Auth refusal and 302 for a browser with no session,
    /// `WWW-Authenticate` appears only on the AJAX-style 401, there is no
    /// redirect to follow on the 403 — and the body is **not** always HTML.
    /// Asked with `Accept: application/json`, Access answers 403 with
    /// `{"message":"Forbidden…"}`, which is why "it never speaks JSON" cannot
    /// be the rule even though it is tempting. Ordered by how much each signal
    /// can be trusted.
    static func isAccessChallenge(_ http: HTTPURLResponse) -> Bool {
        // 1. Access identifying itself, regardless of status, body or redirect.
        if http.value(forHTTPHeaderField: "cf-access-aud") != nil { return true }
        if http.value(forHTTPHeaderField: "cf-access-domain") != nil { return true }
        // 2. The redirect was followed and we landed on the login host.
        if http.url?.host()?.hasSuffix(".cloudflareaccess.com") == true { return true }
        // 3. The challenge header, on the AJAX-shaped 401.
        if http.value(forHTTPHeaderField: "WWW-Authenticate")?
            .lowercased().contains("cloudflare-access") == true { return true }
        // 4. Last resort, for an Access version that stops announcing itself:
        // HTML where this API only ever answers JSON.
        guard http.value(forHTTPHeaderField: "Content-Type")?
            .lowercased().contains("text/html") == true else { return false }
        return http.statusCode == 401 || http.statusCode == 403 || (300..<400).contains(http.statusCode)
    }

    /// Pulls the `error` field out of an error response, if present.
    ///
    /// Only that field, never the whole body, and the body is never logged: an
    /// error from Cloudflare Access carries `ip_address` with the caller's
    /// public IP, along with `ray_id` and `aud`. None of it belongs on screen
    /// or in a log file.
    private static func serverMessage(_ data: Data) -> String {
        struct Payload: Decodable { let error: String }
        return (try? decoder.decode(Payload.self, from: data))?.error ?? ""
    }
}
