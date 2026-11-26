// Generates the bundled AgentChime sounds. Usage: swift scripts/make-sounds.swift <output dir>
// Output: 16-bit PCM mono 44.1 kHz WAV, peak-normalised to 0.7, 5 ms fade in/out.
import Foundation

let rate = 44100.0
let peakTarget = 0.7
let fadeSeconds = 0.005

func render(duration: Double, _ f: (Double) -> Double) -> [Double] {
    (0..<Int(duration * rate)).map { f(Double($0) / rate) }
}

func mix(_ parts: [(start: Double, samples: [Double])]) -> [Double] {
    let total = parts.map { Int($0.start * rate) + $0.samples.count }.max() ?? 0
    var out = [Double](repeating: 0, count: total)
    for part in parts {
        let offset = Int(part.start * rate)
        for (i, s) in part.samples.enumerated() { out[offset + i] += s }
    }
    return out
}

func finish(_ input: [Double]) -> [Double] {
    var s = input
    let peak = s.map { abs($0) }.max() ?? 1
    if peak > 0 { s = s.map { $0 / peak * peakTarget } }
    let fade = Int(fadeSeconds * rate)
    for i in 0..<min(fade, s.count) {
        let g = Double(i) / Double(fade)
        s[i] *= g
        s[s.count - 1 - i] *= g
    }
    return s
}

func note(_ f: Double) -> [Double] {
    render(duration: 0.22) { t in
        (sin(2 * .pi * f * t) + 0.3 * sin(2 * .pi * 2 * f * t)) * exp(-t / 0.12)
    }
}

func knockPulse() -> [Double] {
    render(duration: 0.08) { t in sin(2 * .pi * 180 * t) * exp(-t / 0.03) }
}

let sounds: [String: [Double]] = [
    "Chime": mix([(0, note(1046.5)), (0.12, note(1318.5))]),
    "Ding": render(duration: 0.8) { t in
        (sin(2 * .pi * 880 * t) + 0.2 * sin(2 * .pi * 2640 * t)) * exp(-t / 0.25)
    },
    "Knock": mix([(0, knockPulse()), (0.16, knockPulse())]),
    "Rise": render(duration: 0.35) { t in
        sin(2 * .pi * (500 * t + (1000.0 / (2 * 0.35)) * t * t)) * sin(.pi * t / 0.35)
    },
]

func wav(_ samples: [Double]) -> Data {
    var d = Data()
    func u32(_ v: UInt32) { withUnsafeBytes(of: v.littleEndian) { d.append(contentsOf: $0) } }
    func u16(_ v: UInt16) { withUnsafeBytes(of: v.littleEndian) { d.append(contentsOf: $0) } }
    let bytes = UInt32(samples.count * 2)
    d.append("RIFF".data(using: .ascii)!); u32(36 + bytes)
    d.append("WAVEfmt ".data(using: .ascii)!); u32(16)
    u16(1); u16(1); u32(UInt32(rate)); u32(UInt32(rate) * 2); u16(2); u16(16)
    d.append("data".data(using: .ascii)!); u32(bytes)
    for s in samples { u16(UInt16(bitPattern: Int16((s * 32767).rounded()))) }
    return d
}

guard CommandLine.arguments.count == 2 else {
    FileHandle.standardError.write("usage: make-sounds.swift <output dir>\n".data(using: .utf8)!)
    exit(2)
}
let outDir = URL(fileURLWithPath: CommandLine.arguments[1], isDirectory: true)
try FileManager.default.createDirectory(at: outDir, withIntermediateDirectories: true)
for (name, samples) in sounds {
    try wav(finish(samples)).write(to: outDir.appendingPathComponent("\(name).wav"))
}
