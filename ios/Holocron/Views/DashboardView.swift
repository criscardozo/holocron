import SwiftUI

/// The at-a-glance screen: how the Pi is doing and what needs attention.
struct DashboardView: View {
    @Environment(AppSettings.self) private var settings

    @State private var system: Loadable<SystemStats> = .idle
    @State private var disks: [DiskFolder] = []
    @State private var naming: NamingReport?
    @State private var subtitles: SubtitlesReport?

    var body: some View {
        ScrollView {
            LoadableView(state: system, reload: load) { stats in
                VStack(spacing: 16) {
                    attentionStrip
                    systemCard(stats)
                    if !disks.isEmpty { diskCard }
                }
                .padding(16)
            }
        }
        .background(Noir.bg)
        .navigationTitle("Estado")
        .refreshable { await load() }
        .task { if case .idle = system { await load() } }
    }

    // MARK: - Sections

    @ViewBuilder private var attentionStrip: some View {
        let chips = attentionChips
        if !chips.isEmpty {
            VStack(alignment: .leading, spacing: 8) {
                Text("Atención").sectionTitle()
                ForEach(chips, id: \.self) { chip in
                    HStack(spacing: 8) {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .font(.caption)
                            .foregroundStyle(Noir.accent300)
                        Text(chip).font(.callout)
                        Spacer()
                    }
                    .padding(.horizontal, 12)
                    .padding(.vertical, 8)
                    .background(Color(hex: 0x331808), in: Capsule())
                    .foregroundStyle(Color(hex: 0xFFCBAF))
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private var attentionChips: [String] {
        var chips: [String] = []
        if let n = naming?.count, n > 0 {
            chips.append("\(n) \(n == 1 ? "nombre inválido" : "nombres inválidos")")
        }
        if let s = subtitles?.missing, s > 0 {
            chips.append("\(s) sin subtítulos")
        }
        for disk in disks where disk.isHot {
            chips.append("\(disk.label) \(disk.usedPercent)%")
        }
        return chips
    }

    private func systemCard(_ stats: SystemStats) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label("Sistema", systemImage: "waveform.path.ecg").sectionTitle()
                Spacer()
                if !stats.hostname.isEmpty {
                    Text(stats.hostname).font(.caption).foregroundStyle(Noir.muted)
                }
            }
            statRow("CPU", stats.cpuPercent.map(Format.percent))
            statRow("RAM", ramText(stats))
            statRow("Temp", stats.tempCelsius.map { String(format: "%.1f °C", $0) })
            statRow("Uptime", stats.uptimeSeconds.map(Format.uptime))
            statRow("Load", stats.load1.map { String(format: "%.2f", $0) })
        }
        .card()
    }

    private func ramText(_ stats: SystemStats) -> String? {
        guard let used = stats.memUsedBytes, let total = stats.memTotalBytes, total > 0 else { return nil }
        return "\(Format.bytes(used)) / \(Format.bytes(total))"
    }

    private var diskCard: some View {
        VStack(alignment: .leading, spacing: 14) {
            Label("Disco", systemImage: "internaldrive").sectionTitle()
            ForEach(disks) { disk in
                NavigationLink {
                    DiskDetailView(folder: disk)
                } label: {
                    diskRow(disk)
                }
                .buttonStyle(.plain)
            }
        }
        .card()
    }

    private func diskRow(_ disk: DiskFolder) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(disk.label).font(.callout)
                Spacer()
                if disk.available {
                    Text("\(disk.usedPercent)%")
                        .font(.callout.weight(.bold))
                        .monospacedDigit()
                        .foregroundStyle(disk.isHot ? Noir.accent300 : Noir.text)
                } else {
                    Text("sin leer").font(.caption).foregroundStyle(Noir.danger)
                }
            }
            if disk.available {
                ProgressBar(value: Double(disk.usedPercent) / 100, hot: disk.isHot)
                Text("\(Format.bytes(disk.usedBytes)) / \(Format.bytes(disk.totalBytes))")
                    .font(.caption2)
                    .foregroundStyle(Noir.muted)
            }
        }
    }

    private func statRow(_ key: String, _ value: String?) -> some View {
        HStack {
            Text(key).font(.subheadline).foregroundStyle(Noir.muted)
            Spacer()
            Text(value ?? "—").font(.subheadline.weight(.semibold)).monospacedDigit()
        }
    }

    // MARK: - Loading

    @MainActor private func load() async {
        guard let client = settings.client else {
            system = .failed(APIError.notConfigured.localizedDescription)
            return
        }
        if case .idle = system { system = .loading }
        do {
            // The dashboard aggregates several endpoints; the system call is
            // the one that decides whether the screen can render at all.
            async let stats = client.system()
            async let folders = client.diskFolders()
            async let namingReport = try? client.naming()
            async let subtitlesReport = try? client.subtitles()

            system = .loaded(try await stats)
            disks = (try? await folders) ?? []
            naming = await namingReport
            subtitles = await subtitlesReport
        } catch {
            system = .failed(message(for: error))
        }
    }
}

/// Maps any thrown error to something worth showing a person.
@MainActor
func message(for error: Error) -> String {
    (error as? APIError)?.localizedDescription ?? error.localizedDescription
}
