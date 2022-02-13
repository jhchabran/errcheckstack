package errcheckstack

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/tools/go/analysis/analysistest"
)

var vendoredDeps []string = []string{
	"github.com",
	"golang.org",
	"gopkg.in",
}

func TestAnalyzer(t *testing.T) {
	p, err := filepath.Abs("./testdata/src")
	assert.NoError(t, err)

	files, err := ioutil.ReadDir(p)
	assert.NoError(t, err)

OUTER:
	for _, f := range files {
		for _, v := range vendoredDeps {
			if f.Name() == v {
				continue OUTER
			}
		}

		t.Run(f.Name(), func(t *testing.T) {
			if !f.IsDir() {
				t.Fatalf("cannot run on non-directory: %s", f.Name())
			}

			analysistest.Run(t, analysistest.TestData(), NewAnalyzer(), f.Name())
		})
	}
}
