package engine

import (
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"go-find-version/utils"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var patterns = []string{
	"*php",
	"*.vue",
	"*.ts",
}

type RepoInfo struct {
	Size     int    `json:"size"`
	FullName string `json:"full_name"`
}

type branchProgressMsg struct {
	branch  string
	current int
	total   int
}

type model struct {
	bars    map[string]progress.Model
	sizes   map[string]int
	current map[string]int
	mu      sync.Mutex // Add mutex
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case branchProgressMsg:
		m.mu.Lock()
		defer m.mu.Unlock()
		m.current[msg.branch] = msg.current
		m.sizes[msg.branch] = msg.total
		return m, nil
	}
	return m, nil
}

func (m *model) View() string {
	//TODO Something wrong here
	m.mu.Lock()
	defer m.mu.Unlock()

	var branches []string
	for branch := range m.bars {
		branches = append(branches, branch)
	}
	sort.Strings(branches) // Keep order consistent

	s := ""
	for _, branch := range branches {
		bar := m.bars[branch]
		total := m.sizes[branch]
		current := m.current[branch]
		if total == 0 {
			continue
		}
		percent := float64(current) / float64(total)
		s += fmt.Sprintf("%s: %s\n", branch, bar.ViewAs(percent))
	}
	return s
}

func Run(args utils.Args) {
	owner, repoName := getOwnerAndRepoFromUri(args.GitUrl)
	repoSize, err := getRepoSize(owner, repoName)
	totalSize := 0
	if err == nil {
		totalSize = repoSize
	}

	fmt.Println("-------------")
	fmt.Println("Owner: ", owner)
	fmt.Println("Repo: ", repoName)
	fmt.Println("Size: ", totalSize)
	fmt.Println("-------------")

	utils.PrintInfo("Cloning Repository")

	repo, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL: args.GitUrl,
	})
	if err != nil {
		utils.PrintError(err, "failed to clone repository")
		return
	}
	utils.PrintInfo("Cloned repository: " + args.GitUrl)

	branchesIter, err := repo.Branches()
	utils.PrintInfo("Fetching branches")
	if err != nil {
		utils.PrintError(err, "failed to get branches")
	}

	// Collect branch references
	var branchRefs []*plumbing.Reference
	branchesIter.ForEach(func(ref *plumbing.Reference) error {
		branchRefs = append(branchRefs, ref)
		return nil
	})

	branchFilesMap := make(map[string][]string)
	var branchFilesMu sync.Mutex

	var wg sync.WaitGroup

	// Initialize model with progress bars for each branch
	m := &model{
		bars:    make(map[string]progress.Model),
		sizes:   make(map[string]int),
		current: make(map[string]int),
	}
	for _, ref := range branchRefs {
		branchName := ref.Name().Short()
		m.bars[branchName] = progress.New(
			progress.WithWidth(40),
			progress.WithoutPercentage(),
			progress.WithScaledGradient("#FF7CCB", "#FDFF8C"))
		m.sizes[branchName] = 0 // Will be updated as files are found
		m.current[branchName] = 0
	}

	utils.PrintInfo(strconv.Itoa(len(branchRefs)) + " branches to process")

	p := tea.NewProgram(m)

	done := make(chan struct{})
	go func() {
		// Block until the UI exits
		if _, err := p.Run(); err != nil {
			panic(err)
		}
		utils.PrintInfo("Program exited")
		close(done)
	}()

	for _, ref := range branchRefs {
		branchName := ref.Name().Short()
		wg.Add(1)

		go func(ref *plumbing.Reference, branchName string) {
			defer wg.Done()

			utils.PrintInfo("Processing branch: " + branchName)

			// Get commits for THIS BRANCH specifically
			commitIter, err := repo.Log(&git.LogOptions{
				From: ref.Hash(),
			})
			if err != nil {
				return
			}

			// First pass: collect all commits
			var commits []*object.Commit
			commitIter.ForEach(func(c *object.Commit) error {
				commits = append(commits, c)
				return nil
			})

			utils.PrintInfo("Processing commit: " + strconv.Itoa(len(commits)) + " commits")

			// Second pass: count files
			allFiles := []string{}
			for _, c := range commits {
				files, err := c.Files()
				if err != nil {
					continue
				}
				files.ForEach(func(file *object.File) error {
					allFiles = append(allFiles, file.Name)
					return nil
				})
			}

			// Update progress bar total
			p.Send(branchProgressMsg{
				branch:  branchName,
				current: 0,
				total:   len(allFiles),
			})

			// Third pass: process files
			processed := 0
			var branchFiles []string
			for i := range allFiles {
				branchFiles = appendFile(branchFiles, allFiles[i])
				processed++
				p.Send(branchProgressMsg{
					branch:  branchName,
					current: processed,
					total:   len(allFiles),
				})
			}

			// Save results
			branchFilesMu.Lock()
			branchFilesMap[branchName] = branchFiles
			branchFilesMu.Unlock()
		}(ref, branchName)
	}

	// Wait for all goroutines, then quit Bubble Tea
	go func() {
		wg.Wait()
		utils.PrintInfo("Finished processing branches")
		p.Quit()
	}()

	<-done // Wait for Bubble Tea to finish

	// Merge all files after processing
	var allFiles []string
	for _, files := range branchFilesMap {
		for _, file := range files {
			allFiles = appendFile(allFiles, file)
		}
	}

	interestingFiles := filterFiles(allFiles, patterns)
	for _, s := range interestingFiles {
		fmt.Println(s)
	}
}

func getOwnerAndRepoFromUri(uri string) (string, string) {
	uri = strings.Replace(uri, "https://", "", -1)
	uriParts := strings.Split(uri, "/")
	if len(uriParts) < 3 {
		return "", ""
	}
	return uriParts[1], uriParts[2]
}

func getRepoSize(owner, repo string) (int, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "go-repo-size-checker")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}
	var info RepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return 0, err
	}
	return info.Size, nil
}

func appendFile(files []string, file string) []string {
	for _, f := range files {
		if f == file {
			return files
		}
	}
	return append(files, file)
}

func filterFiles(files []string, patterns []string) []string {
	var ps []gitignore.Pattern
	for _, p := range patterns {
		ps = append(ps, gitignore.ParsePattern(p, nil))
	}
	matcher := gitignore.NewMatcher(ps)
	var filteredFiles []string
	for _, f := range files {
		pathSegments := []string{f}
		if strings.Contains(f, "/") {
			pathSegments = strings.Split(f, "/")
		}
		match := matcher.Match(pathSegments, false)
		if !match {
			filteredFiles = append(filteredFiles, f)
		}
	}
	return filteredFiles
}
