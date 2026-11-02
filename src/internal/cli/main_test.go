package cli

import (
	"os"
	"testing"
)

// TestMain turns start-up off for the package: the tests build command trees by
// the hundred against temporary homes, and start-up would otherwise read .env
// and authama.json from - and migrate - whatever home the environment named.
// startup_test.go turns it back on, one test at a time.
func TestMain(m *testing.M) {
	runStartup = false
	os.Exit(m.Run())
}
