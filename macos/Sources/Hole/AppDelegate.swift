import AppKit

final class AppDelegate: NSObject, NSApplicationDelegate {
    private var controller: StatusMenuController?
    private let server = ServerProcess()

    func applicationDidFinishLaunching(_ notification: Notification) {
        Settings.registerDefaults()
        let client = HoleClient(baseURL: Settings.baseURL, token: Settings.apiToken)
        controller = StatusMenuController(client: client, server: server)
        controller?.start()
    }

    func applicationWillTerminate(_ notification: Notification) {
        // Restore DNS first: with DNS on 127.0.0.1 and no server, this Mac loses name resolution.
        if Settings.manageDNS {
            try? DNSManager.restore()
        }
        // Only stops a server this app launched; an external one keeps running.
        server.stop()
    }
}
