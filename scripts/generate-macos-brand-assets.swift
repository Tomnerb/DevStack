#!/usr/bin/env swift

import AppKit
import Foundation

let root = URL(fileURLWithPath: FileManager.default.currentDirectoryPath)
let logoURL = root.appendingPathComponent("assets/devstack_icon2.png")
let outputURL = root.appendingPathComponent("build/darwin/dmg-background.png")

guard let logo = NSImage(contentsOf: logoURL) else {
    fputs("Could not load \(logoURL.path)\n", stderr)
    exit(1)
}

func color(_ red: CGFloat, _ green: CGFloat, _ blue: CGFloat, _ alpha: CGFloat = 1) -> NSColor {
    NSColor(srgbRed: red / 255, green: green / 255, blue: blue / 255, alpha: alpha)
}

let size = NSSize(width: 540, height: 380)
let canvas = NSImage(size: size)
canvas.lockFocus()

let bounds = NSRect(origin: .zero, size: size)
NSGradient(colors: [color(2, 8, 23), color(3, 19, 46)])?.draw(in: bounds, angle: -18)

let glow = NSBezierPath(ovalIn: NSRect(x: 305, y: 155, width: 330, height: 330))
color(10, 104, 255, 0.16).setFill()
glow.fill()

let cyanGlow = NSBezierPath(ovalIn: NSRect(x: -130, y: -145, width: 350, height: 350))
color(0, 229, 255, 0.08).setFill()
cyanGlow.fill()

color(247, 249, 252, 0.09).setStroke()
let topLine = NSBezierPath()
topLine.move(to: NSPoint(x: 0, y: 379))
topLine.line(to: NSPoint(x: 540, y: 379))
topLine.lineWidth = 1
topLine.stroke()

logo.draw(
    in: NSRect(x: 35, y: 296, width: 54, height: 54),
    from: .zero,
    operation: .sourceOver,
    fraction: 1,
    respectFlipped: true,
    hints: [.interpolation: NSImageInterpolation.high]
)

let titleStyle: [NSAttributedString.Key: Any] = [
    .font: NSFont.systemFont(ofSize: 27, weight: .bold),
    .foregroundColor: color(247, 249, 252),
    .kern: -0.5,
]
("DevStack" as NSString).draw(at: NSPoint(x: 101, y: 315), withAttributes: titleStyle)

let eyebrowStyle: [NSAttributedString.Key: Any] = [
    .font: NSFont.systemFont(ofSize: 9, weight: .semibold),
    .foregroundColor: color(41, 243, 255),
    .kern: 1.7,
]
("CONTAINER WORKSPACE FOR MAC" as NSString).draw(at: NSPoint(x: 103, y: 299), withAttributes: eyebrowStyle)

let panelRect = NSRect(x: 26, y: 78, width: 488, height: 194)
let panel = NSBezierPath(roundedRect: panelRect, xRadius: 24, yRadius: 24)
color(247, 249, 252, 0.055).setFill()
panel.fill()
color(247, 249, 252, 0.12).setStroke()
panel.lineWidth = 1
panel.stroke()

let arrow = NSBezierPath()
arrow.move(to: NSPoint(x: 231, y: 177))
arrow.line(to: NSPoint(x: 308, y: 177))
arrow.move(to: NSPoint(x: 293, y: 192))
arrow.line(to: NSPoint(x: 308, y: 177))
arrow.line(to: NSPoint(x: 293, y: 162))
arrow.lineCapStyle = .round
arrow.lineJoinStyle = .round
arrow.lineWidth = 3
color(0, 229, 255, 0.85).setStroke()
arrow.stroke()

let instructionParagraph = NSMutableParagraphStyle()
instructionParagraph.alignment = .center
let instructionStyle: [NSAttributedString.Key: Any] = [
    .font: NSFont.systemFont(ofSize: 18, weight: .semibold),
    .foregroundColor: color(247, 249, 252, 0.94),
    .paragraphStyle: instructionParagraph,
]
("Drag DevStack to Applications" as NSString).draw(
    in: NSRect(x: 80, y: 95, width: 380, height: 28),
    withAttributes: instructionStyle
)

let footerParagraph = NSMutableParagraphStyle()
footerParagraph.alignment = .center
let footerStyle: [NSAttributedString.Key: Any] = [
    .font: NSFont.systemFont(ofSize: 11, weight: .medium),
    .foregroundColor: color(247, 249, 252, 0.48),
    .paragraphStyle: footerParagraph,
    .kern: 0.2,
]
("Native containers. Clean workspace. Built for macOS." as NSString).draw(
    in: NSRect(x: 50, y: 31, width: 440, height: 18),
    withAttributes: footerStyle
)

canvas.unlockFocus()

guard
    let tiff = canvas.tiffRepresentation,
    let bitmap = NSBitmapImageRep(data: tiff),
    let png = bitmap.representation(using: .png, properties: [:])
else {
    fputs("Could not render DMG background\n", stderr)
    exit(1)
}

try png.write(to: outputURL, options: .atomic)
print("Generated \(outputURL.path)")
