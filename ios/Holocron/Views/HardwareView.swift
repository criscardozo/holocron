import SwiftUI

/// The machine, live: every core, memory, network, disks and the battery that
/// doubles as a UPS.
struct HardwareView: View {
    @Environment(AppSettings.self) private var settings
    @State private var state: Loadable<HardwareReading> = .idle

    var body: some View {
        ScrollView {
            LoadableView(state: state, reload: load) { hw in
                VStack(spacing: 16) {
                    if hw.battery.discharging { upsAlert(hw.battery) }
                    kpis(hw)
                    cpuCard(hw)
                    memoryCard(hw)
                    networkCard(hw)
                    if !hw.disks.isEmpty { diskCard(hw) }
                    if hw.battery.present { batteryCard(hw.battery) }
                }
                .padding(16)
            }
        }
        .background(Palette.bg)
        .navigationTitle("Hardware")
        .livePoll(every: .seconds(3)) { await load() }
    }

    private func upsAlert(_ b: HWBattery) -> some View {
        Label {
            VStack(alignment: .leading, spacing: 4) {
                Text("Se cortó la luz: el servidor está andando con la batería. \(b.percent)\(b.left.isEmpty ? "" : " · quedan unos \(b.left)")")
                    .font(.callout.weight(.semibold))
                // No power-on after a power loss in this BIOS, and no wake
                // from off: once the battery is gone, it stays off.
                Text("Si se agota, se apaga y no vuelve sola cuando vuelve la luz: hay que prenderla a mano.")
                    .font(.caption)
            }
        } icon: {
            Image(systemName: "bolt.slash.fill")
        }
        .foregroundStyle(Palette.danger)
        .card()
    }

    /// The four numbers that answer "is it all right", before the detail.
    private func kpis(_ hw: HardwareReading) -> some View {
        LazyVGrid(columns: [GridItem(.flexible(), spacing: 12), GridItem(.flexible(), spacing: 12)], spacing: 12) {
            kpi("cpu", "violet", "CPU", hw.live ? hw.cpu : "midiendo…", hw.load, warn: false)
            kpi("memorychip", "pink", "Memoria", hw.ram.pct, "\(hw.ram.used) de \(hw.ram.total)", warn: hw.ram.high)
            kpi("thermometer.medium", "amber", "Temperatura", hw.temp.isEmpty ? "—" : hw.temp, "Encendida hace \(hw.uptime)", warn: hw.tempHot)
            if hw.battery.present {
                kpi("battery.100.bolt", "mint", "Batería (UPS)", hw.battery.percent, hw.battery.status,
                    warn: hw.battery.discharging || hw.battery.low)
            }
        }
    }

