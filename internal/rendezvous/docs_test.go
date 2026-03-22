package rendezvous

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"testing"
)

func TestDocSiteLoadsAllPages(t *testing.T) {
	site := newDocSite()

	if len(site.Pages) == 0 {
		t.Fatal("no pages loaded")
	}

	entries, err := docsFS.ReadDir("docs")
	if err != nil {
		t.Fatal(err)
	}
	var mdCount int
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			mdCount++
		}
	}
	if len(site.Pages) != mdCount {
		t.Errorf("loaded %d pages but found %d .md files", len(site.Pages), mdCount)
	}
}

func TestDocSiteSlugsAreUnique(t *testing.T) {
	site := newDocSite()
	seen := map[string]bool{}
	for _, p := range site.Pages {
		if seen[p.Slug] {
			t.Errorf("duplicate slug: %s", p.Slug)
		}
		seen[p.Slug] = true
	}
}

func TestDocSiteBySlugMatchesPages(t *testing.T) {
	site := newDocSite()
	for _, p := range site.Pages {
		found, ok := site.BySlug[p.Slug]
		if !ok {
			t.Errorf("slug %q not in BySlug map", p.Slug)
			continue
		}
		if found.Title != p.Title {
			t.Errorf("BySlug[%q].Title = %q, want %q", p.Slug, found.Title, p.Title)
		}
	}
}

func TestDocPagesHaveTitles(t *testing.T) {
	site := newDocSite()
	for _, p := range site.Pages {
		if p.Title == "" {
			t.Errorf("page %q has empty title", p.Slug)
		}
		if p.Title == p.Slug {
			t.Errorf("page %q title equals slug (no # heading found?)", p.Slug)
		}
	}
}

func TestDocPagesRenderHTML(t *testing.T) {
	site := newDocSite()
	for _, p := range site.Pages {
		html := string(p.HTML)
		if html == "" {
			t.Errorf("page %q rendered to empty HTML", p.Slug)
			continue
		}
		if !strings.Contains(html, "<") {
			t.Errorf("page %q HTML contains no tags", p.Slug)
		}
	}
}

func TestDocPagesOrder(t *testing.T) {
	site := newDocSite()
	for i := 1; i < len(site.Pages); i++ {
		if site.Pages[i].Order < site.Pages[i-1].Order {
			t.Errorf("pages out of order: %q (order %d) before %q (order %d)",
				site.Pages[i-1].Slug, site.Pages[i-1].Order,
				site.Pages[i].Slug, site.Pages[i].Order)
		}
	}
}

func TestDocMermaidBlocksAreValid(t *testing.T) {
	entries, err := docsFS.ReadDir("docs")
	if err != nil {
		t.Fatal(err)
	}

	mermaidFence := regexp.MustCompile("(?s)```mermaid\n(.*?)```")
	validTypes := []string{
		"graph ", "graph\n",
		"flowchart ", "flowchart\n",
		"sequenceDiagram", "classDiagram", "stateDiagram",
		"erDiagram", "gantt", "pie", "gitgraph",
		"mindmap", "timeline", "sankey", "quadrantChart",
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := docsFS.ReadFile(path.Join("docs", e.Name()))
		if err != nil {
			t.Errorf("%s: %v", e.Name(), err)
			continue
		}

		matches := mermaidFence.FindAllSubmatch(data, -1)
		for i, m := range matches {
			body := strings.TrimSpace(string(m[1]))
			if body == "" {
				t.Errorf("%s: mermaid block %d is empty", e.Name(), i+1)
				continue
			}
			valid := false
			for _, vt := range validTypes {
				if strings.HasPrefix(body, vt) {
					valid = true
					break
				}
			}
			if !valid {
				first := body
				if idx := strings.Index(first, "\n"); idx > 0 {
					first = first[:idx]
				}
				t.Errorf("%s: mermaid block %d has unrecognized diagram type: %q", e.Name(), i+1, first)
			}
		}
	}
}

func TestDocMermaidBlocksRenderAsCodeBlocks(t *testing.T) {
	site := newDocSite()
	for _, p := range site.Pages {
		html := string(p.HTML)
		if !strings.Contains(html, "language-mermaid") {
			continue
		}
		if !strings.Contains(html, `<code class="language-mermaid">`) {
			t.Errorf("page %q has mermaid content but goldmark did not produce language-mermaid code block", p.Slug)
		}
	}
}

func TestDocInternalLinksResolve(t *testing.T) {
	site := newDocSite()

	linkRe := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

	entries, err := docsFS.ReadDir("docs")
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := docsFS.ReadFile(path.Join("docs", e.Name()))
		if err != nil {
			continue
		}

		matches := linkRe.FindAllStringSubmatch(string(data), -1)
		for _, m := range matches {
			target := m[2]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "#") ||
				strings.HasPrefix(target, "/") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			slug := strings.SplitN(target, "#", 2)[0]
			if slug == "" {
				continue
			}
			if _, ok := site.BySlug[slug]; !ok {
				t.Errorf("%s: internal link [%s](%s) resolves to unknown slug %q", e.Name(), m[1], target, slug)
			}
		}
	}
}

func TestDocMarkdownHasNoUnclosedFences(t *testing.T) {
	entries, err := docsFS.ReadDir("docs")
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := docsFS.ReadFile(path.Join("docs", e.Name()))
		if err != nil {
			continue
		}

		lines := strings.Split(string(data), "\n")
		inFence := false
		fenceLine := 0
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") {
				if inFence {
					inFence = false
				} else {
					inFence = true
					fenceLine = i + 1
				}
			}
		}
		if inFence {
			t.Errorf("%s: unclosed code fence starting at line %d", e.Name(), fenceLine)
		}
	}
}

