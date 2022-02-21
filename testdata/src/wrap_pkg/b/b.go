package b

import (
	"fmt"
	"wrap_pkg/a"
)

func B() error {
	return a.A()
}

func Canary() error {
	return fmt.Errorf("canary") // want `error returned from external package is unwrapped`
}
