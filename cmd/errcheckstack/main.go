package main

import (
	"fmt"
	"os"

	"github.com/jhchabran/errcheckstack"
	"golang.org/x/tools/go/analysis/singlechecker"
	"gopkg.in/yaml.v3"
)

func main() {
	b, err := os.ReadFile("config.yml")
	if err != nil {
		panic(err)
	}
	var cfg errcheckstack.Config
	err = yaml.Unmarshal(b, &cfg)
	if err != nil {
		panic(err)
	}
	fmt.Println(cfg)
	singlechecker.Main(errcheckstack.NewAnalyzer(cfg))
}
