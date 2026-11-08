// Draws the 1024px app icon: dark rounded square, blue gradient shield, a
// "hole" punched in the middle. Usage: swift Resources/make-icon.swift out.png
import AppKit

let size: CGFloat = 1024
let out = CommandLine.arguments.dropFirst().first ?? "icon_1024.png"

let image = NSImage(size: NSSize(width: size, height: size))
image.lockFocus()

let inset: CGFloat = 100
let bgRect = NSRect(x: inset, y: inset, width: size - inset * 2, height: size - inset * 2)
let bg = NSBezierPath(roundedRect: bgRect, xRadius: 185, yRadius: 185)
NSGradient(starting: NSColor(calibratedRed: 0.10, green: 0.12, blue: 0.17, alpha: 1),
           ending: NSColor(calibratedRed: 0.05, green: 0.06, blue: 0.09, alpha: 1))?
    .draw(in: bg, angle: -90)

// Shield
let cx = size / 2
let shield = NSBezierPath()
shield.move(to: NSPoint(x: cx, y: 780))
shield.curve(to: NSPoint(x: 290, y: 680), controlPoint1: NSPoint(x: 440, y: 740), controlPoint2: NSPoint(x: 360, y: 700))
shield.curve(to: NSPoint(x: cx, y: 230), controlPoint1: NSPoint(x: 290, y: 450), controlPoint2: NSPoint(x: 380, y: 310))
shield.curve(to: NSPoint(x: 734, y: 680), controlPoint1: NSPoint(x: 644, y: 310), controlPoint2: NSPoint(x: 734, y: 450))
shield.curve(to: NSPoint(x: cx, y: 780), controlPoint1: NSPoint(x: 664, y: 700), controlPoint2: NSPoint(x: 584, y: 740))
shield.close()
NSGradient(starting: NSColor(calibratedRed: 0.42, green: 0.62, blue: 1.0, alpha: 1),
           ending: NSColor(calibratedRed: 0.18, green: 0.40, blue: 0.93, alpha: 1))?
    .draw(in: shield, angle: -90)

// The hole
let holeRect = NSRect(x: cx - 105, y: 420, width: 210, height: 210)
NSGradient(starting: NSColor(calibratedRed: 0.02, green: 0.03, blue: 0.05, alpha: 1),
           ending: NSColor(calibratedRed: 0.10, green: 0.12, blue: 0.17, alpha: 1))?
    .draw(in: NSBezierPath(ovalIn: holeRect), relativeCenterPosition: NSPoint(x: 0, y: 0.3))

image.unlockFocus()

guard let tiff = image.tiffRepresentation,
      let rep = NSBitmapImageRep(data: tiff),
      let png = rep.representation(using: .png, properties: [:]) else {
    fatalError("failed to encode PNG")
}
try png.write(to: URL(fileURLWithPath: out))
print("wrote \(out)")
