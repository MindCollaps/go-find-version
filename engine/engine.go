package engine

import (
	"fmt"
	"go-find-version/utils"
	"os"
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
		iterateRepo(args)
		saveFiles(files, owner, repoName)
	} else {
		loadedFiles, err := loadFiles(args.EnumerationGitFile)
		if err != nil {
			utils.PrintError(err, "Failed to load files")
		} else {
			files = append(files, loadedFiles...)
		}
	}

	existingFiles := CheckFileExists(files, args.WebsiteUrl)

	utils.PrintInfo("Enumeration complete")
	for i := range existingFiles {
		fmt.Println(existingFiles[i])
	}
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
