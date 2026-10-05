import Foundation

/// A list that decodes JSON `null` — and a missing key — as empty.
///
/// Go encodes an empty slice that was never allocated as `null`, and the live
/// views send several of those whenever there is nothing to list. Each model
/// handling it by hand would be a dozen custom initialisers that all have to
/// stay right; this is one, applied where the field is declared.
@propertyWrapper
struct Lenient<T: Decodable & Sendable>: Decodable, Sendable {
    var wrappedValue: [T]

    init(wrappedValue: [T]) { self.wrappedValue = wrappedValue }

    init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        wrappedValue = c.decodeNil() ? [] : try c.decode([T].self)
    }
}

extension KeyedDecodingContainer {
    func decode<T>(_ type: Lenient<T>.Type, forKey key: Key) throws -> Lenient<T> {
        try decodeIfPresent(type, forKey: key) ?? Lenient(wrappedValue: [])
    }
}

// The live screens. These mirror the view models the web renders
// (web/templates/viewmodel_*.go), already formatted on the server, so the app
// shows the same words and nothing here decides what a state means.

// MARK: - Hardware

struct HardwareReading: Decodable, Sendable {
    var live: Bool
    var cpu: String
    var cpuSpark: Spark
    @Lenient var cores: [HWCore]
    var temp: String
    var tempHot: Bool
    var load: String
    var uptime: String
    var ram: HWMeter
    var swap: HWMeter
    var zram: String
    var hasSwap: Bool
    @Lenient var links: [HWLink]
    var netSpk: Spark
    @Lenient var disks: [HWDisk]
    var battery: HWBattery
}

struct Spark: Decodable, Sendable {
    var points: String
    var max: String

    /// The polyline's points on the server's 100×24 canvas.
    var values: [CGPoint] {
        points.split(separator: " ").compactMap { pair in
            let xy = pair.split(separator: ",")
            guard xy.count == 2, let x = Double(xy[0]), let y = Double(xy[1]) else { return nil }
            return CGPoint(x: x, y: y)
        }
    }
}

struct HWCore: Decodable, Sendable, Identifiable {
    var name: String
    var busy: String
    var width: String
    var mhz: String
    var hot: Bool
    var id: String { name }
}

struct HWMeter: Decodable, Sendable {
    var used: String
    var total: String
    var pct: String
    var width: String
    var high: Bool
}

struct HWLink: Decodable, Sendable, Identifiable {
    var name: String
    var up: Bool
    var speed: String
    var rx: String
    var tx: String
    var id: String { name }
}

struct HWDisk: Decodable, Sendable, Identifiable {
    var name: String
    var model: String
    var read: String
    var write: String
    var busy: String
    var width: String
    var idle: Bool
    var id: String { name }
}

struct HWBattery: Decodable, Sendable {
    var present: Bool
    var percent: String
    var width: String
    var status: String
    var health: String
    var discharging: Bool
    var left: String
    var low: Bool
}

/// The server sends bar lengths as a number on a 0..100 canvas, as text.
func fraction(_ width: String) -> Double {
    (Double(width) ?? 0) / 100
}

// MARK: - Activity

struct ActivityReading: Decodable, Sendable {
    @Lenient var playing: [ActPlaying]
    var idle: Int
    @Lenient var downloads: [ActDownload]
    @Lenient var recent: [ActRecent]
    var down: String
    var up: String
    @Lenient var errors: [String]
    var hasJellyfin: Bool
    var hasTorrents: Bool
    var hasArr: Bool
    @Lenient var requests: [ActRequest]
    var reqSummary: String
    var hasSeerr: Bool
    @Lenient var upcoming: [ActUpcoming]
    @Lenient var attention: [ActAttention]
    var libAge: String
}

struct ActPlaying: Decodable, Sendable, Identifiable {
    /// Poster paths are optional: servers before 0.18 do not send them.
    var art: String?
    var title: String
    var subtitle: String
    var who: String
    var client: String
    var paused: Bool
    var position: String
    var width: String
    var method: String
    var transcode: Bool
    var hardware: String
    var onCPU: Bool
    @Lenient var reasons: [String]
    var detail: String
    var id: String { who + title + subtitle }
}

struct ActDownload: Decodable, Sendable, Identifiable {
    var app: String
    var subject: String
    var release: String
    var pct: String
    var width: String
    var speed: String
    var eta: String
    var state: String
    var problem: Bool
    @Lenient var messages: [String]
    var id: String { app + subject + release }
}

struct ActRecent: Decodable, Sendable, Identifiable {
    var title: String
    var subtitle: String
    var when: String
    var art: String?
    var id: String { title + subtitle }
}

struct ActRequest: Decodable, Sendable, Identifiable {
    var title: String
    var kind: String
    var by: String
    var when: String
    var state: String
    var done: Bool
    var stuck: Bool
    var art: String?
    var id: String { title + kind + when }
}

struct ActUpcoming: Decodable, Sendable, Identifiable {
    var subject: String
    var when: String
    var kind: String
    var app: String
    var id: String { subject + kind }
}

struct ActAttention: Decodable, Sendable, Identifiable {
    var text: String
    var href: String
    var warn: Bool
    var id: String { text }
}

// MARK: - Services

struct ServicesReading: Decodable, Sendable {
    var configured: Bool
    @Lenient var units: [SvcUnit]
    var down: Int
    @Lenient var timers: [SvcTimer]
    @Lenient var disks: [SvcDisk]
    var smartAge: String
    @Lenient var errors: [String]
}

struct SvcUnit: Decodable, Sendable, Identifiable {
    var name: String
    var state: String
    var ok: Bool
    var since: String
    var id: String { name }
}

struct SvcTimer: Decodable, Sendable, Identifiable {
    var name: String
    var last: String
    var next: String
    var result: String
    var failed: Bool
    var id: String { name }
}

struct SvcDisk: Decodable, Sendable, Identifiable {
    var disk: String
    var model: String
    var asleep: Bool
    var health: String
    var bad: Bool
    @Lenient var facts: [String]
    @Lenient var warn: [String]
    var id: String { disk }
}

// MARK: - Home

/// The start screen: the web's start page, plus the mural's posters.
struct HomeReading: Decodable, Sendable {
    var machine: String
    @Lenient var status: [HomeStat]
    @Lenient var attention: [HomeAttention]
    @Lenient var tiles: [HomeTile]
    @Lenient var recent: [ActRecent]
    @Lenient var mural: [String]
}

struct HomeStat: Decodable, Sendable, Identifiable {
    var text: String
    var tone: String // ok | warn | danger | ""
    var id: String { text }
}

struct HomeAttention: Decodable, Sendable, Identifiable {
    var label: String
    var href: String
    var id: String { label }
}

/// One area. `href` is the web page; the app maps it to its own screen.
struct HomeTile: Decodable, Sendable, Identifiable {
    var href: String
    var tone: String
    var title: String
    var value: String
    var sub: String
    var warn: Bool
    var off: Bool
    var id: String { href }
}

