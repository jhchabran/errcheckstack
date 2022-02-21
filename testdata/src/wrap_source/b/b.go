package b

import (
	"fmt"
	"wrap_source/a"
)

func B() error { // want B:"wrapped"
	return a.A()
}

func Canary() error { // want Canary:"unwrapped"
	return fmt.Errorf("canary") // want `error returned is not wrapped`
}
