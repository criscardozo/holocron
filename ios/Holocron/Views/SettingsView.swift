import SwiftUI

/// Where the server address, API token and (when the server is published
/// through Cloudflare Access) the service token are entered, plus a connection
/// test so setup problems surface here instead of as failures on every tab.
struct SettingsView: View {
    @Environment(AppSettings.self) private var settings

    @State private var testResult: String?
    @State private var testOK = false
    @State private var testing = false

    var body: some View {
        @Bindable var settings = settings

        Form {
            if !settings.isConfigured {
                // The welcome text lives here, in the screen that can act on
                // it, rather than in an overlay over the whole app.
                Section {
                    VStack(alignment: .leading, spacing: 6) {
                        Text("Falta configurar el servidor")
                            .font(.callout.weight(.semibold))
                        Text("Con la dirección y el token, las demás pestañas empiezan a funcionar.")
                            .font(.footnote)
                            .foregroundStyle(Palette.muted)
                    }
                    .padding(.vertical, 2)
                }
                .listRowBackground(Palette.surface)
            }

            Section {
                TextField("https://holocron.merli.store", text: $settings.serverURL)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .keyboardType(.URL)
                    .font(.system(.callout, design: .monospaced))
            } header: {
                Text("Servidor")
            } footer: {
                Text("La dirección de Holocron, por ejemplo https://holocron.merli.store. Se llega desde casa o por Tailscale. Si no ponés esquema, se asume http://")
            }
            .listRowBackground(Palette.surface)

            Section {
                SecureField("Pegá el token", text: $settings.token)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .font(.system(.callout, design: .monospaced))
            } header: {
                Text("Token de la API")
            } footer: {
                Text("Generalo en la web de Holocron: Ajustes → App iOS. Se muestra una sola vez y queda guardado en el Keychain.")
            }
            .listRowBackground(Palette.surface)

            Section {
                Button {
                    Task { await test() }
                } label: {
                    HStack {
                        if testing { ProgressView().controlSize(.small) }
                        Text("Probar conexión")
                    }
                }
                .disabled(!settings.isConfigured || testing)

                if let testResult {
                    Label(testResult, systemImage: testOK ? "checkmark.circle" : "exclamationmark.triangle")
                        .font(.footnote)
                        .foregroundStyle(testOK ? Palette.ok : Palette.danger)
                }
            }
            .listRowBackground(Palette.surface)

            Section {
                NavigationLink {
                    JellyfinLinkView()
                } label: {
                    Label("Conectar con Jellyfin", systemImage: "powerplug")
                }
                .disabled(!settings.isConfigured)
            } header: {
                Text("Jellyfin")
            } footer: {
                Text("Aprobás un código en Jellyfin y el token queda guardado en el servidor, sin buscar API keys a mano. La dirección se carga desde la web.")
            }
            .listRowBackground(Palette.surface)

            Section {
                Text("Holocron \(appVersion)")
                    .font(.footnote)
                    .foregroundStyle(Palette.muted)
            }
            .listRowBackground(Palette.surface)
        }
        .scrollContentBackground(.hidden)
        .background(Palette.bg)
        .navigationTitle("Ajustes")
    }

    private var appVersion: String {
        let info = Bundle.main.infoDictionary
        let version = info?["CFBundleShortVersionString"] as? String ?? "?"
        return "v\(version)"
    }

    @MainActor private func test() async {
        guard let client = settings.client else { return }
        testing = true
        defer { testing = false }
        do {
            let stats = try await client.system()
            testOK = true
            testResult = stats.hostname.isEmpty
                ? "Conectado."
                : "Conectado a \(stats.hostname)."
        } catch {
            testOK = false
            testResult = message(for: error)
        }
    }
}
