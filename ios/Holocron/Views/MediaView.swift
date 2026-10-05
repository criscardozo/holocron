import SwiftUI

/// The Jellyfin inventory: counters, the sync job, and the library as posters.
struct MediaView: View {
    @Environment(AppSettings.self) private var settings

    @State private var state: Loadable<MediaLibrary> = .idle
    @State private var banner: String?

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
        .background(Palette.bg)
        .navigationTitle("Medios")
        .refreshable { await load() }
        .task { if case .idle = state { await load() } }
    }

    /// Posters, not a list: this is a film library, and a cover is found at a
    /// glance where a title has to be read.
    private func content(_ library: MediaLibrary) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                VStack(alignment: .leading, spacing: 10) {
                    stats(library)
                    actions(library)
                    if let banner {
                        Text(banner).font(.footnote).foregroundStyle(Palette.muted)
                    }
                }
                .card()

                if library.items.isEmpty {
                    Text("Sin inventario. Tocá «Sincronizar».")
                        .foregroundStyle(Palette.muted)
                } else {
                    LazyVGrid(columns: [GridItem(.adaptive(minimum: 100), spacing: 12)], alignment: .leading, spacing: 16) {
                        ForEach(library.items) { item in
                            posterCard(item)
                        }
                    }
                    if library.truncated == true {
                        Text("Mostrando \(library.items.count) de \(library.total ?? 0) ítems.")
                            .font(.footnote).foregroundStyle(Palette.muted)
                    }
                }
            }
            .padding(16)
        }
    }

    private func posterCard(_ item: MediaItem) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            PosterImage(path: item.art, title: item.title)
            Text(item.title).font(.caption.weight(.semibold)).lineLimit(1)
            Text("\(item.type == "movie" ? "Película" : "Serie") · \(item.year > 0 ? String(item.year) : "—")")
                .font(.caption2).foregroundStyle(Palette.muted).lineLimit(1)
            if !item.hasSubsEs {
                Text("sin subs ES").font(.system(size: 9, weight: .semibold)).foregroundStyle(Palette.danger)
            }
        }
        .accessibilityElement(children: .combine)
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
                .foregroundStyle(tinted ? Palette.accent : Palette.text)
            Text(caption).font(.caption2).foregroundStyle(Palette.muted)
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

            NavigationLink { QualityView() } label: {
                Label("Calidad", systemImage: "gauge.with.dots.needle.bottom.50percent")
            }
            .buttonStyle(.borderless)
        }
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
