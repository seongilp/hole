import Foundation

/// Runs the bundled server binary (Contents/MacOS/hole-server — a distinct name because APFS is
/// case-insensitive and "hole" would overwrite the "Hole" app binary) as a child process.
/// Only a server started by this app is ever stopped by it.
///
/// Launching never blocks the main thread: exec can stall while macOS assesses the binary
/// (Gatekeeper), and a stalled child cannot handle SIGTERM, so stop() escalates to SIGKILL.
final class ServerProcess {
    private static let stopGrace: TimeInterval = 2
    private let queue = DispatchQueue(label: "com.zihado.Hole.server")
    private var process: Process? // touched only on the main thread
    private(set) var lastExitMessage: String?

    var isRunning: Bool { process?.isRunning ?? false }

    static var binaryURL: URL? { Bundle.main.url(forAuxiliaryExecutable: "hole-server") }

    static var logURL: URL {
        FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Logs/Hole/hole.log")
    }

    private static var dataDir: URL {
        FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("Hole", isDirectory: true)
    }

    /// Launches the server off the main thread. `completion` (nil on success) and `onExit`
    /// both run on the main queue.
    func start(dnsAddr: String, httpAddr: String,
               onExit: @escaping (String) -> Void,
               completion: @escaping (Error?) -> Void) {
        guard !isRunning else { return completion(nil) }
        queue.async {
            do {
                let p = try Self.makeProcess(dnsAddr: dnsAddr, httpAddr: httpAddr)
                p.terminationHandler = { [weak self] proc in
                    let message = "서버 종료됨 (코드 \(proc.terminationStatus)) — 로그 확인"
                    DispatchQueue.main.async {
                        self?.lastExitMessage = message
                        self?.process = nil
                        onExit(message)
                    }
                }
                try p.run()
                DispatchQueue.main.async {
                    self.process = p
                    self.lastExitMessage = nil
                    completion(nil)
                }
            } catch {
                DispatchQueue.main.async { completion(error) }
            }
        }
    }

    private static func makeProcess(dnsAddr: String, httpAddr: String) throws -> Process {
        guard let binary = binaryURL else {
            throw HoleError.api("앱 번들에 hole 서버가 없음")
        }
        let fm = FileManager.default
        try fm.createDirectory(at: dataDir, withIntermediateDirectories: true)
        try fm.createDirectory(at: logURL.deletingLastPathComponent(), withIntermediateDirectories: true)
        if !fm.fileExists(atPath: logURL.path) {
            fm.createFile(atPath: logURL.path, contents: nil)
        }
        let log = try FileHandle(forWritingTo: logURL)
        log.seekToEndOfFile()

        let p = Process()
        p.executableURL = binary
        p.arguments = ["-dns", dnsAddr, "-http", httpAddr, "-data", dataDir.path]
        p.standardOutput = log
        p.standardError = log
        return p
    }

    /// Stops the server: SIGTERM, then SIGKILL if it has not exited within the grace period.
    func stop() {
        guard let p = process, p.isRunning else { return }
        p.terminationHandler = nil
        p.terminate()
        let deadline = Date().addingTimeInterval(Self.stopGrace)
        while p.isRunning && Date() < deadline {
            usleep(50_000)
        }
        if p.isRunning {
            kill(p.processIdentifier, SIGKILL)
        }
        process = nil
    }
}
