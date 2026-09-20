// Package main — dedup: content-hash duplicate file finder.
//
// Pipeline: walk → size prefilter → fast xxh64 head-hash → full SHA-256
// verification. Only identical SHA-256 digests count as duplicates, so the
// fast first pass never decides anything on its own.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/efealtiparmakoglu/dedup/xxh64"
)

const headChunk = 4 << 10 // 4 KiB

// File is one regular file discovered under a root.
type File struct {
	Path string
	Size int64
}

// Group is a set of identical files; the first entry is the suggested
// keeper, the rest are duplicates.
type Group struct {
	Hash       string   `json:"hash"`
	Size       int64    `json:"size"`
	Keep       string   `json:"keep"`
	Duplicates []string `json:"duplicates"`
}

// Report summarizes a run.
type Report struct {
	ScannedFiles   int     `json:"scanned_files"`
	ScannedBytes   int64   `json:"scanned_bytes"`
	Groups         int     `json:"duplicate_groups"`
	DuplicateFiles int     `json:"duplicate_files"`
	WastedBytes    int64   `json:"wasted_bytes"`
	DeletedFiles   int     `json:"deleted_files,omitempty"`
	DeletedBytes   int64   `json:"deleted_bytes,omitempty"`
	GroupsDetail   []Group `json:"groups"`
}

type options struct {
	minSize int64
	workers int
}

// collect walks every root and returns regular files >= minSize.
func collect(roots []string, minSize int64) ([]File, int64, error) {
	var files []File
	var total int64
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !d.Type().IsRegular() {
				return nil // symlinks etc. are never duplicates of real files
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Size() < minSize {
				return nil
			}
			files = append(files, File{Path: path, Size: info.Size()})
			total += info.Size()
			return nil
		})
		if err != nil {
			return nil, 0, err
		}
	}
	return files, total, nil
}

// findDuplicates groups files by size, then xxh64 head chunk, then full
// SHA-256 — returning groups sorted deterministically.
func findDuplicates(files []File, opts options) ([]Group, error) {
	// 1. size prefilter
	bySize := map[int64][]File{}
	sizes := []int64{}
	for _, f := range files {
		if len(bySize[f.Size]) == 0 {
			sizes = append(sizes, f.Size)
		}
		bySize[f.Size] = append(bySize[f.Size], f)
	}
	sort.Slice(sizes, func(i, j int) bool { return sizes[i] < sizes[j] })

	// 2. fast head-hash pass (parallel)
	var mu sync.Mutex
	byHead := map[string][]File{} // size + head hash + path
	type headKey struct {
		size int64
		head uint64
	}
	for _, size := range sizes {
		group := bySize[size]
		if len(group) < 2 {
			continue
		}
		workers := opts.workers
		if workers < 1 {
			workers = 4
		}
		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup
		for _, f := range group {
			wg.Add(1)
			sem <- struct{}{}
			go func(f File) {
				defer func() { <-sem; wg.Done() }()
				h, err := headHash(f.Path)
				if err != nil {
					return // unreadable files are reported later by sha pass
				}
				mu.Lock()
				key := fmt.Sprintf("%d:%016x", f.Size, h)
				byHead[key] = append(byHead[key], f)
				mu.Unlock()
			}(f)
		}
		wg.Wait()
	}

	// 3. full SHA-256 verification pass
	bySha := map[string][]File{}
	keys := []string{}
	for key, group := range byHead {
		if len(group) < 2 {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	workers := opts.workers
	if workers < 1 {
		workers = 4
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var shaMu sync.Mutex
	for _, key := range keys {
		for _, f := range byHead[key] {
			wg.Add(1)
			sem <- struct{}{}
			go func(f File) {
				defer func() { <-sem; wg.Done() }()
				sum, err := fullHash(f.Path)
				if err != nil {
					fmt.Fprintf(os.Stderr, "dedup: %s: %v\n", f.Path, err)
					return
				}
				shaMu.Lock()
				bySha[sum] = append(bySha[sum], f)
				shaMu.Unlock()
			}(f)
		}
	}
	wg.Wait()

	var groups []Group
	shaKeys := []string{}
	for k := range bySha {
		shaKeys = append(shaKeys, k)
	}
	sort.Strings(shaKeys)
	for _, sum := range shaKeys {
		group := bySha[sum]
		if len(group) < 2 {
			continue
		}
		sort.Slice(group, func(i, j int) bool { return group[i].Path < group[j].Path })
		dupes := make([]string, 0, len(group)-1)
		for _, f := range group[1:] {
			dupes = append(dupes, f.Path)
		}
		groups = append(groups, Group{
			Hash: sum, Size: group[0].Size, Keep: group[0].Path, Duplicates: dupes,
		})
	}
	return groups, nil
}

func headHash(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	buf := make([]byte, headChunk)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return 0, err
	}
	_ = n
	return xxh64.Sum(buf, 0), nil
}

func fullHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// removeDuplicates deletes the duplicate paths, keeping one file per group.
// Returns the number of files and bytes removed.
func removeDuplicates(groups []Group) (int, int64, error) {
	removed, bytes := 0, int64(0)
	for _, g := range groups {
		for _, path := range g.Duplicates {
			if err := os.Remove(path); err != nil {
				return removed, bytes, err
			}
			removed++
			bytes += g.Size
		}
	}
	return removed, bytes, nil
}
