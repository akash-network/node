package common

import (
	"fmt"
	"os"
)

func HandleError(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
