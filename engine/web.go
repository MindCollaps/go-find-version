package engine

import (
	"fmt"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gocolly/colly"
	"net/url"
	"path"
	"strings"
	"sync"
)

type fileCheckedMsg struct{}

type webModel struct {
	progress progress.Model
	total    int
	done     int
}

func (m *webModel) Init() tea.Cmd {
	return nil
}

func (m *webModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case fileCheckedMsg:
		m.done++
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

func (m *webModel) View() string {
	percent := float64(m.done) / float64(m.total)
	return fmt.Sprintf(
		"Checking files...\n%s\n%d/%d completed",
		m.progress.ViewAs(percent),
		m.done,
		m.total,
	)
}

func CheckFileExists(files []string, baseURI string) []string {
	c := colly.NewCollector(
		colly.Async(true),
		colly.UserAgent("FileChecker/1.0"),
	)

	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 5,
	})

	var (
		foundFiles []string
		mu         sync.Mutex
		wg         sync.WaitGroup
	)

	prgs := progress.New(
		progress.WithWidth(40),
		progress.WithoutPercentage(),
		progress.WithScaledGradient("#FF7CCB", "#FDFF8C"),
	)
	m := &webModel{
		progress: prgs,
		total:    len(files),
	}

	p := tea.NewProgram(m)

	// Start Bubble Tea in separate goroutine
	go func() {
		if _, err := p.Run(); err != nil {
			fmt.Println("Error running UI:", err)
		}
	}()

	c.OnResponse(func(r *colly.Response) {
		defer wg.Done()
		if filename := r.Request.Ctx.Get("filename"); filename != "" {
			mu.Lock()
			foundFiles = append(foundFiles, filename)
			mu.Unlock()
		}
		p.Send(fileCheckedMsg{})
	})

	c.OnError(func(_ *colly.Response, err error) {
		defer wg.Done()
		p.Send(fileCheckedMsg{})
	})

	for _, file := range files {
		fullURL, err := buildFullURL(baseURI, file)
		if err != nil {
			continue
		}

		wg.Add(1)
		ctx := colly.NewContext()
		ctx.Put("filename", file)
		c.Request("HEAD", fullURL, nil, ctx, nil)
	}

	wg.Wait()
	p.Quit()

	return foundFiles
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
