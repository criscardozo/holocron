package power_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestInstallerPowerHelpers runs scripts/install_power_test.sh, which exercises
// the half of the installer that creates the units this package signals.
//
// It lives here, in Go, so that `go test ./...` and CI cover it. A shell script
// nobody runs is not a test, and this is precisely the code that has twice
// undone a deliberate decision on the next reinstall — first ReadWritePaths,
// then by handing back a control for a service that had been turned off on
// purpose. Reading the script caught neither.
func TestInstallerPowerHelpers(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the installer is a POSIX shell script")
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file")
	}
	script := filepath.Join(filepath.Dir(thisFile), "..", "..", "scripts", "install_power_test.sh")

	out, err := exec.CommandContext(t.Context(), "bash", script).CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(string(out), "ok:") {
		t.Errorf("the script did not report success:\n%s", out)
	}
}
