package engine

import (
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	"go-find-version/utils"
)

func Run(args utils.Args) {
	repo, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL: args.GitUrl,
	})

	if err != nil {
		utils.PrintError(fmt.Errorf("failed to clone repository: %w", err))
		return
	}

	utils.PrintInfo("Cloned repository: " + args.GitUrl)

	_, err = repo.CommitObjects()

	if err != nil {
		utils.PrintError(fmt.Errorf("failed to retreive commits: %w", err))
	}
}
