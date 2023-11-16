package main

import (
	"fmt"
	"os"

	"github.com/catalogfi/cobi/cli"
)

var BinaryVersion = "undefined"

func main() {
	if err := cli.Run(BinaryVersion); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
