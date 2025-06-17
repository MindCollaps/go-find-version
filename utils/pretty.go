package utils

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

type Args struct {
	GitUrl     string `arg:"-g,--git,required" help:"Source of git repository."`
	WebsiteUrl string `arg:"-u,--url" help:"Source of the vulnerable website."` // TODO: Make required
	DisableWeb bool   `arg:"-w,--web" help:"Disables the website."`
	Port       int    `arg:"-p,--port" default:"8080" help:"Port for the website."`
}

func PrintError(err error) {
	if err != nil {
		prefix := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			Bold(true).
			Render("[!] Error:")

		message := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Render(" " + err.Error())

		fmt.Println(prefix + message)
	}
}

func PrintInfo(info string) {
	prefix := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#0000FF")).
		Bold(true).
		Render("[*] Info:")

	message := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Render(" " + info)

	fmt.Println(prefix + message)
}
