package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/dan/moe/web"
)

// renderer holds pre-compiled page templates. Each page template is the layout
// combined with that page's specific template, so "title" and "content" blocks
// are resolved per-page without collision.
type renderer struct {
	pages map[string]*template.Template
}

// newRenderer parses the layout template once, then clones it for each page
// template, producing a separate compiled template per page.
func newRenderer() (*renderer, error) {
	// Template functions available in all templates.
	funcMap := template.FuncMap{
		"pages": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i + 1
			}
			return s
		},
		"osOptions": func() []string {
			return []string{"iOS", "Android", "Windows", "macOS"}
		},
		"mapIntVal": func(m map[string]int, key string) int {
			if m == nil {
				return 0
			}
			return m[key]
		},
		"providerStatus": func(m map[string]*ProviderStatus, key string) string {
			if m == nil {
				return "unchecked"
			}
			s, ok := m[key]
			if !ok || s == nil {
				return "unchecked"
			}
			return s.Status
		},
		"isJSON": func(s string) bool {
			s = strings.TrimSpace(s)
			return (strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) ||
				(strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]"))
		},
		"prettyJSON": func(s string) template.HTML {
			s = strings.TrimSpace(s)
			var v any
			if err := json.Unmarshal([]byte(s), &v); err != nil {
				return template.HTML(template.HTMLEscapeString(s))
			}
			b, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return template.HTML(template.HTMLEscapeString(s))
			}
			return template.HTML("<pre class=\"json-block\">" + template.HTMLEscapeString(string(b)) + "</pre>")
		},
		"toJSON": func(v any) template.JS {
			b, err := json.Marshal(v)
			if err != nil {
				return template.JS("null")
			}
			return template.JS(b)
		},
		"timeAgo": func(v any) string {
			var t time.Time
			switch tv := v.(type) {
			case time.Time:
				t = tv
			default:
				return "never"
			}
			return timeAgoString(t)
		},
	}

	// Parse the layout first.
	layout, err := template.New("layout.html").Funcs(funcMap).ParseFS(web.TemplateFS, "templates/layout.html")
	if err != nil {
		return nil, fmt.Errorf("parse layout: %w", err)
	}

	// Discover page templates (everything except layout.html).
	entries, err := fs.ReadDir(web.TemplateFS, "templates")
	if err != nil {
		return nil, fmt.Errorf("read template dir: %w", err)
	}

	pages := make(map[string]*template.Template)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == "layout.html" {
			continue
		}

		// Clone layout and overlay the page template on top.
		clone, err := layout.Clone()
		if err != nil {
			return nil, fmt.Errorf("clone layout for %s: %w", name, err)
		}
		if _, err := clone.ParseFS(web.TemplateFS, "templates/"+name); err != nil {
			return nil, fmt.Errorf("parse page %s: %w", name, err)
		}
		pages[name] = clone
	}

	return &renderer{pages: pages}, nil
}

// render executes the named page template with the given data. The page
// parameter is the template filename (e.g. "dashboard.html").
func (rn *renderer) render(w http.ResponseWriter, page string, data any) {
	tmpl, ok := rn.pages[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// renderBlock executes a specific named block from a page template, without
// the surrounding layout. Used for htmx partial/fragment responses.
func (rn *renderer) renderBlock(w http.ResponseWriter, page, block string, data any) {
	tmpl, ok := rn.pages[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, block, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// timeAgoString formats a time.Time as a human-readable "X ago" string.
func timeAgoString(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
