import Foundation

struct JSONMember: Equatable {
    var key: String
    var value: JSONValue
}

/// Order-preserving JSON value. Numbers keep their original spelling.
indirect enum JSONValue: Equatable {
    case object([JSONMember])
    case array([JSONValue])
    case string(String)
    case number(String)
    case bool(Bool)
    case null

    struct ParseError: Error {
        let offset: Int
        let message: String
    }

    // MARK: Accessors

    subscript(key: String) -> JSONValue? {
        guard case .object(let members) = self else { return nil }
        return members.first(where: { $0.key == key })?.value
    }

    mutating func set(_ key: String, _ value: JSONValue) {
        guard case .object(var members) = self else { return }
        if let i = members.firstIndex(where: { $0.key == key }) {
            members[i].value = value
        } else {
            members.append(JSONMember(key: key, value: value))
        }
        self = .object(members)
    }

    mutating func removeKey(_ key: String) {
        guard case .object(var members) = self else { return }
        members.removeAll { $0.key == key }
        self = .object(members)
    }

    var arrayValue: [JSONValue]? { if case .array(let a) = self { return a }; return nil }
    var stringValue: String? { if case .string(let s) = self { return s }; return nil }
    var boolValue: Bool? { if case .bool(let b) = self { return b }; return nil }
    var objectKeys: [String]? {
        if case .object(let m) = self { return m.map { $0.key } }
        return nil
    }

    // MARK: Serialization (matches JSON.stringify(v, null, 2))

    func serialized() -> String {
        var out = ""
        write(to: &out, indent: 0)
        return out
    }

    private func write(to out: inout String, indent: Int) {
        switch self {
        case .null: out += "null"
        case .bool(let b): out += b ? "true" : "false"
        case .number(let n): out += n
        case .string(let s): JSONValue.writeString(s, to: &out)
        case .array(let items):
            if items.isEmpty { out += "[]"; return }
            let pad = String(repeating: " ", count: indent + 2)
            out += "[\n"
            for (i, item) in items.enumerated() {
                out += pad
                item.write(to: &out, indent: indent + 2)
                out += i < items.count - 1 ? ",\n" : "\n"
            }
            out += String(repeating: " ", count: indent) + "]"
        case .object(let members):
            if members.isEmpty { out += "{}"; return }
            let pad = String(repeating: " ", count: indent + 2)
            out += "{\n"
            for (i, m) in members.enumerated() {
                out += pad
                JSONValue.writeString(m.key, to: &out)
                out += ": "
                m.value.write(to: &out, indent: indent + 2)
                out += i < members.count - 1 ? ",\n" : "\n"
            }
            out += String(repeating: " ", count: indent) + "}"
        }
    }

    private static func writeString(_ s: String, to out: inout String) {
        out += "\""
        for u in s.unicodeScalars {
            switch u {
            case "\"": out += "\\\""
            case "\\": out += "\\\\"
            case "\u{08}": out += "\\b"
            case "\u{0C}": out += "\\f"
            case "\n": out += "\\n"
            case "\r": out += "\\r"
            case "\t": out += "\\t"
            default:
                if u.value < 0x20 {
                    let hex = String(u.value, radix: 16)
                    out += "\\u" + String(repeating: "0", count: 4 - hex.count) + hex
                } else {
                    out.unicodeScalars.append(u)
                }
            }
        }
        out += "\""
    }

    // MARK: Parsing (strict RFC 8259)

    static func parse(_ data: Data) throws -> JSONValue {
        var p = Parser(bytes: [UInt8](data))
        p.skipWhitespace()
        let v = try p.parseValue(depth: 0)
        p.skipWhitespace()
        if p.pos != p.bytes.count { throw p.fail("unexpected trailing content") }
        return v
    }

    private struct Parser {
        let bytes: [UInt8]
        var pos = 0
        static let maxDepth = 512

        init(bytes: [UInt8]) { self.bytes = bytes }

        func fail(_ message: String) -> ParseError { ParseError(offset: pos, message: message) }

        mutating func skipWhitespace() {
            while pos < bytes.count, [0x20, 0x09, 0x0A, 0x0D].contains(bytes[pos]) { pos += 1 }
        }

        mutating func parseValue(depth: Int) throws -> JSONValue {
            if depth > Parser.maxDepth { throw fail("nesting too deep") }
            guard pos < bytes.count else { throw fail("unexpected end of input") }
            switch bytes[pos] {
            case UInt8(ascii: "{"): return try parseObject(depth: depth)
            case UInt8(ascii: "["): return try parseArray(depth: depth)
            case UInt8(ascii: "\""): return .string(try parseString())
            case UInt8(ascii: "t"): try expect("true"); return .bool(true)
            case UInt8(ascii: "f"): try expect("false"); return .bool(false)
            case UInt8(ascii: "n"): try expect("null"); return .null
            case UInt8(ascii: "-"), UInt8(ascii: "0")...UInt8(ascii: "9"): return .number(try parseNumber())
            default: throw fail("unexpected character")
            }
        }

        mutating func expect(_ word: String) throws {
            let w = Array(word.utf8)
            guard pos + w.count <= bytes.count, Array(bytes[pos..<pos + w.count]) == w else {
                throw fail("invalid literal")
            }
            pos += w.count
        }

        mutating func parseObject(depth: Int) throws -> JSONValue {
            pos += 1
            var members: [JSONMember] = []
            skipWhitespace()
            if pos < bytes.count, bytes[pos] == UInt8(ascii: "}") { pos += 1; return .object(members) }
            while true {
                skipWhitespace()
                guard pos < bytes.count, bytes[pos] == UInt8(ascii: "\"") else { throw fail("expected string key") }
                let key = try parseString()
                skipWhitespace()
                guard pos < bytes.count, bytes[pos] == UInt8(ascii: ":") else { throw fail("expected ':'") }
                pos += 1
                skipWhitespace()
                let value = try parseValue(depth: depth + 1)
                members.append(JSONMember(key: key, value: value))
                skipWhitespace()
                guard pos < bytes.count else { throw fail("unterminated object") }
                if bytes[pos] == UInt8(ascii: ",") { pos += 1; continue }
                if bytes[pos] == UInt8(ascii: "}") { pos += 1; return .object(members) }
                throw fail("expected ',' or '}'")
            }
        }

        mutating func parseArray(depth: Int) throws -> JSONValue {
            pos += 1
            var items: [JSONValue] = []
            skipWhitespace()
            if pos < bytes.count, bytes[pos] == UInt8(ascii: "]") { pos += 1; return .array(items) }
            while true {
                skipWhitespace()
                items.append(try parseValue(depth: depth + 1))
                skipWhitespace()
                guard pos < bytes.count else { throw fail("unterminated array") }
                if bytes[pos] == UInt8(ascii: ",") { pos += 1; continue }
                if bytes[pos] == UInt8(ascii: "]") { pos += 1; return .array(items) }
                throw fail("expected ',' or ']'")
            }
        }

        mutating func parseNumber() throws -> String {
            let start = pos
            func isDigit(_ i: Int) -> Bool { i < bytes.count && bytes[i] >= 0x30 && bytes[i] <= 0x39 }
            if bytes[pos] == UInt8(ascii: "-") { pos += 1 }
            guard isDigit(pos) else { throw fail("invalid number") }
            if bytes[pos] == UInt8(ascii: "0") {
                pos += 1
                if isDigit(pos) { throw fail("leading zero") }
            } else {
                while isDigit(pos) { pos += 1 }
            }
            if pos < bytes.count, bytes[pos] == UInt8(ascii: ".") {
                pos += 1
                guard isDigit(pos) else { throw fail("invalid fraction") }
                while isDigit(pos) { pos += 1 }
            }
            if pos < bytes.count, bytes[pos] == UInt8(ascii: "e") || bytes[pos] == UInt8(ascii: "E") {
                pos += 1
                if pos < bytes.count, bytes[pos] == UInt8(ascii: "+") || bytes[pos] == UInt8(ascii: "-") { pos += 1 }
                guard isDigit(pos) else { throw fail("invalid exponent") }
                while isDigit(pos) { pos += 1 }
            }
            return String(decoding: bytes[start..<pos], as: UTF8.self)
        }

        mutating func parseHex4() throws -> UInt32 {
            guard pos + 4 <= bytes.count else { throw fail("truncated \\u escape") }
            var v: UInt32 = 0
            for _ in 0..<4 {
                let c = bytes[pos]
                let d: UInt32
                switch c {
                case 0x30...0x39: d = UInt32(c - 0x30)
                case 0x41...0x46: d = UInt32(c - 0x41 + 10)
                case 0x61...0x66: d = UInt32(c - 0x61 + 10)
                default: throw fail("invalid \\u escape")
                }
                v = v * 16 + d
                pos += 1
            }
            return v
        }

        mutating func parseString() throws -> String {
            pos += 1
            var raw: [UInt8] = []
            while true {
                guard pos < bytes.count else { throw fail("unterminated string") }
                let c = bytes[pos]
                if c == UInt8(ascii: "\"") {
                    pos += 1
                    guard let s = String(bytes: raw, encoding: .utf8) else {
                        throw fail("invalid UTF-8 in string")
                    }
                    return s
                }
                if c < 0x20 { throw fail("unescaped control character") }
                if c != UInt8(ascii: "\\") { raw.append(c); pos += 1; continue }
                pos += 1
                guard pos < bytes.count else { throw fail("unterminated escape") }
                let e = bytes[pos]
                pos += 1
                switch e {
                case UInt8(ascii: "\""): raw.append(0x22)
                case UInt8(ascii: "\\"): raw.append(0x5C)
                case UInt8(ascii: "/"): raw.append(0x2F)
                case UInt8(ascii: "b"): raw.append(0x08)
                case UInt8(ascii: "f"): raw.append(0x0C)
                case UInt8(ascii: "n"): raw.append(0x0A)
                case UInt8(ascii: "r"): raw.append(0x0D)
                case UInt8(ascii: "t"): raw.append(0x09)
                case UInt8(ascii: "u"):
                    var code = try parseHex4()
                    if (0xD800...0xDBFF).contains(code) {
                        guard pos + 1 < bytes.count, bytes[pos] == UInt8(ascii: "\\"), bytes[pos + 1] == UInt8(ascii: "u") else {
                            throw fail("lone high surrogate")
                        }
                        pos += 2
                        let low = try parseHex4()
                        guard (0xDC00...0xDFFF).contains(low) else { throw fail("invalid low surrogate") }
                        code = 0x10000 + ((code - 0xD800) << 10) + (low - 0xDC00)
                    } else if (0xDC00...0xDFFF).contains(code) {
                        throw fail("lone low surrogate")
                    }
                    guard let scalar = Unicode.Scalar(code) else { throw fail("invalid code point") }
                    raw.append(contentsOf: Array(String(Character(scalar)).utf8))
                default: throw fail("invalid escape")
                }
            }
        }
    }
}
