package session

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Asker interface {
	StringVar(string, string, bool) string
}

type asker struct {
	targetMode  ModeType
	currentMode ModeType
}

func NewInteractiveAsker(currentMode ModeType) Asker {
	return &asker{targetMode: ModeTypeInteractive, currentMode: currentMode}
}

func (a *asker) StringVar(str string, question string, required bool) string {
	return AskStringVar(a.currentMode, a.targetMode, str, question, required)

}

func AskStringVar(currentMode ModeType, targetMode ModeType, str string, question string, required bool) string {
	if currentMode != targetMode {
		return str
	}
	for len(str) == 0 {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("\n", question)
		res, _ := reader.ReadString('\n')
		if res = strings.TrimSpace(res); res != "" {
			str = res
		}
		if required == true {
			continue
		}
	}
	return str
}
