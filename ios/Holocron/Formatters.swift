import Foundation

enum Format {
    /// Binary (IEC) units, matching the server's HumanBytes.
    static func bytes(_ value: UInt64) -> String {
        let unit: UInt64 = 1024
        if value < unit { return "\(value) B" }
        var div = unit
        var exp = 0
        var n = value / unit
        while n >= unit {
            div *= unit
            n /= unit
            exp += 1
        }
        let units = ["KiB", "MiB", "GiB", "TiB", "PiB", "EiB"]
        return String(format: "%.1f %@", Double(value) / Double(div), units[min(exp, units.count - 1)])
    }

    static func bytes(_ value: Int64) -> String {
        value <= 0 ? "0 B" : bytes(UInt64(value))
    }

    static func speed(_ bytesPerSecond: Int64) -> String {
        bytesPerSecond <= 0 ? "—" : "\(bytes(bytesPerSecond))/s"
    }

    static func percent(_ value: Double) -> String {
        String(format: "%.0f%%", value)
    }

    /// Compact uptime, e.g. "12d 4h" or "3h 20m".
    static func uptime(_ seconds: Int64) -> String {
        let days = seconds / 86_400
        let hours = (seconds % 86_400) / 3_600
        let minutes = (seconds % 3_600) / 60
        if days > 0 { return "\(days)d \(hours)h" }
        if hours > 0 { return "\(hours)h \(minutes)m" }
        return "\(minutes)m"
    }

    static func year(_ value: Int) -> String {
        value == 0 ? "—" : String(value)
    }

    /// The server sends UTC in SQLite's "2006-01-02 15:04:05" shape.
    static func relative(fromUTC stored: String) -> String {
        let parser = DateFormatter()
        parser.dateFormat = "yyyy-MM-dd HH:mm:ss"
        parser.timeZone = TimeZone(identifier: "UTC")
        parser.locale = Locale(identifier: "en_US_POSIX")
        guard let date = parser.date(from: stored) else { return stored }

        let formatter = RelativeDateTimeFormatter()
        formatter.locale = Locale(identifier: "es_AR")
        formatter.unitsStyle = .full
        return formatter.localizedString(for: date, relativeTo: Date())
    }
}
