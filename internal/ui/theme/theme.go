package theme

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/mateuslh/lealing/sdk/protocol"
)

// Theme é a paleta já materializada em estilos do lipgloss.
//
// Os estilos são calculados uma única vez, na construção: View() roda a cada
// frame e a cada tecla, então montar estilo dentro do render é justamente o
// tipo de custo que faz uma TUI parecer travada em terminais lentos.
type Theme struct {
	Palette

	// Estrutura da janela.
	App     lipgloss.Style
	Topbar  lipgloss.Style
	Brand   lipgloss.Style
	Meta    lipgloss.Style
	Status  lipgloss.Style
	Divider lipgloss.Style

	// Painéis.
	Panel      lipgloss.Style
	PanelFocus lipgloss.Style
	PanelTitle lipgloss.Style

	// Tipografia.
	Title    lipgloss.Style
	Subtitle lipgloss.Style
	Body     lipgloss.Style
	Dim      lipgloss.Style
	Ghost    lipgloss.Style
	Strong   lipgloss.Style

	// Listas.
	Item         lipgloss.Style
	ItemSelected lipgloss.Style
	ItemGlyph    lipgloss.Style
	ItemDesc     lipgloss.Style
	MatchHint    lipgloss.Style

	// Elementos.
	Badge     lipgloss.Style
	Pill      lipgloss.Style
	KeyCap    lipgloss.Style
	KeyLabel  lipgloss.Style
	Counter   lipgloss.Style
	Scrollbar lipgloss.Style
	Cursor    lipgloss.Style
}

// New materializa um tema a partir de uma paleta.
func New(p Palette) *Theme {
	t := &Theme{Palette: p}

	t.App = lipgloss.NewStyle().Foreground(p.Text)
	t.Topbar = lipgloss.NewStyle().Foreground(p.Muted).Padding(0, 2)
	t.Brand = lipgloss.NewStyle().Bold(true)
	t.Meta = lipgloss.NewStyle().Foreground(p.Faint)
	t.Status = lipgloss.NewStyle().Foreground(p.Muted).Padding(0, 2)
	t.Divider = lipgloss.NewStyle().Foreground(p.Border)

	t.Panel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Border).
		Padding(0, 1)
	t.PanelFocus = t.Panel.BorderForeground(p.BorderFocus)
	t.PanelTitle = lipgloss.NewStyle().Foreground(p.Muted).Bold(true)

	t.Title = lipgloss.NewStyle().Foreground(p.Text).Bold(true)
	t.Subtitle = lipgloss.NewStyle().Foreground(p.Muted)
	t.Body = lipgloss.NewStyle().Foreground(p.Text)
	t.Dim = lipgloss.NewStyle().Foreground(p.Muted)
	t.Ghost = lipgloss.NewStyle().Foreground(p.Faint)
	t.Strong = lipgloss.NewStyle().Foreground(p.Primary).Bold(true)

	t.Item = lipgloss.NewStyle().Foreground(p.Text)
	t.ItemSelected = lipgloss.NewStyle().Foreground(p.Text).Bold(true)
	t.ItemGlyph = lipgloss.NewStyle().Foreground(p.Primary)
	t.ItemDesc = lipgloss.NewStyle().Foreground(p.Faint)
	t.MatchHint = lipgloss.NewStyle().Foreground(p.Accent).Bold(true)

	t.Badge = lipgloss.NewStyle().Padding(0, 1).Foreground(p.OnPrimary).Background(p.Primary)
	t.Pill = lipgloss.NewStyle().Padding(0, 1).Foreground(p.Muted).Background(p.Overlay)
	t.KeyCap = lipgloss.NewStyle().Foreground(p.Text).Background(p.Overlay).Padding(0, 1)
	t.KeyLabel = lipgloss.NewStyle().Foreground(p.Faint)
	t.Counter = lipgloss.NewStyle().Foreground(p.Faint)
	t.Scrollbar = lipgloss.NewStyle().Foreground(p.Border)
	t.Cursor = lipgloss.NewStyle().Foreground(p.Primary).Bold(true)

	return t
}

// Default devolve o tema padrão do lealing.
func Default() *Theme { return New(Midnight()) }

// From materializa a paleta negociada pelo protocolo. Os campos estruturais
// que screen-v1 não transmite usam a mesma superfície, sem inventar cores na
// tela da tool.
func From(source protocol.Theme) *Theme {
	fallback := Midnight()
	color := func(value string, defaultColor lipgloss.AdaptiveColor) lipgloss.AdaptiveColor {
		if value == "" {
			return defaultColor
		}
		return lipgloss.AdaptiveColor{Dark: value, Light: value}
	}
	p := fallback
	p.Base = color(source.Surface, fallback.Base)
	p.Surface = color(source.Surface, fallback.Surface)
	p.Overlay = color(source.Surface, fallback.Overlay)
	p.Border = color(source.Border, fallback.Border)
	p.BorderFocus = p.Border
	p.Text = color(source.Text, fallback.Text)
	p.Muted = color(source.Muted, fallback.Muted)
	p.Faint = color(source.Faint, fallback.Faint)
	p.Primary = color(source.Primary, fallback.Primary)
	p.Secondary = color(source.Secondary, fallback.Secondary)
	p.Accent = color(source.Accent, fallback.Accent)
	p.Success = color(source.Success, fallback.Success)
	p.Warning = color(source.Warning, fallback.Warning)
	p.Danger = color(source.Danger, fallback.Danger)
	p.Spectrum = []lipgloss.AdaptiveColor{p.Primary, p.Secondary, p.Accent, p.Success, p.Warning, p.Danger}
	return New(p)
}

// SpectrumStyle devolve um estilo de texto na cor do spectrum indicada.
//
// Não se chama Accent porque Palette já embute um token com esse nome, e um
// método sombreando o campo promovido tornaria t.Accent ambíguo na leitura.
func (t *Theme) SpectrumStyle(i int) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.SpectrumAt(i))
}

// Key renderiza um atalho no formato "tecla ação", usado na barra de status.
func (t *Theme) Key(cap, label string) string {
	return t.KeyCap.Render(cap) + " " + t.KeyLabel.Render(label)
}

// Logo devolve o wordmark do lealing em gradiente, na versão compacta.
func (t *Theme) Logo() string {
	return t.Brand.Render(Text("lealing", t.Gradient))
}
