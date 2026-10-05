import SwiftUI

/// Keeps a screen's data fresh while it is on screen, and only then.
///
/// The web gets these readings over Server-Sent Events. On the phone a short
/// poll is simpler and costs the same where it matters: the server samples only
/// while somebody asks, and this asks only while the view is visible — `.task`
/// is cancelled the moment it leaves, so a screen in a background tab costs
/// nothing.
struct LivePoll: ViewModifier {
    let every: Duration
    let refresh: @MainActor () async -> Void

    func body(content: Content) -> some View {
        content.task {
            while !Task.isCancelled {
                await refresh()
                try? await Task.sleep(for: every)
            }
        }
    }
}

extension View {
    func livePoll(every: Duration, _ refresh: @escaping @MainActor () async -> Void) -> some View {
        modifier(LivePoll(every: every, refresh: refresh))
    }
}

/// A sparkline drawn from the server's polyline, which is laid out on a
/// 100×24 canvas with the newest point on the right.
struct SparkLine: View {
    let spark: Spark
    var color: Color = Palette.accent

    var body: some View {
        let pts = spark.values
        if pts.count > 1 {
            VStack(alignment: .trailing, spacing: 2) {
                Text(spark.max).font(.caption2).foregroundStyle(Palette.muted).monospacedDigit()
                GeometryReader { geo in
                    Path { p in
                        for (i, pt) in pts.enumerated() {
                            let x = pt.x / 100 * geo.size.width
                            let y = pt.y / 24 * geo.size.height
                            if i == 0 { p.move(to: CGPoint(x: x, y: y)) } else { p.addLine(to: CGPoint(x: x, y: y)) }
                        }
                    }
                    .stroke(color, lineWidth: 1.5)
                }
                .frame(height: 44)
            }
        }
    }
}

/// One label/value row inside a card.
struct StatRow: View {
    let key: String
    let value: String

    var body: some View {
        HStack {
            Text(key).font(.subheadline).foregroundStyle(Palette.muted)
            Spacer()
            Text(value).font(.subheadline.weight(.semibold)).monospacedDigit()
        }
    }
}
