import XCTest

final class JSONValueTests: IsolatedTestCase {
    private func parse(_ s: String) throws -> JSONValue { try JSONValue.parse(Data(s.utf8)) }

    func testRoundTripClaudeFixtureIsByteIdentical() throws {
        let d = fixtureData("claude-settings.json")
        XCTAssertFalse(d.isEmpty)
        XCTAssertEqual(try JSONValue.parse(d).serialized() + "\n", String(decoding: d, as: UTF8.self))
    }

    func testRoundTripCodexFixtureIsByteIdentical() throws {
        let d = fixtureData("codex-hooks.json")
        XCTAssertFalse(d.isEmpty)
        XCTAssertEqual(try JSONValue.parse(d).serialized() + "\n", String(decoding: d, as: UTF8.self))
    }

    func testNumbersKeepTheirSpelling() throws {
        let out = try parse(#"{"a": 1.0, "c": -2.5e3}"#).serialized()
        XCTAssertTrue(out.contains(#""a": 1.0"#))
        XCTAssertTrue(out.contains(#""c": -2.5e3"#))
    }

    func testStringEscapes() throws {
        let v = try parse(#"{"s":"a\"b\\c\n\u0001/é😀"}"#)
        XCTAssertEqual(v.serialized(), "{\n  \"s\": " + #""a\"b\\c\n\u0001/é😀""# + "\n}")
    }

    func testSurrogatePairDecodes() throws {
        XCTAssertEqual(try parse(#"{"s":"😀"}"#)["s"]?.stringValue, "😀")
    }

    func testEscapedNulDoesNotTruncate() throws {
        let v = try parse(#"{"s":"a\u0000b"}"#)
        XCTAssertEqual(v["s"]?.stringValue, "a\u{0}b")
        XCTAssertEqual(v.serialized(), "{\n  \"s\": " + #""a\u0000b""# + "\n}")
    }

    func testEmptyContainers() throws {
        XCTAssertEqual(try parse(#"{"a":[],"b":{}}"#).serialized(), "{\n  \"a\": [],\n  \"b\": {}\n}")
    }

    func testSetKeepsPositionAndAppendsNewKeys() throws {
        var v = try parse(#"{"x":1,"y":2}"#)
        v.set("x", .number("3"))
        v.set("z", .number("4"))
        XCTAssertEqual(v.objectKeys, ["x", "y", "z"])
        XCTAssertEqual(v["x"], .number("3"))
    }

    func testRemoveKeyAndAccessors() throws {
        var v = try parse(#"{"a":true,"b":"s","c":[1]}"#)
        XCTAssertEqual(v["a"]?.boolValue, true)
        XCTAssertEqual(v["b"]?.stringValue, "s")
        XCTAssertEqual(v["c"]?.arrayValue, [.number("1")])
        XCTAssertNil(v["missing"])
        XCTAssertNil(JSONValue.null["a"])
        v.removeKey("b")
        XCTAssertEqual(v.objectKeys, ["a", "c"])
    }

    func testRejectsInvalidJSON() {
        for s in ["{\"a\": 1,}", "{\"a\": 1} x", "// c\n{}", "", "[1,]", "{\"a\": 01}", "\"\u{1}\"", "{\"a\" 1}"] {
            XCTAssertThrowsError(try parse(s), s)
        }
    }

    func testAcceptsWhitespaceAndScalarsAtTopLevel() throws {
        XCTAssertEqual(try parse(" \n null \t").serialized(), "null")
        XCTAssertEqual(try parse("[1, 2]").serialized(), "[\n  1,\n  2\n]")
    }
}
