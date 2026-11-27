// Generates the bundled AgentChime sounds. Usage: swift scripts/make-sounds.swift <output dir>
// Output: 16-bit PCM mono 44.1 kHz WAV, peak-normalised to 0.7, 5 ms fade in/out.
import Foundation

let rate = 44100.0
let peakTarget = 0.7
let fadeSeconds = 0.005
let tau = 2 * Double.pi

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

// Smooth attack so no note starts with a click.
func attack(_ t: Double, _ a: Double) -> Double { min(1, t / a) }

// Inharmonic bell: partials with individual decay.
func bell(_ f: Double, dur: Double, decay: Double) -> [Double] {
    let parts: [(Double, Double, Double)] = [(1, 1, 1), (2.76, 0.45, 0.6), (5.4, 0.2, 0.35), (8.93, 0.08, 0.2)]
    return render(duration: dur) { t in
        parts.reduce(0) { $0 + $1.1 * sin(tau * f * $1.0 * t) * exp(-t / (decay * $1.2)) } * attack(t, 0.002)
    }
}

// Marimba: fundamental plus 4x partial, fast decay.
func marimba(_ f: Double, dur: Double = 0.4) -> [Double] {
    render(duration: dur) { t in
        (sin(tau * f * t) * exp(-t / 0.12) + 0.35 * sin(tau * 4 * f * t) * exp(-t / 0.03)) * attack(t, 0.001)
    }
}

// Soft pluck: Karplus-Strong-like via decaying harmonics with falling brightness.
func pluck(_ f: Double, dur: Double = 0.5) -> [Double] {
    render(duration: dur) { t in
        var v = 0.0
        for h in 1...6 { v += sin(tau * f * Double(h) * t) / Double(h * h) * exp(-t * Double(h) * 6) }
        return v * exp(-t / 0.3) * attack(t, 0.001)
    }
}

// Glass: high sine pair with slight detune beating.
func glass(_ f: Double, dur: Double = 0.9) -> [Double] {
    render(duration: dur) { t in
        (sin(tau * f * t) + 0.6 * sin(tau * f * 1.005 * t) + 0.25 * sin(tau * f * 3.01 * t)) * exp(-t / 0.35) * attack(t, 0.004)
    }
}

// Soft pad swell: slow attack and release.
func pad(_ f: Double, dur: Double) -> [Double] {
    render(duration: dur) { t in
        let env = sin(Double.pi * t / dur)
        return (sin(tau * f * t) + 0.5 * sin(tau * f * 1.5 * t) + 0.25 * sin(tau * f * 2 * t)) * env * env
    }
}

// Gliding sine with vibrato-free sweep from f0 to f1.
func sweep(_ f0: Double, _ f1: Double, dur: Double) -> [Double] {
    render(duration: dur) { t in
        let phase = f0 * t + (f1 - f0) / (2 * dur) * t * t
        return sin(tau * phase) * sin(Double.pi * t / dur)
    }
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
    "Aurora": mix([(0, pad(523.25, dur: 1.2)), (0.1, pad(659.25, dur: 1.1)), (0.2, pad(783.99, dur: 1.0))]),
    "Bloom": mix([(0, sweep(330, 660, dur: 0.5)), (0.3, bell(880, dur: 0.9, decay: 0.3))]),
    "Breeze": render(duration: 0.8) { t in
        (sin(tau * 392 * t) + 0.5 * sin(tau * 587.33 * t * (1 + 0.01 * sin(tau * 5 * t)))) * sin(Double.pi * t / 0.8)
    },
    "Crystal": mix([(0, glass(1568)), (0.15, glass(2093))]),
    "Drift": mix([(0, sweep(784, 523.25, dur: 0.6)), (0.35, sweep(659.25, 440, dur: 0.6))]),
    "Ember": mix([(0, pluck(220)), (0.14, pluck(329.63)), (0.28, pluck(261.63, dur: 0.7))]),
    "Lumen": mix([(0, bell(1174.66, dur: 1.0, decay: 0.4)), (0.18, bell(1568, dur: 1.0, decay: 0.4))]),
    "Pebble": mix([(0, marimba(523.25)), (0.1, marimba(392)), (0.2, marimba(293.66, dur: 0.5))]),
    "Ripple": mix((0..<5).map { i in
        (Double(i) * 0.09, marimba(440 * pow(1.2599, Double(i)), dur: 0.35).map { $0 * pow(0.8, Double(i)) })
    }),
    "Tide": mix([(0, pad(196, dur: 1.4)), (0.0, pad(293.66, dur: 1.4)), (0.15, sweep(392, 349.23, dur: 1.0))]),
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
