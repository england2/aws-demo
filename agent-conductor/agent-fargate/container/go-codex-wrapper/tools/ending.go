package tools

import (
	_ "embed"
	"fmt"
)

// For readability, this file/tool *should be* named "print-report-guide.go", but is named "ending"
// to dissuade the agent from calling it at any time other than right before shutdown.

//go:embed report_guide.md
var reportGuide string

func Ending() {
	fmt.Println(reportGuide)
}
