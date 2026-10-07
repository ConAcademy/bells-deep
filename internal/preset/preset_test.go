package preset

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	s, _ := Builtin("mixture")
	s.Camera.Yaw = 1.1
	s.Camera.Target = [3]float64{1, 2, 3}
	s.Overlays = 27
	dir := t.TempDir()
	if err := Save(dir, "test", s); err != nil {
		t.Fatal(err)
	}
	got, e := Load(filepath.Join(dir, "test.bell.json"))
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(s, got) {
		t.Fatalf("roundtrip: %#v != %#v", s, got)
	}
	if e = Save(dir, "../escape", s); e == nil {
		t.Fatal("accepted traversal")
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatal(files)
	}
}
func TestPartialAndUnknownJSON(t *testing.T) {
	s, e := Decode([]byte(`{"future":42,"components":[{"rho":2},{"mu_x":1}]}`))
	if e != nil {
		t.Fatal(e)
	}
	if s.Components[0].SigmaX != 1 || s.Components[0].Rho != .97 || s.Components[1].Weight != 1 || s.Domain != 4 {
		t.Fatal(s)
	}
	if _, e = Decode([]byte(`{"domain":`)); e == nil {
		t.Fatal("accepted invalid JSON")
	}
	for _, name := range Names {
		if _, e = Builtin(name); e != nil {
			t.Fatal(e)
		}
	}
}
