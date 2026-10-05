import SwiftUI

/// The parts the server is made of: its units, its scheduled jobs and the
/// health of its disks, read from systemd and Ginebra's own SMART report.
struct ServicesView: View {
    @Environment(AppSettings.self) private var settings
    @State private var state: Loadable<ServicesReading> = .idle

    var body: some View {
        ScrollView {
            LoadableView(state: state, reload: load) { svc in
                VStack(spacing: 16) {
                    ForEach(svc.errors, id: \.self) { e in
                        Label(e, systemImage: "exclamationmark.triangle")
                            .font(.callout).foregroundStyle(Palette.accent300).card()
                    }
                    if !svc.configured {
                        Text("El servidor no tiene unidades configuradas para vigilar.")
                            .font(.callout).foregroundStyle(Palette.muted).card()
                    }
                    if !svc.units.isEmpty { unitsCard(svc) }
                    if !svc.timers.isEmpty { timersCard(svc.timers) }
                    if let drift = svc.drift { driftCard(drift) }
                    if !svc.disks.isEmpty { disksCard(svc) }
                }
                .padding(16)
            }
        }
        .background(Palette.bg)
        .navigationTitle("Servicios")
        .refreshable { await load() }
        .livePoll(every: .seconds(15)) { await load() }
    }

    private func unitsCard(_ svc: ServicesReading) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Label("Unidades", systemImage: "server.rack").sectionTitle()
                Spacer()
                if svc.down > 0 { Pill(text: "\(svc.down) \(svc.down == 1 ? "caída" : "caídas")", kind: .no) }
            }
            ForEach(svc.units) { u in
                HStack(alignment: .firstTextBaseline) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(u.name).font(.callout).lineLimit(1)
                        if !u.since.isEmpty {
                            Text(u.since).font(.caption).foregroundStyle(Palette.muted)
                        }
                    }
                    Spacer()
                    Pill(text: u.state, kind: u.ok ? .yes : .no).fixedSize()
                }
            }
        }
        .card(accented: svc.down > 0)
    }

    private func timersCard(_ timers: [SvcTimer]) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Label("Tareas programadas", systemImage: "clock.arrow.circlepath").sectionTitle()
            ForEach(timers) { t in
                VStack(alignment: .leading, spacing: 2) {
                    HStack(alignment: .firstTextBaseline) {
                        Text(t.name).font(.callout).lineLimit(1)
                        Spacer()
                        if !t.result.isEmpty { Pill(text: t.result, kind: t.failed ? .no : .yes).fixedSize() }
                    }
                    Text("Última: \(t.last.isEmpty ? "—" : t.last) · próxima: \(t.next.isEmpty ? "—" : t.next)")
                        .font(.caption).foregroundStyle(Palette.muted)
                }
            }
        }
        .card()
    }

    /// The server's own check that what runs is what its repo says.
    private func driftCard(_ d: SvcDrift) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Label("Instalación", systemImage: "checklist").sectionTitle()
                Spacer()
                Text(d.age).font(.caption).foregroundStyle(Palette.muted)
            }
            if d.differ.isEmpty {
                HStack {
                    Pill(text: "coincide con el repo", kind: .yes)
                    if !d.commit.isEmpty {
                        Text(d.commit).font(.caption.monospaced()).foregroundStyle(Palette.muted)
                    }
                }
            } else {
                Pill(text: "\(d.differ.count) \(d.differ.count == 1 ? "diferencia" : "diferencias") con el repo", kind: .no)
                ForEach(d.differ) { it in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(it.file).font(.caption.monospaced()).lineLimit(2)
                        Text(it.problem).font(.caption).foregroundStyle(Palette.accent300)
                    }
                }
            }
            if d.stale {
                Text("El chequeo dejó de correr: este resultado es viejo.")
                    .font(.caption).foregroundStyle(Palette.accent300)
            }
        }
        .card(accented: !d.differ.isEmpty || d.stale)
    }

    private func disksCard(_ svc: ServicesReading) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label("Salud de los discos", systemImage: "internaldrive").sectionTitle()
                Spacer()
                if !svc.smartAge.isEmpty {
                    Text(svc.smartAge).font(.caption).foregroundStyle(Palette.muted)
                }
            }
            ForEach(svc.disks) { d in
                VStack(alignment: .leading, spacing: 4) {
                    HStack(alignment: .firstTextBaseline) {
                        Text(d.disk).font(.callout.monospaced())
                        Text(d.model).font(.caption).foregroundStyle(Palette.muted).lineLimit(1)
                        Spacer()
                        if d.asleep { Pill(text: "dormido", kind: .neutral).fixedSize() }
                        Pill(text: d.health, kind: d.bad ? .no : .yes).fixedSize()
                    }
                    if !d.facts.isEmpty {
                        Text(d.facts.joined(separator: " · ")).font(.caption).foregroundStyle(Palette.muted)
                    }
                    ForEach(d.warn, id: \.self) { w in
                        Text(w).font(.caption).foregroundStyle(Palette.accent300)
                    }
                }
            }
        }
        .card()
    }

    @MainActor private func load() async {
        guard let client = settings.client else {
            state = .failed(APIError.notConfigured.localizedDescription)
            return
        }
        if case .idle = state { state = .loading }
        do {
            state = .loaded(try await client.services())
        } catch {
            if state.value == nil { state = .failed(message(for: error)) }
        }
    }
}
