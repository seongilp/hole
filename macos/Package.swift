// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "Hole",
    platforms: [
        .macOS(.v13)
    ],
    targets: [
        .executableTarget(
            name: "Hole",
            path: "Sources/Hole",
            swiftSettings: [
                .unsafeFlags(["-O"], .when(configuration: .release))
            ]
        )
    ]
)
