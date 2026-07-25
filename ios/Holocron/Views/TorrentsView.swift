import SwiftUI

/// Torrent management: the screen most worth having on a phone, since magnet
/// links usually arrive there. Refreshes itself while it is on screen.
struct TorrentsView: View {
    @Environment(AppSettings.self) private var settings

    @State private var state: Loadable<TorrentList> = .idle
    @State private var magnet = ""
    @State private var banner: String?
    @State private var working = false

    var body: some View {
        LoadableView(state: state, reload: load) { list in
            if !list.configured {
                NotConfiguredState(service: "qBittorrent")
            } else {
                content(list)
            }
        }
        .background(Noir.bg)
        .navigationTitle("Torrents")
        .refreshable { await load() }
        .task { await pollWhileVisible() }
    }

    private func content(_ list: TorrentList) -> some View {
        List {
            Section {
                addMagnetRow
            }
            .listRowBackground(Noir.surface)

            if let banner {
                Section {
                    Text(banner).font(.footnote).foregroundStyle(Noir.muted)
                }
                .listRowBackground(Noir.surface)
            }

            Section {
                summary(list)
            }
            .listRowBackground(Noir.surface)

            if list.torrents.isEmpty {
                Section {
                    Text("No hay torrents.").foregroundStyle(Noir.muted)
                }
                .listRowBackground(Noir.surface)
            } else {
                Section("Descargas") {
                    ForEach(list.torrents) { torrent in
                        row(torrent)
                    }
                }
                .listRowBackground(Noir.surface)
            }
        }
        .scrollContentBackground(.hidden)
    }

    private var addMagnetRow: some View {
        HStack(spacing: 10) {
            Image(systemName: "plus.circle").foregroundStyle(Noir.accent)
            TextField("Pegá un magnet: acá…", text: $magnet)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .font(.callout)
            Button("Agregar") { Task { await addMagnet() } }
                .buttonStyle(.borderless)
                .disabled(magnet.isEmpty || working)
        }
    }

    private func summary(_ list: TorrentList) -> some View {
        HStack(spacing: 20) {
            VStack(alignment: .leading) {
                Text("\(list.active ?? 0) / \(list.total ?? 0)")
                    .font(.title3.weight(.semibold)).monospacedDigit()
                Text("activos").font(.caption2).foregroundStyle(Noir.muted)
            }
            VStack(alignment: .leading) {
                Label(Format.speed(list.dlSpeed ?? 0), systemImage: "arrow.down")
                    .font(.subheadline).monospacedDigit().foregroundStyle(Noir.ok)
                Label(Format.speed(list.upSpeed ?? 0), systemImage: "arrow.up")
                    .font(.subheadline).monospacedDigit().foregroundStyle(Noir.accent300)
            }
            Spacer()
        }
    }

    private func row(_ torrent: Torrent) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(torrent.name)
                .font(.system(.footnote, design: .monospaced))
                .lineLimit(2)

            HStack(spacing: 8) {
                Pill(text: torrent.status.label, kind: pillKind(torrent.status))
                Spacer()
                Text(Format.bytes(torrent.sizeBytes))
                    .font(.caption2).monospacedDigit().foregroundStyle(Noir.muted)
            }

            ProgressBar(value: torrent.progress)

            HStack {
                Text("\(Int(torrent.progress * 100))%")
                    .font(.caption2).monospacedDigit().foregroundStyle(Noir.muted)
                Spacer()
                Text("↓ \(Format.speed(torrent.dlSpeed))  ↑ \(Format.speed(torrent.upSpeed))")
                    .font(.caption2).monospacedDigit().foregroundStyle(Noir.muted)
            }
        }
        .padding(.vertical, 4)
        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
            Button(role: .destructive) {
                Task { await act(torrent, "delete") }
            } label: {
                Label("Borrar", systemImage: "trash")
            }
            Button {
                Task { await act(torrent, torrent.paused ? "resume" : "pause") }
            } label: {
                Label(torrent.paused ? "Reanudar" : "Pausar",
                      systemImage: torrent.paused ? "play.fill" : "pause.fill")
            }
            .tint(Noir.accent)
        }
    }

    private func pillKind(_ status: Torrent.Status) -> Pill.Kind {
        switch status {
        case .downloading: .warn
        case .seeding: .yes
        case .paused: .neutral
        case .failed: .no
        }
    }

    // MARK: - Actions

    @MainActor private func load() async {
        guard let client = settings.client else {
            state = .failed(APIError.notConfigured.localizedDescription)
            return
        }
        if case .idle = state { state = .loading }
        do {
            state = .loaded(try await client.torrents())
        } catch {
            state = .failed(message(for: error))
        }
    }

    /// Keeps the list fresh while the tab is visible, mirroring the web UI's
    /// 3-second refresh. The task is cancelled when the view goes away.
    @MainActor private func pollWhileVisible() async {
        await load()
        while !Task.isCancelled {
            try? await Task.sleep(for: .seconds(3))
            if Task.isCancelled { break }
            guard let client = settings.client else { return }
            if let fresh = try? await client.torrents() {
                state = .loaded(fresh)
            }
        }
    }

    @MainActor private func addMagnet() async {
        guard let client = settings.client else { return }
        working = true
        defer { working = false }
        do {
            try await client.addMagnet(magnet)
            magnet = ""
            banner = "Magnet agregado."
            await load()
        } catch {
            banner = message(for: error)
        }
    }

    @MainActor private func act(_ torrent: Torrent, _ action: String) async {
        guard let client = settings.client else { return }
        do {
            try await client.torrentAction(hash: torrent.hash, action: action)
            await load()
        } catch {
            banner = message(for: error)
        }
    }
}
