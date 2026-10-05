import SwiftUI

/// The start screen, laid out like Ginebra's portal and the web's start page:
/// the name over the poster wall, a row of readings, one tile per area that
/// is also the way in, and what arrived lately as posters. It is the web's
/// own view over the API, so the two say the same thing.
struct DashboardView: View {
    @Environment(AppSettings.self) private var settings
    @State private var state: Loadable<HomeReading> = .idle

    private let columns = [GridItem(.flexible(), spacing: 12), GridItem(.flexible(), spacing: 12)]

    var body: some View {
        ScrollView {
            LoadableView(state: state, reload: load) { home in
                VStack(spacing: 18) {
                    hero(home)
                    if !home.attention.isEmpty { attention(home.attention) }
                    LazyVGrid(columns: columns, spacing: 12) {
                        ForEach(home.tiles) { tile in
                            tileLink(tile)
                        }
                    }
                    if !home.recent.isEmpty {
                        VStack(alignment: .leading, spacing: 10) {
                            Text("Agregado hace poco").font(.headline)
                            PosterRow(items: home.recent)
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
                .padding(16)
            }
        }
        .background {
            ZStack {
                Palette.bg.ignoresSafeArea()
                if let mural = state.value?.mural, !mural.isEmpty {
                    MuralBackground(paths: mural)
                }
            }
        }
        .navigationTitle("Inicio")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                NavigationLink { ManagementView() } label: {
                    Label("Gestión", systemImage: "slider.horizontal.3")
                }
            }
        }
        .refreshable { await load() }
        .task { await load() }
    }

    // MARK: - Sections

    private func hero(_ home: HomeReading) -> some View {
        VStack(spacing: 6) {
            Text("Holocron")
                .font(.system(size: 38, weight: .heavy))
                .kerning(-1)
            Text("El panel de \(home.machine)")
                .font(.callout)
                .foregroundStyle(Palette.muted)
            // Centred when they fit, scrolling sideways when they do not.
            ViewThatFits(in: .horizontal) {
                statusRow(home.status)
                ScrollView(.horizontal, showsIndicators: false) {
                    statusRow(home.status).padding(.horizontal, 2)
                }
            }
            .padding(.top, 8)
        }
        .frame(maxWidth: .infinity)
        .padding(.top, 8)
    }

    private func statusRow(_ status: [HomeStat]) -> some View {
        HStack(spacing: 8) {
            ForEach(status) { st in statusChip(st) }
        }
    }

    private func statusChip(_ st: HomeStat) -> some View {
        HStack(spacing: 6) {
            if st.tone == "ok" {
                Circle().fill(Palette.ok).frame(width: 7, height: 7)
            }
            Text(st.text)
                .font(.caption.monospacedDigit())
                .foregroundStyle(st.tone == "danger" ? Palette.danger : st.tone == "warn" ? Palette.warn : Palette.text)
        }
        .padding(.horizontal, 11)
        .padding(.vertical, 6)
        .background(.ultraThinMaterial, in: Capsule())
        .overlay(Capsule().strokeBorder(Palette.divider))
    }

    private func attention(_ items: [HomeAttention]) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            ForEach(items) { a in
                Label(a.label, systemImage: "exclamationmark.triangle.fill")
                    .font(.callout)
                    .foregroundStyle(Palette.accent200)
                    .padding(.horizontal, 12)
                    .padding(.vertical, 8)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Palette.accent900, in: Capsule())
            }
        }
    }

    @ViewBuilder private func tileLink(_ tile: HomeTile) -> some View {
        if let destination = Self.destination(for: tile.href) {
            NavigationLink { destination } label: { tileCard(tile) }
                .buttonStyle(.plain)
        } else {
            tileCard(tile)
        }
    }

    private func tileCard(_ tile: HomeTile) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Image(systemName: Self.symbol(for: tile.href))
                .font(.system(size: 17, weight: .semibold))
                .foregroundStyle(.white)
                .frame(width: 36, height: 36)
                .background(Self.tone(tile.tone), in: RoundedRectangle(cornerRadius: 10))
                .padding(.bottom, 6)
            Text(tile.title).font(.subheadline.weight(.semibold))
            Text(tile.value)
                .font(.title3.weight(.bold))
                .monospacedDigit()
                .foregroundStyle(tile.warn ? Palette.accent300 : Palette.text)
                .lineLimit(1)
                .minimumScaleFactor(0.7)
            Text(tile.sub)
                .font(.caption2)
                .foregroundStyle(Palette.muted)
                .lineLimit(2)
                .frame(maxWidth: .infinity, minHeight: 28, alignment: .topLeading)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.ultraThinMaterial, in: RoundedRectangle(cornerRadius: 18))
        .overlay {
            RoundedRectangle(cornerRadius: 18)
                .strokeBorder(tile.warn ? Palette.accent300.opacity(0.5) : Palette.divider)
        }
        .opacity(tile.off ? 0.55 : 1)
    }

    // MARK: - Mapping the web's areas onto the app

    private static func destination(for href: String) -> AnyView? {
        switch href {
        case "/activity": AnyView(ActivityView())
        case "/hardware": AnyView(HardwareView())
        case "/services": AnyView(ServicesView())
        case "/disk": AnyView(DiskFoldersView())
        case "/media": AnyView(MediaView())
        case "/quality": AnyView(QualityView())
        case "/torrents": AnyView(TorrentsView())
        default: nil // Nombres lives on the web only
        }
    }

    static func symbol(for href: String) -> String {
        switch href {
        case "/activity": "waveform.path.ecg"
        case "/hardware": "cpu"
        case "/services": "server.rack"
        case "/disk": "internaldrive"
        case "/media": "film"
        case "/quality": "gauge.with.dots.needle.bottom.50percent"
        case "/naming": "tag"
        case "/torrents": "arrow.down.circle"
        default: "square.grid.2x2"
        }
    }

    /// The tile colours, from the G▶'s facets and the state lights, as in
    /// the web's `tone-*` classes.
    static func tone(_ name: String) -> LinearGradient {
        let pair: (Color, Color) = switch name {
        case "pink": (Palette.pink, Palette.accent)
        case "violet": (Palette.accent, Palette.accent400)
        case "teal": (Palette.ok, Color(hex: 0x0EA5E9))
        case "lilac": (Palette.accent300, Palette.pink)
        case "indigo": (Palette.accent400, Color(hex: 0x4F46E5))
        case "amber": (Palette.warn, Color(hex: 0xF97316))
        case "sky": (Color(hex: 0x0EA5E9), Palette.accent)
        case "mint": (Palette.ok, Palette.accent)
        default: (Palette.accent, Palette.accent400)
        }
        return LinearGradient(colors: [pair.0, pair.1], startPoint: .topLeading, endPoint: .bottomTrailing)
    }

    // MARK: - Loading

    @MainActor private func load() async {
        guard let client = settings.client else {
            state = .failed(APIError.notConfigured.localizedDescription)
            return
        }
        if case .idle = state { state = .loading }
        do {
            state = .loaded(try await client.home())
        } catch {
            if state.value == nil { state = .failed(message(for: error)) }
        }
    }
}

