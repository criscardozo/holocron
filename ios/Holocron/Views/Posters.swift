import SwiftUI

// Posters come from the server's /art/ route, the same the web uses. The
// server fetched each one from Jellyfin or TMDb once and serves it from disk;
// the app never talks to either, and needs no token for them — the route is
// as open as the web pages it feeds.

extension APIClient {
    /// The absolute URL of a poster path the API sent ("/art/jf/…").
    nonisolated func artURL(_ path: String?) -> URL? {
        guard let path, !path.isEmpty else { return nil }
        return URL(string: path, relativeTo: baseURL)?.absoluteURL
    }
}

/// One poster, in the 2:3 of a film poster. With no image — none sent, or not
/// loaded yet — the title's initial over the placeholder gradient, as on the
/// web.
struct PosterImage: View {
    @Environment(AppSettings.self) private var settings
    let path: String?
    let title: String
    var corner: CGFloat = 10

    var body: some View {
        Color.clear
            .aspectRatio(2.0 / 3.0, contentMode: .fit)
            .overlay {
                if let url = settings.client?.artURL(path) {
                    AsyncImage(url: url) { phase in
                        if let image = phase.image {
                            image.resizable().scaledToFill()
                        } else {
                            placeholder
                        }
                    }
                } else {
                    placeholder
                }
            }
            .clipShape(RoundedRectangle(cornerRadius: corner))
    }

    private var placeholder: some View {
        ZStack {
            LinearGradient(colors: [Palette.surface2, Palette.accent900],
                           startPoint: .topLeading, endPoint: .bottomTrailing)
            Text(String(title.trimmingCharacters(in: .whitespaces).prefix(1)).uppercased())
                .font(.title2.weight(.bold))
                .foregroundStyle(Palette.text.opacity(0.3))
        }
    }
}

/// A row of posters that scrolls sideways, with a caption under each.
struct PosterRow: View {
    let items: [ActRecent]

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            LazyHStack(alignment: .top, spacing: 12) {
                ForEach(items) { r in
                    VStack(alignment: .leading, spacing: 3) {
                        PosterImage(path: r.art, title: r.title)
                        Text(r.title).font(.caption.weight(.semibold)).lineLimit(1)
                        Text(r.subtitle.isEmpty ? r.when : "\(r.subtitle) · \(r.when)")
                            .font(.caption2).foregroundStyle(Palette.muted).lineLimit(2)
                    }
                    .frame(width: 112)
                }
            }
        }
    }
}

/// The poster wall behind the start screen, the portal's mural: the library's
/// own posters, still and dimmed under a veil that is darkest where the
/// content is.
struct MuralBackground: View {
    let paths: [String]

    var body: some View {
        GeometryReader { geo in
            let columns = max(3, Int(geo.size.width / 96))
            let rows = Int(geo.size.height / 140) + 2
            let count = paths.isEmpty ? 0 : columns * rows
            LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: 4), count: columns), spacing: 4) {
                ForEach(0..<count, id: \.self) { i in
                    PosterImage(path: paths[i % paths.count], title: "", corner: 4)
                }
            }
            .padding(4)
        }
        .opacity(0.32)
        .saturation(0.85)
        .overlay {
            LinearGradient(colors: [Palette.bg.opacity(0.85), Palette.bg.opacity(0.55), Palette.bg.opacity(0.95)],
                           startPoint: .top, endPoint: .bottom)
        }
        .ignoresSafeArea()
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }
}
