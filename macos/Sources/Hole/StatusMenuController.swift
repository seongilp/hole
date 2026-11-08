import AppKit
import ServiceManagement

/// Owns the menu bar item: polls the server, renders status, handles actions.
@MainActor
final class StatusMenuController: NSObject, NSMenuDelegate {
    private enum State {
        case loading
        case online(HoleStatus, HoleStats)
        case offline(String)
    }

    private static let pollInterval: TimeInterval = 3
    private static let topBlockedCount = 8
    private static let pauseOptions: [(String, TimeInterval)] = [("5분", 300), ("30분", 1800), ("1시간", 3600)]

    private let client: HoleClient
    private let server: ServerProcess
    private let statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
    private let number: NumberFormatter = {
        let f = NumberFormatter()
        f.numberStyle = .decimal
        return f
    }()

    private var state: State = .loading
    private var pollTimer: Timer?
    private var pausedUntil: Date?
    private var resumeTask: Task<Void, Never>?
    private var polling = false
    private var autoStartAttempted = false

    // Menu items updated in place on every poll.
    private let stateItem = NSMenuItem()
    private let statsItem = NSMenuItem()
    private let domainsItem = NSMenuItem()
    private let topBlockedItem = NSMenuItem(title: "많이 차단된 도메인", action: nil, keyEquivalent: "")
    private let toggleItem = NSMenuItem()
    private let pauseItem = NSMenuItem(title: "일시 정지", action: nil, keyEquivalent: "")
    private let refreshItem = NSMenuItem()
    private let serverItem = NSMenuItem()
    private let loginItem = NSMenuItem()

    init(client: HoleClient, server: ServerProcess) {
        self.client = client
        self.server = server
        super.init()
    }

