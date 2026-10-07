package main

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"fmt"
	"github.com/NimbleMarkets/bells-deep/internal/mixture"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	"github.com/charmbracelet/x/ansi"
	"math"
	"strings"
)

// All geometry is in terminal cells. Picture dimensions exclude borders,
// headings, HUD, and footer. The same budget drives dispatch and Kitty placement.
type layout struct {
	pictureW, pictureH, pictureX, pictureY, sidebar int
	compact                                         bool
}

func (m *model) layout() layout {
	cols, rows := max(1, m.cols), max(1, m.rows)
	if m.fullscreen {
		return layout{pictureW: cols, pictureH: max(1, rows-2)}
	}
	if rows < 14 || cols < 32 {
		return layout{pictureW: cols, pictureH: max(1, rows-4), pictureY: 1, compact: true}
	}
	l := layout{pictureW: cols - 2, pictureH: rows - 8, pictureX: 1, pictureY: 2}
	if cols >= 85 {
		l.sidebar = 31
		l.pictureW -= l.sidebar + 1
	} else {
		l.pictureH--
	}
	return l
}

// cellLine truncates escape-aware by display cells, never byte/rune count.
// It also clips an old larger frame during the resize-to-new-frame interval.
func cellLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, "")
	return s + strings.Repeat(" ", max(0, width-ansi.StringWidth(s)))
}
func cellLines(s string, width, height int) []string {
	lines := strings.Split(s, "\n")
	out := make([]string, max(0, height))
	for i := range out {
		if i < len(lines) {
			out[i] = cellLine(lines[i], width)
		} else {
			out[i] = strings.Repeat(" ", max(0, width))
		}
	}
	return out
}
func fixedBox(s string, width, height int) []string {
	lines := cellLines(s, width, height)
	out := []string{muted.Render("╭" + strings.Repeat("─", width) + "╮")}
	for _, line := range lines {
		out = append(out, muted.Render("│")+line+muted.Render("│"))
	}
	return append(out, muted.Render("╰"+strings.Repeat("─", width)+"╯"))
}
func (m *model) View() tea.View {
	s := m.store.Snapshot()
	l := m.layout()
	lines := []string{}
	switch m.dialog {
	case "help":
		lines = strings.Split("BELLS—DEEP · HELP\n\n↑↓ controls  ←→ step  Shift+←→ ten steps\n[ ] component  n add  x delete  r randomize  R mixture\nSpace pause  0 reset  Home fit  hjkl / drag orbit  +/- zoom\n1 grid  2 ellipses  3 marginals  4 samples  5 contours\nf / Ctrl+F viewport  F3 gallery  Esc back  q quit\n\n/help /pause /play /reset /component N /add /delete\n/randomize /set rho 0.8 /preset correlated\n/save name /load file.bell.json /gallery /fullscreen /quit\n\nHUD density is unscaled; peak normalization changes the surface.\nEsc to close", "\n")
	case "gallery":
		lines = []string{"GALLERY · ↑↓ choose · Enter load · Esc back", "", "BUILTINS / SAVED SCENES"}
		count := max(1, min(7, m.rows-8))
		start := max(0, m.galleryIndex-count/2)
		end := min(len(m.gallery), start+count)
		for i := start; i < end; i++ {
			item := m.gallery[i]
			label := "builtin"
			if item.path != "" {
				label = "saved"
			}
			line := fmt.Sprintf("  %-24s %s", item.name, label)
			if i == m.galleryIndex {
				line = purple.Render("›" + line[1:])
			}
			lines = append(lines, line)
		}
		if len(m.gallery) > 0 {
			lines = append(lines, "")
			lines = append(lines, strings.Split(m.galleryPic.View().Content, "\n")...)
		}
	default:
		viewport := m.pic.View().Content
		if m.fullscreen {
			lines = cellLines(viewport, l.pictureW, l.pictureH)
		} else if l.compact {
			lines = append(lines, "BELLS—DEEP")
			lines = append(lines, cellLines(viewport, l.pictureW, l.pictureH)...)
			lines = append(lines, fmt.Sprintf("peak %.4g  %s %.3f", s.Stats.Peak[2], mixture.Controls[m.focus].Name, s.Scene.Values(s.Selected)[m.focus]))
		} else {
			lines = append(lines, "BELLS—DEEP  "+muted.Render("Gaussian explorer"))
			body := fixedBox(viewport, l.pictureW, l.pictureH)
			if l.sidebar > 0 {
				side := []string{fmt.Sprintf("BELL %d / %d", s.Selected+1, len(s.Scene.Components)), ""}
				values := s.Scene.Values(s.Selected)
				visible := min(len(mixture.Controls), len(body)-2)
				start := max(0, min(m.focus-visible+1, len(mixture.Controls)-visible))
				for i := start; i < start+visible; i++ {
					p := mixture.Controls[i]
					value := values[i]
					lo, hi := p.Min, p.Max
					if p.Name == "mu_x" || p.Name == "mu_y" {
						lo, hi = -s.Scene.Domain, s.Scene.Domain
					}
					pos := max(0, min(9, int((value-lo)/(hi-lo)*9)))
					bar := strings.Repeat("─", pos) + "●" + strings.Repeat("─", 9-pos)
					line := fmt.Sprintf("%-9s %5.2f %s", p.Name, value, bar)
					if i == m.focus {
						line = purple.Render(line)
					}
					side = append(side, line)
				}
				for i := range body {
					right := ""
					if i < len(side) {
						right = side[i]
					}
					body[i] += " " + cellLine(right, l.sidebar)
				}
			}
			lines = append(lines, body...)
			if l.sidebar == 0 {
				lines = append(lines, purple.Render(fmt.Sprintf("%s %.3f · ↑↓ select / ←→ adjust", mixture.Controls[m.focus].Name, s.Scene.Values(s.Selected)[m.focus])))
			}
			e := s.Stats.Ellipses[s.Selected]
			c := s.Scene.Components[s.Selected]
			lines = append(lines, fmt.Sprintf("peak (%+.3f, %+.3f) %.5f · det Σ %.5g · condition %.3f", s.Stats.Peak[0], s.Stats.Peak[1], s.Stats.Peak[2], e.Det, e.Condition), fmt.Sprintf("1σ axes %.3f / %.3f · eigenvalues %.3f / %.3f · rotation %.1f°", e.Axes[0], e.Axes[1], e.Eigenvalues[0], e.Eigenvalues[1], e.Rotation*180/math.Pi), fmt.Sprintf("weights %.3f → %.3f · selected %.3f · singular (<1e−3): %t", s.Stats.WeightBefore, s.Stats.WeightAfter, c.Weight/s.Stats.WeightBefore, s.Stats.Singular))
		}
	}
	// Reserve footer before clipping any dialog or exceptionally small viewport.
	bodyHeight := max(0, m.rows-2)
	lines = cellLines(strings.Join(lines, "\n"), max(1, m.cols), bodyHeight)
	rw, rh := m.dimensions()
	warning := ""
	if s.Stats.Warning {
		warning = " · ⚠ covariance ridge"
	}
	mode := "GPU / glyph"
	if m.pic.Mode() == picture.PictureKitty {
		mode = "GPU / Kitty"
	}
	if m.gpu == nil {
		mode = "CPU analytic preview"
	}
	status := fmt.Sprintf("%.0f fps · %d×%d · yaw %.2f pitch %.2f r %.1f · %s%s", m.metrics.FPS, rw, rh, s.Scene.Camera.Yaw, s.Scene.Camera.Pitch, s.Scene.Camera.Radius, mode, warning)
	if s.Paused {
		status += " · paused"
	}
	prompt := muted.Render(m.status)
	if m.prompt {
		prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#78A7EA")).Render(m.input.View())
	}
	lines = append(lines, muted.Render(status), prompt)
	content := strings.Join(cellLines(strings.Join(lines, "\n"), max(1, m.cols), max(1, m.rows)), "\n")
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "bells-deep"
	return v
}
