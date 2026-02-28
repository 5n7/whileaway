package output

import (
	"fmt"

	"github.com/charmbracelet/glamour"
)

// RenderTerminal renders markdown to the terminal using glamour.
func RenderTerminal(content string) error {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(120),
	)
	if err != nil {
		return fmt.Errorf("creating terminal renderer: %w", err)
	}

	rendered, err := renderer.Render(content)
	if err != nil {
		return fmt.Errorf("rendering markdown: %w", err)
	}

	fmt.Print(rendered)
	return nil
}
