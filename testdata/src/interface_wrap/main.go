package main

import (
	"interface_wrap/a"
	"interface_wrap/b"
)

func main() {
	aer := a.As{}
	b.B(&aer)
	b.Canary()
}
