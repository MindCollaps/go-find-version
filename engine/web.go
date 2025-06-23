package engine

import (
	"fmt"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/gocolly/colly"
	"go-find-version/utils"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

type CommitScore struct {
	Hash  plumbing.Hash
	Score int
	Time  time.Time
}

type fileCheckedMsg struct {
	error bool
}

type webFetchModel struct {
	progress  progress.Model
	total     int
	done      int
	doneError int
}

func (m *webFetchModel) Init() tea.Cmd {
	return nil
}

func (m *webFetchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case fileCheckedMsg:
		m.done++
		if msg.error {
			m.doneError++
		}

		percent := float64(m.done) / float64(m.total)
		cmd := m.progress.SetPercent(percent)
		return m, cmd
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *webFetchModel) View() string {
	percent := float64(m.done) / float64(m.total)
	return fmt.Sprintf(
		"%s %d/%d files checked\n✅  %d\n❌  %d",
		m.progress.ViewAs(percent),
		m.done,
		m.total,
		m.done-m.doneError,
		m.doneError,
	)
}

func checkFileHashes(files []string, baseURI string) map[string]plumbing.Hash {
	utils.PrintInfo("Checking files on webserver")
	c := colly.NewCollector(
		colly.Async(true),
		colly.UserAgent("FileChecker/1.0"),
	)

	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 5,
		Delay:       50 * time.Millisecond,
	})

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)

	fileHashes := make(map[string]plumbing.Hash)

	prgs := progress.New(
		progress.WithWidth(40),
		progress.WithoutPercentage(),
		progress.WithScaledGradient("#FF7CCB", "#FDFF8C"),
	)
	m := &webFetchModel{
		progress: prgs,
		total:    len(files),
	}

	p := tea.NewProgram(m)
	go func() {
		if _, err := p.Run(); err != nil {
			fmt.Println("Error running UI:", err)
		}
	}()

	c.OnResponse(func(r *colly.Response) {
		defer wg.Done()
		filename := r.Request.Ctx.Get("filename")
		if filename == "" {
			return
		}

		hasher := plumbing.NewHasher(plumbing.BlobObject, int64(len(r.Body)))
		hasher.Write(r.Body)

		mu.Lock()
		fileHashes[filename] = hasher.Sum()
		mu.Unlock()

		p.Send(fileCheckedMsg{
			error: false,
		})
	})

	c.OnError(func(_ *colly.Response, err error) {
		defer wg.Done()
		p.Send(fileCheckedMsg{
			error: true,
		})
	})

	for _, file := range files {
		fullURL, err := buildFullURL(baseURI, file)
		if err != nil {
			continue
		}

		wg.Add(1)
		ctx := colly.NewContext()
		ctx.Put("filename", file)
		c.Request("GET", fullURL, nil, ctx, nil)
	}

	wg.Wait()
	p.Quit()

	utils.PrintInfo("Files checked")

	return fileHashes
}

// buildFullURL constructs a valid URL from base and file path
func buildFullURL(base, file string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}

	// Clean and join paths
	u.Path = path.Join(u.Path, strings.TrimPrefix(file, "/"))
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}
