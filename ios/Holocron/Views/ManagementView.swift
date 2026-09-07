import SwiftUI

/// Control of the machine itself: restart a service, reboot, power off.
///
/// The friction here is deliberately uneven. A restart comes back on its own,
/// so a confirmation dialog is enough. Powering off does not — a Raspberry Pi 4
/// has no wake-on-LAN, so the only way back is somebody walking to it — and a
/// dialog would put that one tap away from the button, which is exactly the
/// gesture a thumb learns. So it holds instead.
///
/// Over the public address it holds *and* asks for consent first. It used to be
/// refused outright, which was the wrong call: leaving the house is exactly
/// when you might want to shut the machine down, and the app cannot know
/// whether somebody is home to turn it back on. So the consequence is stated
/// and has to be accepted, rather than the decision being made for you.
struct ManagementView: View {
    @Environment(AppSettings.self) private var settings

    @State private var state: Loadable<ManageStatus> = .idle
    @State private var banner: String?
    @State private var bannerIsError = false
    /// Actions whose consequence has been accepted, by key. Kept per action
    /// rather than as one flag so accepting one thing never arms another, and
    /// cleared once the action is requested.
    @State private var acknowledged: Set<String> = []

    var body: some View {
        LoadableView(state: state, reload: load) { status in
            List {
                if let banner {
                    Section {
                        Label(banner, systemImage: bannerIsError ? "exclamationmark.triangle" : "checkmark.circle")
                            .font(.footnote)
                            .foregroundStyle(bannerIsError ? Noir.danger : Noir.ok)
                    }
                    .listRowBackground(Noir.surface)
                }

                preflightSection(status)

                if let pending = status.pending {
                    Section {
                        HStack(spacing: 8) {
                            ProgressView().controlSize(.small)
                            Text("Pedido: \(pending). Esperando a que el sistema lo tome…")
                                .font(.footnote)
                        }
                    }
                    .listRowBackground(Noir.surface)
                } else if !status.available {
                    Section {
                        Text("El ayudante con privilegios no está instalado en la Pi, así que no se puede reiniciar ni apagar nada desde acá.")
                            .font(.footnote)
                            .foregroundStyle(Noir.muted)
                    }
                    .listRowBackground(Noir.surface)
                } else {
                    actionsSection(status)
                }
            }
            .scrollContentBackground(.hidden)
        }
        .background(Noir.bg)
        .navigationTitle("ObiWan")
        .refreshable { await load() }
        .task { if case .idle = state { await load() } }
    }

    // MARK: - Sections

    @ViewBuilder
    private func preflightSection(_ status: ManageStatus) -> some View {
        Section {
            if !status.warnings.isEmpty {
                ForEach(status.warnings, id: \.self) { warning in
                    Label(warning, systemImage: "exclamationmark.triangle")
                        .font(.footnote)
                        .foregroundStyle(Noir.accent300)
                }
            } else if status.checked {
                Label("Nada en curso: no hay nadie reproduciendo ni torrents activos.",
                      systemImage: "checkmark.circle")
                    .font(.footnote)
                    .foregroundStyle(Noir.ok)
            } else {
                // Not the same as an all-clear, and shown differently on
                // purpose: an empty list because nothing is happening and an
                // empty list because nothing could be asked look identical.
                Text("No se pudo consultar qué está en curso, así que no hay forma de saber si esto interrumpe algo.")
                    .font(.footnote)
                    .foregroundStyle(Noir.muted)
            }
        } header: {
            Text("Ahora mismo")
        }
        .listRowBackground(Noir.surface)
    }

    @ViewBuilder
    private func actionsSection(_ status: ManageStatus) -> some View {
        ForEach(status.actions) { action in
            Section {
                if action.needsToken && !settings.isOnHomeNetwork {
                    strandGate(action)
                } else if action.needsToken {
                    HoldToConfirmButton(label: action.label) {
                        Task { await run(action) }
                    }
                } else {
                    ConfirmButton(
                        label: action.label,
                        question: "¿\(action.label)?",
                        detail: action.detail,
                        role: action.interrupts ? .destructive : nil
                    ) {
                        Task { await run(action) }
                    }
                }
                Text(action.detail)
                    .font(.caption2)
                    .foregroundStyle(Noir.muted)
            }
            .listRowBackground(Noir.surface)
        }
    }

    /// The guard that does the real work. A dialog only helps if it is read,
    /// and this one knows something the person tapping might not have thought
    /// about — so it asks them to state it rather than just showing it.
    ///
    /// The toggle arms the button; it does not perform the action. Two separate
    /// gestures, and the first one is the one that carries the sentence.
    private func strandGate(_ action: ManageAction) -> some View {
        let armed = acknowledged.contains(action.key)
        return VStack(alignment: .leading, spacing: 10) {
            Label("Estás entrando por la dirección pública",
                  systemImage: "antenna.radiowaves.left.and.right")
                .font(.caption)
                .foregroundStyle(Noir.accent300)

            Toggle(isOn: Binding(
                get: { acknowledged.contains(action.key) },
                set: { on in
                    if on { acknowledged.insert(action.key) } else { acknowledged.remove(action.key) }
                }
            )) {
                Text("Entiendo que no se puede encender a distancia y que queda apagada hasta que alguien vaya hasta ella.")
                    .font(.caption)
            }
            .tint(Noir.danger)

            HoldToConfirmButton(label: action.label, enabled: armed) {
                Task { await run(action) }
            }
        }
    }

