package util

import (
	"fmt"
	"testing"
)

func TestPrintWorkerName(t *testing.T) {
	fmt.Println(GenerateWorkerName())
}
