package config

import "testing"

func TestParseDisks(t *testing.T) {
	t.Parallel()
	got, bad := parseDisks("Disco4:/mnt/disco4:biblioteca; Disco3:/mnt/disco3:biblioteca;Sistema:/:descargas:/srv/descargas;roto;X:relativo:rol")
	if len(got) != 3 || len(bad) != 2 {
		t.Fatalf("got %+v, bad %v", got, bad)
	}
	if got[0] != (DiskSpec{Name: "Disco4", Mount: "/mnt/disco4", Role: "biblioteca", Scan: "/mnt/disco4"}) {
		t.Errorf("disco4 = %+v", got[0])
	}
	if got[2].Mount != "/" || got[2].Scan != "/srv/descargas" || got[2].Role != "descargas" {
		t.Errorf("sistema = %+v", got[2])
	}
}

func TestSplitList(t *testing.T) {
	got := splitList(" Anime ; ;Museum;")
	if len(got) != 2 || got[0] != "Anime" || got[1] != "Museum" {
		t.Errorf("splitList = %q, want [Anime Museum]", got)
	}
	if got := splitList(""); got != nil {
		t.Errorf("splitList(\"\") = %q, want nil", got)
	}
}
