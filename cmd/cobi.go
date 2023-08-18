package main

import (
	"fmt"
	"os"

	"github.com/catalogfi/cobi"
)

var BinaryVersion = "undefined"

func main() {
	if err := cobi.Run(BinaryVersion); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
