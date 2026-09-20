// main.go — dedup CLI: find (and optionally remove) duplicate files.
//
// Usage: dedup [options] <dir>...
//
//	-delete        remove duplicates (keeping one file per group)
//	-json          JSON report to stdout
//	-min BYTES     ignore files smaller than BYTES (default 1)
//	-workers N     hashing concurrency (default 8)
//	-q             quiet: only errors and the final summary
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const version = "1.0.0"

const usage = `dedup ` + version + ` - content-hash duplicate file finder

USAGE:
    dedup [options] <dir>...

OPTIONS:
    -delete          remove duplicates, keeping the first file of each group
    -json            machine-readable JSON report
    -min BYTES       skip files smaller than BYTES (default 1)
    -workers N       hashing concurrency (default 8)
    -q               suppress group listing, print summary only
    -h, -help        this help
    -V, -version     version

Duplicates are verified with full SHA-256 digests — the fast xxh64 head
pass never decides alone. With -delete the FIRST file of every group is
kept; everything else is removed. Without -delete nothing is touched.

EXAMPLES:
    dedup ~/photos
    dedup -json ~/photos > report.json
    dedup -delete -min 1M ~/downloads
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	var roots []string
	delete := false
	jsonOut := false
	quiet := false
	minSize := int64(1)
	workers := 8

	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "-h", "-help", "--help":
			fmt.Print(usage)
			return 0
		case "-V", "-version", "--version":
			fmt.Println("dedup", version)
			return 0
		case "-delete":
			delete = true
		case "-json":
			jsonOut = true
		case "-q", "-quiet":
			quiet = true
		case "-min":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "dedup: -min needs a number")
				return 2
			}
			v, err := strconv.ParseInt(args[i], 10, 64)
			if err != nil || v < 0 {
				fmt.Fprintln(os.Stderr, "dedup: bad -min value")
				return 2
			}
			minSize = v
		case "-workers":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "dedup: -workers needs a number")
				return 2
			}
			v, err := strconv.Atoi(args[i])
			if err != nil || v < 1 {
				fmt.Fprintln(os.Stderr, "dedup: bad -workers value")
				return 2
			}
			workers = v
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "dedup: unknown option %q (try -help)\n", a)
				return 2
			}
			roots = append(roots, a)
		}
		i++
	}

	if len(roots) == 0 {
		fmt.Fprintln(os.Stderr, "dedup: no directories given (try -help)")
		return 2
	}

	files, totalBytes, err := collect(roots, minSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dedup: %v\n", err)
		return 1
	}

	groups, err := findDuplicates(files, options{minSize: minSize, workers: workers})
	if err != nil {
		fmt.Fprintf(os.Stderr, "dedup: %v\n", err)
		return 1
	}

	wasted := int64(0)
	dupFiles := 0
	for _, g := range groups {
		wasted += g.Size * int64(len(g.Duplicates))
		dupFiles += len(g.Duplicates)
	}

	if !jsonOut && !quiet {
		for _, g := range groups {
			for _, d := range g.Duplicates {
				fmt.Printf("%s  [duplicate of %s]\n", d, g.Keep)
			}
		}
		fmt.Println()
	}

	if jsonOut {
		report := Report{
			ScannedFiles:   len(files),
			ScannedBytes:   totalBytes,
			Groups:         len(groups),
			DuplicateFiles: dupFiles,
			WastedBytes:    wasted,
			GroupsDetail:   groups,
		}
		if delete {
			removed, bytes := removeCount(groups)
			report.DeletedFiles = removed
			report.DeletedBytes = bytes
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "dedup: json: %v\n", err)
			return 1
		}
	} else {
		action := "found"
		if delete {
			action = "removed"
		}
		fmt.Printf("%d files scanned (%d bytes); %d duplicate groups, %d duplicate files, %d wasted bytes %s\n",
			len(files), totalBytes, len(groups), dupFiles, wasted, action)
	}

	if delete {
		removed, bytes, err := removeDuplicates(groups)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dedup: remove failed after %d files: %v\n", removed, err)
			return 1
		}
		if !jsonOut {
			fmt.Printf("deleted %d files (%d bytes)\n", removed, bytes)
		}
	}
	return 0
}

func removeCount(groups []Group) (int, int64) {
	n, b := 0, int64(0)
	for _, g := range groups {
		n += len(g.Duplicates)
		b += g.Size * int64(len(g.Duplicates))
	}
	return n, b
}
