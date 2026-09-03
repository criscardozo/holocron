import SwiftUI

/// Where the server address, API token and (when the server is published
/// through Cloudflare Access) the service token are entered, plus a connection
/// test so setup problems surface here instead of as failures on every tab.
struct SettingsView: View {
    @Environment(AppSettings.self) private var settings

    @State private var editingSecret = false
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
                            .foregroundStyle(Noir.muted)
                    }
                    .padding(.vertical, 2)
                }
                .listRowBackground(Noir.surface)
            }

            Section {
                TextField("192.168.1.10:8090", text: $settings.serverURL)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .keyboardType(.URL)
                    .font(.system(.callout, design: .monospaced))
            } header: {
                Text("Servidor")
            } footer: {
                Text("La dirección de tu Raspberry Pi en la red local. Si no ponés esquema, se asume http://")
            }
            .listRowBackground(Noir.surface)

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
            .listRowBackground(Noir.surface)

            Section {
                TextField("algo.access", text: $settings.accessClientID)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .font(.system(.callout, design: .monospaced))
                if settings.accessClientSecret.isEmpty || editingSecret {
                    SecureField("Client Secret", text: $settings.accessClientSecret)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .font(.system(.callout, design: .monospaced))
                } else {
                    // Once saved it is only shown by its tail: enough to tell
                    // which secret is loaded, not enough to leak in a
                    // screenshot or over someone's shoulder.
                    HStack {
                        Text(Self.masked(settings.accessClientSecret))
                            .font(.system(.callout, design: .monospaced))
                            .foregroundStyle(Noir.muted)
                        Spacer()
                        Button("Cambiar") { editingSecret = true }
                            .font(.footnote)
                    }
                }
            } header: {
                Text("Cloudflare Access")
            } footer: {
                Text("Sólo si entrás por el dominio público. Access exige estos dos y Holocron sigue exigiendo el token: dos capas. En la red local dejalos vacíos.")
            }
            .listRowBackground(Noir.surface)

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
                        .foregroundStyle(testOK ? Noir.ok : Noir.danger)
                }
            }
            .listRowBackground(Noir.surface)

            Section {
                NavigationLink {
                    PlexLinkView()
                } label: {
                    Label("Conectar con Plex", systemImage: "powerplug")
                }
                .disabled(!settings.isConfigured)
            } header: {
                Text("Plex")
            } footer: {
                Text("Vincula tu cuenta por plex.tv y guarda el token en el servidor, sin buscarlo a mano.")
            }
            .listRowBackground(Noir.surface)

            Section {
                Text("Holocron \(appVersion)")
                    .font(.footnote)
                    .foregroundStyle(Noir.muted)
            }
            .listRowBackground(Noir.surface)
        }
        .scrollContentBackground(.hidden)
        .background(Noir.bg)
        .navigationTitle("Ajustes")
    }

    /// Shows only the tail of a stored secret: enough to tell which one is
    /// loaded, not enough to leak in a screenshot.
    private static func masked(_ secret: String) -> String {
        let tail = secret.suffix(4)
        return tail.isEmpty ? "" : "••••••••" + tail
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
