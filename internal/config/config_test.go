package config

import (
	"os"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Config", func() {

	ginkgo.Describe("load with no file", func() {
		ginkgo.It("returns defaults when path is nil", func() {
			cfg := load(nil)
			gomega.Expect(cfg.PageSize).To(gomega.Equal(50))
			gomega.Expect(cfg.PRList.SidebarWidth).To(gomega.Equal(50))
			gomega.Expect(cfg.Cache.Enabled).To(gomega.BeTrue())
		})

		ginkgo.It("returns defaults when file does not exist", func() {
			path := "/nonexistent/config.toml"
			cfg := load(&path)
			gomega.Expect(cfg.PageSize).To(gomega.Equal(50))
		})
	})

	ginkgo.Describe("load from TOML file", func() {
		var tmpDir string

		ginkgo.BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "pry-config-test")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.AfterEach(func() {
			os.RemoveAll(tmpDir)
		})

		ginkgo.It("overrides defaults with file values", func() {
			path := filepath.Join(tmpDir, "config.toml")
			content := `
editor = "nvim"
page_size = 25
`
			gomega.Expect(os.WriteFile(path, []byte(content), 0644)).To(gomega.Succeed())

			cfg := load(&path)
			gomega.Expect(cfg.Editor).To(gomega.Equal("nvim"))
			gomega.Expect(cfg.PageSize).To(gomega.Equal(25))
		})

		ginkgo.It("loads filter configuration", func() {
			path := filepath.Join(tmpDir, "config.toml")
			content := `
[[filters]]
name = "My PRs"
qualifier = "author:@me"

[[filters]]
name = "All"
qualifier = ""
`
			gomega.Expect(os.WriteFile(path, []byte(content), 0644)).To(gomega.Succeed())

			cfg := load(&path)
			gomega.Expect(cfg.Filters).To(gomega.HaveLen(2))
			gomega.Expect(cfg.Filters[0].Name).To(gomega.Equal("My PRs"))
			gomega.Expect(cfg.Filters[0].Qualifier).To(gomega.Equal("author:@me"))
			gomega.Expect(cfg.Filters[1].Name).To(gomega.Equal("All"))
			gomega.Expect(cfg.Filters[1].Qualifier).To(gomega.BeEmpty())
		})

		ginkgo.It("loads column configuration", func() {
			path := filepath.Join(tmpDir, "config.toml")
			content := `columns = ["number", "title", "author"]`
			gomega.Expect(os.WriteFile(path, []byte(content), 0644)).To(gomega.Succeed())

			cfg := load(&path)
			gomega.Expect(cfg.Columns).To(gomega.Equal([]string{"number", "title", "author"}))
		})

		ginkgo.It("loads cache configuration", func() {
			path := filepath.Join(tmpDir, "config.toml")
			content := `
[cache]
enabled = false
ttl = "10m"
`
			gomega.Expect(os.WriteFile(path, []byte(content), 0644)).To(gomega.Succeed())

			cfg := load(&path)
			gomega.Expect(cfg.Cache.Enabled).To(gomega.BeFalse())
			gomega.Expect(cfg.Cache.TTL).To(gomega.Equal("10m"))
		})

		ginkgo.It("loads pr_list configuration", func() {
			path := filepath.Join(tmpDir, "config.toml")
			content := `
[pr_list]
sidebar_width = 80
sidebar_visible = true
`
			gomega.Expect(os.WriteFile(path, []byte(content), 0644)).To(gomega.Succeed())

			cfg := load(&path)
			gomega.Expect(cfg.PRList.SidebarWidth).To(gomega.Equal(80))
			gomega.Expect(cfg.PRList.SidebarVisible).To(gomega.BeTrue())
		})

		ginkgo.It("loads file_tree configuration", func() {
			path := filepath.Join(tmpDir, "config.toml")
			content := `
[file_tree]
owner_filter = false
`
			gomega.Expect(os.WriteFile(path, []byte(content), 0644)).To(gomega.Succeed())

			cfg := load(&path)
			gomega.Expect(cfg.FileTree.OwnerFilter).ToNot(gomega.BeNil())
			gomega.Expect(*cfg.FileTree.OwnerFilter).To(gomega.BeFalse())
		})

		ginkgo.It("merges file values with defaults", func() {
			path := filepath.Join(tmpDir, "config.toml")
			content := `editor = "vim"
page_size = 30
`
			gomega.Expect(os.WriteFile(path, []byte(content), 0644)).To(gomega.Succeed())

			cfg := load(&path)
			gomega.Expect(cfg.Editor).To(gomega.Equal("vim"))
			gomega.Expect(cfg.PageSize).To(gomega.Equal(30))
			// Columns and filters fall back to defaults via accessor methods
			gomega.Expect(cfg.PRColumns()).To(gomega.Equal(DefaultColumns()))
			gomega.Expect(cfg.PRFilters()).To(gomega.HaveLen(4))
		})
	})

	ginkgo.Describe("CacheTTLDuration", func() {
		ginkgo.It("returns 5m default when TTL is empty", func() {
			cfg := Config{}
			gomega.Expect(cfg.CacheTTLDuration().Minutes()).To(gomega.Equal(5.0))
		})

		ginkgo.It("parses valid duration string", func() {
			cfg := Config{Cache: CacheConfig{TTL: "10m"}}
			gomega.Expect(cfg.CacheTTLDuration().Minutes()).To(gomega.Equal(10.0))
		})

		ginkgo.It("parses hour duration", func() {
			cfg := Config{Cache: CacheConfig{TTL: "1h"}}
			gomega.Expect(cfg.CacheTTLDuration().Hours()).To(gomega.Equal(1.0))
		})

		ginkgo.It("returns 5m default for invalid duration", func() {
			cfg := Config{Cache: CacheConfig{TTL: "invalid"}}
			gomega.Expect(cfg.CacheTTLDuration().Minutes()).To(gomega.Equal(5.0))
		})
	})

	ginkgo.Describe("PRColumns", func() {
		ginkgo.It("returns configured columns when set", func() {
			cfg := Config{Columns: []string{"number", "title"}}
			gomega.Expect(cfg.PRColumns()).To(gomega.Equal([]string{"number", "title"}))
		})

		ginkgo.It("returns defaults when columns are empty", func() {
			cfg := Config{}
			gomega.Expect(cfg.PRColumns()).To(gomega.Equal(DefaultColumns()))
		})
	})

	ginkgo.Describe("PRFilters", func() {
		ginkgo.It("converts config filters to domain types", func() {
			cfg := Config{
				Filters: []FilterConfig{
					{Name: "My PRs", Qualifier: "author:@me"},
				},
			}
			filters := cfg.PRFilters()
			gomega.Expect(filters).To(gomega.HaveLen(1))
			gomega.Expect(filters[0].Name).To(gomega.Equal("My PRs"))
			gomega.Expect(filters[0].Qualifier).To(gomega.Equal("author:@me"))
		})

		ginkgo.It("falls back to default filters when empty", func() {
			cfg := Config{}
			filters := cfg.PRFilters()
			gomega.Expect(filters).To(gomega.HaveLen(4))
			gomega.Expect(filters[0].Name).To(gomega.Equal("My PRs"))
		})
	})

	ginkgo.Describe("CacheEnabled", func() {
		ginkgo.It("returns the cache enabled flag", func() {
			cfg := Config{Cache: CacheConfig{Enabled: true}}
			gomega.Expect(cfg.CacheEnabled()).To(gomega.BeTrue())

			cfg = Config{Cache: CacheConfig{Enabled: false}}
			gomega.Expect(cfg.CacheEnabled()).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("DefaultFilters", func() {
		ginkgo.It("returns 4 built-in filters", func() {
			filters := DefaultFilters()
			gomega.Expect(filters).To(gomega.HaveLen(4))
			gomega.Expect(filters[0].Name).To(gomega.Equal("My PRs"))
			gomega.Expect(filters[1].Name).To(gomega.Equal("Assigned to Me"))
			gomega.Expect(filters[2].Name).To(gomega.Equal("Needs My Review"))
			gomega.Expect(filters[3].Name).To(gomega.Equal("Involved"))
		})
	})

	ginkgo.Describe("LoadFrom", func() {
		ginkgo.It("loads config from a given path", func() {
			tmpDir, err := os.MkdirTemp("", "pry-config-test")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			defer os.RemoveAll(tmpDir)

			path := filepath.Join(tmpDir, "config.toml")
			content := `page_size = 100`
			gomega.Expect(os.WriteFile(path, []byte(content), 0644)).To(gomega.Succeed())

			cfg := LoadFrom(path)
			gomega.Expect(cfg.PageSize).To(gomega.Equal(100))
		})
	})
})
