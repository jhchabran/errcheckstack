package b

import (
	"fmt"
	"wrap_source/a"
)

func B() error { // want B:"wrapped"
	return a.A()
}

func Canary() error { // want Canary:"naked"
	return fmt.Errorf("canary") // want `error returned from external package is not wrapped`
}
