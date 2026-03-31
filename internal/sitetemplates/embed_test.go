package sitetemplates

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/petervdpas/goop2/internal/orm/schema"
)

func TestAllTemplatesHaveSchemas(t *testing.T) {
	templates, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) == 0 {
		t.Fatal("expected at least one template")
	}

	for _, meta := range templates {
		t.Run(meta.Name, func(t *testing.T) {
			files, err := SiteFiles(meta.Dir)
			if err != nil {
				t.Fatal(err)
			}

			schemaCount := 0
			for rel, data := range files {
				if !strings.HasPrefix(rel, "schemas/") || !strings.HasSuffix(rel, ".json") {
					continue
				}
				schemaCount++

				var tbl schema.Table
				if err := json.Unmarshal(data, &tbl); err != nil {
					t.Fatalf("invalid JSON in %s: %v", rel, err)
				}
				if err := tbl.Validate(); err != nil {
					t.Fatalf("validation failed for %s: %v", rel, err)
				}
				if tbl.Access == nil {
					t.Fatalf("%s: missing Access policy", rel)
				}
				if tbl.Access.Insert == "" {
					t.Fatalf("%s: Access.Insert is empty", rel)
				}
				if tbl.Access.Read == "" {
					t.Fatalf("%s: Access.Read is empty", rel)
				}
			}

			if schemaCount == 0 {
				t.Fatal("no schemas/*.json found — template has no ORM schemas")
			}
		})
	}
}

func TestNoTemplateHasSchemaSQL(t *testing.T) {
	templates, err := List()
	if err != nil {
		t.Fatal(err)
	}

	for _, meta := range templates {
		_, err := Schema(meta.Dir)
		if err == nil {
			t.Fatalf("template %q still has schema.sql — should be removed (ORM-only)", meta.Name)
		}
	}
}

func TestAllTemplateManifestsNoTablesPolicies(t *testing.T) {
	templates, err := List()
	if err != nil {
		t.Fatal(err)
	}

	for _, meta := range templates {
		if len(meta.Tables) > 0 {
			t.Fatalf("template %q manifest still has 'tables' field — policies should be in schemas/*.json", meta.Name)
		}
	}
}

func TestTemplateFilesIncludeSchemas(t *testing.T) {
	files, err := SiteFiles("blog")
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := files["schemas/posts.json"]; !ok {
		t.Fatal("blog SiteFiles should include schemas/posts.json")
	}
	if _, ok := files["schemas/blog_config.json"]; !ok {
		t.Fatal("blog SiteFiles should include schemas/blog_config.json")
	}
}

func TestSiteFilesExcludesManifest(t *testing.T) {
	files, err := SiteFiles("blog")
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := files["manifest.json"]; ok {
		t.Fatal("SiteFiles should NOT include manifest.json")
	}
}
