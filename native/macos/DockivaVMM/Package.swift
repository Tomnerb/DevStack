// swift-tools-version: 5.9

import PackageDescription

let package = Package(
    name: "DockivaVMM",
    platforms: [
        .macOS(.v13)
    ],
    products: [
        .executable(
            name: "dockiva-vmm",
            targets: ["DockivaVMM"]
        )
    ],
    targets: [
        .executableTarget(
            name: "DockivaVMM",
            exclude: [
                "main.swift.20260910-163040.bak",
                "main.swift.20260910-164234.bak"
            ],
            linkerSettings: [
                .linkedFramework("Virtualization")
            ]
        )
    ]
)
