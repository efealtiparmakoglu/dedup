package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func buildTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root+"/a/one.txt", "shared content 123")   // group 1 keep
	write(t, root+"/b/two.txt", "shared content 123")   // group 1 dupe
	write(t, root+"/c/three.txt", "shared content 123") // group 1 dupe
	write(t, root+"/unique1.bin", "unique data one")
	write(t, root+"/unique2.bin", "unique data two with different length")
	write(t, root+"/big/x.bin", "0000000000") // group 2 keep (10 bytes)
	write(t, root+"/big/y.bin", "0000000000") // group 2 dupe
	return root
}

func TestFindDuplicates(t *testing.T) {
	root := buildTree(t)
	files, total, err := collect([]string{root}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 7 {
		t.Fatalf("expected 7 files, got %d", len(files))
	}
	if total <= 0 {
		t.Fatal("total bytes must be positive")
	}

	groups, err := findDuplicates(files, options{minSize: 1, workers: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %+v", len(groups), groups)
	}

	byKeep := map[string]Group{}
	for _, g := range groups {
		byKeep[g.Keep] = g
		if len(g.Duplicates) < 1 {
			t.Fatalf("group %s has no duplicates", g.Keep)
		}
		// verify the hash really is the SHA-256 of the content
		raw, _ := os.ReadFile(g.Keep)
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != g.Hash {
			t.Fatalf("group hash is not sha256 of content")
		}
	}
	if _, ok := byKeep[root+"/a/one.txt"]; !ok {
		t.Fatalf("first sorted file should be kept: %+v", groups)
	}
	if _, ok := byKeep[root+"/big/x.bin"]; !ok {
		t.Fatalf("x.bin should be kept: %+v", groups)
	}
}

func TestMinSizeFilter(t *testing.T) {
	root := buildTree(t)
	files, _, err := collect([]string{root}, 11)
	if err != nil {
		t.Fatal(err)
	}
	// files >= 11 bytes survive: three 18-byte + 15-byte + 37-byte files
	if len(files) != 5 {
		t.Fatalf("min-size filter wrong: %+v", files)
	}
	for _, f := range files {
		if f.Size < 11 {
			t.Fatalf("file under min-size leaked: %+v", f)
		}
	}
}

func TestDeleteRemovesDuplicatesOnly(t *testing.T) {
	root := buildTree(t)
	files, _, err := collect([]string{root}, 1)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := findDuplicates(files, options{minSize: 1, workers: 4})
	if err != nil {
		t.Fatal(err)
	}
	removed, bytes, err := removeDuplicates(groups)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 3 {
		t.Fatalf("expected 3 removals, got %d", removed)
	}
	if bytes != int64(2*18+10) {
		t.Fatalf("unexpected removed bytes: %d", bytes)
	}
	for _, g := range groups {
		if _, err := os.Stat(g.Keep); err != nil {
			t.Fatalf("keeper %s was removed", g.Keep)
		}
		for _, d := range g.Duplicates {
			if _, err := os.Stat(d); !os.IsNotExist(err) {
				t.Fatalf("duplicate %s still exists", d)
			}
		}
	}
}

func TestEmptyFilesAreDuplicates(t *testing.T) {
	root := t.TempDir()
	write(t, root+"/e1", "")
	write(t, root+"/e2", "")
	files, _, _ := collect([]string{root}, 0)
	groups, err := findDuplicates(files, options{minSize: 0, workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Duplicates) != 1 {
		t.Fatalf("empty files should form one group: %+v", groups)
	}
	fmt.Fprintln(os.Stderr, "") // keep output tidy
}
