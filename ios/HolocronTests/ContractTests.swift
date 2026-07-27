import Foundation
import Testing

@testable import Holocron

/// These decode payloads captured from a real Holocron server (see
/// Fixtures/README.md). They exist to catch the most likely way this app
/// breaks: the Go API changing a field name or type while the Swift models
/// keep expecting the old shape.
struct ContractTests {
    private func fixture(_ name: String) throws -> Data {
        let url = try #require(
            Bundle(for: BundleToken.self).url(forResource: name, withExtension: "json"),
            "missing fixture \(name).json"
        )
        return try Data(contentsOf: url)
    }

    private func decode<T: Decodable>(_ type: T.Type, _ name: String) throws -> T {
        try JSONDecoder().decode(type, from: fixture(name))
    }

    @Test func systemStats() throws {
        let stats = try decode(SystemStats.self, "system")
        // Metrics come from /proc and are absent off-device; the models must
        // tolerate that rather than failing to decode.
        #expect(!stats.hostname.isEmpty)
    }

    @Test func diskFolders() throws {
        let payload = try decode(DiskFolders.self, "disk")
        let folder = try #require(payload.folders.first)
        #expect(folder.id > 0)
        #expect(!folder.label.isEmpty)
        #expect(folder.available)
        #expect(folder.totalBytes > 0)
        #expect((0...100).contains(folder.usedPercent))
    }

    @Test func diskDetail() throws {
        let detail = try decode(DiskDetail.self, "disk_detail")
        #expect(detail.folder.id > 0)
        #expect(!detail.scanning)
        #expect(detail.scannedAt != nil)
        #expect(!detail.top.isEmpty)
        #expect(detail.top.allSatisfy { $0.isDir })
    }

    @Test func diskBrowse() throws {
        let listing = try decode(DiskListing.self, "disk_browse")
        #expect(!listing.entries.isEmpty)
        #expect(listing.totalBytes > 0)
        let entry = try #require(listing.entries.first)
        #expect(!entry.path.isEmpty)
    }

    @Test func naming() throws {
        let report = try decode(NamingReport.self, "naming")
        #expect(report.count == report.issues.count)
        let issue = try #require(report.issues.first)
        #expect(issue.type == "movies")
        #expect(issue.typeLabel == "Peli")
        #expect(issue.found != issue.expected)
    }

    @Test func media() throws {
        let library = try decode(MediaLibrary.self, "media")
        #expect(library.configured)
        #expect(library.total == 2)
        #expect(library.items.count == 2)

        let movie = try #require(library.items.first { $0.type == "movie" })
        #expect(movie.title == "Dune: Parte Dos")
        #expect(movie.year == 2024)
        #expect(movie.hasNfo)
        #expect(!movie.hasSubsEs)
    }

    @Test func subtitles() throws {
        let report = try decode(SubtitlesReport.self, "subtitles")
        #expect(report.configured)
        #expect(report.missing == 1)
        #expect(!report.truncated)
        let item = try #require(report.items.first)
        #expect(item.title == "Dune: Parte Dos")
    }

    @Test func torrents() throws {
        let list = try decode(TorrentList.self, "torrents")
        #expect(!list.configured) // captured with qBittorrent unconfigured
        #expect(list.torrents.isEmpty)
    }

    /// Hand-written from the server's `apiTorrent` shape, because capturing a
    /// real one needs a live qBittorrent. Keep it in step with api.go.
    @Test func torrentRow() throws {
        let list = try decode(TorrentList.self, "torrents_populated")
        #expect(list.configured)
        #expect(list.active == 1)

        // Categories come from qBittorrent and drive the add-magnet picker.
        #expect(list.categories == ["Peliculas", "Series"])
        let filed = try #require(list.torrents.first { $0.hash == "a1" })
        #expect(filed.category == "Series")
        let uncategorised = try #require(list.torrents.first { $0.hash == "c3" })
        #expect(uncategorised.category.isEmpty)

        let downloading = try #require(list.torrents.first { !$0.paused })
        #expect(downloading.status == .downloading)
        #expect(downloading.status.label == "Descargando")
        #expect(downloading.progress > 0 && downloading.progress < 1)

        let paused = try #require(list.torrents.first { $0.paused })
        #expect(paused.status == .paused)

        let seeding = try #require(list.torrents.first { $0.state == "uploading" })
        #expect(seeding.status == .seeding)

        let failed = try #require(list.torrents.first { $0.state.contains("error") })
        #expect(failed.status == .failed)
    }

    @Test func plexLinkPending() throws {
        let status = try decode(PlexLinkStatus.self, "plex_link_pending")
        #expect(status.isPending)
        #expect(!status.isLinked)
        #expect(status.code == "QWER")
        #expect(status.authUrl?.contains("app.plex.tv") == true)
    }

    @Test func plexLinkLinked() throws {
        let status = try decode(PlexLinkStatus.self, "plex_link_linked")
        #expect(status.isLinked)
        let server = try #require(status.servers?.first)
        #expect(server.name == "Pi de casa")
        #expect(server.baseUrl == "http://192.168.1.20:32400")
    }
}

/// Anchors Bundle(for:) to the test bundle.
private final class BundleToken {}
