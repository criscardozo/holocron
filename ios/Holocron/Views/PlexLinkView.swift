import SwiftUI

/// Drives the plex.tv device-link flow from the phone: show a code, wait for
/// the user to authorise it at plex.tv, then let them pick a discovered server.
/// Saves the token on the server, so the web UI ends up configured too.
struct PlexLinkView: View {
    @Environment(AppSettings.self) private var settings
    @Environment(\.dismiss) private var dismiss
    @Environment(\.openURL) private var openURL

    @State private var status: PlexLinkStatus?
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
        .navigationTitle("Conectar con Plex")
        .navigationBarTitleDisplayMode(.inline)
        .task { await poll() }
    }

    // MARK: - Sections

    private var startSection: some View {
        Section {
            Text("Vinculá tu cuenta y Holocron obtiene el token solo, sin que tengas que buscarlo en el navegador.")
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
        }
        .listRowBackground(Noir.surface)
    }

    private func pendingSection(_ status: PlexLinkStatus) -> some View {
        Section {
            Text("1. Abrí plex.tv/link e iniciá sesión.")
                .font(.callout)
            if let auth = status.authUrl, let url = URL(string: auth) {
                Button("Abrir plex.tv") { openURL(url) }
            }
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
                Text("Esperando autorización…")
                    .font(.footnote)
                    .foregroundStyle(Noir.muted)
            }
        }
        .listRowBackground(Noir.surface)
    }

    private func linkedSection(_ status: PlexLinkStatus) -> some View {
        Group {
            Section {
                Label("Cuenta vinculada. El token quedó guardado.", systemImage: "checkmark.circle")
                    .font(.callout)
                    .foregroundStyle(Noir.ok)
            }
            .listRowBackground(Noir.surface)

            let servers = status.servers ?? []
            if servers.isEmpty {
                Section {
                    Text("No se detectaron servidores. Cargá la URL desde la web de Holocron.")
                        .font(.footnote)
                        .foregroundStyle(Noir.muted)
                    Button("Listo") { dismiss() }
                }
                .listRowBackground(Noir.surface)
            } else {
                Section("Elegí el servidor") {
                    ForEach(servers) { server in
                        Button {
                            Task { await select(server) }
                        } label: {
                            VStack(alignment: .leading, spacing: 3) {
                                Text(server.name).font(.callout)
                                Text(server.baseUrl)
                                    .font(.system(.caption2, design: .monospaced))
                                    .foregroundStyle(Noir.muted)
                            }
                        }
                        .disabled(working)
                    }
                }
                .listRowBackground(Noir.surface)
            }
        }
    }

    // MARK: - Actions

    @MainActor private func start() async {
        guard let client = settings.client else { return }
        working = true
        error = nil
        defer { working = false }
        do {
            status = try await client.startPlexLink()
            await poll()
        } catch {
            self.error = message(for: error)
        }
    }

    /// Polls the server while a code is outstanding. The task is cancelled when
    /// the view goes away, so this stops on its own.
    @MainActor private func poll() async {
        guard let client = settings.client else { return }
        while !Task.isCancelled {
            guard let current = try? await client.plexLinkStatus() else { return }
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

    @MainActor private func select(_ server: PlexServer) async {
        guard let client = settings.client else { return }
        working = true
        defer { working = false }
        do {
            try await client.selectPlexServer(baseURL: server.baseUrl)
            dismiss()
        } catch {
            self.error = message(for: error)
        }
    }
}
