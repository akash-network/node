package session

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func AskStringArgs(mode Mode, modeType ModeType, args []string, question string, required bool) []string {

	if mode.Type() != modeType {
		return args
	}

	for len(args) == 0 {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print(question)
		res, _ := reader.ReadString('\n')
		if res = strings.TrimSpace(res); res != "" {
			args = []string{res}
		}
		if required == true {
			continue
		}
	}
	return args
}
