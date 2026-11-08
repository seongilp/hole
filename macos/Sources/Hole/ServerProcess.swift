import Foundation

/// Runs the bundled `hole` binary (Contents/MacOS/hole-server — a distinct name because APFS is
/// case-insensitive and "hole" would overwrite the "Hole" app binary) as a child process.
/// Only a server started by this app is ever stopped by it.
final class ServerProcess {
    private var process: Process?
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

    /// Starts the server. `onExit` runs on the main queue if it stops.
    func start(dnsAddr: String, httpAddr: String, onExit: @escaping (String) -> Void) throws {
        guard !isRunning else { return }
        guard let binary = Self.binaryURL else {
            throw HoleError.api("앱 번들에 hole 서버가 없음")
        }
        let fm = FileManager.default
        try fm.createDirectory(at: Self.dataDir, withIntermediateDirectories: true)
        try fm.createDirectory(at: Self.logURL.deletingLastPathComponent(), withIntermediateDirectories: true)
        if !fm.fileExists(atPath: Self.logURL.path) {
            fm.createFile(atPath: Self.logURL.path, contents: nil)
        }
        let log = try FileHandle(forWritingTo: Self.logURL)
        log.seekToEndOfFile()

        let p = Process()
        p.executableURL = binary
        p.arguments = ["-dns", dnsAddr, "-http", httpAddr, "-data", Self.dataDir.path]
        p.standardOutput = log
        p.standardError = log
        p.terminationHandler = { [weak self] proc in
            let message = "서버 종료됨 (코드 \(proc.terminationStatus)) — 로그 확인"
            DispatchQueue.main.async {
                self?.lastExitMessage = message
                onExit(message)
            }
        }
        try p.run()
        process = p
        lastExitMessage = nil
    }

    func stop() {
        guard let p = process, p.isRunning else { return }
        p.terminationHandler = nil
        p.terminate()
        p.waitUntilExit()
        process = nil
    }
}
