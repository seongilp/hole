import AppKit

/// Menu bar icon: the app icon's shield with a hole. The hole lights up green while blocking.
enum StatusIcon {
    enum Kind {
        case blocking   // blue shield, green light
        case paused     // monochrome shield, empty hole
        case problem    // blue shield, red light
        case connecting // monochrome outline
    }

    private static let size = NSSize(width: 16, height: 18)
    // Source geometry from Resources/make-icon.swift (1024px canvas).
    private static let srcMinX: CGFloat = 290, srcMinY: CGFloat = 230, srcHeight: CGFloat = 550
    private static let scale = (size.height - 2) / srcHeight
    private static let shieldBlue = NSColor(calibratedRed: 0.30, green: 0.52, blue: 0.98, alpha: 1)
    private static let green = NSColor(calibratedRed: 0.20, green: 0.84, blue: 0.35, alpha: 1)
    private static let red = NSColor(calibratedRed: 1.0, green: 0.27, blue: 0.23, alpha: 1)

    private static var cache: [Kind: NSImage] = [:]

    static func image(_ kind: Kind) -> NSImage {
        if let cached = cache[kind] { return cached }
        let image = NSImage(size: size, flipped: false) { _ in
            draw(kind)
            return true
        }
        image.isTemplate = (kind == .paused || kind == .connecting)
        image.accessibilityDescription = "Hole"
        cache[kind] = image
        return image
    }

    private static func draw(_ kind: Kind) {
        let shield = shieldPath()
        let hole = NSBezierPath(ovalIn: holeRect(radius: 105))
        switch kind {
        case .connecting:
            NSColor.black.setStroke()
            shield.lineWidth = 1.2
            shield.stroke()
            hole.lineWidth = 1.2
            hole.stroke()
        case .paused:
            NSColor.black.setFill()
            fill(shield, minus: hole)
        case .blocking, .problem:
            shieldBlue.setFill()
            fill(shield, minus: hole)
            (kind == .blocking ? green : red).setFill()
            NSBezierPath(ovalIn: holeRect(radius: 78)).fill()
        }
    }

    private static func fill(_ shape: NSBezierPath, minus cutout: NSBezierPath) {
        let path = shape.copy() as! NSBezierPath
        path.append(cutout)
        path.windingRule = .evenOdd
        path.fill()
    }

    private static func point(_ x: CGFloat, _ y: CGFloat) -> NSPoint {
        let width = (734 - srcMinX) * scale
        let padX = (size.width - width) / 2
        return NSPoint(x: (x - srcMinX) * scale + padX, y: (y - srcMinY) * scale + 1)
    }

    private static func shieldPath() -> NSBezierPath {
        let p = NSBezierPath()
        p.move(to: point(512, 780))
        p.curve(to: point(290, 680), controlPoint1: point(440, 740), controlPoint2: point(360, 700))
        p.curve(to: point(512, 230), controlPoint1: point(290, 450), controlPoint2: point(380, 310))
        p.curve(to: point(734, 680), controlPoint1: point(644, 310), controlPoint2: point(734, 450))
        p.curve(to: point(512, 780), controlPoint1: point(664, 700), controlPoint2: point(584, 740))
        p.close()
        return p
    }

    private static func holeRect(radius r: CGFloat) -> NSRect {
        let c = point(512, 525)
        return NSRect(x: c.x - r * scale, y: c.y - r * scale, width: 2 * r * scale, height: 2 * r * scale)
    }
}
