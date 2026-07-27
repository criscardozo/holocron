import Foundation

// Wire models for the Holocron JSON API (see docs/api.md). Field names match
// the server's JSON exactly, so no custom CodingKeys are needed.

struct SystemStats: Codable {
    var cpuPercent: Double?
    var memUsedBytes: UInt64?
    var memTotalBytes: UInt64?
    var memPercent: Double?
    var tempCelsius: Double?
    var uptimeSeconds: Int64?
    var load1: Double?
    var hostname: String
}

struct DiskFolder: Codable, Identifiable, Hashable {
    var id: Int64
    var label: String
    var path: String
    var totalBytes: UInt64
    var usedBytes: UInt64
    var freeBytes: UInt64
    var usedPercent: Int
    var available: Bool

    /// Near-full folders are highlighted, matching the web UI's "hot" bar.
    var isHot: Bool { usedPercent >= 90 }
}

struct DiskFolders: Codable {
    var folders: [DiskFolder]
}

struct DiskEntry: Codable, Identifiable, Hashable {
    var name: String
    var path: String
    var bytes: UInt64
    var isDir: Bool

    var id: String { path }
}

struct DiskDetail: Codable {
    var folder: DiskFolder
    var scanning: Bool
    var top: [DiskEntry]
    var scannedAt: String?
}

struct DiskListing: Codable {
    var path: String
    var parent: String
    var totalBytes: UInt64
    var entries: [DiskEntry]
}

struct NamingIssue: Codable, Identifiable, Hashable {
    var path: String
    var type: String
    var found: String
    var expected: String

    var id: String { path }

    var typeLabel: String {
        switch type {
        case "movies": "Peli"
        case "tv": "Serie"
        default: type
        }
    }
}

struct NamingReport: Codable {
    var count: Int
    var issues: [NamingIssue]
}

struct MediaItem: Codable, Identifiable, Hashable {
    var path: String
    var title: String
    var year: Int
    var type: String
    var hasNfo: Bool
    var hasSubsEs: Bool

    var id: String { path }
}

struct MediaLibrary: Codable {
    var configured: Bool
    var total: Int?
    var withNfo: Int?
    var withoutSubsEs: Int?
    var syncing: Bool?
    var generatingNfo: Bool?
    var items: [MediaItem]
    var truncated: Bool?
}

struct SubtitleMissing: Codable, Identifiable, Hashable {
    var path: String
    var title: String
    var year: Int
    var type: String

    var id: String { path }
}

struct SubtitlesReport: Codable {
    var configured: Bool
    var missing: Int
    var items: [SubtitleMissing]
    var truncated: Bool
}

struct SubtitleResult: Codable, Identifiable, Hashable {
    var fileId: String
    var fileName: String
    var release: String
    var language: String

    var id: String { fileId }
}

struct SubtitleResults: Codable {
    var results: [SubtitleResult]
}

struct Torrent: Codable, Identifiable, Hashable {
    var hash: String
    var name: String
    var state: String
    var progress: Double
    var sizeBytes: Int64
    var dlSpeed: Int64
    var upSpeed: Int64
    var seeds: Int
    var leechs: Int
    var paused: Bool

    var id: String { hash }

    /// Coarse status derived from qBittorrent's raw state, mirroring
    /// `torrentClass`/`torrentLabel` on the server side.
    enum Status {
        case downloading, seeding, paused, failed

        var label: String {
            switch self {
            case .downloading: "Descargando"
            case .seeding: "Sembrando"
            case .paused: "Pausado"
            case .failed: "Error"
            }
        }
    }

    var status: Status {
        let s = state.lowercased()
        if paused { return .paused }
        if s.contains("error") || s.contains("missing") { return .failed }
        if s.contains("up") { return .seeding }
        return .downloading
    }
}

struct TorrentList: Codable {
    var configured: Bool
    var total: Int?
    var active: Int?
    var dlSpeed: Int64?
    var upSpeed: Int64?
    var torrents: [Torrent]
}

// MARK: - Plex device link

struct PlexServer: Codable, Identifiable, Hashable {
    var name: String
    var baseUrl: String

    var id: String { baseUrl }
}

/// Where the plex.tv device-link flow stands. Mirrors `plexauth.State`.
struct PlexLinkStatus: Codable {
    var state: String
    var code: String?
    var authUrl: String?
    var servers: [PlexServer]?

    var isPending: Bool { state == "pending" }
    var isLinked: Bool { state == "linked" }
    var isExpired: Bool { state == "expired" }
}
