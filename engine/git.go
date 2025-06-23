package engine

import (
	"context"
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
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var patterns = []string{
	//"*.php",
	"*.vue",
	"*.ts",
}

type branchProgressMsg struct {
	branch  string
	current int
	total   int
}

type gitIterateRepoModel struct {
	bars      map[string]progress.Model
	sizes     map[string]int
	current   map[string]int
	mu        sync.Mutex
	branchPrg progress.Model
}

type gitBasicModel struct {
	progress progress.Model
	total    int
	done     int
	done2    int
	title    string
	message  string
	message2 string
}

type CachedRepo struct {
	size                  int
	owner, repoName, path string
	repo                  *git.Repository
	mirror                bool
}

type countMsg struct{}
type countMsg2 struct{}

var clonedRepo *CachedRepo

func (m *gitBasicModel) Init() tea.Cmd {
	return nil
}

func (m *gitBasicModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case countMsg:
		m.done++
		return m, nil
	case countMsg2:
		m.done2++
		return m, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
	case tea.QuitMsg:
		return m, tea.Quit
	}
	return m, nil
}

func (m *gitBasicModel) View() string {
	percent := float64(m.done) / float64(m.total)
	return fmt.Sprintf(
		"%s\n%s %d/%d %s\n%d %s",
		m.title,
		m.progress.ViewAs(percent),
		m.done,
		m.total,
		m.message,
		m.done2,
		m.message2,
	)
}

func (m *gitIterateRepoModel) Init() tea.Cmd {
	return nil
}

func (m *gitIterateRepoModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m *gitIterateRepoModel) View() string {
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

func loadRepoFromPath(repoPath string, mirror bool) (*git.Repository, error) {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	}
	repoURL := "file://" + filepath.ToSlash(absPath)

	memStorage := memory.NewStorage()
	repo, err := git.Clone(memStorage, nil, &git.CloneOptions{
		URL:      repoURL,
		Mirror:   mirror,
		Progress: os.Stdout,
		Tags:     git.NoTags,
		//Depth:    10000,
	})
	if err != nil {
		return nil, err
	}

	return repo, nil
}

func cloneRepo(uri string, mirror bool) (*CachedRepo, error) {
	owner, repoName := getOwnerAndRepoFromUri(uri)

	if clonedRepo != nil {
		if clonedRepo.mirror == mirror {
			return clonedRepo, nil
		}
	}

	dataDir := makeDataDir()
	if dataDir == "" {
		return nil, fmt.Errorf("data directory unavailable")
	}
	repoPath := filepath.Join(dataDir, owner, repoName)

	var repo *git.Repository

	if _, err := os.Stat(repoPath); err == nil {
		utils.PrintInfo("Loading repository: " + repoPath)
		repo, err = loadRepoFromPath(repoPath, mirror)
		if err != nil {
			utils.PrintWarning("Removing corrupted repository: " + repoPath)
			os.RemoveAll(repoPath)
			repo = nil
		}
	}

	if repo == nil {
		utils.PrintInfo("Cloning repository: " + uri + " into " + repoPath)

		if err := os.MkdirAll(filepath.Dir(repoPath), 0755); err != nil {
			return nil, err
		}

		_, err := git.PlainClone(repoPath, true, &git.CloneOptions{
			URL:      uri,
			Mirror:   true,
			Progress: os.Stdout,
			Tags:     git.NoTags,
			Depth:    10000,
		})
		if err != nil {
			return nil, err
		}

		repo, err = loadRepoFromPath(repoPath, mirror)
		if err != nil {
			return nil, err
		}
	}

	size := getRepoSize(owner, repoName)

	newRepo := &CachedRepo{
		owner:    owner,
		repoName: repoName,
		repo:     repo,
		path:     repoPath,
		size:     size,
		mirror:   mirror,
	}

	clonedRepo = newRepo
	return newRepo, nil
}

