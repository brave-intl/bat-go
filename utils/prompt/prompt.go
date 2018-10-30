package prompt

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Bool prompts for y/n input returning a bool
func Bool() (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("(y/n): ")
		var text string
		text, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}
		if strings.ToLower(strings.TrimSpace(text)) == "n" {
			return false, nil
		} else if strings.ToLower(strings.TrimSpace(text)) == "y" {
			return true, nil
		} else {
			fmt.Println("Input must be \"y\" or \"n\"")
		}
	}
}
