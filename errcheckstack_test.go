package errcheckstack

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	_ "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"golang.org/x/tools/go/analysis/analysistest"
)

var vendoredDeps []string = []string{
	"github.com",
	"golang.org",
	"gopkg.in",
	"modules.txt",
}

func TestAnalyzer(t *testing.T) {
	p, err := filepath.Abs("./testdata/src")
	assert.NoError(t, err)

	files, err := ioutil.ReadDir(p)
	assert.NoError(t, err)

OUTER:
	for _, f := range files {
		if f.Name() != "wrap_source" {
			continue
		}
		for _, v := range vendoredDeps {
			if f.Name() == v {
				continue OUTER
			}
		}
		if !f.IsDir() {
			t.Fatalf("cannot run on non-directory: %s", f.Name())
		}

		Analyzer.Flags.Set("module", f.Name())
		t.Run(f.Name(), func(t *testing.T) {
			analysistest.Run(t, analysistest.TestData(), Analyzer, f.Name()+"/...")
		})
	}
}
