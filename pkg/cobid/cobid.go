package cobid

import "github.com/catalogfi/cobi/pkg/cobid/executor"

type cobi struct {
	executors executor.Executors
}

func NewCobi() {
	bot
}