    // MARK: - Actions

    @MainActor private func load() async {
        guard let client = settings.client else {
            state = .failed(APIError.notConfigured.localizedDescription)
            return
        }
        if case .idle = state { state = .loading }
        do {
            state = .loaded(try await client.manage())
        } catch {
            state = .failed(error.localizedDescription)
        }
    }

    @MainActor private func run(_ action: ManageAction) async {
        guard let client = settings.client else { return }
        banner = nil
        let consented = acknowledged.contains(action.key)
        do {
            try await client.runManageAction(action.key, acknowledged: consented)
            acknowledged.remove(action.key)
            bannerIsError = false
            // The acknowledgement is the last thing this request can tell us:
            // for a reboot or a power off, the server is about to stop being
            // there, and losing contact after this point is the action working
            // rather than a failure.
            banner = "Pedido: \(action.label). " + afterword(action)
            if !action.interrupts {
                await load()
            }
        } catch {
            bannerIsError = true
            banner = error.localizedDescription
        }
    }

    private func afterword(_ action: ManageAction) -> String {
        if action.key == "poweroff" {
            return "Cuando deje de responder, ya está apagada."
        }
        if action.interrupts {
            return "Va a dejar de responder un rato y vuelve sola."
        }
        return "Vuelve solo en unos segundos."
    }
}

/// A button that has to be held. Used for the one action that cannot be undone
/// remotely: a second tap is a gesture the thumb learns, and a hold is not.
private struct HoldToConfirmButton: View {
    let label: String
    /// False while a precondition is unmet — off the home network, until the
    /// consequence has been accepted. Disabled rather than hidden: a button
    /// that is not there explains nothing.
    var enabled: Bool = true
    let action: () -> Void

    /// Long enough to be a decision, short enough not to feel broken.
    private let duration: TimeInterval = 1.5

    @State private var progress: Double = 0
    @State private var holding: Task<Void, Never>?

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            ZStack(alignment: .leading) {
                RoundedRectangle(cornerRadius: 8)
                    .fill(Noir.surface2)
                RoundedRectangle(cornerRadius: 8)
                    .fill(Noir.danger.opacity(0.35))
                    .scaleEffect(x: progress, y: 1, anchor: .leading)
                Label(progress > 0 ? "Mantené apretado…" : label, systemImage: "power")
                    .font(.callout.weight(.semibold))
                    .foregroundStyle(enabled ? Noir.danger : Noir.muted)
                    .padding(.horizontal, 12)
            }
            .frame(height: 44)
            .contentShape(RoundedRectangle(cornerRadius: 8))
            // A real gesture rather than onTapGesture on a shape, and with the
            // accessibility parts spelled out: holding is not an option under
            // VoiceOver, so the element is a button there with its own action.
            .gesture(
                DragGesture(minimumDistance: 0)
                    .onChanged { _ in start() }
                    .onEnded { _ in cancel() }
            )
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(label)
            .accessibilityHint(enabled
                ? "Mantené apretado para confirmar"
                : "Primero aceptá la advertencia de arriba")
            .accessibilityAddTraits(.isButton)
            .accessibilityAction { if enabled { action() } }

            Text(enabled ? "Mantené apretado para confirmar" : "Aceptá la advertencia para habilitarlo")
                .font(.caption2)
                .foregroundStyle(Noir.muted)
        }
        .padding(.vertical, 4)
        .onDisappear { cancel() }
    }

    private func start() {
        guard enabled, holding == nil else { return }
        holding = Task { @MainActor in
            let step = 0.05
            while progress < 1 {
                try? await Task.sleep(for: .seconds(step))
                if Task.isCancelled { return }
                progress += step / duration
            }
            progress = 0
            holding = nil
            action()
        }
    }

    private func cancel() {
        holding?.cancel()
        holding = nil
        progress = 0
    }
}

/// A button with a confirmation dialog, for the actions that recover on their
/// own.
private struct ConfirmButton: View {
    let label: String
    let question: String
    let detail: String
    var role: ButtonRole?
    let action: () -> Void

    @State private var asking = false

    var body: some View {
        Button(role: role) { asking = true } label: {
            Text(label)
        }
        .confirmationDialog(question, isPresented: $asking, titleVisibility: .visible) {
            Button(label, role: .destructive, action: action)
            Button("Cancelar", role: .cancel) {}
        } message: {
            Text(detail)
        }
    }
}
