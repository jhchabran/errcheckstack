package b

import (
	"fmt"
	"wrap_pkg/a"
)

func B() error { // want B:"wrapped"
	return a.A()
}

func Canary() error { // want Canary:"unwrapped"
	return fmt.Errorf("canary") // want `error returned from external package is not wrapped`
}
