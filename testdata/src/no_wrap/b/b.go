package b

import "no_wrap/a"

func B() error { // want B:"naked"
	return a.A() // want `error returned from external package is not wrapped`
}
