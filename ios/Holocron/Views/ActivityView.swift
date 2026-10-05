import SwiftUI

/// What the server is doing right now: who is watching, what is downloading,
/// what was asked for and what is coming. One place instead of four apps.
struct ActivityView: View {
    @Environment(AppSettings.self) private var settings
    @State private var state: Loadable<ActivityReading> = .idle

    var body: some View {
        ScrollView {
            LoadableView(state: state, reload: load) { act in
                VStack(spacing: 16) {
                    ForEach(act.errors, id: \.self) { e in
                        Label(e, systemImage: "exclamationmark.triangle")
                            .font(.callout).foregroundStyle(Palette.accent300).card()
                    }
                    if !act.attention.isEmpty { attentionCard(act.attention) }
                    if act.hasJellyfin { playingCard(act) }
                    if act.hasTorrents || act.hasArr { downloadsCard(act) }
                    if act.hasSeerr { requestsCard(act) }
                    if !act.upcoming.isEmpty { upcomingCard(act.upcoming) }
                    if !act.recent.isEmpty { recentCard(act) }
                }
                .padding(16)
            }
        }
        .background(Palette.bg)
        .navigationTitle("Actividad")
        .refreshable { await load() }
        .livePoll(every: .seconds(5)) { await load() }
    }

    private func attentionCard(_ items: [ActAttention]) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Atención").sectionTitle()
            ForEach(items) { a in
                Label(a.text, systemImage: a.warn ? "exclamationmark.circle" : "info.circle")
                    .font(.callout)
                    .foregroundStyle(a.warn ? Palette.accent300 : Palette.text)
            }
        }
        .card(accented: true)
    }

    private func playingCard(_ act: ActivityReading) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Label("Reproduciendo", systemImage: "play.tv").sectionTitle()
            if act.playing.isEmpty {
                Text(act.idle > 0
                     ? "Nadie está mirando nada. \(act.idle) \(act.idle == 1 ? "sesión abierta" : "sesiones abiertas") sin reproducir."
                     : "Nadie está mirando nada.")
                    .font(.callout).foregroundStyle(Palette.muted)
            }
            ForEach(act.playing) { p in
                VStack(alignment: .leading, spacing: 4) {
                    HStack(alignment: .firstTextBaseline) {
                        Text(p.title).font(.callout.weight(.semibold)).lineLimit(1)
                        Spacer()
                        if p.paused { Pill(text: "en pausa", kind: .neutral) }
                        Pill(text: p.method, kind: p.onCPU ? .warn : .neutral)
                    }
                    if !p.subtitle.isEmpty {
                        Text(p.subtitle).font(.caption).foregroundStyle(Palette.muted).lineLimit(1)
                    }
                    ProgressBar(value: fraction(p.width))
                    Text([p.who, p.client, p.position].filter { !$0.isEmpty }.joined(separator: " · "))
                        .font(.caption.monospacedDigit()).foregroundStyle(Palette.muted)
                    if p.transcode {
                        Text([p.hardware, p.detail].filter { !$0.isEmpty }.joined(separator: " · "))
                            .font(.caption).foregroundStyle(p.onCPU ? Palette.accent300 : Palette.muted)
                        ForEach(p.reasons, id: \.self) { r in
                            Text("• \(r)").font(.caption2).foregroundStyle(Palette.muted)
                        }
                    }
                }
            }
        }
        .card()
    }

    private func downloadsCard(_ act: ActivityReading) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label("Descargas", systemImage: "arrow.down.circle").sectionTitle()
                Spacer()
                Text("↓ \(act.down) · ↑ \(act.up)").font(.caption.monospacedDigit()).foregroundStyle(Palette.muted)
            }
            if act.downloads.isEmpty {
                Text("No se está bajando nada.").font(.callout).foregroundStyle(Palette.muted)
            }
            ForEach(act.downloads) { d in
                VStack(alignment: .leading, spacing: 4) {
                    HStack(alignment: .firstTextBaseline) {
                        Text(d.subject.isEmpty ? d.release : d.subject).font(.callout.weight(.semibold)).lineLimit(1)
                        Spacer()
                        Pill(text: d.state, kind: d.problem ? .no : .neutral)
                    }
                    ProgressBar(value: fraction(d.width))
                    Text([d.app, d.pct, d.speed, d.eta].filter { !$0.isEmpty }.joined(separator: " · "))
                        .font(.caption.monospacedDigit()).foregroundStyle(Palette.muted)
                    ForEach(d.messages, id: \.self) { m in
                        Text(m).font(.caption2).foregroundStyle(Palette.accent300)
                    }
                }
            }
        }
        .card()
    }

    private func requestsCard(_ act: ActivityReading) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label("Pedidos", systemImage: "hand.raised").sectionTitle()
                Spacer()
                Text(act.reqSummary).font(.caption).foregroundStyle(Palette.muted)
            }
            if act.requests.isEmpty {
                Text("No hay pedidos.").font(.callout).foregroundStyle(Palette.muted)
            }
            ForEach(act.requests) { r in
                HStack(alignment: .firstTextBaseline) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(r.title).font(.callout).lineLimit(1)
                        Text([r.kind, r.by, r.when].filter { !$0.isEmpty }.joined(separator: " · "))
                            .font(.caption).foregroundStyle(Palette.muted)
                    }
                    Spacer()
                    Pill(text: r.state, kind: r.done ? .yes : r.stuck ? .warn : .neutral)
                }
            }
        }
        .card()
    }

    private func upcomingCard(_ items: [ActUpcoming]) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Label("Próximamente", systemImage: "calendar").sectionTitle()
            ForEach(items) { u in
                HStack(alignment: .firstTextBaseline) {
                    Text(u.subject).font(.callout).lineLimit(1)
                    Spacer()
                    Text(u.when).font(.caption).foregroundStyle(Palette.muted)
                }
            }
        }
        .card()
    }

    private func recentCard(_ act: ActivityReading) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Label("Recién agregado", systemImage: "sparkles").sectionTitle()
                Spacer()
                if !act.libAge.isEmpty {
                    Text(act.libAge).font(.caption).foregroundStyle(Palette.muted)
                }
            }
            ForEach(act.recent) { r in
                HStack(alignment: .firstTextBaseline) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(r.title).font(.callout).lineLimit(1)
                        if !r.subtitle.isEmpty {
                            Text(r.subtitle).font(.caption).foregroundStyle(Palette.muted).lineLimit(1)
                        }
                    }
                    Spacer()
                    Text(r.when).font(.caption).foregroundStyle(Palette.muted)
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
            state = .loaded(try await client.activity())
        } catch {
            if state.value == nil { state = .failed(message(for: error)) }
        }
    }
}
