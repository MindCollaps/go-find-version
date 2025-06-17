package engine

import (
	"github.com/gocolly/colly"
	"io"
	"net/http"
)

func fetchURL(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func crawl() {
	c := colly.NewCollector()
	c.OnResponse(func(r *colly.Response) {

	})

}
