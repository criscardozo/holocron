import SwiftUI

/// The state of one screen's data. Every screen loads the same way, so the
/// loading, empty, error and unconfigured cases are handled in one place
/// instead of being reinvented per view.
enum Loadable<Value> {
    case idle
    case loading
    case loaded(Value)
    case failed(String)

    var value: Value? {
        if case let .loaded(v) = self { return v }
        return nil
    }
}

/// Renders a Loadable, showing a spinner, an error with a retry button, or the
/// content. `reload` is invoked by pull-to-refresh and by the retry button.
struct LoadableView<Value, Content: View>: View {
    let state: Loadable<Value>
    let reload: @MainActor () async -> Void
    @ViewBuilder let content: (Value) -> Content

    var body: some View {
        switch state {
        case .idle, .loading:
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        case let .failed(message):
            ErrorState(message: message, retry: reload)
        case let .loaded(value):
            content(value)
        }
    }
}

struct ErrorState: View {
    let message: String
    let retry: @MainActor () async -> Void

    var body: some View {
        VStack(spacing: 14) {
            Image(systemName: "exclamationmark.triangle")
                .font(.largeTitle)
                .foregroundStyle(Noir.accent300)
            Text(message)
                .font(.callout)
                .multilineTextAlignment(.center)
                .foregroundStyle(Noir.muted)
            Button("Reintentar") { Task { await retry() } }
                .buttonStyle(.bordered)
        }
        .padding(32)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

/// Shown when a feature needs credentials that are not set on the server.
struct NotConfiguredState: View {
    let service: String

    var body: some View {
        ContentUnavailableView {
            Label("\(service) no configurado", systemImage: "gearshape")
        } description: {
            Text("Configuralo desde la interfaz web de Holocron, en Ajustes.")
        }
    }
}

/// The empty-but-healthy case, e.g. nothing left to do.
struct AllGoodState: View {
    let message: String

    var body: some View {
        ContentUnavailableView {
            Label(message, systemImage: "checkmark.circle")
        } description: {
            EmptyView()
        }
    }
}
