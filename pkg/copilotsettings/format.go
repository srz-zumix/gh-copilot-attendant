package copilotsettings

import (
	"fmt"
	"io"
	"os"

	"github.com/cli/go-gh/v2/pkg/template"
	"github.com/itchyny/gojq"
)

// ValidateOutputExpression checks the expression that cmdutil's exporter will use.
func ValidateOutputExpression(jqExpression, templateExpression string) error {
	if jqExpression != "" {
		query, err := gojq.Parse(jqExpression)
		if err != nil {
			return fmt.Errorf("invalid --jq expression: %w", err)
		}
		if _, err := gojq.Compile(query, gojq.WithEnvironLoader(os.Environ)); err != nil {
			return fmt.Errorf("invalid --jq expression: %w", err)
		}
		return nil
	}
	if templateExpression != "" {
		if err := template.New(io.Discard, 80, false).Parse(templateExpression); err != nil {
			return fmt.Errorf("invalid --template expression: %w", err)
		}
	}
	return nil
}
