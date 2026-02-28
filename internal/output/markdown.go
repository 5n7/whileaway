package output

import (
	"fmt"
	"os"
)

// WriteMarkdownFile writes the report to a file.
func WriteMarkdownFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("writing markdown file: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Report written to %s\n", path)
	return nil
}
