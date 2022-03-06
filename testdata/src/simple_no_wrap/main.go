package main

import (
	"encoding/json"
)

func main() {
	do()
}

func do() error { // want do:"naked"
	_, err := json.Marshal(struct{}{})
	if err != nil {
		return err // want `error returned from external package is not wrapped`
	}

	return nil
}
