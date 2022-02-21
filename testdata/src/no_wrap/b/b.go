package b

import "no_wrap/a"

func B() error { // want B:"unwrapped"
	return a.A() // want `error returned from external package is not wrapped`
}
