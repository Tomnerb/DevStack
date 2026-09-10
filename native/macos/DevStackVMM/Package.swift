// swift-tools-version: 5.9

import PackageDescription

let package = Package(
    name: "DevStackVMM",
    platforms: [
        .macOS(.v13)
    ],
    products: [
        .executable(
            name: "devstack-vmm",
            targets: ["DevStackVMM"]
        )
    ],
    targets: [
        .executableTarget(
            name: "DevStackVMM",
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
