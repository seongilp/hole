import Foundation

/// User-tunable settings, overridable with `defaults write com.zihado.Hole <key> <value>`.
enum Settings {
    /// 4653 spells HOLE on a phone keypad; deliberately not a common dev port.
    static let defaultHTTPAddr = "127.0.0.1:4653"
    private static let defaults = UserDefaults.standard

    static func registerDefaults() {
        defaults.register(defaults: [
            "httpAddr": defaultHTTPAddr,
            "dnsAddr": ":53",
            "autoStartServer": true,
            "manageDNS": false,
        ])
    }

    static var httpAddr: String { defaults.string(forKey: "httpAddr") ?? defaultHTTPAddr }
    static var dnsAddr: String { defaults.string(forKey: "dnsAddr") ?? ":53" }
    static var apiToken: String? { defaults.string(forKey: "apiToken") }
    static var autoStartServer: Bool { defaults.bool(forKey: "autoStartServer") }
    /// The user asked Hole to keep this Mac's DNS pointed at it while the server is up.
    static var manageDNS: Bool {
        get { defaults.bool(forKey: "manageDNS") }
        set { defaults.set(newValue, forKey: "manageDNS") }
    }

    static var baseURL: URL {
        URL(string: "http://\(httpAddr)") ?? URL(string: "http://\(defaultHTTPAddr)")!
    }
}
