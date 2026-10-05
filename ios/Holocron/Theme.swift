import SwiftUI

/// Ginebra's palette, kept in step with `web/static/styles.css`, which takes it
/// from Ginebra's own portal: near-black ground, surfaces with a hairline
/// border and no shadows, light violet as the accent with the icon's pink and
/// lilac beside it. Holocron is the server's tool, so it wears the server's
/// look on the phone too.
enum Palette {
    static let bg = Color(hex: 0x07090E)
    static let surface = Color(hex: 0x13171F)
    static let surface2 = Color(hex: 0x1D2432)
    static let text = Color(hex: 0xEEF1F6)
    static let accent = Color(hex: 0xA78BFA)
    static let accent200 = Color(hex: 0xDDD6FE)
    static let accent300 = Color(hex: 0xC084FC)
    static let accent400 = Color(hex: 0x7C3AED)
    static let accent900 = Color(hex: 0x1A1430)
    static let pink = Color(hex: 0xF472B6)
    static let ok = Color(hex: 0x34D399)
    static let danger = Color(hex: 0xF43F5E)
    static let warn = Color(hex: 0xFACC15)

    static let muted = Color(hex: 0xEEF1F6).opacity(0.52)
    static let divider = Color.white.opacity(0.09)
}

extension Color {
    init(hex: UInt32) {
        self.init(
            .sRGB,
            red: Double((hex >> 16) & 0xFF) / 255,
            green: Double((hex >> 8) & 0xFF) / 255,
            blue: Double(hex & 0xFF) / 255,
            opacity: 1
        )
    }
}

/// A card surface matching the web UI's `.card .elev-sm`.
struct CardBackground: ViewModifier {
    var accented = false

    func body(content: Content) -> some View {
        content
            .padding(16)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Palette.surface, in: RoundedRectangle(cornerRadius: 16))
            .overlay(alignment: .leading) {
                if accented {
                    Rectangle()
                        .fill(Palette.accent)
                        .frame(width: 3)
                        .clipShape(RoundedRectangle(cornerRadius: 2))
                }
            }
            .overlay {
                RoundedRectangle(cornerRadius: 16)
                    .strokeBorder(Palette.divider, lineWidth: 1)
            }
    }
}

extension View {
    func card(accented: Bool = false) -> some View {
        modifier(CardBackground(accented: accented))
    }

    /// The uppercase, letter-spaced section label used across the UI.
    func sectionTitle() -> some View {
        font(.caption.weight(.semibold))
            .textCase(.uppercase)
            .kerning(1.2)
            .foregroundStyle(Palette.muted)
    }
}

/// The proportion bar used for disk usage and torrent progress.
struct ProgressBar: View {
    var value: Double // 0...1
    var hot = false

    var body: some View {
        GeometryReader { geo in
            ZStack(alignment: .leading) {
                Capsule().fill(Palette.text.opacity(0.09))
                Capsule()
                    .fill(hot
                          ? LinearGradient(colors: [Palette.accent, Palette.pink],
                                           startPoint: .leading, endPoint: .trailing)
                          : LinearGradient(colors: [Palette.accent, Palette.accent],
                                           startPoint: .leading, endPoint: .trailing))
                    .frame(width: max(0, min(1, value)) * geo.size.width)
            }
        }
        .frame(height: 7)
    }
}

/// Small pill used for yes/no and status.
struct Pill: View {
    enum Kind { case yes, no, warn, neutral }

    var text: String
    var kind: Kind

    private var colors: (fg: Color, bg: Color) {
        switch kind {
        case .yes: (Palette.ok, Palette.ok.opacity(0.16))
        case .no: (Palette.danger, Palette.danger.opacity(0.15))
        case .warn: (Palette.accent300, Palette.accent900)
        case .neutral: (Palette.muted, Palette.text.opacity(0.09))
        }
    }

    var body: some View {
        Text(text)
            .font(.caption2.weight(.medium))
            .padding(.horizontal, 9)
            .padding(.vertical, 3)
            .foregroundStyle(colors.fg)
            .background(colors.bg, in: Capsule())
    }
}
