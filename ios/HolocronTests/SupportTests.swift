import Foundation
import Testing

@testable import Holocron

struct URLNormalisationTests {
    @Test func acceptsWhatSomeoneActuallyTypes() {
        // A host:port read off a router is the common case, so it must work
        // without the user knowing to type a scheme.
        #expect(AppSettings.normalisedURL("192.168.1.10:8090")?.absoluteString == "http://192.168.1.10:8090")
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
            .notConfigured, .unauthorized, .noToken, .notReachable, .accessDenied,
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


/// Cloudflare Access does not fail like the API does: URLSession follows its
/// redirect and hands back the login page with status 200, so without this the
/// user would see "el servidor respondió algo inesperado" and go looking in the
/// wrong place.
struct AccessChallengeTests {
    private func response(_ url: String, _ status: Int, contentType: String?,
                          challenge: String? = nil, accessAUD: Bool = false) -> HTTPURLResponse {
        var headers: [String: String] = [:]
        if let contentType { headers["Content-Type"] = contentType }
        if let challenge { headers["WWW-Authenticate"] = challenge }
        if accessAUD {
            headers["cf-access-aud"] = String(repeating: "d", count: 64)
            headers["cf-access-domain"] = "holocron.merli.store"
        }
        return HTTPURLResponse(url: URL(string: url)!, statusCode: status,
                               httpVersion: nil, headerFields: headers)!
    }

    @Test func serviceAuthRefusalIsCaughtEvenWhenItAnswersJSON() {
        // The measured shape of a Service Auth refusal when the client asks for
        // JSON: 403, application/json, {"message":"Forbidden…"} — no redirect
        // and no WWW-Authenticate. Every rule except the cf-access headers
        // misses this one, and it is the request the app actually makes.
        let http = response("https://holocron.merli.store/api/v1/system", 403,
                            contentType: "application/json; charset=utf-8", accessAUD: true)
        #expect(APIClient.isAccessChallenge(http))
    }

    @Test func serviceAuthRefusalInHTMLIsCaughtToo() {
        // Same refusal without an explicit Accept, which is what URLSession
        // sends by default.
        let http = response("https://holocron.merli.store/api/v1/system", 403,
                            contentType: "text/html", accessAUD: true)
        #expect(APIClient.isAccessChallenge(http))
    }

    @Test func theLoginHostIsAChallenge() {
        let http = response("https://example.cloudflareaccess.com/cdn-cgi/access/login/x", 200,
                            contentType: "text/html; charset=utf-8")
        #expect(APIClient.isAccessChallenge(http))
    }

    @Test func theChallengeHeaderIsEnoughOnItsOwn() {
        // What Access actually answers to an API-shaped request with no
        // credentials: 401, HTML, and this header. Measured, not assumed.
        let http = response("https://holocron.merli.store/api/v1/system", 401,
                            contentType: "text/html; charset=UTF-8",
                            challenge: #"Cloudflare-Access resource_metadata="https://holocron.merli.store/.well-known""#)
        #expect(APIClient.isAccessChallenge(http))
    }

    @Test func aRedirectToTheLoginIsAChallenge() {
        // And this is what it answers when the service token headers are wrong
        // — the likely case in practice, someone mis-pasting the secret. The
        // status is 302, which is why the rule cannot be a list of codes.
        let http = response("https://holocron.merli.store/api/v1/system", 302,
                            contentType: "text/html; charset=UTF-8")
        #expect(APIClient.isAccessChallenge(http))
    }

    @Test func htmlOnAForbiddenIsAChallenge() {
        let http = response("https://holocron.merli.store/api/v1/system", 403,
                            contentType: "text/html")
        #expect(APIClient.isAccessChallenge(http))
    }

    @Test func holocronsOwnUnauthorizedIsNot() {
        // Holocron answers 401 as JSON when the bearer token is wrong. Calling
        // that an Access problem would send the user to fix the wrong setting.
        let http = response("https://holocron.merli.store/api/v1/system", 401,
                            contentType: "application/json")
        #expect(!APIClient.isAccessChallenge(http))
    }

    @Test func aNormalResponseIsNot() {
        let http = response("https://holocron.merli.store/api/v1/system", 200,
                            contentType: "application/json")
        #expect(!APIClient.isAccessChallenge(http))
    }

    @Test func aResponseWithoutAContentTypeIsNot() {
        let http = response("https://holocron.merli.store/api/v1/media/sync", 202, contentType: nil)
        #expect(!APIClient.isAccessChallenge(http))
    }
}
