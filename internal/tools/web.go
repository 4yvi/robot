package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

const userAgent = "Mozilla/5.0 (compatible; RobotAssistant/1.0)"

// SearchResult is one web search hit.
type SearchResult struct {
	Title, URL, Snippet string
}

func webSearchTool(env *Env) Tool {
	return &funcTool{
		name:     "web_search",
		desc:     "Поиск в интернете через DuckDuckGo. Возвращает заголовки, ссылки и описания. Используй для характеристик, цен, новостей и любых свежих фактов.",
		props:    map[string]any{"query": str("поисковый запрос")},
		required: []string{"query"},
		run: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct{ Query string }
			if err := decode(args, &a); err != nil {
				return "", err
			}
			if strings.TrimSpace(a.Query) == "" {
				return "", fmt.Errorf("query is empty")
			}
			form := url.Values{"q": {a.Query}, "kl": {"ru-ru"}}
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://html.duckduckgo.com/html/", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("User-Agent", userAgent)
			resp, err := env.client().Do(req)
			if err != nil {
				return "", err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return "", fmt.Errorf("search failed: %s", resp.Status)
			}
			res, err := ParseDuckDuckGo(io.LimitReader(resp.Body, 2<<20))
			if err != nil {
				return "", err
			}
			if len(res) == 0 {
				return "Ничего не найдено.", nil
			}
			var b strings.Builder
			for i, r := range res {
				fmt.Fprintf(&b, "%d. %s\n%s\n%s\n", i+1, r.Title, r.URL, r.Snippet)
			}
			return b.String(), nil
		},
	}
}

// ParseDuckDuckGo extracts results from DuckDuckGo's HTML result page.
func ParseDuckDuckGo(r io.Reader) ([]SearchResult, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	var out []SearchResult
	var cur *SearchResult
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			switch {
			case hasClass(n, "result__a"):
				if cur != nil {
					out = append(out, *cur)
				}
				cur = &SearchResult{Title: text(n), URL: cleanDDGURL(attr(n, "href"))}
			case hasClass(n, "result__snippet") && cur != nil:
				cur.Snippet = text(n)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if cur != nil {
		out = append(out, *cur)
	}
	if len(out) > 8 {
		out = out[:8]
	}
	return out, nil
}

// cleanDDGURL unwraps duckduckgo.com/l/?uddg=<url> redirects.
func cleanDDGURL(h string) string {
	if strings.HasPrefix(h, "//") {
		h = "https:" + h
	}
	u, err := url.Parse(h)
	if err != nil {
		return h
	}
	if t := u.Query().Get("uddg"); t != "" {
		return t
	}
	return h
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

func text(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

const maxPageRunes = 6000

func fetchPageTool(env *Env) Tool {
	return &funcTool{
		name:     "fetch_page",
		desc:     "Загружает веб-страницу и возвращает её текст (обрезается до разумного размера).",
		props:    map[string]any{"url": str("адрес страницы, http или https")},
		required: []string{"url"},
		run: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct{ URL string }
			if err := decode(args, &a); err != nil {
				return "", err
			}
			u, err := url.Parse(a.URL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return "", fmt.Errorf("only http and https URLs are allowed")
			}
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			req.Header.Set("User-Agent", userAgent)
			resp, err := env.client().Do(req)
			if err != nil {
				return "", err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return "", fmt.Errorf("fetch failed: %s", resp.Status)
			}
			t, err := ExtractText(io.LimitReader(resp.Body, 3<<20))
			if err != nil {
				return "", err
			}
			return truncate(t, maxPageRunes), nil
		},
	}
}

// ExtractText returns the readable text of an HTML page.
func ExtractText(r io.Reader) (string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", err
	}
	skip := map[string]bool{"script": true, "style": true, "noscript": true, "nav": true, "footer": true, "header": true, "svg": true, "form": true}
	block := map[string]bool{"p": true, "div": true, "br": true, "li": true, "h1": true, "h2": true, "h3": true, "h4": true, "tr": true, "section": true, "article": true}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && skip[n.Data] {
			return
		}
		if n.Type == html.TextNode {
			if t := strings.TrimSpace(n.Data); t != "" {
				b.WriteString(t)
				b.WriteByte(' ')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && block[n.Data] {
			b.WriteByte('\n')
		}
	}
	walk(doc)
	lines := strings.Split(b.String(), "\n")
	var out []string
	for _, l := range lines {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n"), nil
}
