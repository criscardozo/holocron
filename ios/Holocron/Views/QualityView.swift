import SwiftUI

/// What is wrong with the library, rather than how big it is: the same five
/// categories the web panel shows, with their lists.
///
/// Reached from Medios rather than living in its own tab. The tab bar already
/// holds five, and a sixth would push everything into a "More" menu.
struct QualityView: View {
    @Environment(AppSettings.self) private var settings

    @State private var state: Loadable<QualityReport> = .idle
    @State private var scanning = false
    @State private var selected: QualityCategory = .subsMissing
    /// Item ids already asked for, so a row cannot be pressed twice and each
    /// press costs one call to Jellyfin's metadata provider.
    @State private var requested: Set<String> = []
    @State private var banner: String?

    var body: some View {
        LoadableView(state: state, reload: load) { report in
            if !report.configured {
                ContentUnavailableView {
                    Label("Jellyfin no vinculado", systemImage: "gearshape")
                } description: {
                    Text("Vinculá Jellyfin para poder analizar la biblioteca.")
                }
            } else if report.hasReport != true {
                notAnalysedYet
            } else {
                content(report)
            }
        }
        .background(Noir.bg)
        .navigationTitle("Calidad")
        .refreshable { await load() }
        .task { if case .idle = state { await load() } }
    }

    private var notAnalysedYet: some View {
        ContentUnavailableView {
            Label("Sin analizar", systemImage: "gauge.with.dots.needle.bottom.50percent")
        } description: {
            Text("El análisis le pide a Jellyfin la biblioteca entera, episodios incluidos, así que tarda un rato.")
        } actions: {
            scanButton
        }
    }

    private func content(_ report: QualityReport) -> some View {
        List {
            Section {
                HStack(spacing: 20) {
                    stat("\(report.total ?? 0)", "hallazgos", tinted: (report.total ?? 0) > 0)
                    stat("\(report.scanned ?? 0)", "ítems revisados", tinted: false)
                    Spacer()
                }
                scanButton
                if let banner {
                    Text(banner).font(.footnote).foregroundStyle(Noir.muted)
                }
            }
            .listRowBackground(Noir.surface)

            Section {
                Picker("Categoría", selection: $selected) {
                    ForEach(QualityCategory.allCases) { category in
                        Text("\(category.label) (\(report.count(category)))").tag(category)
                    }
                }
                .pickerStyle(.menu)
                Text(selected.hint)
                    .font(.footnote)
                    .foregroundStyle(Noir.muted)
            }
            .listRowBackground(Noir.surface)

            findingsSection(report)
        }
        .scrollContentBackground(.hidden)
    }

    @ViewBuilder
    private func findingsSection(_ report: QualityReport) -> some View {
        let rows = report.findings(in: selected)
        Section {
            if rows.isEmpty {
                Text("Nada por acá.").foregroundStyle(Noir.muted)
            } else {
                ForEach(rows) { finding in
                    row(finding, report: report)
                }
            }
        } footer: {
            if report.truncated(selected) {
                Text("Se muestran \(rows.count) de \(report.count(selected)).")
            }
        }
        .listRowBackground(Noir.surface)
    }

    private func row(_ finding: QualityFinding, report: QualityReport) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            Text(finding.title).font(.callout)
            if !finding.path.isEmpty {
                Text(finding.path)
                    .font(.system(.caption2, design: .monospaced))
                    .foregroundStyle(Noir.muted)
                    .lineLimit(1)
            }
            HStack(spacing: 6) {
                Pill(text: finding.kind, kind: .neutral)
                if !finding.detail.isEmpty {
                    Text(finding.detail).font(.caption2).foregroundStyle(Noir.muted)
                }
                Spacer()
                // Offered only where re-reading the metadata is the actual fix,
                // and only to an administrator: Jellyfin refuses anyone else,
                // and saying so up front beats failing after the press.
                if selected.refreshable, report.admin == true {
                    if requested.contains(finding.itemId) {
                        Text("Pedido").font(.caption2).foregroundStyle(Noir.ok)
                    } else {
                        Button("Refrescar") {
                            Task { await refresh(finding) }
                        }
                        .font(.caption2)
                        .buttonStyle(.borderless)
                    }
                }
            }
            if selected.refreshable, report.admin == false {
                Text("Requiere una cuenta administradora de Jellyfin.")
                    .font(.caption2)
                    .foregroundStyle(Noir.muted)
            }
        }
        .padding(.vertical, 2)
    }

    private var scanButton: some View {
        Button {
            Task { await scan() }
        } label: {
            if scanning {
                HStack(spacing: 6) { ProgressView().controlSize(.small); Text("Analizando…") }
            } else {
                Label("Analizar la biblioteca", systemImage: "magnifyingglass")
            }
        }
        .buttonStyle(.borderless)
        .disabled(scanning)
    }

    private func stat(_ value: String, _ caption: String, tinted: Bool) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(value)
                .font(.title2.weight(.bold)).monospacedDigit()
                .foregroundStyle(tinted ? Noir.accent : Noir.text)
            Text(caption).font(.caption2).foregroundStyle(Noir.muted)
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
            let report = try await client.quality()
            state = .loaded(report)
            scanning = report.scanning == true
            // Land on a category that has something in it, rather than on an
            // empty list that reads as "nothing to do".
            if report.count(selected) == 0,
               let first = QualityCategory.allCases.first(where: { report.count($0) > 0 }) {
                selected = first
            }
            if scanning { await pollWhileScanning() }
        } catch {
            state = .failed(error.localizedDescription)
        }
    }

    @MainActor private func scan() async {
        guard let client = settings.client else { return }
        banner = nil
        do {
            try await client.startQualityScan()
            scanning = true
            await pollWhileScanning()
        } catch {
            banner = error.localizedDescription
        }
    }

    /// Polls until the scan finishes, then reloads. Cancelled with the view.
    @MainActor private func pollWhileScanning() async {
        guard let client = settings.client else { return }
        while !Task.isCancelled {
            try? await Task.sleep(for: .seconds(3))
            guard let report = try? await client.quality() else { return }
            if report.scanning != true {
                scanning = false
                state = .loaded(report)
                requested.removeAll()
                return
            }
        }
    }

    @MainActor private func refresh(_ finding: QualityFinding) async {
        guard let client = settings.client else { return }
        do {
            try await client.refreshQualityItem(finding.itemId)
            requested.insert(finding.itemId)
        } catch {
            banner = error.localizedDescription
        }
    }
}