func TestDocExpectedSlugs(t *testing.T) {
	site := newDocSite()

	expected := []string{
		"overview", "quickstart", "connecting", "configuration",
		"templates", "scripting", "groups", "advanced", "faq",
		"api", "executor", "sdk",
	}

	for _, slug := range expected {
		if _, ok := site.BySlug[slug]; !ok {
			t.Errorf("expected slug %q not found", slug)
		}
	}
}

func TestDocNoRawHTMLInMermaidBlocks(t *testing.T) {
	entries, err := docsFS.ReadDir("docs")
	if err != nil {
		t.Fatal(err)
	}

	mermaidFence := regexp.MustCompile("(?s)```mermaid\n(.*?)```")
	htmlTag := regexp.MustCompile("<[a-zA-Z/]")

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := docsFS.ReadFile(path.Join("docs", e.Name()))
		if err != nil {
			continue
		}

		matches := mermaidFence.FindAllSubmatch(data, -1)
		for i, m := range matches {
			if htmlTag.Match(m[1]) {
				t.Errorf("%s: mermaid block %d contains raw HTML tags", e.Name(), i+1)
			}
		}
	}
}

func TestDocMermaidBlockCount(t *testing.T) {
	entries, err := docsFS.ReadDir("docs")
	if err != nil {
		t.Fatal(err)
	}

	mermaidFence := regexp.MustCompile("(?s)```mermaid\n(.*?)```")
	total := 0

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := docsFS.ReadFile(path.Join("docs", e.Name()))
		if err != nil {
			continue
		}
		matches := mermaidFence.FindAllSubmatch(data, -1)
		total += len(matches)
	}

	if total == 0 {
		t.Error("no mermaid blocks found in any documentation file")
	}

	t.Logf("found %d mermaid blocks across documentation", total)
}

func TestDocPagesHaveMinimumContent(t *testing.T) {
	site := newDocSite()
	for _, p := range site.Pages {
		html := string(p.HTML)
		if len(html) < 100 {
			t.Errorf("page %q has suspiciously short content (%d bytes)", p.Slug, len(html))
		}
	}
}

func TestDocHeadingHierarchy(t *testing.T) {
	entries, err := docsFS.ReadDir("docs")
	if err != nil {
		t.Fatal(err)
	}

	headingRe := regexp.MustCompile(`^(#{1,6})\s+(.+)`)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := docsFS.ReadFile(path.Join("docs", e.Name()))
		if err != nil {
			continue
		}

		lines := strings.Split(string(data), "\n")
		inFence := false
		prevLevel := 0
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") {
				inFence = !inFence
				continue
			}
			if inFence {
				continue
			}
			m := headingRe.FindStringSubmatch(trimmed)
			if m == nil {
				continue
			}
			level := len(m[1])
			if prevLevel > 0 && level > prevLevel+1 {
				t.Errorf("%s:%d: heading jumps from h%d to h%d (%q)", e.Name(), i+1, prevLevel, level, m[2])
			}
			prevLevel = level
		}
	}
}

func TestDocNoTODOsOrFixmes(t *testing.T) {
	entries, err := docsFS.ReadDir("docs")
	if err != nil {
		t.Fatal(err)
	}

	markers := regexp.MustCompile(`(?i)\b(TODO|FIXME|HACK|XXX)\b`)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := docsFS.ReadFile(path.Join("docs", e.Name()))
		if err != nil {
			continue
		}

		lines := strings.Split(string(data), "\n")
		inFence := false
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") {
				inFence = !inFence
				continue
			}
			if inFence {
				continue
			}
			if m := markers.FindString(line); m != "" {
				t.Errorf("%s:%d: found %q marker in documentation", e.Name(), i+1, m)
			}
		}
	}
}

func TestDocFirstPageIsOverview(t *testing.T) {
	site := newDocSite()
	if len(site.Pages) == 0 {
		t.Fatal("no pages")
	}
	if site.Pages[0].Slug != "overview" {
		t.Errorf("first page slug = %q, want %q", site.Pages[0].Slug, "overview")
	}
}

func TestDocPagesHaveNavigation(t *testing.T) {
	site := newDocSite()
	if len(site.Pages) < 2 {
		t.Skip("fewer than 2 pages")
	}
	for i, p := range site.Pages {
		hasPrev := i > 0
		hasNext := i < len(site.Pages)-1
		if hasPrev {
			prev := site.Pages[i-1]
			if _, ok := site.BySlug[prev.Slug]; !ok {
				t.Errorf("page %q prev page %q not in BySlug", p.Slug, prev.Slug)
			}
		}
		if hasNext {
			next := site.Pages[i+1]
			if _, ok := site.BySlug[next.Slug]; !ok {
				t.Errorf("page %q next page %q not in BySlug", p.Slug, next.Slug)
			}
		}
	}
}

func TestDocConfigPageHasAllSections(t *testing.T) {
	site := newDocSite()
	p, ok := site.BySlug["configuration"]
	if !ok {
		t.Fatal("configuration page not found")
	}
	html := string(p.HTML)

	sections := []string{"identity", "paths", "p2p", "presence", "profile", "viewer", "lua"}
	for _, s := range sections {
		heading := fmt.Sprintf("<h3>%s</h3>", s)
		if !strings.Contains(html, heading) {
			t.Errorf("configuration page missing section heading: %s", s)
		}
	}
}
