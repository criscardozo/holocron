import Foundation

/// Errors surfaced to the UI. The messages are user-facing and in Spanish;
/// technical detail stays in the underlying error.
enum APIError: LocalizedError, Equatable {
    case notConfigured
    case unauthorized
    case noToken
    case notReachable
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

    init(baseURL: URL, token: String) {
        self.baseURL = baseURL
        self.token = token
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

    // MARK: - Machine management

    func manage() async throws -> ManageStatus {
        try await get("manage")
    }

    /// `acknowledged` carries the consent the server asks for when powering
    /// off over the public address. Sent only when the person actually gave
    /// it: passing it always would make the server-side check decorative.
    func runManageAction(_ key: String, acknowledged: Bool = false) async throws {
        var fields = ["action": key]
        if acknowledged { fields["ack"] = "1" }
        try await sendForm("manage/action", fields: fields)
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

    // MARK: - Live screens

    func home() async throws -> HomeReading {
        try await get("home")
    }

    func hardware() async throws -> HardwareReading {
        try await get("hardware")
    }

    func activity() async throws -> ActivityReading {
        try await get("activity")
    }

    func services() async throws -> ServicesReading {
        try await get("services")
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
