import Foundation

struct HoleStatus: Decodable {
    let enabled: Bool
    let listDomains: Int
    let totalBlockedDomains: Int
    let cacheEntries: Int
    let lastError: String?
}

struct HoleCount: Decodable {
    let name: String
    let count: Int
}

struct HoleStats: Decodable {
    let total: Int
    let blocked: Int
    let forwarded: Int
    let cached: Int
    let errors: Int
    let blockedPercent: Double
    let topBlocked: [HoleCount]
}

enum HoleError: LocalizedError {
    case unreachable
    case unauthorized
    case api(String)

    var errorDescription: String? {
        switch self {
        case .unreachable: return "서버에 연결할 수 없음"
        case .unauthorized: return "API 토큰이 올바르지 않음"
        case .api(let message): return message
        }
    }
}

private struct Envelope<T: Decodable>: Decodable {
    let success: Bool
    let data: T?
    let error: String?
}

/// Talks to the hole HTTP API. Requests use a short timeout so a dead server
/// never stalls the menu.
final class HoleClient: @unchecked Sendable {
    let baseURL: URL
    private let token: String?
    private let session: URLSession
    private let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.keyDecodingStrategy = .convertFromSnakeCase
        return d
    }()

    init(baseURL: URL, token: String?) {
        self.baseURL = baseURL
        self.token = token
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = 2
        config.timeoutIntervalForResource = 130 // list refresh can take a while
        session = URLSession(configuration: config)
    }

    func status() async throws -> HoleStatus { try await send("GET", "/api/status") }
    func stats() async throws -> HoleStats { try await send("GET", "/api/stats") }

    func setEnabled(_ on: Bool) async throws -> HoleStatus {
        try await send("POST", "/api/status", body: ["enabled": on])
    }

    func refreshLists() async throws -> HoleStatus {
        try await send("POST", "/api/lists/refresh", timeout: 120)
    }

    private func send<T: Decodable>(_ method: String, _ path: String,
                                    body: [String: Any]? = nil,
                                    timeout: TimeInterval = 2) async throws -> T {
        var req = URLRequest(url: baseURL.appendingPathComponent(path))
        req.httpMethod = method
        req.timeoutInterval = timeout
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if let token, !token.isEmpty {
            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        if let body {
            req.httpBody = try JSONSerialization.data(withJSONObject: body)
        }

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: req)
        } catch {
            throw HoleError.unreachable
        }
        if (response as? HTTPURLResponse)?.statusCode == 401 {
            throw HoleError.unauthorized
        }
        let env: Envelope<T>
        do {
            env = try decoder.decode(Envelope<T>.self, from: data)
        } catch {
            throw HoleError.api("응답을 해석할 수 없음: \(error.localizedDescription)")
        }
        guard env.success, let payload = env.data else {
            throw HoleError.api(env.error ?? "알 수 없는 오류")
        }
        return payload
    }
}
