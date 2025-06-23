package utils

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

func PrintError(err error, msg string) {
	if err != nil {
		prefix := lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")).
			Bold(true).
			Render("[!] Error:")

		message := lipgloss.NewStyle().
			Render(" " + msg + "\n" + err.Error())

		fmt.Println(prefix + message)
	}
}

func PrintInfo(info string) {
	prefix := lipgloss.NewStyle().
		Foreground(lipgloss.Color("4")).
		Bold(true).
		Render("[*] Info:")

	message := lipgloss.NewStyle().
		Render(" " + info)

	fmt.Println(prefix + message)
}

func PrintWarning(warning string) {
	prefix := lipgloss.NewStyle().
		Foreground(lipgloss.Color("4")).
		Bold(true).
		Render("[!] Warning:")
	message := lipgloss.NewStyle().
		Render(" " + warning)
	fmt.Println(prefix + message)
}
