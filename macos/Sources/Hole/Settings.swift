import Foundation

/// User-tunable settings, overridable with `defaults write com.zihado.Hole <key> <value>`.
enum Settings {
    private static let defaults = UserDefaults.standard

    static func registerDefaults() {
        defaults.register(defaults: [
            "httpAddr": "127.0.0.1:8080",
            "dnsAddr": ":53",
            "autoStartServer": true,
        ])
    }

    static var httpAddr: String { defaults.string(forKey: "httpAddr") ?? "127.0.0.1:8080" }
    static var dnsAddr: String { defaults.string(forKey: "dnsAddr") ?? ":53" }
    static var apiToken: String? { defaults.string(forKey: "apiToken") }
    static var autoStartServer: Bool { defaults.bool(forKey: "autoStartServer") }

    static var baseURL: URL {
        URL(string: "http://\(httpAddr)") ?? URL(string: "http://127.0.0.1:8080")!
    }
}
