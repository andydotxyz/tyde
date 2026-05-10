package launcher

import (
	"regexp"
	"strconv"

	"fyne.io/fyne/v2"

	"fyshos.com/tyde"
	wmTheme "fyshos.com/tyde/theme"

	"github.com/Knetic/govaluate"
)

var (
	exprRegex = regexp.MustCompile(`^[0-9.+\-*/()<^>:%]+$`)
	numRegex  = regexp.MustCompile(`^[0-9.]+$`)
)

var calcMeta = tyde.ModuleMetadata{
	Name:        "Launcher: Calculate",
	NewInstance: newCalcSuggest,
}

type calc struct{}

func (c *calc) Destroy() {
}

func (c *calc) LaunchSuggestions(input string) []tyde.LaunchSuggestion {
	if !c.isExpression(input) {
		return nil
	}

	result, err := c.eval(input)
	if err != nil {
		return nil
	}
	return []tyde.LaunchSuggestion{&calcItem{sum: input, result: result}}
}

func (c *calc) eval(sum string) (string, error) {
	expression, err := govaluate.NewEvaluableExpression(sum)
	if err != nil {
		return "", err
	}

	res, err := expression.Evaluate(nil)
	if err != nil {
		return "", err
	}

	if f, ok := res.(float64); ok {
		return strconv.FormatFloat(f, 'f', -1, 64), nil
	}

	return "", nil
}

func (c *calc) Metadata() tyde.ModuleMetadata {
	return calcMeta
}

// isExpression will return true if input is a mathematical expression unless it just contains a number
func (c *calc) isExpression(input string) bool {
	return exprRegex.MatchString(input) && !numRegex.MatchString(input)
}

// newCalcSuggest creates a new module that will show calculations in the launcher suggestions
func newCalcSuggest() tyde.Module {
	return &calc{}
}

type calcItem struct {
	sum, result string
}

func (i *calcItem) Icon() fyne.Resource {
	return wmTheme.CalculateIcon
}

func (i *calcItem) Title() string {
	return i.sum + " = " + i.result
}

func (i *calcItem) Launch() {
	fyne.CurrentApp().Clipboard().SetContent(i.result)
}
