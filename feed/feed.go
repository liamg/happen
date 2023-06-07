package feed

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
)

type Source struct {
	Name       string        `yaml:"name"`
	Url        string        `yaml:"url"`
	Background string        `yaml:"bg"`
	Foreground string        `yaml:"fg"`
	MaxAge     time.Duration `yaml:"maxAge"`
}

type Item struct {
	ID          string
	Source      Source
	Title       string
	Description string
	Url         string
	ImageUrl    string
	Published   time.Time
}

type mgr struct {
	config *Config
}

func New(c *Config) *mgr {
	return &mgr{
		config: c,
	}
}

func (m *mgr) Read() ([]Item, error) {
	var all []Item
	var wg sync.WaitGroup
	var mu sync.Mutex
	errChan := make(chan error, len(m.config.Sources))
	for _, source := range m.config.Sources {
		wg.Add(1)
		go func(s Source) {
			defer wg.Done()
			items, err := s.GetItems()
			if err != nil {
				errChan <- err
				return
			}
			mu.Lock()
			defer mu.Unlock()
			all = append(all, items...)
		}(source)
	}
	wg.Wait()
	close(errChan)
	for err := range errChan {
		return nil, err
	}
	return m.sort(all), nil
}

func (m *mgr) sort(items []Item) []Item {
	sort.Slice(items, func(i, j int) bool {
		a := items[i]
		b := items[j]
		return a.Published.After(b.Published)
	})
	return items
}

func (s *Source) GetItems() ([]Item, error) {
	feed, err := gofeed.NewParser().ParseURL(s.Url)
	if err != nil {
		return nil, err
	}
	if s.Name == "" {
		s.Name = feed.Title
	}
	var items []Item
	for _, item := range feed.Items {
		if item.Link == "" {
			continue
		}
		date := parseTime(item.Published)
		if s.MaxAge > 0 && time.Since(date) > s.MaxAge {
			continue
		}
		hasher := sha256.New()
		hasher.Write([]byte(item.Link))
		hash := hex.EncodeToString(hasher.Sum(nil))
		local := Item{
			ID:          hash,
			Source:      *s,
			Title:       item.Title,
			Description: getDescription(item),
			Url:         item.Link,
			Published:   date,
		}
		if item.Image != nil && item.Image.URL != "" {
			local.ImageUrl = item.Image.URL
		}
		items = append(items, local)
	}
	return items, nil
}

var formats = []string{
	time.RFC1123,
	time.RFC1123Z,
	time.RFC3339,
	time.RFC3339Nano,
	time.RFC822,
	time.RFC822Z,
	time.RFC850,
	time.RubyDate,
	time.UnixDate,
	time.ANSIC,
}

func parseTime(s string) time.Time {
	for _, format := range formats {
		t, err := time.Parse(format, s)
		if err == nil {
			return t.Local()
		}
	}
	return time.Time{}
}

func getDescription(item *gofeed.Item) string {
	for _, value := range []string{
		item.Description,
		item.Content,
	} {
		if value == "" {
			continue
		}
		if extracted := harvest(value); extracted != "" {
			return extracted
		}
	}
	return item.Link
}

var htmlStripper = regexp.MustCompile(`<.*?>`)

// This method uses a regular expresion to remove HTML tags.
func stripHtmlRegex(s string) string {
	return htmlStripper.ReplaceAllString(s, "")
}

func harvest(s string) string {
	s = html.UnescapeString(s)
	s = stripHtmlRegex(s)
	s = strings.TrimSpace(s)
	s = strings.TrimSpace(strings.Split(s, "\n")[0])
	s = strings.TrimSpace(strings.Split(s, "\r")[0])
	s = strings.TrimSpace(strings.Split(s, "\t")[0])
	for _, r := range s {
		if r > 255 {
			return ""
		}
	}
	s = strings.ReplaceAll(s, "[link]", " ")
	s = strings.ReplaceAll(s, "[comments]", " ")
	for {
		after := strings.ReplaceAll(s, "  ", " ")
		if after == s {
			break
		}
		s = after
	}
	s = strings.TrimSpace(s)
	if len(s) < 40 {
		return ""
	}
	return s
}
