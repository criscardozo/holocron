import SwiftUI

@main
struct HolocronApp: App {
    @State private var settings = AppSettings()

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(settings)
                .tint(Noir.accent)
                .preferredColorScheme(.dark)
        }
    }
}
