import SwiftUI

struct RootView: View {
    @Environment(AppSettings.self) private var settings

    var body: some View {
        // The iOS 18 `Tab` builder would be tidier, but the classic API keeps
        // the deployment target at iOS 17 so older phones can run this.
        TabView {
            NavigationStack { DashboardView() }
                .tabItem { Label("Estado", systemImage: "waveform.path.ecg") }
            NavigationStack { TorrentsView() }
                .tabItem { Label("Torrents", systemImage: "arrow.down.circle") }
            NavigationStack { SubtitlesView() }
                .tabItem { Label("Subtítulos", systemImage: "captions.bubble") }
            NavigationStack { MediaView() }
                .tabItem { Label("Medios", systemImage: "film") }
            NavigationStack { SettingsView() }
                .tabItem { Label("Ajustes", systemImage: "gearshape") }
        }
        .overlay {
            if !settings.isConfigured {
                WelcomeOverlay()
            }
        }
    }
}

/// First-run state: without a server address and token there is nothing to
/// show, so the app points at Ajustes instead of failing on every tab.
private struct WelcomeOverlay: View {
    var body: some View {
        ZStack {
            Noir.bg.ignoresSafeArea()
            VStack(spacing: 16) {
                Image(systemName: "diamond")
                    .font(.system(size: 48))
                    .foregroundStyle(Noir.accent)
                Text("Holocron")
                    .font(.title.weight(.semibold))
                Text("Cargá la dirección de tu Raspberry Pi y el token de la API en la pestaña **Ajustes** para empezar.")
                    .font(.callout)
                    .multilineTextAlignment(.center)
                    .foregroundStyle(Noir.muted)
                    .padding(.horizontal, 32)
            }
        }
    }
}
