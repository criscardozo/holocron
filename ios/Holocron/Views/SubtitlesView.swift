import SwiftUI

/// Media missing a Spanish subtitle, with search and download per item.
struct SubtitlesView: View {
    @Environment(AppSettings.self) private var settings

    @State private var state: Loadable<SubtitlesReport> = .idle

    var body: some View {
        LoadableView(state: state, reload: load) { report in
            if !report.configured {
                NotConfiguredState(service: "OpenSubtitles")
            } else if report.items.isEmpty {
                AllGoodState(message: "Todo tiene subtítulos en español")
            } else {
                List {
                    Section {
                        ForEach(report.items) { item in
                            NavigationLink {
                                SubtitleSearchView(item: item)
                            } label: {
                                VStack(alignment: .leading, spacing: 3) {
                                    Text(item.title).font(.callout)
                                    Text("\(Format.year(item.year)) · \(item.type)")
                                        .font(.caption2).foregroundStyle(Noir.muted)
                                }
                            }
                        }
                    } header: {
                        Text("\(report.missing) pendientes")
                    } footer: {
                        if report.truncated {
                            Text("Mostrando \(report.items.count) de \(report.missing).")
                        }
                    }
                    .listRowBackground(Noir.surface)
                }
                .scrollContentBackground(.hidden)
            }
        }
        .background(Noir.bg)
        .navigationTitle("Subtítulos")
        .refreshable { await load() }
        .task { if case .idle = state { await load() } }
    }

    @MainActor private func load() async {
        guard let client = settings.client else {
            state = .failed(APIError.notConfigured.localizedDescription)
            return
        }
        if case .idle = state { state = .loading }
        do {
            state = .loaded(try await client.subtitles())
        } catch {
            state = .failed(message(for: error))
        }
    }
}

/// Search results for one media item, with a download action per release.
struct SubtitleSearchView: View {
    @Environment(AppSettings.self) private var settings
    let item: SubtitleMissing

    @State private var state: Loadable<[SubtitleResult]> = .idle
    @State private var downloading: String?
    @State private var banner: String?

    var body: some View {
        LoadableView(state: state, reload: search) { results in
            List {
                if let banner {
                    Section {
                        Text(banner).font(.footnote)
                    }
                    .listRowBackground(Noir.surface)
                }
                if results.isEmpty {
                    Section {
                        Text("Sin resultados.").foregroundStyle(Noir.muted)
                    }
                    .listRowBackground(Noir.surface)
                } else {
                    Section("Resultados en español") {
                        ForEach(results) { result in
                            row(result)
                        }
                    }
                    .listRowBackground(Noir.surface)
                }
            }
            .scrollContentBackground(.hidden)
        }
        .background(Noir.bg)
        .navigationTitle(item.title)
        .navigationBarTitleDisplayMode(.inline)
        .task { if case .idle = state { await search() } }
    }

    private func row(_ result: SubtitleResult) -> some View {
        HStack {
            VStack(alignment: .leading, spacing: 3) {
                Text(result.fileName)
                    .font(.system(.footnote, design: .monospaced))
                    .lineLimit(2)
                if !result.release.isEmpty {
                    Text(result.release).font(.caption2).foregroundStyle(Noir.muted)
                }
            }
            Spacer()
            if downloading == result.fileId {
                ProgressView().controlSize(.small)
            } else {
                Button {
                    Task { await download(result) }
                } label: {
                    Image(systemName: "arrow.down.circle")
                }
                .buttonStyle(.borderless)
            }
        }
    }

    @MainActor private func search() async {
        guard let client = settings.client else {
            state = .failed(APIError.notConfigured.localizedDescription)
            return
        }
        if case .idle = state { state = .loading }
        do {
            state = .loaded(try await client.searchSubtitles(title: item.title, year: item.year))
        } catch {
            state = .failed(message(for: error))
        }
    }

    @MainActor private func download(_ result: SubtitleResult) async {
        guard let client = settings.client else { return }
        downloading = result.fileId
        defer { downloading = nil }
        do {
            try await client.downloadSubtitle(fileID: result.fileId, path: item.path)
            banner = "Descargado."
        } catch {
            banner = message(for: error)
        }
    }
}
