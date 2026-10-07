package preset

import (
	"encoding/json"
	"fmt"
	"github.com/NimbleMarkets/bells-deep/internal/mixture"
	"os"
	"path/filepath"
	"strings"
)

var Names = []string{"isotropic", "correlated", "ridge", "offset", "mixture", "thin"}

func Builtin(name string) (mixture.Scene, error) {
	s := mixture.Default()
	switch name {
	case "isotropic":
	case "correlated":
		s.Components[0].Rho = .8
	case "ridge":
		s.Components[0].Rho = .97
	case "thin":
		s.Components[0].SigmaX = 2
		s.Components[0].SigmaY = .2
	case "offset":
		s.Add(0)
		s.Components[0].MuX = -1.5
		s.Components[1].MuX = 1.5
		s.Components[1].MuY = 0
	case "mixture":
		s.Add(0)
		s.Add(0)
		for i, w := range []float64{.5, .3, .2} {
			s.Components[i].Weight = w
		}
		s.Components[0].MuX = -1.5
		s.Components[1].MuX = 1.5
		s.Components[1].MuY = 1
		s.Components[1].Rho = .6
		s.Components[2].MuY = -1.5
		s.Components[2].SigmaX = .6
		s.Components[2].SigmaY = .6
	default:
		return s, fmt.Errorf("unknown preset %q", name)
	}
	s.Fit()
	return s, nil
}
func Decode(data []byte) (mixture.Scene, error) {
	s := mixture.Default()
	var raw struct {
		Components []json.RawMessage `json:"components"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return mixture.Default(), err
	}
	for i, v := range raw.Components {
		c := mixture.DefaultComponent()
		c.Color = mixture.Colors[i%8]
		if err := json.Unmarshal(v, &c); err != nil {
			return mixture.Default(), err
		}
		s.Components[i] = c
	}
	mixture.Clamp(&s)
	return s, nil
}
func Load(path string) (mixture.Scene, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return mixture.Default(), e
	}
	return Decode(b)
}
func Save(dir, name string, s mixture.Scene) error {
	name = strings.TrimSuffix(name, ".bell.json")
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("name must be a filename")
	}
	mixture.Clamp(&s)
	b, e := json.MarshalIndent(s, "", "  ")
	if e != nil {
		return e
	}
	if e = os.MkdirAll(dir, 0755); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".bell-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(f.Name(), filepath.Join(dir, name+".bell.json"))
}
func List(dir string) []string { v, _ := filepath.Glob(filepath.Join(dir, "*.bell.json")); return v }