func iterateRepo(gitUri string) []string {
	repository, err := cloneRepo(gitUri, true)
	if err != nil {
		utils.PrintError(err, "failed to clone repository")
	}

	repo := repository.repo

	fmt.Println("-------------")
	fmt.Println("Owner: ", repository.owner)
	fmt.Println("Repo: ", repository.repoName)
	fmt.Println("Size: ", repository.size)
	fmt.Println("-------------")

	utils.PrintInfo("Cloning Repository")

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

	m := &gitIterateRepoModel{
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

func findFirstFilesCommits(repoUri string, webserverHashes map[string]plumbing.Hash) (map[string]plumbing.Hash, error) {
	repository, err := cloneRepo(repoUri, false)
	if err != nil {
		return nil, err
	}

	repo := repository.repo

	result := make(map[string]plumbing.Hash)

	prgs := progress.New(
		progress.WithWidth(40),
		progress.WithoutPercentage(),
		progress.WithScaledGradient("#FF7CCB", "#FDFF8C"),
	)

	m := &gitBasicModel{
		progress: prgs,
		title:    "Finding first commits",
		message:  "files found",
		total:    len(webserverHashes),
		done:     0,
		message2: "commits checked",
		done2:    0,
	}

	p := tea.NewProgram(m)

	utils.PrintInfo("Finding commits for files")

	go func() {
		if _, err := p.Run(); err != nil {
			fmt.Println("Error running UI:", err)
		}
	}()

	remainingFiles := make(map[string]plumbing.Hash, len(webserverHashes))
	for k, v := range webserverHashes {
		remainingFiles[k] = v
	}

	// Create commit iterator (reverse chronological order)
	commitIter, err := repo.Log(&git.LogOptions{
		Order: git.LogOrderCommitterTime,
	})
	if err != nil {
		return nil, err
	}
	defer commitIter.Close()

	for len(remainingFiles) > 0 {
		commit, err := commitIter.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		p.Send(countMsg2{})

		var parentTree *object.Tree
		if parents := commit.Parents(); parents != nil {
			if parent, err := parents.Next(); err == nil {
				parentTree, _ = parent.Tree()
			}
		}

		currentTree, err := commit.Tree()
		if err != nil {
			continue
		}

		changes, err := object.DiffTreeWithOptions(
			context.Background(),
			parentTree,
			currentTree,
			&object.DiffTreeOptions{
				DetectRenames: true,
			},
		)
		if err != nil {
			continue
		}

		for _, change := range changes {
			file := change.To.Name
			if hash, exists := remainingFiles[file]; exists {
				if change.To.TreeEntry.Hash == hash {
					result[file] = commit.Hash
					delete(remainingFiles, file)
				}
			}
		}

		currentProgress := len(webserverHashes) - len(remainingFiles)
		if currentProgress > m.done {
			m.done = currentProgress
			p.Send(countMsg{})
		}
	}

	p.Quit()
	return result, nil
}

func findDeploymentRange(repoUri string, fileCommits map[string]plumbing.Hash) (plumbing.Hash, plumbing.Hash, []CommitScore, error) {
	repository, err := cloneRepo(repoUri, false)
	if err != nil {
		return plumbing.Hash{}, plumbing.Hash{}, nil, err
	}

	repo := repository.repo

	commitScores := make(map[plumbing.Hash]int)
	for _, commitHash := range fileCommits {
		commitScores[commitHash]++
	}

	utils.PrintInfo("Finding deployment range")

	// Rank commits by match frequency and recency
	var scores []CommitScore
	for hash, count := range commitScores {
		if commit, err := repo.CommitObject(hash); err == nil {
			scores = append(scores, CommitScore{
				Hash:  hash,
				Score: count,
				Time:  commit.Author.When,
			})
		}
	}

	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		return scores[i].Time.After(scores[j].Time)
	})

	if len(scores) == 0 {
		return plumbing.ZeroHash, plumbing.ZeroHash, nil, fmt.Errorf("no matching commits found")
	}

	upperCommit := scores[0].Hash
	lowerCommit := findNextFileChange(repo, upperCommit, fileCommits)

	return upperCommit, lowerCommit, scores[:min(5, len(scores))], nil
}

func findNextFileChange(repo *git.Repository, bestCommit plumbing.Hash, fileCommits map[string]plumbing.Hash) plumbing.Hash {
	commitIter, _ := repo.Log(&git.LogOptions{Order: git.LogOrderCommitterTime})
	fileSet := make(map[string]struct{})
	for f := range fileCommits {
		fileSet[f] = struct{}{}
	}

	foundBest := false
	for {
		commit, err := commitIter.Next()
		if err != nil {
			break
		}

		if commit.Hash == bestCommit {
			foundBest = true
			continue
		}

		if !foundBest {
			continue
		}

		changes, _ := getChangedFiles(commit)
		for _, file := range changes {
			if _, exists := fileSet[file]; exists {
				return commit.Hash
			}
		}
	}
	return plumbing.ZeroHash
}

func getChangedFiles(commit *object.Commit) ([]string, error) {
	if commit.NumParents() == 0 {
		return nil, nil
	}

	parent, _ := commit.Parent(0)
	currentTree, _ := commit.Tree()
	parentTree, _ := parent.Tree()

	changes, _ := object.DiffTree(parentTree, currentTree)
	files := make([]string, 0, len(changes))
	for _, change := range changes {
		files = append(files, change.To.Name)
	}
	return files, nil
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

func getRepoSize(owner, repo string) int {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("User-Agent", "go-repo-size-checker")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0
	}
	var info RepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return 0
	}
	return info.Size
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

func getGradientColor(ratio float64) lipgloss.Color {
	if ratio < 0.5 {
		return "1" // Red
	} else if ratio < 0.9 {
		return "3" // Yellow
	}
	return "2" // Green
}