    private func kpi(_ symbol: String, _ tone: String, _ label: String, _ value: String, _ sub: String, warn: Bool) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 8) {
                Image(systemName: symbol)
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundStyle(.white)
                    .frame(width: 26, height: 26)
                    .background(DashboardView.tone(tone), in: RoundedRectangle(cornerRadius: 7))
                Text(label).font(.caption).foregroundStyle(Palette.muted)
            }
            Text(value)
                .font(.title2.weight(.bold)).monospacedDigit()
                .foregroundStyle(warn ? Palette.danger : Palette.text)
                .lineLimit(1).minimumScaleFactor(0.7)
                .padding(.top, 4)
            Text(sub).font(.caption2).foregroundStyle(Palette.muted).lineLimit(1)
        }
        .card(accented: warn)
    }

    private func cpuCard(_ hw: HardwareReading) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label("CPU", systemImage: "cpu").sectionTitle()
                Spacer()
                Text(hw.live ? hw.cpu : "midiendo…").font(.title3.weight(.bold)).monospacedDigit()
            }
            SparkLine(spark: hw.cpuSpark)
            ForEach(hw.cores) { c in
                HStack(spacing: 8) {
                    Text(c.name).font(.caption.monospaced()).foregroundStyle(Palette.muted).frame(width: 18)
                    ProgressBar(value: fraction(c.width), hot: c.hot)
                    Text(c.busy).font(.caption.monospacedDigit()).frame(width: 40, alignment: .trailing)
                    Text(c.mhz).font(.caption.monospacedDigit()).foregroundStyle(Palette.muted).frame(width: 52, alignment: .trailing)
                }
            }
            StatRow(key: "Temperatura", value: hw.temp.isEmpty ? "—" : hw.temp)
            StatRow(key: "Carga 1 · 5 · 15", value: hw.load)
            StatRow(key: "Encendido hace", value: hw.uptime)
        }
        .card()
    }

    private func memoryCard(_ hw: HardwareReading) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label("Memoria", systemImage: "memorychip").sectionTitle()
                Spacer()
                Text(hw.ram.pct).font(.title3.weight(.bold)).monospacedDigit()
            }
            meter("RAM", hw.ram)
            if hw.hasSwap { meter("Swap", hw.swap) }
            if !hw.zram.isEmpty {
                Text("zram: \(hw.zram)").font(.caption).foregroundStyle(Palette.muted)
            }
        }
        .card()
    }

    private func meter(_ label: String, _ m: HWMeter) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text(label).font(.subheadline).foregroundStyle(Palette.muted)
                Spacer()
                Text("\(m.used) de \(m.total)").font(.caption.monospacedDigit())
            }
            ProgressBar(value: fraction(m.width), hot: m.high)
        }
    }

    private func networkCard(_ hw: HardwareReading) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Label("Red", systemImage: "network").sectionTitle()
            SparkLine(spark: hw.netSpk, color: Palette.pink)
            ForEach(hw.links) { l in
                HStack {
                    Text(l.name).font(.subheadline.monospaced())
                    if !l.up {
                        Pill(text: "sin enlace", kind: .neutral)
                    } else if !l.speed.isEmpty {
                        Text(l.speed).font(.caption).foregroundStyle(Palette.muted)
                    }
                    Spacer()
                    Text("↓ \(l.rx) · ↑ \(l.tx)").font(.caption.monospacedDigit())
                }
            }
        }
        .card()
    }

    private func diskCard(_ hw: HardwareReading) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Label("Discos", systemImage: "internaldrive").sectionTitle()
            ForEach(hw.disks) { d in
                VStack(alignment: .leading, spacing: 4) {
                    HStack {
                        Text(d.name).font(.subheadline.monospaced())
                        Text(d.model).font(.caption).foregroundStyle(Palette.muted).lineLimit(1)
                    }
                    ProgressBar(value: fraction(d.width))
                    Text(d.idle ? "sin actividad" : "\(d.busy) · ↓ \(d.read) · ↑ \(d.write)")
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(d.idle ? Palette.muted : Palette.text)
                }
            }
        }
        .card()
    }

    private func batteryCard(_ b: HWBattery) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label("Batería", systemImage: b.discharging ? "battery.25" : "battery.100.bolt").sectionTitle()
                Spacer()
                Text(b.percent).font(.title3.weight(.bold)).monospacedDigit()
            }
            ProgressBar(value: fraction(b.width), hot: b.low)
            StatRow(key: "Estado", value: b.status)
            if !b.health.isEmpty { StatRow(key: "Salud", value: "\(b.health) de la original") }
            if !b.left.isEmpty { StatRow(key: "Queda", value: b.left) }
            Text("Hace de UPS: si se corta la luz, esta pantalla lo avisa.")
                .font(.caption).foregroundStyle(Palette.muted)
        }
        .card(accented: b.discharging)
    }

    @MainActor private func load() async {
        guard let client = settings.client else {
            state = .failed(APIError.notConfigured.localizedDescription)
            return
        }
        if case .idle = state { state = .loading }
        do {
            state = .loaded(try await client.hardware())
        } catch {
            // Keep the last reading on a transient failure: a poll that blips
            // should not blank a screen that was showing real numbers.
            if state.value == nil { state = .failed(message(for: error)) }
        }
    }
}
