#!/usr/bin/env swift
// Run from any directory: swift desktop/branding/generate.swift
// SVG and PNG share the geometry below. No external renderer or image service.
import AppKit

let directory = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
let nativeDirectory = directory.deletingLastPathComponent().appendingPathComponent("internal/host/assets/brand")
let graphite = "#303832"
let sage = "#739873"
let tile = "#F3F4EF"
let outline = "#DFE2D9"
let ring = "M51.696155 28.527036 A20 20 0 1 1 35.472964 12.303845"

func geometry(node: String) -> String {
    """
    <path d="\(ring)" fill="none" stroke="\(graphite)" stroke-width="6.5" stroke-linecap="round"/>
    <circle cx="46.142136" cy="17.857864" r="4.3" fill="\(node)"/>
    """
}

func svg(_ body: String, width: Int = 64, height: Int = 64) -> String {
    """
    <svg xmlns="http://www.w3.org/2000/svg" width="\(width)" height="\(height)" viewBox="0 0 \(width) \(height)">
    \(body)
    </svg>

    """
}

func write(_ name: String, _ value: String, to folder: URL = directory) throws {
    try value.write(to: folder.appendingPathComponent(name), atomically: true, encoding: .utf8)
}

func color(_ hex: String) -> CGColor {
    let rgb = UInt32(hex.dropFirst(), radix: 16)!
    return CGColor(srgbRed: CGFloat((rgb >> 16) & 255) / 255,
                   green: CGFloat((rgb >> 8) & 255) / 255,
                   blue: CGFloat(rgb & 255) / 255, alpha: 1)
}

func drawMark(_ context: CGContext, monochrome: Bool = false) {
    context.setStrokeColor(color(graphite))
    context.setLineWidth(6.5)
    context.setLineCap(.round)
    context.addArc(center: CGPoint(x: 32, y: 32), radius: 20,
                   startAngle: -.pi / 18, endAngle: 14 * .pi / 9, clockwise: false)
    context.strokePath()
    context.setFillColor(color(monochrome ? graphite : sage))
    context.fillEllipse(in: CGRect(x: 46.142136 - 4.3, y: 17.857864 - 4.3, width: 8.6, height: 8.6))
}

func png(size: Int, icon: Bool, monochrome: Bool = false) throws -> Data {
    let context = CGContext(data: nil, width: size, height: size,
                            bitsPerComponent: 8, bytesPerRow: size * 4,
                            space: CGColorSpace(name: CGColorSpace.sRGB)!,
                            bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
    context.setAllowsAntialiasing(true)
    context.translateBy(x: 0, y: CGFloat(size))
    context.scaleBy(x: CGFloat(size) / (icon ? 1024 : 64), y: -CGFloat(size) / (icon ? 1024 : 64))
    if icon {
        let path = CGPath(roundedRect: CGRect(x: 100, y: 100, width: 824, height: 824),
                          cornerWidth: 184, cornerHeight: 184, transform: nil)
        context.addPath(path)
        context.setFillColor(color(tile))
        context.setStrokeColor(color(outline))
        context.setLineWidth(2.5)
        context.drawPath(using: .fillStroke)
        context.translateBy(x: 160, y: 160)
        context.scaleBy(x: 11, y: 11)
    }
    drawMark(context, monochrome: monochrome)
    let image = NSBitmapImageRep(cgImage: context.makeImage()!)
    guard let data = image.representation(using: .png, properties: [:]) else {
        throw NSError(domain: "OpenMPA.Brand", code: 1)
    }
    return data
}

try FileManager.default.createDirectory(at: nativeDirectory, withIntermediateDirectories: true)
try write("mark.svg", svg(geometry(node: sage)))
try write("mark.svg", svg(geometry(node: graphite).replacingOccurrences(of: graphite, with: "currentColor")), to: nativeDirectory)
try write("logo.svg", svg("""
<g transform="translate(0 4) scale(0.875)">\(geometry(node: sage))</g>
<text x="68" y="44" fill="\(graphite)" font-family="-apple-system, BlinkMacSystemFont, Helvetica, Arial, sans-serif" font-size="34" font-weight="600" letter-spacing="-1">OpenMPA</text>
""", width: 256))
try write("app-icon.svg", svg("""
<rect x="100" y="100" width="824" height="824" rx="184" fill="\(tile)" stroke="\(outline)" stroke-width="2.5"/>
<g transform="translate(160 160) scale(11)">\(geometry(node: sage))</g>
""", width: 1024, height: 1024))
try png(size: 1024, icon: true).write(to: directory.appendingPathComponent("app-icon.png"), options: .atomic)
print("Generated editable SVG, native monochrome mark and 1024px app icon.")
