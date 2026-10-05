package system

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestTempPrefersTheCPUPackageOverZoneZero is the Acer case: zone 0 is acpitz
// and reads like a cool motherboard while the CPU package is what throttles.
// Not parallel: it swaps the package-level sysRoot.
func TestTempPrefersTheCPUPackageOverZoneZero(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root+"/class/thermal/thermal_zone0/type", "acpitz\n")
	writeFile(t, root+"/class/thermal/thermal_zone0/temp", "27800\n")
	writeFile(t, root+"/class/thermal/thermal_zone1/type", "x86_pkg_temp\n")
	writeFile(t, root+"/class/thermal/thermal_zone1/temp", "50000\n")
	writeFile(t, root+"/class/hwmon/hwmon0/name", "acpitz\n")
	writeFile(t, root+"/class/hwmon/hwmon3/name", "coretemp\n")
	writeFile(t, root+"/class/hwmon/hwmon3/temp1_label", "Package id 0\n")
	writeFile(t, root+"/class/hwmon/hwmon3/temp1_input", "51000\n")
	writeFile(t, root+"/class/hwmon/hwmon3/temp2_label", "Core 0\n")
	writeFile(t, root+"/class/hwmon/hwmon3/temp2_input", "49000\n")

	old := sysRoot
	sysRoot = root
	t.Cleanup(func() { sysRoot = old })

	got, err := tempC()
	if err != nil || got != 51 {
		t.Fatalf("tempC() = %v, %v; want the coretemp package reading, 51", got, err)
	}

	// Without coretemp, the x86_pkg_temp zone, still not zone 0.
	if err := os.RemoveAll(root + "/class/hwmon/hwmon3"); err != nil {
		t.Fatal(err)
	}
	if got, _ := tempC(); got != 50 {
		t.Errorf("without coretemp: %v, want the x86_pkg_temp zone, 50", got)
	}

	// And the Pi, which only has zone 0, keeps working.
	if err := os.RemoveAll(root + "/class/thermal/thermal_zone1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := tempC(); got != 27.8 {
		t.Errorf("fallback: %v, want zone 0, 27.8", got)
	}
}