    func start() {
        statusItem.button?.imagePosition = .imageLeading
        statusItem.menu = buildMenu()
        render()
        poll()
        let timer = Timer(timeInterval: Self.pollInterval, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.poll() }
        }
        timer.tolerance = 0.5
        RunLoop.main.add(timer, forMode: .common)
        pollTimer = timer
    }

    // MARK: - Menu construction

    private func buildMenu() -> NSMenu {
        let menu = NSMenu()
        menu.delegate = self
        menu.autoenablesItems = false

        for item in [stateItem, statsItem, domainsItem] {
            item.isEnabled = false
            menu.addItem(item)
        }
        topBlockedItem.submenu = NSMenu()
        menu.addItem(topBlockedItem)
        menu.addItem(.separator())

        toggleItem.target = self
        toggleItem.action = #selector(toggleBlocking)
        toggleItem.keyEquivalent = "b"
        menu.addItem(toggleItem)

        let pauseMenu = NSMenu()
        for (index, option) in Self.pauseOptions.enumerated() {
            let item = NSMenuItem(title: "\(option.0) 동안 끄기", action: #selector(pauseBlocking(_:)), keyEquivalent: "")
            item.target = self
            item.tag = index
            pauseMenu.addItem(item)
        }
        pauseItem.submenu = pauseMenu
        menu.addItem(pauseItem)

        refreshItem.title = "차단 목록 갱신"
        refreshItem.target = self
        refreshItem.action = #selector(refreshLists)
        refreshItem.keyEquivalent = "r"
        menu.addItem(refreshItem)

        menu.addItem(.separator())
        menu.addItem(actionItem("대시보드 열기", #selector(openDashboard), key: "d"))
        menu.addItem(actionItem("서버 로그 보기", #selector(openLog), key: ""))
        serverItem.target = self
        serverItem.action = #selector(toggleServer)
        menu.addItem(serverItem)
        loginItem.title = "로그인 시 실행"
        loginItem.target = self
        loginItem.action = #selector(toggleLoginItem)
        menu.addItem(loginItem)

        menu.addItem(.separator())
        menu.addItem(actionItem("Hole 종료", #selector(quit), key: "q"))
        return menu
    }

    private func actionItem(_ title: String, _ action: Selector, key: String) -> NSMenuItem {
        let item = NSMenuItem(title: title, action: action, keyEquivalent: key)
        item.target = self
        return item
    }

    func menuWillOpen(_ menu: NSMenu) {
        loginItem.state = SMAppService.mainApp.status == .enabled ? .on : .off
        poll()
    }

    // MARK: - Polling

    private func poll() {
        guard !polling else { return }
        polling = true
        Task {
            defer { polling = false }
            do {
                async let status = client.status()
                async let stats = client.stats()
                state = .online(try await status, try await stats)
            } catch {
                state = .offline(server.lastExitMessage ?? error.localizedDescription)
                autoStartIfNeeded(error)
            }
            render()
        }
    }

    /// On first failure to connect, launch the bundled server if configured.
    private func autoStartIfNeeded(_ error: Error) {
        guard !autoStartAttempted, Settings.autoStartServer,
              case HoleError.unreachable = error else { return }
        autoStartAttempted = true
        startServer()
    }

    // MARK: - Rendering

    private func render() {
        renderButton()
        renderMenu()
    }

    private func renderButton() {
        guard let button = statusItem.button else { return }
        let symbol: String
        var title = ""
        switch state {
        case .loading:
            symbol = "shield"
        case .offline:
            symbol = "exclamationmark.shield"
        case .online(let status, let stats):
            symbol = status.enabled ? "shield.lefthalf.filled" : "shield.slash"
            if status.enabled && stats.total > 0 {
                title = String(format: " %.0f%%", stats.blockedPercent)
            }
        }
        let image = NSImage(systemSymbolName: symbol, accessibilityDescription: "Hole")
        image?.isTemplate = true
        button.image = image
        button.title = title
        button.toolTip = stateItem.title
    }

    private func renderMenu() {
        switch state {
        case .loading:
            stateItem.title = "연결 중…"
            setVisible(stats: false)
        case .offline(let message):
            stateItem.title = "⚠︎ \(message)"
            setVisible(stats: false)
        case .online(let status, let stats):
            stateItem.title = stateTitle(enabled: status.enabled)
            statsItem.title = "쿼리 \(fmt(stats.total)) · 차단 \(fmt(stats.blocked)) (\(String(format: "%.1f", stats.blockedPercent))%)"
            domainsItem.title = "차단 도메인 \(fmt(status.totalBlockedDomains))개 · 캐시 \(fmt(status.cacheEntries))"
            toggleItem.title = status.enabled ? "차단 끄기" : "차단 켜기"
            renderTopBlocked(stats.topBlocked)
            setVisible(stats: true)
        }
        let running = server.isRunning
        serverItem.title = running ? "내장 서버 중지" : "내장 서버 시작"
        serverItem.isEnabled = running || !isOnline
    }

    private var isOnline: Bool {
        if case .online = state { return true }
        return false
    }

    private func stateTitle(enabled: Bool) -> String {
        if enabled { return "● 광고 차단 중" }
        if let until = pausedUntil, until > Date() {
            let minutes = max(1, Int(ceil(until.timeIntervalSinceNow / 60)))
            return "○ 일시 정지 (\(minutes)분 후 재개)"
        }
        return "○ 차단 꺼짐"
    }

    private func setVisible(stats visible: Bool) {
        for item in [statsItem, domainsItem, topBlockedItem] { item.isHidden = !visible }
        for item in [toggleItem, pauseItem, refreshItem] { item.isEnabled = visible }
    }

    private func renderTopBlocked(_ top: [HoleCount]) {
        guard let submenu = topBlockedItem.submenu else { return }
        submenu.removeAllItems()
        if top.isEmpty {
            let empty = NSMenuItem(title: "아직 차단된 요청 없음", action: nil, keyEquivalent: "")
            empty.isEnabled = false
            submenu.addItem(empty)
            return
        }
        for entry in top.prefix(Self.topBlockedCount) {
            let item = NSMenuItem(title: "\(entry.name)  —  \(fmt(entry.count))", action: nil, keyEquivalent: "")
            item.isEnabled = false
            submenu.addItem(item)
        }
    }

    private func fmt(_ n: Int) -> String { number.string(from: NSNumber(value: n)) ?? String(n) }

    // MARK: - Actions

    @objc private func toggleBlocking() {
        guard case .online(let status, _) = state else { return }
        cancelPause()
        setEnabled(!status.enabled)
    }

    @objc private func pauseBlocking(_ sender: NSMenuItem) {
        let duration = Self.pauseOptions[sender.tag].1
        cancelPause()
        pausedUntil = Date().addingTimeInterval(duration)
        setEnabled(false)
        resumeTask = Task { [weak self] in
            try? await Task.sleep(for: .seconds(duration))
            guard !Task.isCancelled else { return }
            self?.pausedUntil = nil
            self?.setEnabled(true)
        }
    }

    private func cancelPause() {
        resumeTask?.cancel()
        resumeTask = nil
        pausedUntil = nil
    }

    private func setEnabled(_ on: Bool) {
        Task {
            do {
                _ = try await client.setEnabled(on)
            } catch {
                showError("차단 상태 변경 실패", error)
            }
            poll()
        }
    }

    @objc private func refreshLists() {
        refreshItem.title = "차단 목록 갱신 중…"
        refreshItem.isEnabled = false
        Task {
            do {
                _ = try await client.refreshLists()
            } catch {
                showError("차단 목록 갱신 실패", error)
            }
            refreshItem.title = "차단 목록 갱신"
            refreshItem.isEnabled = true
            poll()
        }
    }

    @objc private func openDashboard() {
        NSWorkspace.shared.open(client.baseURL)
    }

    @objc private func openLog() {
        let url = ServerProcess.logURL
        if FileManager.default.fileExists(atPath: url.path) {
            NSWorkspace.shared.open(url)
        } else {
            showMessage("아직 로그가 없습니다", "내장 서버를 실행하면 \(url.path)에 기록됩니다.")
        }
    }

    @objc private func toggleServer() {
        if server.isRunning {
            server.stop()
            state = .offline("내장 서버 중지됨")
            render()
        } else {
            startServer()
        }
    }

    private static let startupTimeout: TimeInterval = 10

    private func startServer() {
        state = .loading
        render()
        server.start(dnsAddr: Settings.dnsAddr, httpAddr: Settings.httpAddr, onExit: { [weak self] message in
            self?.state = .offline(message)
            self?.render()
        }, completion: { [weak self] error in
            guard let self else { return }
            if let error {
                self.state = .offline("서버 시작 실패: \(error.localizedDescription)")
                self.render()
                return
            }
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.7) { [weak self] in self?.poll() }
            DispatchQueue.main.asyncAfter(deadline: .now() + Self.startupTimeout) { [weak self] in
                self?.warnIfStillStarting()
            }
        })
    }

    /// A server that was launched but never answers is usually stuck in a macOS security check.
    private func warnIfStillStarting() {
        guard server.isRunning, !isOnline else { return }
        state = .offline("서버가 응답하지 않음 — 로그 확인")
        render()
    }

    @objc private func toggleLoginItem() {
        let service = SMAppService.mainApp
        do {
            if service.status == .enabled {
                try service.unregister()
            } else {
                try service.register()
            }
        } catch {
            showError("로그인 항목 설정 실패", error)
        }
        loginItem.state = service.status == .enabled ? .on : .off
    }

    @objc private func quit() {
        NSApp.terminate(nil)
    }

    // MARK: - Alerts

    private func showError(_ title: String, _ error: Error) {
        showMessage(title, error.localizedDescription)
    }

    private func showMessage(_ title: String, _ detail: String) {
        NSApp.activate(ignoringOtherApps: true)
        let alert = NSAlert()
        alert.messageText = title
        alert.informativeText = detail
        alert.runModal()
    }
}
