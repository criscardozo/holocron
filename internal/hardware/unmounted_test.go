package hardware

import "testing"

func TestMarkUnmounted(t *testing.T) {
	t.Parallel()
	disks := []Disk{{Name: "nvme0n1"}, {Name: "sda"}, {Name: "sdb"}, {Name: "sdc"}}
	// Ginebra: root and swap on the NVMe, Disco4 and Disco3 mounted, the
	// personal SSD (sda) left alone.
	markUnmounted(disks, map[string]bool{"nvme0n1p2": true, "nvme0n1p3": true, "sdb1": true, "sdc1": true})
	for _, d := range disks {
		if want := d.Name == "sda"; d.Unmounted != want {
			t.Errorf("%s: unmounted = %v, want %v", d.Name, d.Unmounted, want)
		}
	}
	// Nothing readable: mark nothing rather than everything.
	none := []Disk{{Name: "sda"}}
	markUnmounted(none, map[string]bool{})
	if none[0].Unmounted {
		t.Error("without /proc/mounts nothing should be marked")
	}
}
