import SwiftUI

/// The Noir palette, kept in step with `web/static/styles.css` so the app and
/// the web UI read as the same product.
enum Noir {
    static let bg = Color(hex: 0x121110)
    static let surface = Color(hex: 0x1A1817)
    static let surface2 = Color(hex: 0x221F1D)
    static let text = Color(hex: 0xF1EDE9)
    static let accent = Color(hex: 0xFF6A2B)
    static let accent300 = Color(hex: 0xFFB088)
    static let ok = Color(hex: 0x6BBF8F)
    static let danger = Color(hex: 0xE08A8A)

    static let muted = Color(hex: 0xF1EDE9).opacity(0.55)
    static let divider = Color(hex: 0xF1EDE9).opacity(0.12)
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
            .background(Noir.surface, in: RoundedRectangle(cornerRadius: 10))
            .overlay(alignment: .leading) {
                if accented {
                    Rectangle()
                        .fill(Noir.accent)
                        .frame(width: 3)
                        .clipShape(RoundedRectangle(cornerRadius: 2))
                }
            }
            .overlay {
                RoundedRectangle(cornerRadius: 10)
                    .strokeBorder(Noir.divider, lineWidth: 1)
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
            .foregroundStyle(Noir.muted)
    }
}

/// The proportion bar used for disk usage and torrent progress.
struct ProgressBar: View {
    var value: Double // 0...1
    var hot = false

    var body: some View {
        GeometryReader { geo in
            ZStack(alignment: .leading) {
                Capsule().fill(Noir.text.opacity(0.09))
                Capsule()
                    .fill(hot
                          ? LinearGradient(colors: [Noir.accent, Color(hex: 0xFF8A4D)],
                                           startPoint: .leading, endPoint: .trailing)
                          : LinearGradient(colors: [Noir.accent, Noir.accent],
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
        case .yes: (Noir.ok, Noir.ok.opacity(0.16))
        case .no: (Noir.danger, Noir.danger.opacity(0.15))
        case .warn: (Noir.accent300, Color(hex: 0x331808))
        case .neutral: (Noir.muted, Noir.text.opacity(0.09))
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
