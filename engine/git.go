package engine

import (
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"go-find-version/utils"
	"io"
	"net/http"
	"os"
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

type branchProgressMsg struct {
	branch  string
	current int
	total   int
}

type model struct {
	bars      map[string]progress.Model
	sizes     map[string]int
	current   map[string]int
	mu        sync.Mutex
	branchPrg progress.Model
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

	case tea.QuitMsg:
		return m, tea.Quit

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *model) View() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var branches []string
	for branch := range m.bars {
		branches = append(branches, branch)
	}
	sort.Strings(branches)

	// Find the longest branch name
	maxNameLen := 0
	for _, branch := range branches {
		if len(branch) > maxNameLen {
			maxNameLen = len(branch)
		}
	}

	finishedBranches := 0
	for _, branch := range branches {
		total := m.sizes[branch]
		current := m.current[branch]
		if total != 0 && current == total {
			finishedBranches++
		}
	}

	ratio := float64(finishedBranches) / float64(len(branches))
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(getGradientColor(ratio)).
		Padding(0, 1)

	// Prepare the branch name style with fixed width
	branchNameStyle := lipgloss.NewStyle().Width(maxNameLen)

	paddedNum := branchNameStyle.Render(style.Render(fmt.Sprintf("%d/%d", finishedBranches, len(branches))))

	s := fmt.Sprintf("%s %s\n",
		paddedNum,
		m.branchPrg.ViewAs(ratio),
	)

	for _, branch := range branches {
		bar := m.bars[branch]
		total := m.sizes[branch]
		current := m.current[branch]
		if total == 0 {
			continue
		}
		percent := float64(current) / float64(total)

		// Pad branch name with lipgloss
		paddedBranch := branchNameStyle.Render(branch)

		msg := strconv.Itoa(total) + " Commits"
		if current == total {
			msg += " ✅"
		}

		s += fmt.Sprintf("%s: %s %s\n", paddedBranch, bar.ViewAs(percent), msg)
	}
	return s
}

func getGradientColor(ratio float64) lipgloss.Color {
	if ratio < 0.5 {
		return "1" // Red
	} else if ratio < 0.9 {
		return "3" // Yellow
	}
	return "2" // Green
}

func iterateRepo(args utils.Args) []string {
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
		URL:        args.GitUrl,
		Progress:   os.Stdout,
		Mirror:     true,
		NoCheckout: true,
		Tags:       git.NoTags,
	})
	if err != nil {
		utils.PrintError(err, "failed to clone repository")
		return []string{}
	}
	utils.PrintInfo("Cloned repository: " + args.GitUrl)

	utils.PrintInfo("Fetching branches")
	branchesIter, err := repo.Branches()
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
	sem := make(chan struct{}, 3) // Three routines parallel

	// Initialize model with progress bars for each branch
	m := &model{
		bars:    make(map[string]progress.Model),
		sizes:   make(map[string]int),
		current: make(map[string]int),
		mu:      sync.Mutex{},
		branchPrg: progress.New(
			progress.WithWidth(40),
			progress.WithoutPercentage(),
			progress.WithScaledGradient("#190087", "#C364FA")),
	}

	for _, ref := range branchRefs {
		branchName := ref.Name().Short()
		m.bars[branchName] = progress.New(
			progress.WithWidth(40),
			progress.WithoutPercentage(),
			progress.WithScaledGradient("#FF7CCB", "#FDFF8C"))
		m.sizes[branchName] = 0
		m.current[branchName] = 1
	}

	utils.PrintInfo(strconv.Itoa(len(branchRefs)) + " branches to process")

	p := tea.NewProgram(m)

	go func() {
		p.Run()
	}()

	for _, ref := range branchRefs {
		branchName := ref.Name().Short()
		wg.Add(1)
		sem <- struct{}{}

		go func(ref *plumbing.Reference, branchName string) {
			defer wg.Done()
			defer func() { <-sem }()

			commitIter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
			if err != nil {
				return
			}

			// 1. Use lightweight commit history
			var commits []*plumbing.Hash
			commitIter.ForEach(func(c *object.Commit) error {
				commits = append(commits, &c.Hash)
				return nil
			})

			commitIter.Close()

			// 2. Tree-based file collection
			fileSet := make(map[string]struct{})
			for i, commitHash := range commits {
				err := processCommit(repo, *commitHash, fileSet, p, branchName, i+1, len(commits))
				if err != nil {
					continue
				}

				// Update progress every 10 commits or on last commit
				if i%10 == 0 || i == len(commits)-1 {
					p.Send(branchProgressMsg{
						branch:  branchName,
						current: i + 1,
						total:   len(commits),
					})
				}
			}

			// 3. Convert map to slice
			branchFiles := make([]string, 0, len(fileSet))
			for file := range fileSet {
				branchFiles = append(branchFiles, file)
			}

			branchFilesMu.Lock()
			branchFilesMap[branchName] = branchFiles
			branchFilesMu.Unlock()
		}(ref, branchName)
	}

	wg.Wait()
	p.Quit()
	utils.PrintInfo("Finished processing branches")

	// Merge all files after processing
	uniqueFiles := make(map[string]struct{})
	for _, files := range branchFilesMap {
		for _, file := range files {
			uniqueFiles[file] = struct{}{}
		}
	}

	allFiles := make([]string, 0, len(uniqueFiles))
	for file := range uniqueFiles {
		allFiles = append(allFiles, file)
	}

	interestingFiles := filterFiles(allFiles, patterns)

	return interestingFiles
}

func processCommit(repo *git.Repository, hash plumbing.Hash, fileSet map[string]struct{}, p *tea.Program, branchName string, current, total int) error {
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return err
	}
	tree, err := commit.Tree()
	if err != nil {
		return err
	}
	treeWalker := object.NewTreeWalker(tree, true, nil)
	defer treeWalker.Close()

	for {
		name, entry, err := treeWalker.Next()
		if err == io.EOF {
			break
		}
		if err != nil || entry.Mode == filemode.Dir {
			continue
		}
		fileSet[name] = struct{}{}
	}
	return nil
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
