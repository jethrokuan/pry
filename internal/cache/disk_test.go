package cache_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/cache"
)

var _ = Describe("Disk", func() {
	var (
		dir string
		c   *cache.Disk
	)

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		c = cache.NewDisk(dir)
	})

	It("returns false for missing keys", func() {
		var s string
		Expect(c.Get("nonexistent", &s)).To(BeFalse())
	})

	It("round-trips a value", func() {
		c.Set("greeting", "hello", time.Minute)

		var got string
		Expect(c.Get("greeting", &got)).To(BeTrue())
		Expect(got).To(Equal("hello"))
	})

	It("round-trips structs", func() {
		type item struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}

		c.Set("item", item{Name: "widget", Count: 3}, time.Minute)

		var got item
		Expect(c.Get("item", &got)).To(BeTrue())
		Expect(got.Name).To(Equal("widget"))
		Expect(got.Count).To(Equal(3))
	})

	It("returns false for expired entries", func() {
		c.Set("ephemeral", "gone", time.Millisecond)
		time.Sleep(5 * time.Millisecond)

		var s string
		Expect(c.Get("ephemeral", &s)).To(BeFalse())
	})

	It("deletes a single key", func() {
		c.Set("a", 1, time.Minute)
		c.Set("b", 2, time.Minute)

		c.Delete("a")

		var v int
		Expect(c.Get("a", &v)).To(BeFalse())
		Expect(c.Get("b", &v)).To(BeTrue())
		Expect(v).To(Equal(2))
	})

	It("deletes by prefix", func() {
		c.Set("pr__1", "one", time.Minute)
		c.Set("pr__2", "two", time.Minute)
		c.Set("user", "alice", time.Minute)

		c.DeleteByPrefix("pr__")

		var s string
		Expect(c.Get("pr__1", &s)).To(BeFalse())
		Expect(c.Get("pr__2", &s)).To(BeFalse())
		Expect(c.Get("user", &s)).To(BeTrue())
		Expect(s).To(Equal("alice"))
	})

	It("persists across instances", func() {
		c.Set("persistent", "data", time.Minute)

		c2 := cache.NewDisk(dir)
		var got string
		Expect(c2.Get("persistent", &got)).To(BeTrue())
		Expect(got).To(Equal("data"))
	})

	It("creates the directory lazily", func() {
		subdir := filepath.Join(dir, "sub", "nested")
		c2 := cache.NewDisk(subdir)

		c2.Set("key", "val", time.Minute)

		var got string
		Expect(c2.Get("key", &got)).To(BeTrue())
		Expect(got).To(Equal("val"))
	})

	It("handles corrupt files gracefully", func() {
		os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{corrupt"), 0o644)

		var s string
		Expect(c.Get("bad", &s)).To(BeFalse())
	})

	Describe("Noop", func() {
		It("never returns cached values", func() {
			n := cache.Noop{}
			n.Set("key", "val", time.Minute)

			var s string
			Expect(n.Get("key", &s)).To(BeFalse())
		})
	})
})
