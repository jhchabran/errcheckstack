package main

import (
	"interface_no_wrap/a"
	"interface_no_wrap/b"
)

func main() {
	aer := a.As{}
	b.B(&aer)
	b.Canary()
}
