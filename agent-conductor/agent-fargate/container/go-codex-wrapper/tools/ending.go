package tools

import (
	_ "embed"
	"fmt"
)

// For readability, this file/tool *should be* named "print-report-guide", but is named "ending"
// to dissuade the agent from calling it at any time other than right before shutdown.

//go:embed ending-report.md
var endingReport string

//go:embed ending.go.md
var EndingGuide string

func Ending() {
	fmt.Println(endingReport)
}
