import XCTest

final class SoundLibraryTests: IsolatedTestCase {
    func testSystemSoundsIncludeDefaults() {
        let values = SoundLibrary.systemSounds().map(\.value)
        XCTAssertTrue(values.contains("Glass"))
        XCTAssertTrue(values.contains("Sosumi"))
        XCTAssertEqual(values, values.sorted())
    }

    func testBundledSoundsFiltersAndSorts() throws {
        let dir = home.appendingPathComponent("lib", isDirectory: true)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        for name in ["b.wav", "A.MP3", "CREDITS.md", "x.txt"] {
            try Data([0]).write(to: dir.appendingPathComponent(name))
        }
        let sounds = SoundLibrary.bundledSounds(in: dir)
        XCTAssertEqual(sounds.map(\.value), ["A.MP3", "b.wav"])
        XCTAssertTrue(sounds.allSatisfy { $0.source == .bundled })
    }

    func testRepoSoundsArePlayable() throws {
        let dir = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent()
            .appendingPathComponent("Resources/Sounds")
        let files = try FileManager.default.contentsOfDirectory(atPath: dir.path)
            .filter { SoundLibrary.audioExtensions.contains(($0 as NSString).pathExtension.lowercased()) }
        XCTAssertEqual(files.count, 14)
        let credits = try String(contentsOf: dir.appendingPathComponent("CREDITS.md"))
        for f in files {
            XCTAssertTrue(SoundPlayer.isPlayable(dir.appendingPathComponent(f)), f)
            XCTAssertTrue(credits.contains(f), "CREDITS.md missing \(f)")
        }
    }

    func testBuiltInListsFourteenSounds() throws {
        let dir = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent()
            .appendingPathComponent("Resources/Sounds")
        let sounds = SoundLibrary.bundledSounds(in: dir)
        XCTAssertEqual(sounds.count, 14)
        XCTAssertTrue(sounds.allSatisfy { $0.source == .bundled })
        let names = Set(sounds.map { $0.displayName })
        for n in ["Aurora", "Bloom", "Breeze", "Crystal", "Drift", "Ember", "Lumen", "Pebble", "Ripple", "Tide"] {
            XCTAssertTrue(names.contains(n), n)
        }
    }
}
