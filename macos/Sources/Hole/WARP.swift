import Foundation

/// Cloudflare WARP coexistence. In its DNS modes WARP owns the system resolver (stub
/// 127.0.2.2/3) and fights anything else that sets DNS; in "Traffic only" (tunnel_only)
/// it leaves DNS alone, so Hole filters names while WARP still tunnels the traffic.
/// Calls are blocking: run them off the main thread.
enum WARP {
    /// Local proxy addresses WARP writes into every network service while it owns DNS.
    static let stubs: Set<String> = ["127.0.2.2", "127.0.2.3", "fd01:db8:1111::2", "fd01:db8:1111::3"]

    private static let candidates = [
        "/usr/local/bin/warp-cli",
        "/Applications/Cloudflare WARP.app/Contents/Resources/warp-cli",
    ]
    private static let commandTimeout: TimeInterval = 4
    private static let releaseTimeout: TimeInterval = 6
    private static let trafficOnlyMode = "tunnel_only"

    enum Failure: LocalizedError {
        case holdsDNS
        case command(String)
        var errorDescription: String? {
            switch self {
            case .holdsDNS:
                return "Cloudflare WARP가 DNS를 맡고 있습니다. WARP를 'Traffic only' 모드로 바꾼 뒤 다시 시도하세요."
            case .command(let msg):
                return "WARP 모드 변경 실패: \(msg)\n회사가 관리하는 WARP라면 관리자 정책 때문에 바꿀 수 없습니다."
            }
        }
    }

    static func isStub(_ server: String) -> Bool { stubs.contains(server.lowercased()) }

    /// True while WARP is connected in a mode that takes over the system resolver.
    static func holdsDNS() -> Bool {
        guard let cli = cliPath(),
              let status = try? run(cli, ["status"]), isConnected(status),
              let settings = try? run(cli, ["settings"]) else { return false }
        return proxiesDNS(mode: mode(fromSettings: settings))
    }

    /// Switches WARP to Traffic only, then waits for it to hand the resolver back.
    static func switchToTrafficOnly(stubsGone: () -> Bool) throws {
        guard let cli = cliPath() else { throw Failure.command("warp-cli를 찾을 수 없음") }
        try run(cli, ["mode", trafficOnlyMode])
        let deadline = Date().addingTimeInterval(releaseTimeout)
        while !stubsGone() && Date() < deadline {
            usleep(300_000)
        }
    }

    // MARK: - Parsing

    /// `warp-cli status` prints "Status update: Connected".
    static func isConnected(_ status: String) -> Bool {
        for line in status.split(separator: "\n") {
            if let range = line.range(of: "Status update:") {
                return line[range.upperBound...].trimmingCharacters(in: .whitespaces) == "Connected"
            }
        }
        return false
    }

    /// The "Mode:" line of `warp-cli settings` ("(user set)\tMode: WarpTunnelOnly"), letters only.
    static func mode(fromSettings settings: String) -> String {
        for line in settings.split(separator: "\n") {
            if let range = line.range(of: "\tMode:") {
                return line[range.upperBound...].lowercased().filter { $0.isLetter || $0 == "+" }
            }
        }
        return ""
    }

    /// An unknown mode is not treated as a conflict: never block the user on a parse miss.
    static func proxiesDNS(mode: String) -> Bool {
        !mode.isEmpty && !mode.contains("tunnelonly") && !mode.contains("proxy")
    }

    // MARK: - Process

    private static func cliPath() -> String? {
        candidates.first { FileManager.default.isExecutableFile(atPath: $0) }
    }

    @discardableResult
    private static func run(_ cli: String, _ args: [String]) throws -> String {
        let p = Process()
        p.executableURL = URL(fileURLWithPath: cli)
        p.arguments = ["--accept-tos"] + args
        let out = Pipe()
        p.standardOutput = out
        p.standardError = out
        try p.run()
        // warp-cli can hang when its daemon is down; never let that stall the DNS queue.
        let watchdog = DispatchWorkItem { if p.isRunning { p.terminate() } }
        DispatchQueue.global().asyncAfter(deadline: .now() + commandTimeout, execute: watchdog)
        let data = out.fileHandleForReading.readDataToEndOfFile()
        p.waitUntilExit()
        watchdog.cancel()
        let text = String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
        guard p.terminationStatus == 0 else {
            throw Failure.command(text.isEmpty ? "warp-cli \(args.joined(separator: " ")) 실패" : text)
        }
        return text
    }
}
