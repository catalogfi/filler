package handlers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/catalogfi/cobi/utils"
)

func Status(service Service, account uint32) bool {
	if service == "executor" {
		if _, err := os.Stat(filepath.Join(utils.DefaultCobiDirectory(), fmt.Sprintf("executor_account_%d.pid", account))); err != nil {
			return false
		}

	}
	if _, err := os.Stat(filepath.Join(utils.DefaultCobiDirectory(), fmt.Sprintf("%s.pid", service))); err != nil {
		return false
	}

	return true
}
