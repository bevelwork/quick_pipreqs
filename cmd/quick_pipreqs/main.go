package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bevelwork/quick_pipreqs/version"
)

func main() {
	var (
		dryRun      bool
		maxDepth    int
		concurrency int
		verbose     bool
	)
	flag.BoolVar(&dryRun, "dry-run", false, "print actions without executing")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.IntVar(&maxDepth, "max-depth", 2, "maximum recursion depth (0 = only root)")
	flag.IntVar(&concurrency, "concurrency", 12, "max concurrent updates (1-12)")
	flag.BoolVar(&verbose, "verbose", false, "print verbose output")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <path>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 1 {
		if *showVersion {
			fmt.Println(version.Full)
			return
		}
		flag.Usage()
		os.Exit(2)
	}
	if *showVersion {
		fmt.Println(version.Full)
		return
	}

	// log discovered directories
	logger := log.New(os.Stdout, "", log.LstdFlags)

	// Validation
	pipreqsVersion, err := runCmd("pipreqs", []string{"--version"}, ".")
	if err != nil {
		logger.Fatalf("error: pipreqs not found in PATH: %v", err)
		return
	}
	logger.Printf("pipreqs version: %s", pipreqsVersion)

	root := flag.Arg(0)

	reqDirs, err := findRequirementsDirs(root, maxDepth)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if len(reqDirs) == 0 {
		fmt.Println("no requirements.txt found; running pipreqs in root:", root)
		reqDirs = []string{root}
	}

	// deterministic processing order
	sort.Strings(reqDirs)

	logger.Printf("discovered %d directories to process", len(reqDirs))
	if verbose {
		for _, d := range reqDirs {
			logger.Println(" -", d)
		}
	}

	if concurrency < 1 {
		fmt.Fprintln(os.Stderr, "invalid --concurrency:", concurrency, "(must be >= 1)")
		os.Exit(2)
	}
	if concurrency > 12 {
		concurrency = 12
	}

	// early check for pipreqs availability (skip in dry-run)
	if !dryRun {
		if _, err := exec.LookPath("pipreqs"); err != nil {
			fmt.Fprintln(os.Stderr, "pipreqs not found in PATH:", err)
			os.Exit(1)
		}
	}

	var updatedCount uint64
	var errorCount uint64
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	// Create context for cancellation and coordination
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, dir := range reqDirs {
		wg.Add(1)
		sem <- struct{}{}
		go func(d string) {
			defer wg.Done()
			defer func() { <-sem }()

			// Check if context is cancelled
			select {
			case <-ctx.Done():
				return
			default:
			}

			changed, err := updateRequirements(d, dryRun)
			if err != nil {
				// Don't print error output during progress display to avoid scrolling
				// Errors will be shown in final summary
				atomic.AddUint64(&errorCount, 1)
			} else {
				if changed {
					atomic.AddUint64(&updatedCount, 1)
				}
			}
		}(dir)
	}
	wg.Wait()

	// Cancel context to stop progress display
	cancel()

	fmt.Println("processed:", len(reqDirs), "updated:", atomic.LoadUint64(&updatedCount), "errors:", atomic.LoadUint64(&errorCount))
}

func findRequirementsDirs(root string, maxDepth int) ([]string, error) {
	var matched []string
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(rootAbs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("path is not a directory: " + rootAbs)
	}

	err = filepath.WalkDir(rootAbs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// depth limit
		if maxDepth >= 0 {
			rel, _ := filepath.Rel(rootAbs, path)
			if rel != "." {
				depth := strings.Count(rel, string(os.PathSeparator))
				if depth > maxDepth {
					if d.IsDir() {
						return fs.SkipDir
					}
					return nil
				}
			}
		}
		// no exclusions
		if !d.IsDir() && strings.EqualFold(d.Name(), "requirements.txt") {
			matched = append(matched, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// de-duplicate
	seen := make(map[string]struct{}, len(matched))
	out := make([]string, 0, len(matched))
	for _, dir := range matched {
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		out = append(out, dir)
	}
	return out, nil
}

func updateRequirements(dir string, dryRun bool) (bool, error) {
	reqPath := filepath.Join(dir, "requirements.txt")
	backupPath := reqPath + ".bak"

	if dryRun {
		// Don't print dry-run details during progress display to avoid scrolling
		return false, nil
	}

	// move current requirements.txt to .bak (overwrite any existing .bak)
	var preHash string
	preExists := false
	if _, err := os.Stat(reqPath); err == nil {
		preExists = true
		if h, err := fileHash(reqPath); err == nil {
			preHash = h
		}
		// remove old backup if present to mimic a clean move
		_ = os.Remove(backupPath)
		if err := os.Rename(reqPath, backupPath); err != nil {
			return false, err
		}
	}

	args := []string{"."}
	if out, err := runCmd("pipreqs", args, dir); err != nil {
		return false, fmt.Errorf("pipreqs failed: %w\n%s", err, string(out))
	}
	// check post state
	postExists := false
	postHash := ""
	if _, err := os.Stat(reqPath); err == nil {
		postExists = true
		if h, err := fileHash(reqPath); err == nil {
			postHash = h
		}
	}
	changed := (!preExists && postExists) || (preExists && postExists && preHash != postHash)
	return changed, nil
}

func runCmd(bin string, args []string, workDir string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = workDir
	cmd.Env = os.Environ()
	return cmd.CombinedOutput()
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
