import Foundation

/// Points macOS DNS at Hole (127.0.0.1) on every enabled network service, and restores
/// each service's previous servers afterwards. Calls are blocking: run them off the main thread.
enum DNSManager {
    static let holeServer = "127.0.0.1"
    private static let tool = "/usr/sbin/networksetup"
    private static let savedKey = "savedDNSServers" // [service: [server]], [] = automatic (DHCP)

    enum Failure: LocalizedError {
        case command(String)
        var errorDescription: String? {
            switch self { case .command(let msg): return msg }
        }
    }

    /// True if any enabled network service currently uses Hole for DNS.
    static func isPointedAtHole() -> Bool {
        guard let all = try? currentServers() else { return false }
        return all.values.contains { $0.first == holeServer }
    }

    /// Sets DNS to Hole, remembering each service's original servers first.
    static func pointAtHole() throws {
        let current = try currentServers()
        var saved = savedServers()
        for (service, servers) in current where servers.first != holeServer {
            saved[service] = servers
        }
        UserDefaults.standard.set(saved, forKey: savedKey)
        try forEach(Array(current.keys)) { try run(["-setdnsservers", $0, holeServer]) }
    }

    /// Restores the servers saved by `pointAtHole` (automatic/DHCP if none were saved).
    static func restore() throws {
        let saved = savedServers()
        let onHole = try currentServers().filter { $0.value.first == holeServer }.map(\.key)
        try forEach(onHole) { service in
            let previous = saved[service] ?? []
            try run(["-setdnsservers", service] + (previous.isEmpty ? ["empty"] : previous))
        }
        UserDefaults.standard.removeObject(forKey: savedKey)
    }

    /// DNS servers of every enabled service, fetched in parallel.
    private static func currentServers() throws -> [String: [String]] {
        let names = try services()
        var result: [String: [String]] = [:]
        let lock = NSLock()
        try forEach(names) { service in
            let servers = try servers(of: service)
            lock.lock(); result[service] = servers; lock.unlock()
        }
        return result
    }

    /// Runs `body` for each item concurrently; networksetup calls take ~0.2s each.
    private static func forEach(_ items: [String], _ body: (String) throws -> Void) throws {
        var firstError: Error?
        let lock = NSLock()
        DispatchQueue.concurrentPerform(iterations: items.count) { i in
            do {
                try body(items[i])
            } catch {
                lock.lock(); if firstError == nil { firstError = error }; lock.unlock()
            }
        }
        if let firstError { throw firstError }
    }

    private static func savedServers() -> [String: [String]] {
        UserDefaults.standard.dictionary(forKey: savedKey) as? [String: [String]] ?? [:]
    }

    /// Enabled network services (disabled ones are prefixed with "*").
    private static func services() throws -> [String] {
        try run(["-listallnetworkservices"])
            .split(separator: "\n")
            .dropFirst() // explanatory header line
            .map(String.init)
            .filter { !$0.hasPrefix("*") && !$0.isEmpty }
    }

    /// Manually configured servers; [] when the service uses DHCP-provided DNS.
    private static func servers(of service: String) throws -> [String] {
        let out = try run(["-getdnsservers", service])
        if out.contains("aren't any DNS Servers") { return [] }
        return out.split(separator: "\n").map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
    }

    @discardableResult
    private static func run(_ args: [String]) throws -> String {
        let p = Process()
        p.executableURL = URL(fileURLWithPath: tool)
        p.arguments = args
        let out = Pipe()
        p.standardOutput = out
        p.standardError = out
        try p.run()
        let data = out.fileHandleForReading.readDataToEndOfFile()
        p.waitUntilExit()
        let text = String(decoding: data, as: UTF8.self)
        guard p.terminationStatus == 0, !text.contains("Error") else {
            throw Failure.command("networksetup \(args.first ?? ""): \(text.trimmingCharacters(in: .whitespacesAndNewlines))")
        }
        return text
    }
}
