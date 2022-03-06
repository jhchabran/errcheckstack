package b

import (
	"fmt"
	"interface_no_wrap/a"
)

func B(aer a.Aer) error { // want B:"naked"
	err := aer.A()
	return err // want `error returned from interface type is not wrapped`
}

func Canary() error { // want Canary:"naked"
	return fmt.Errorf("canary") // want `error returned from external package is not wrapped`
}
