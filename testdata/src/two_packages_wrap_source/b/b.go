package b

import (
	"fmt"
	"two_packages_wrap_source/a"
)

func B() error { // want B:"wrapped"
	return a.A()
}

func Canary() error { // want Canary:"unwrapped"
	return fmt.Errorf("canary") // want `is not wrapped`
}
