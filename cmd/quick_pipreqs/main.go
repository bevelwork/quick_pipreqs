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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bevelwork/quick_pipreqs/version"
)

// getConsoleHeight attempts to detect the console height, returns default if unable
func getConsoleHeight() int {
	// Try to get terminal size using stty
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	output, err := cmd.Output()
	if err != nil {
		return 24 // default fallback
	}

	// Parse output: "rows cols"
	parts := strings.Fields(string(output))
	if len(parts) >= 1 {
		if height, err := strconv.Atoi(parts[0]); err == nil && height > 0 {
			return height
		}
	}

	return 24 // default fallback
}

// ProgressState represents the state of a directory being processed
type ProgressState int

const (
	StateWaiting ProgressState = iota
	StateActive
	StateDone
)

// ProgressTracker tracks the progress of all directories
type ProgressTracker struct {
	states map[string]ProgressState
	mutex  sync.RWMutex
	total  int
	root   string // base path for relative path display
	ctx    context.Context
}

// NewProgressTracker creates a new progress tracker
func NewProgressTracker(dirs []string, root string, ctx context.Context) *ProgressTracker {
	states := make(map[string]ProgressState)
	for _, dir := range dirs {
		states[dir] = StateWaiting
	}
	return &ProgressTracker{
		states: states,
		total:  len(dirs),
		root:   root,
		ctx:    ctx,
	}
}

// SetState updates the state of a directory
func (pt *ProgressTracker) SetState(dir string, state ProgressState) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()
	pt.states[dir] = state
}

// GetCounts returns the counts for each state
func (pt *ProgressTracker) GetCounts() (waiting, active, done int) {
	pt.mutex.RLock()
	defer pt.mutex.RUnlock()

	for _, state := range pt.states {
		switch state {
		case StateWaiting:
			waiting++
		case StateActive:
			active++
		case StateDone:
			done++
		}
	}
	return
}

// GetActiveDirs returns a list of currently active directories
func (pt *ProgressTracker) GetActiveDirs() []string {
	pt.mutex.RLock()
	defer pt.mutex.RUnlock()

	var active []string
	for dir, state := range pt.states {
		if state == StateActive {
			active = append(active, dir)
		}
	}
	return active
}

// PrintProgress prints the current progress with active directories in a fixed 6-line buffer
func (pt *ProgressTracker) PrintProgress() {
	// Get active directories
	activeDirs := pt.GetActiveDirs()

	// Move cursor up 6 lines to start of our buffer area
	fmt.Printf("\033[6A")

	// Clear and print each of the 6 lines
	for i := 0; i < 6; i++ {
		fmt.Printf("\r\033[K") // Clear current line and return to beginning

		if i < len(activeDirs) {
			// Print active directory
			relPath, err := filepath.Rel(pt.root, activeDirs[i])
			if err != nil {
				relPath = activeDirs[i] // fallback to absolute path if relative fails
			}
			fmt.Printf("Active: %s", relPath)
		}
		// If no active directory for this line, it remains empty (cleared above)

		if i < 5 { // Move to next line for lines 1-5
			fmt.Printf("\n")
		}
	}

	// Print progress bar on the 6th line (no newline after this)
	waiting, active, done := pt.GetCounts()
	fmt.Printf("\r\033[K[ %d: Waiting, %d Active, %d Done ]", waiting, active, done)
}

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

	// log discovered directories
	logger := log.New(os.Stdout, "", log.LstdFlags)
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

	// Create progress tracker
	progress := NewProgressTracker(reqDirs, root, ctx)

	// Get console height and scroll down by one full screen to ensure clean state
	consoleHeight := getConsoleHeight()
	fmt.Printf("\033[%dS", consoleHeight) // Scroll down by console height
	fmt.Printf("\033[1;1H")               // Move cursor to top-left after scroll

	// Initialize the 6-line display buffer
	fmt.Println() // Start on a new line
	for i := 0; i < 6; i++ {
		fmt.Println() // Print 6 empty lines for our buffer
	}

	// Move cursor back to the beginning of our buffer area
	fmt.Printf("\033[6A")

	// Start progress display goroutine
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				progress.PrintProgress()
			case <-ctx.Done():
				progress.PrintProgress()
				// Move cursor to end of progress bar and add final newline
				fmt.Printf("\n")
				return
			}
		}
	}()

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

			// Mark as active when starting
			progress.SetState(d, StateActive)

			// Don't print verbose output during progress display to avoid scrolling
			// Verbose output is disabled when progress tracking is active

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

			// Mark as done when finished
			progress.SetState(d, StateDone)
		}(dir)
	}
	wg.Wait()

	// Cancel context to stop progress display
	cancel()

	// Scroll down by one terminal height before final output
	fmt.Printf("\033[%dS", consoleHeight) // Scroll down by console height
	fmt.Printf("\033[1;1H")               // Move cursor to top-left after scroll

	// Print final status bar
	fmt.Printf("[ %d: Waiting, %d Active, %d Done ]\n", 0, 0, len(reqDirs))
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
