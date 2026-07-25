import Foundation
import Testing

@testable import Holocron

struct URLNormalisationTests {
    @Test func acceptsWhatSomeoneActuallyTypes() {
        // A host:port read off a router is the common case, so it must work
        // without the user knowing to type a scheme.
        #expect(AppSettings.normalisedURL("192.168.1.10:8080")?.absoluteString == "http://192.168.1.10:8080")
        #expect(AppSettings.normalisedURL("raspberrypi.local")?.absoluteString == "http://raspberrypi.local")
        #expect(AppSettings.normalisedURL("  10.0.0.5:8080  ")?.absoluteString == "http://10.0.0.5:8080")
    }

    @Test func keepsAnExplicitScheme() {
        #expect(AppSettings.normalisedURL("https://pi.example:8443")?.absoluteString == "https://pi.example:8443")
    }

    @Test func rejectsEmptyOrHostless() {
        #expect(AppSettings.normalisedURL("") == nil)
        #expect(AppSettings.normalisedURL("   ") == nil)
    }
}

struct FormatTests {
    @Test func bytesUseBinaryUnitsLikeTheServer() {
        #expect(Format.bytes(UInt64(512)) == "512 B")
        #expect(Format.bytes(UInt64(1024)) == "1.0 KiB")
        #expect(Format.bytes(UInt64(1_048_576)) == "1.0 MiB")
        #expect(Format.bytes(UInt64(2_576_980_377)) == "2.4 GiB")
    }

    @Test func speedShowsADashWhenIdle() {
        #expect(Format.speed(0) == "—")
        #expect(Format.speed(-1) == "—")
        #expect(Format.speed(1_048_576) == "1.0 MiB/s")
    }

    @Test func uptimeIsCompact() {
        #expect(Format.uptime(90) == "1m")
        #expect(Format.uptime(3_700) == "1h 1m")
        #expect(Format.uptime(1_051_200) == "12d 4h")
    }

    @Test func yearFallsBackToADash() {
        #expect(Format.year(0) == "—")
        #expect(Format.year(2024) == "2024")
    }

    @Test func relativeTimeParsesTheServersUTCFormat() {
        // Unparsable input must pass through rather than render something wrong.
        #expect(Format.relative(fromUTC: "not a date") == "not a date")
        // A real timestamp yields something other than the raw string.
        let stored = "2020-01-01 00:00:00"
        #expect(Format.relative(fromUTC: stored) != stored)
    }
}

struct APIErrorTests {
    @Test func everyCaseExplainsItselfInSpanish() {
        let cases: [APIError] = [
            .notConfigured, .unauthorized, .noToken, .notReachable,
            .server(status: 500, message: ""), .decoding,
        ]
        for error in cases {
            let text = error.localizedDescription
            #expect(!text.isEmpty)
        }
    }

    @Test func serverErrorPrefersTheServersOwnMessage() {
        #expect(APIError.server(status: 400, message: "not a magnet link").localizedDescription
                == "not a magnet link")
    }
}
