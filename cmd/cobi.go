package main

import (
	"fmt"
	"os"

	"github.com/catalogfi/cobi"
)

func main() {
	if err := cobi.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
