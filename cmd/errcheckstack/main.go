package main

import (
	"github.com/jhchabran/errcheckstack/errcheckstack"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(errcheckstack.NewAnalyzer())
}
