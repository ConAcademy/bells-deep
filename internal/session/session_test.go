package session

import (
	"github.com/NimbleMarkets/bells-deep/internal/mixture"
	"sync"
	"testing"
)

func TestCommandsAndSnapshots(t *testing.T) {
	s := New(mixture.Default(), t.TempDir())
	for _, cmd := range []string{"/add", "/set rho 0.8", "/component 1", "/pause", "/play", "/preset mixture", "/save demo"} {
		if _, e := s.Command(cmd, true); e != nil {
			t.Fatal(cmd, e)
		}
	}
	a := s.Snapshot()
	a.Scene.Components[0].Weight = 99
	if s.Snapshot().Scene.Components[0].Weight == 99 {
		t.Fatal("snapshot aliases live data")
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Go(func() {
			for j := 0; j < 20; j++ {
				s.Snapshot()
				s.Command("/set rho 0.2", false)
			}
		})
	}
	wg.Wait()
}
