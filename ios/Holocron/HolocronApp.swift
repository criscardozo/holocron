import SwiftUI

@main
struct HolocronApp: App {
    @State private var settings = AppSettings()

    init() {
        // Posters are served with a week's Cache-Control; a disk cache this
        // size keeps a whole library's worth (36 KB each, measured on Ginebra)
        // so the grid and the mural load from the phone after the first look.
        URLCache.shared = URLCache(memoryCapacity: 24 << 20, diskCapacity: 160 << 20)
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(settings)
                .tint(Palette.accent)
                .preferredColorScheme(.dark)
        }
    }
}
