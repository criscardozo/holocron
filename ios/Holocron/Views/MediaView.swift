import SwiftUI

/// The Jellyfin inventory: counters, the sync job, and the item list.
struct MediaView: View {
    @Environment(AppSettings.self) private var settings

    @State private var state: Loadable<MediaLibrary> = .idle
    @State private var banner: String?
    @State private var disks: [DiskFolder] = []

    var body: some View {
        LoadableView(state: state, reload: load) { library in
            if !library.configured {
                // Jellyfin is the one service the app can configure by itself,
                // through Quick Connect.
                ContentUnavailableView {
                    Label("Jellyfin no vinculado", systemImage: "gearshape")
                } description: {
                    Text("Aprobá un código en Jellyfin y Holocron guarda el token solo.")
                } actions: {
                    NavigationLink("Conectar con Jellyfin") { JellyfinLinkView() }
                        .buttonStyle(.borderedProminent)
                }
            } else {
                content(library)
            }
        }
        .background(Noir.bg)
        .navigationTitle("Medios")
        .refreshable { await load() }
        .task { if case .idle = state { await load() } }
    }

    private func content(_ library: MediaLibrary) -> some View {
        List {
            Section {
                stats(library)
                actions(library)
                if let banner {
                    Text(banner).font(.footnote).foregroundStyle(Noir.muted)
                }
            }
            .listRowBackground(Noir.surface)

            if !disks.isEmpty {
                Section("Disco") {
                    ForEach(disks) { disk in
                        NavigationLink {
                            DiskDetailView(folder: disk)
                        } label: {
                            HStack {
                                Text(disk.label).font(.callout)
                                Spacer()
                                if disk.available {
                                    Text("\(disk.usedPercent)%")
                                        .font(.callout).monospacedDigit()
                                        .foregroundStyle(disk.isHot ? Noir.accent300 : Noir.muted)
                                }
                            }
                        }
                    }
                }
                .listRowBackground(Noir.surface)
            }

            if library.items.isEmpty {
                Section {
                    Text("Sin inventario. Tocá «Sincronizar».")
                        .foregroundStyle(Noir.muted)
                }
                .listRowBackground(Noir.surface)
            } else {
                Section {
                    ForEach(library.items) { item in
                        itemRow(item)
                    }
                } header: {
                    Text("Inventario")
                } footer: {
                    if library.truncated == true {
                        Text("Mostrando \(library.items.count) de \(library.total ?? 0) ítems.")
                    }
                }
                .listRowBackground(Noir.surface)
            }
        }
        .scrollContentBackground(.hidden)
    }

    private func stats(_ library: MediaLibrary) -> some View {
        HStack(spacing: 24) {
            stat("\(library.total ?? 0)", "ítems", tinted: false)
            stat("\(library.movies ?? 0)", "películas", tinted: false)
            stat("\(library.withoutSubsEs ?? 0)", "sin subs ES", tinted: (library.withoutSubsEs ?? 0) > 0)
            Spacer()
        }
        .padding(.vertical, 4)
    }

    private func stat(_ value: String, _ caption: String, tinted: Bool) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(value)
                .font(.title2.weight(.bold)).monospacedDigit()
                .foregroundStyle(tinted ? Noir.accent : Noir.text)
            Text(caption).font(.caption2).foregroundStyle(Noir.muted)
        }
    }

    private func actions(_ library: MediaLibrary) -> some View {
        HStack(spacing: 12) {
            Button {
                Task { await run { try await $0.syncMedia() } }
            } label: {
                if library.syncing == true {
                    HStack(spacing: 6) { ProgressView().controlSize(.small); Text("Sincronizando…") }
                } else {
                    Label("Sincronizar", systemImage: "arrow.triangle.2.circlepath")
                }
            }
            .buttonStyle(.borderless)
            .disabled(library.syncing == true)
        }
    }

    private func itemRow(_ item: MediaItem) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            Text(item.title).font(.callout)
            Text(item.path)
                .font(.system(.caption2, design: .monospaced))
                .foregroundStyle(Noir.muted)
                .lineLimit(1)
            HStack(spacing: 6) {
                Pill(text: item.type == "movie" ? "Peli" : "Serie", kind: .neutral)
                Pill(text: item.hasSubsEs ? "subs ES" : "sin subs", kind: item.hasSubsEs ? .yes : .no)
            }
        }
        .padding(.vertical, 2)
    }

    // MARK: - Loading

    @MainActor private func load() async {
        guard let client = settings.client else {
            state = .failed(APIError.notConfigured.localizedDescription)
            return
        }
        if case .idle = state { state = .loading }
        do {
            state = .loaded(try await client.media())
            disks = (try? await client.diskFolders()) ?? []
        } catch {
            state = .failed(message(for: error))
        }
    }

    /// Kicks off a job and refreshes, so the buttons reflect the new state.
    @MainActor private func run(_ action: (APIClient) async throws -> Void) async {
        guard let client = settings.client else { return }
        do {
            try await action(client)
            banner = "Trabajo iniciado."
            await load()
        } catch {
            banner = message(for: error)
        }
    }
}
