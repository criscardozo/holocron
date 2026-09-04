import SwiftUI

/// Drives Jellyfin's Quick Connect from the phone: ask for a code, show it, and
/// wait for it to be approved in Jellyfin. The token is stored on the server, so
/// the web UI ends up configured too.
///
/// Simpler than the Plex flow it replaces, and deliberately so: Jellyfin has no
/// cloud service to discover servers through, so the address is set in the web
/// UI first and there is no server to pick afterwards.
struct JellyfinLinkView: View {
    @Environment(AppSettings.self) private var settings
    @Environment(\.dismiss) private var dismiss

    @State private var status: JellyfinLinkStatus?
    @State private var error: String?
    @State private var working = false

    var body: some View {
        List {
            if let error {
                Section {
                    Label(error, systemImage: "exclamationmark.triangle")
                        .font(.footnote)
                        .foregroundStyle(Noir.danger)
                }
                .listRowBackground(Noir.surface)
            }

            if let status, status.isPending {
                pendingSection(status)
            } else if let status, status.isLinked {
                linkedSection(status)
            } else {
                startSection
            }
        }
        .scrollContentBackground(.hidden)
        .background(Noir.bg)
        .navigationTitle("Conectar con Jellyfin")
        .navigationBarTitleDisplayMode(.inline)
        .task { await poll() }
    }

    // MARK: - Sections

    private var startSection: some View {
        Section {
            Text("Jellyfin te da un código, lo aprobás desde tu perfil y Holocron guarda el token solo.")
                .font(.callout)
                .foregroundStyle(Noir.muted)
            Button {
                Task { await start() }
            } label: {
                HStack {
                    if working { ProgressView().controlSize(.small) }
                    Text("Obtener código")
                }
            }
            .disabled(working)
        } footer: {
            Text("La dirección del servidor se carga antes, en la web de Holocron: Jellyfin no tiene un servicio en la nube por el que descubrirlo.")
        }
        .listRowBackground(Noir.surface)
    }

    private func pendingSection(_ status: JellyfinLinkStatus) -> some View {
        Section {
            Text("1. Abrí Jellyfin y entrá a tu perfil → Quick Connect.")
                .font(.callout)
            Text("2. Ingresá este código:")
                .font(.callout)
            Text(status.code ?? "")
                .font(.system(size: 34, weight: .bold, design: .monospaced))
                .kerning(8)
                .foregroundStyle(Noir.accent)
                .frame(maxWidth: .infinity)
                .padding(.vertical, 12)
                .background(Noir.surface2, in: RoundedRectangle(cornerRadius: 10))
                .textSelection(.enabled)
            HStack(spacing: 8) {
                ProgressView().controlSize(.small)
                Text("Esperando que lo autorices…")
                    .font(.footnote)
                    .foregroundStyle(Noir.muted)
            }
        }
        .listRowBackground(Noir.surface)
    }

    private func linkedSection(_ status: JellyfinLinkStatus) -> some View {
        Section {
            Label(linkedMessage(status), systemImage: "checkmark.circle")
                .font(.callout)
                .foregroundStyle(Noir.ok)
            // Said here rather than after a 403 later: the metadata refresh in
            // the quality panel needs an administrator.
            if status.admin == false {
                Text("Esa cuenta no es administradora, así que no se le puede pedir a Jellyfin que vuelva a leer metadata.")
                    .font(.footnote)
                    .foregroundStyle(Noir.muted)
            }
            Button("Listo") { dismiss() }
        }
        .listRowBackground(Noir.surface)
    }

    private func linkedMessage(_ status: JellyfinLinkStatus) -> String {
        guard let user = status.user, !user.isEmpty else {
            return "Vinculado. El token quedó guardado."
        }
        return "Vinculado como \(user). El token quedó guardado."
    }

    // MARK: - Actions

    @MainActor private func start() async {
        guard let client = settings.client else { return }
        working = true
        error = nil
        defer { working = false }
        do {
            status = try await client.startJellyfinLink()
            await poll()
        } catch {
            // The server explains the actionable cases in its own words — no
            // address loaded yet, Quick Connect switched off — and APIError
            // carries that message through.
            self.error = error.localizedDescription
        }
    }

    /// Polls while a code is outstanding. The task is cancelled when the view
    /// goes away, so this stops on its own.
    @MainActor private func poll() async {
        guard let client = settings.client else { return }
        while !Task.isCancelled {
            guard let current = try? await client.jellyfinLinkStatus() else { return }
            status = current
            if current.isLinked { return }
            if current.isExpired {
                error = "El código venció. Pedí uno nuevo."
                status = nil
                return
            }
            if !current.isPending { return }
            try? await Task.sleep(for: .seconds(2))
        }
    }
}
