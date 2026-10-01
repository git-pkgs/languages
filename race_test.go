//go:build race

package languages_test

// Race instrumentation randomly discards sync.Pool entries.
const raceEnabled = true
