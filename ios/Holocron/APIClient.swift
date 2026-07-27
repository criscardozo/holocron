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
            "No se pudo conectar con el servidor. ¿Estás en la misma red?"
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

    // MARK: - Plex device link

    func startPlexLink() async throws -> PlexLinkStatus {
        try await request("plex/link", method: "POST")
    }

    func plexLinkStatus() async throws -> PlexLinkStatus {
        try await get("plex/link")
    }

    func selectPlexServer(baseURL: String) async throws {
        struct Body: Encodable { let baseUrl: String }
        try await send("plex/link/server", method: "POST", body: Body(baseUrl: baseURL))
    }

    // MARK: - Media

    func media() async throws -> MediaLibrary {
        try await get("media")
    }

    func syncMedia() async throws {
        try await send("media/sync", method: "POST")
    }

    func generateNFO() async throws {
        try await send("media/nfo", method: "POST")
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

    func addMagnet(_ magnet: String) async throws {
        struct Body: Encodable { let magnet: String }
        try await send("torrents", method: "POST", body: Body(magnet: magnet))
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

    private func perform(
        _ path: String,
        method: String,
        query: [URLQueryItem],
        body: (some Encodable)?
    ) async throws -> Data {
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
        if let body {
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = try JSONEncoder().encode(body)
        }

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
    private static func serverMessage(_ data: Data) -> String {
        struct Payload: Decodable { let error: String }
        return (try? decoder.decode(Payload.self, from: data))?.error ?? ""
    }
}
