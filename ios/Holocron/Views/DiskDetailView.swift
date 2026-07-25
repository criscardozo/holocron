import SwiftUI

/// Usage for one watched folder, plus a drill-down through the tree. Pushing a
/// new instance of BrowseView per level gives back navigation for free.
struct DiskDetailView: View {
    @Environment(AppSettings.self) private var settings
    let folder: DiskFolder

    @State private var state: Loadable<DiskDetail> = .idle
    @State private var scanning = false

    var body: some View {
        LoadableView(state: state, reload: load) { detail in
            List {
                Section {
                    gauge(detail.folder)
                    scanRow(detail)
                }
                .listRowBackground(Noir.surface)

                if detail.top.isEmpty {
                    Section {
                        Text(detail.scanning
                             ? "Escaneando…"
                             : "Todavía no se escaneó esta carpeta.")
                            .foregroundStyle(Noir.muted)
                    }
                    .listRowBackground(Noir.surface)
                } else {
                    Section("Carpetas más grandes") {
                        ForEach(detail.top) { entry in
                            NavigationLink {
                                BrowseView(folderID: folder.id, path: entry.path, title: entry.name)
                            } label: {
                                entryRow(entry, largest: detail.top.first?.bytes ?? entry.bytes)
                            }
                        }
                    }
                    .listRowBackground(Noir.surface)
                }
            }
            .scrollContentBackground(.hidden)
        }
        .background(Noir.bg)
        .navigationTitle(folder.label)
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await load() }
        .task { if case .idle = state { await load() } }
    }

    private func gauge(_ folder: DiskFolder) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("\(folder.usedPercent)%")
                .font(.system(size: 44, weight: .bold))
                .monospacedDigit()
                .foregroundStyle(Noir.accent)
            ProgressBar(value: Double(folder.usedPercent) / 100, hot: folder.isHot)
            Text("\(Format.bytes(folder.usedBytes)) usados · \(Format.bytes(folder.freeBytes)) libres de \(Format.bytes(folder.totalBytes))")
                .font(.caption)
                .foregroundStyle(Noir.muted)
            Text(folder.path)
                .font(.system(.caption2, design: .monospaced))
                .foregroundStyle(Noir.muted)
        }
        .padding(.vertical, 4)
    }

    private func scanRow(_ detail: DiskDetail) -> some View {
        HStack {
            if detail.scanning || scanning {
                ProgressView().controlSize(.small)
                Text("Escaneando…").font(.callout).foregroundStyle(Noir.muted)
            } else {
                Button {
                    Task { await startScan() }
                } label: {
                    Label("Escanear", systemImage: "arrow.clockwise")
                }
                .buttonStyle(.borderless)
                if let scannedAt = detail.scannedAt {
                    Spacer()
                    Text(Format.relative(fromUTC: scannedAt))
                        .font(.caption2)
                        .foregroundStyle(Noir.muted)
                }
            }
        }
    }

    private func entryRow(_ entry: DiskEntry, largest: UInt64) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Image(systemName: entry.isDir ? "folder" : "doc")
                    .font(.caption)
                    .foregroundStyle(entry.isDir ? Noir.accent : Noir.muted)
                Text(entry.name).font(.system(.footnote, design: .monospaced)).lineLimit(1)
                Spacer()
                Text(Format.bytes(entry.bytes))
                    .font(.caption).monospacedDigit()
            }
            ProgressBar(value: largest > 0 ? Double(entry.bytes) / Double(largest) : 0)
        }
        .padding(.vertical, 2)
    }

    @MainActor private func load() async {
        guard let client = settings.client else {
            state = .failed(APIError.notConfigured.localizedDescription)
            return
        }
        if case .idle = state { state = .loading }
        do {
            state = .loaded(try await client.diskDetail(id: folder.id))
        } catch {
            state = .failed(message(for: error))
        }
    }

    @MainActor private func startScan() async {
        guard let client = settings.client else { return }
        scanning = true
        defer { scanning = false }
        _ = try? await client.startDiskScan(id: folder.id)
        // Give the job a moment to register, then poll until it finishes.
        for _ in 0..<60 {
            try? await Task.sleep(for: .seconds(2))
            guard let detail = try? await client.diskDetail(id: folder.id) else { break }
            state = .loaded(detail)
            if !detail.scanning { break }
        }
    }
}

/// One level of the drill-down.
struct BrowseView: View {
    @Environment(AppSettings.self) private var settings
    let folderID: Int64
    let path: String
    let title: String

    @State private var state: Loadable<DiskListing> = .idle

    var body: some View {
        LoadableView(state: state, reload: load) { listing in
            List {
                Section {
                    Text(listing.path)
                        .font(.system(.caption2, design: .monospaced))
                        .foregroundStyle(Noir.muted)
                    Text("Total: \(Format.bytes(listing.totalBytes))")
                        .font(.caption)
                }
                .listRowBackground(Noir.surface)

                Section {
                    ForEach(listing.entries) { entry in
                        if entry.isDir {
                            NavigationLink {
                                BrowseView(folderID: folderID, path: entry.path, title: entry.name)
                            } label: {
                                row(entry, largest: listing.entries.first?.bytes ?? entry.bytes)
                            }
                        } else {
                            row(entry, largest: listing.entries.first?.bytes ?? entry.bytes)
                        }
                    }
                }
                .listRowBackground(Noir.surface)
            }
            .scrollContentBackground(.hidden)
        }
        .background(Noir.bg)
        .navigationTitle(title)
        .navigationBarTitleDisplayMode(.inline)
        .task { if case .idle = state { await load() } }
    }

    private func row(_ entry: DiskEntry, largest: UInt64) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Image(systemName: entry.isDir ? "folder" : "doc")
                    .font(.caption)
                    .foregroundStyle(entry.isDir ? Noir.accent : Noir.muted)
                Text(entry.name).font(.system(.footnote, design: .monospaced)).lineLimit(1)
                Spacer()
                Text(Format.bytes(entry.bytes)).font(.caption).monospacedDigit()
            }
            ProgressBar(value: largest > 0 ? Double(entry.bytes) / Double(largest) : 0)
        }
        .padding(.vertical, 2)
    }

    @MainActor private func load() async {
        guard let client = settings.client else {
            state = .failed(APIError.notConfigured.localizedDescription)
            return
        }
        if case .idle = state { state = .loading }
        do {
            state = .loaded(try await client.diskBrowse(id: folderID, path: path))
        } catch {
            state = .failed(message(for: error))
        }
    }
}