/// The watched disks, each leading to its detail. What the old start screen's
/// disk card was, now one tap behind the Disco tile.
struct DiskFoldersView: View {
    @Environment(AppSettings.self) private var settings
    @State private var state: Loadable<[DiskFolder]> = .idle

    var body: some View {
        ScrollView {
            LoadableView(state: state, reload: load) { disks in
                VStack(spacing: 12) {
                    if disks.isEmpty {
                        AllGoodState(message: "No hay carpetas de disco configuradas")
                    }
                    ForEach(disks) { disk in
                        NavigationLink { DiskDetailView(folder: disk) } label: { row(disk) }
                            .buttonStyle(.plain)
                    }
                }
                .padding(16)
            }
        }
        .background(Palette.bg)
        .navigationTitle("Disco")
        .refreshable { await load() }
        .task { if case .idle = state { await load() } }
    }

    private func row(_ disk: DiskFolder) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(disk.label).font(.callout.weight(.semibold))
                Spacer()
                if disk.available {
                    Text("\(disk.usedPercent) %")
                        .font(.callout.weight(.bold)).monospacedDigit()
                        .foregroundStyle(disk.isHot ? Palette.accent300 : Palette.text)
                } else {
                    Text("sin leer").font(.caption).foregroundStyle(Palette.danger)
                }
            }
            if disk.available {
                ProgressBar(value: Double(disk.usedPercent) / 100, hot: disk.isHot)
                Text("\(Format.bytes(disk.usedBytes)) de \(Format.bytes(disk.totalBytes))")
                    .font(.caption2).foregroundStyle(Palette.muted)
            }
        }
        .card()
    }

    @MainActor private func load() async {
        guard let client = settings.client else {
            state = .failed(APIError.notConfigured.localizedDescription)
            return
        }
        if case .idle = state { state = .loading }
        do {
            state = .loaded(try await client.diskFolders())
        } catch {
            state = .failed(message(for: error))
        }
    }
}

/// Maps any thrown error to something worth showing a person.
@MainActor
func message(for error: Error) -> String {
    (error as? APIError)?.localizedDescription ?? error.localizedDescription
}
