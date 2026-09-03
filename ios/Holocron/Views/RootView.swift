import SwiftUI

struct RootView: View {
    @Environment(AppSettings.self) private var settings
    @State private var tab: Tab = .dashboard

    enum Tab: Hashable { case dashboard, torrents, subtitles, media, settings }

    var body: some View {
        // The iOS 18 `Tab` builder would be tidier, but the classic API keeps
        // the deployment target at iOS 17 so older phones can run this.
        TabView(selection: $tab) {
            NavigationStack { DashboardView() }
                .tabItem { Label("Estado", systemImage: "waveform.path.ecg") }
                .tag(Tab.dashboard)
            NavigationStack { TorrentsView() }
                .tabItem { Label("Torrents", systemImage: "arrow.down.circle") }
                .tag(Tab.torrents)
            NavigationStack { SubtitlesView() }
                .tabItem { Label("Subtítulos", systemImage: "captions.bubble") }
                .tag(Tab.subtitles)
            NavigationStack { MediaView() }
                .tabItem { Label("Medios", systemImage: "film") }
                .tag(Tab.media)
            NavigationStack { SettingsView() }
                .tabItem { Label("Ajustes", systemImage: "gearshape") }
                .tag(Tab.settings)
        }
        .onAppear {
            // First run, or credentials that no longer load: open where the
            // setup is instead of on a tab that can only report an error.
            //
            // This used to be a full-screen overlay that said "go to the
            // Ajustes tab" while covering the tab bar, so there was no way to
            // reach it — the app looked stuck on its first screen. Selecting
            // the tab does the same job and leaves every other one reachable,
            // and each of them already explains itself when unconfigured.
            if !settings.isConfigured {
                tab = .settings
            }
        }
    }
}
