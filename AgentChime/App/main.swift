import Foundation

// A closed pipe (the old app quitting while the new one is about to say `ready`) must never kill us.
signal(SIGPIPE, SIG_IGN)

// `--version` is answered before any UI, path, log or deploy work (the updater runs it on a new bundle).
if CommandLine.arguments.contains("--version") {
    print(Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "")
    exit(0)
}

#if AGENTCHIME_UPDATE_TEST
// Test-only: a build that dies after an update before it says `ready` (the e2e rollback case).
if CommandLine.arguments.contains("--after-update"),
   ProcessInfo.processInfo.environment["AGENTCHIME_TEST_FAIL_AFTER_UPDATE"] == "1" {
    exit(1)
}
#endif

AgentChimeApp.main()
