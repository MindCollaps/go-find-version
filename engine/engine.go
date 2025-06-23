package engine

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"go-find-version/utils"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RepoInfo struct {
	Size     int    `json:"size"`
	FullName string `json:"full_name"`
}

func Run(args utils.Args) {
	owner, repoName := getOwnerAndRepoFromUri(args.GitUrl)

	files := []string{}

	if args.EnumerationGitFile == "" {
		files = iterateRepo(args.GitUrl)
		saveFiles(files, owner, repoName)
	} else {
		loadedFiles, err := loadFiles(args.EnumerationGitFile)
		if err != nil {
			utils.PrintError(err, "Failed to load files")
		} else {
			files = append(files, loadedFiles...)
		}
	}

	utils.PrintInfo(fmt.Sprintf("Found %d files that will be checked on the remote server", len(files)))

	fileHashes := checkFileHashes(files, args.WebsiteUrl)

	utils.PrintInfo(fmt.Sprintf("Found %d files on remote server", len(fileHashes)))

	commits, err := findFirstFilesCommits(args.GitUrl, fileHashes)

	utils.PrintInfo(fmt.Sprintf("Found %d files in commits", len(commits)))

	if err != nil {
		utils.PrintError(err, "Failed to find first commits")
	}

	lower, upper, scores, err := findDeploymentRange(args.GitUrl, commits)

	if err != nil {
		utils.PrintError(err, "Failed to find deployment range")
	}

	displayDeploymentInfo(args.GitUrl, lower, upper, scores)
}

func displayDeploymentInfo(repoUri string, lower, upper plumbing.Hash, scores []CommitScore) {
	repository, err := cloneRepo(repoUri, false)
	if err != nil {
		return
	}

	repo := repository.repo
	lowerCommit, _ := repo.CommitObject(lower)
	upperCommit, _ := repo.CommitObject(upper)

	commitCount := 0
	commitIter, _ := repo.Log(&git.LogOptions{
		Order: git.LogOrderCommitterTime,
	})
	_ = commitIter.ForEach(func(c *object.Commit) error {
		if c.Hash == upper {
			return nil
		}
		if c.Hash == lower {
			return storer.ErrStop
		}
		commitCount++
		return nil
	})

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FF7CCB")).
		Underline(true).
		MarginBottom(1)

	subHeaderStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FDFF8C")).
		MarginBottom(1)

	commitHashStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7FFFD4")).
		Bold(true)

	commitMessageStyle := lipgloss.NewStyle().
		Italic(true).
		Foreground(lipgloss.Color("#FFD700"))

	authorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#87CEEB"))

	dateStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#D3D3D3")).
		Italic(true)

	linkStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00BFFF")).
		Underline(true)

	countStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF69B4")).
		Bold(true)

	// Generate GitHub links
	lowerLink := fmt.Sprintf("https://github.com/%s/%s/commit/%s", repository.owner, repository.repoName, lower)
	upperLink := fmt.Sprintf("https://github.com/%s/%s/commit/%s", repository.owner, repository.repoName, upper)
	compareLink := fmt.Sprintf("https://github.com/%s/%s/compare/%s..%s", repository.owner, repository.repoName, lower, upper)

	var output strings.Builder

	output.WriteString(headerStyle.Render("🚀 Deployment Analysis Results\n"))

	// Lower commit info
	output.WriteString(subHeaderStyle.Render("Webserver State Source\n"))
	output.WriteString(fmt.Sprintf("  %s %s\n",
		commitHashStyle.Render(lower.String()[:7]),
		linkStyle.Render(lowerLink),
	))
	output.WriteString(fmt.Sprintf("  📝 %s\n", commitMessageStyle.Render(firstLine(lowerCommit.Message))))
	output.WriteString(fmt.Sprintf("  👤 %s\n", authorStyle.Render(lowerCommit.Author.Name)))
	output.WriteString(fmt.Sprintf("  📅 %s\n\n", dateStyle.Render(lowerCommit.Author.When.Format(time.RFC1123))))

	// Upper commit info
	output.WriteString(subHeaderStyle.Render("Next Change Detected\n"))
	output.WriteString(fmt.Sprintf("  %s %s\n",
		commitHashStyle.Render(upper.String()[:7]),
		linkStyle.Render(upperLink),
	))
	output.WriteString(fmt.Sprintf("  📝 %s\n", commitMessageStyle.Render(firstLine(upperCommit.Message))))
	output.WriteString(fmt.Sprintf("  👤 %s\n", authorStyle.Render(upperCommit.Author.Name)))
	output.WriteString(fmt.Sprintf("  📅 %s\n\n", dateStyle.Render(upperCommit.Author.When.Format(time.RFC1123))))

	// Commit range info
	output.WriteString(subHeaderStyle.Render("Deployment Range\n"))
	output.WriteString(fmt.Sprintf("  Commits between states: %s\n", countStyle.Render(fmt.Sprintf("%d", commitCount))))
	output.WriteString(fmt.Sprintf("  Compare changes: %s\n\n", linkStyle.Render(compareLink)))

	// Top commits
	output.WriteString(subHeaderStyle.Render("✨ Top Matching Commits\n"))
	for i, score := range scores[:min(5, len(scores))] {
		commit, _ := repo.CommitObject(score.Hash)
		commitLink := fmt.Sprintf("https://github.com/%s/%s/commit/%s", repository.owner, repository.repoName, score.Hash)

		output.WriteString(fmt.Sprintf("\n  %s. %s %s",
			countStyle.Render(fmt.Sprintf("%d", i+1)),
			commitHashStyle.Render(score.Hash.String()[:7]),
			linkStyle.Render(commitLink),
		))
		output.WriteString(fmt.Sprintf("     📁 %s files matched\n", countStyle.Render(fmt.Sprintf("%d", score.Score))))
		output.WriteString(fmt.Sprintf("     💬 %s\n", commitMessageStyle.Render(firstLine(commit.Message))))
	}

	fmt.Println(output.String())
}

func firstLine(s string) string {
	if idx := strings.Index(s, "\n"); idx != -1 {
		return s[:idx]
	}
	return s
}

func getFilename(owner, repoName string) string {
	today := time.Now().Format("2006-01-02")
	return fmt.Sprintf("%s-%s-%s-interesting_files.txt", today, owner, repoName)
}

func saveFiles(interestingFiles []string, owner, repoName string) error {
	filename := getFilename(owner, repoName)
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	for _, filepath := range interestingFiles {
		_, err = file.WriteString(filepath + "\n")
		if err != nil {
			return fmt.Errorf("failed to write file: %v", err)
		}
	}
	return nil
}

func loadFiles(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}
	lines := strings.Split(string(data), "\n")
	var result []string
	for _, line := range lines {
		if line != "" {
			result = append(result, line)
		}
	}
	return result, nil
}

func makeDataDir() string {
	execDir, err := os.Getwd()
	if err != nil {
		utils.PrintError(err, "failed to get current working directory")
		return ""
	}

	dataDir := filepath.Join(execDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		utils.PrintError(err, "failed to create data directory")
		return ""
	}
	return dataDir + "/"
}
