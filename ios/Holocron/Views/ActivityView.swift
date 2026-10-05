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
                        Label("No responde: \(e)", systemImage: "exclamationmark.triangle")
                            .font(.callout).foregroundStyle(Palette.accent300).card()
                    }
                    if act.hasJellyfin { playingSection(act) }
                    if !act.recent.isEmpty { recentRow(act.recent) }
                    if act.hasSeerr { requestsCard(act) }
                    if act.hasTorrents || act.hasArr { downloadsCard(act) }
                    if !act.upcoming.isEmpty { upcomingCard(act.upcoming) }
                    if !act.attention.isEmpty { attentionCard(act.attention) }
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

    /// What is playing, large, each over its own poster blurred: the first
    /// thing on the screen, because it is the thing happening.
    @ViewBuilder private func playingSection(_ act: ActivityReading) -> some View {
        if act.playing.isEmpty {
            VStack(alignment: .leading, spacing: 6) {
                Text("Reproduciendo").kicker()
                Text("Nadie está mirando nada.").font(.title3.weight(.semibold))
                if act.idle > 0 {
                    Text("\(act.idle) \(act.idle == 1 ? "sesión abierta" : "sesiones abiertas") sin reproducir.")
                        .font(.caption).foregroundStyle(Palette.muted)
                }
            }
            .card()
        } else {
            ForEach(act.playing) { p in playingHero(p) }
        }
    }

    private func playingHero(_ p: ActPlaying) -> some View {
        HStack(alignment: .bottom, spacing: 14) {
            PosterImage(path: p.art, title: p.title)
                .frame(width: 92)
                .shadow(color: .black.opacity(0.5), radius: 14, y: 8)
            VStack(alignment: .leading, spacing: 4) {
                Text(p.paused ? "En pausa" : "Reproduciendo").kicker()
                Text(p.title).font(.title2.weight(.bold)).lineLimit(2)
                if !p.subtitle.isEmpty {
                    Text(p.subtitle).font(.subheadline).foregroundStyle(Palette.text.opacity(0.8)).lineLimit(1)
                }
                Text(p.who).font(.caption).foregroundStyle(Palette.muted).lineLimit(2)
                ProgressBar(value: fraction(p.width)).padding(.top, 4)
                HStack(spacing: 6) {
                    Text(p.position).font(.caption2.monospacedDigit()).foregroundStyle(Palette.muted)
                    Pill(text: p.method, kind: p.transcode ? .neutral : .yes)
                    if !p.hardware.isEmpty { Pill(text: p.hardware, kind: p.onCPU ? .no : .yes) }
                }
                if p.transcode, !(p.detail.isEmpty && p.reasons.isEmpty) {
                    Text(([p.detail] + p.reasons).filter { !$0.isEmpty }.joined(separator: " · "))
                        .font(.caption2).foregroundStyle(p.onCPU ? Palette.accent300 : Palette.muted)
                }
            }
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background {
            ZStack {
                PosterImage(path: p.art, title: p.title, corner: 0)
                    .scaledToFill()
                    .blur(radius: 28)
                    .opacity(0.55)
                LinearGradient(colors: [Palette.bg.opacity(0.92), Palette.bg.opacity(0.5)],
                               startPoint: .leading, endPoint: .trailing)
            }
        }
        .clipShape(RoundedRectangle(cornerRadius: 18))
        .overlay(RoundedRectangle(cornerRadius: 18).strokeBorder(Palette.divider))
    }

    private func recentRow(_ items: [ActRecent]) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Agregado hace poco").font(.headline)
            PosterRow(items: items)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
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
                HStack(spacing: 12) {
                    PosterImage(path: r.art, title: r.title, corner: 5)
                        .frame(width: 34)
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
