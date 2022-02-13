package b

import (
	"fmt"
	"two_packages_wrap_pkg/a"
)

func B() error {
	return a.A()
}

func Canary() error {
	return fmt.Errorf("canary") // want `error returned from external package is unwrapped`
}
