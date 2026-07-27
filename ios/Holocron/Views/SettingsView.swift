import SwiftUI

/// Where the server address and API token are entered, plus a connection test
/// so setup problems surface here instead of as failures on every other tab.
struct SettingsView: View {
    @Environment(AppSettings.self) private var settings

    @State private var testResult: String?
    @State private var testOK = false
    @State private var testing = false

    var body: some View {
        @Bindable var settings = settings

        Form {
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
