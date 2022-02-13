package errcheckstack

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	p, err := filepath.Abs("./testdata/src")
	assert.NoError(t, err)

	files, err := ioutil.ReadDir(p)
	assert.NoError(t, err)

	for _, f := range files {
		t.Run(f.Name(), func(t *testing.T) {
			if !f.IsDir() {
				t.Fatalf("cannot run on non-directory: %s", f.Name())
			}

			// dirPath, err := filepath.Abs(path.Join("./testdata", f.Name()))
			// assert.NoError(t, err)

			analysistest.Run(t, analysistest.TestData(), NewAnalyzer(), f.Name())
		})
	}
}
