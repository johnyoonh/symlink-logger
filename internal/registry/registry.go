package registry

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Candidate struct {
	OldPath      string
	TargetPath   string
	DiscoveredOn string
	Source       string
}

const Header = "old_path\ttarget_path\tdiscovered_on\tsource"

func Scan(root string, now time.Time) ([]Candidate, error) {
	return ScanWithOptions(root, now, false)
}

func ScanWithOptions(root string, now time.Time, recursive bool) ([]Candidate, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if !recursive {
		return scanTopLevel(root, now)
	}

	discoveredOn := now.Format("2006-01-02")
	var candidates []Candidate
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return nil
		}
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		candidates = append(candidates, Candidate{
			OldPath:      path,
			TargetPath:   target,
			DiscoveredOn: discoveredOn,
			Source:       "recursive-symlink",
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].OldPath < candidates[j].OldPath
	})
	return candidates, nil
}

func scanTopLevel(root string, now time.Time) ([]Candidate, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	discoveredOn := now.Format("2006-01-02")
	var candidates []Candidate
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		oldPath := filepath.Join(root, entry.Name())
		target, err := os.Readlink(oldPath)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, Candidate{
			OldPath:      oldPath,
			TargetPath:   target,
			DiscoveredOn: discoveredOn,
			Source:       "top-level-symlink",
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].OldPath < candidates[j].OldPath
	})
	return candidates, nil
}

func Replace(candidate Candidate) error {
	if candidate.OldPath == "" || candidate.TargetPath == "" {
		return errors.New("candidate old path and target path are required")
	}
	info, err := os.Lstat(candidate.OldPath)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s is not a symlink", candidate.OldPath)
	}
	current, err := os.Readlink(candidate.OldPath)
	if err != nil {
		return err
	}
	if current != candidate.TargetPath {
		return fmt.Errorf("%s points to %s, registry says %s", candidate.OldPath, current, candidate.TargetPath)
	}
	targetInfo, err := os.Stat(ResolvedTargetPath(candidate))
	if err != nil {
		return err
	}
	if !targetInfo.IsDir() {
		return fmt.Errorf("%s is not a directory target", candidate.TargetPath)
	}
	if err := os.Remove(candidate.OldPath); err != nil {
		return err
	}
	return os.Mkdir(candidate.OldPath, 0o755)
}

func ResolvedTargetPath(candidate Candidate) string {
	if filepath.IsAbs(candidate.TargetPath) {
		return candidate.TargetPath
	}
	return filepath.Clean(filepath.Join(filepath.Dir(candidate.OldPath), candidate.TargetPath))
}

func WriteTSV(w io.Writer, candidates []Candidate) error {
	if _, err := fmt.Fprintln(w, Header); err != nil {
		return err
	}
	for _, c := range candidates {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.OldPath, c.TargetPath, c.DiscoveredOn, c.Source); err != nil {
			return err
		}
	}
	return nil
}

func ReadTSV(path string) ([]Candidate, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var candidates []Candidate
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if lineNo == 1 && line == Header {
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 4 {
			return nil, fmt.Errorf("%s:%d: expected 4 tab-separated fields", path, lineNo)
		}
		candidates = append(candidates, Candidate{
			OldPath:      parts[0],
			TargetPath:   parts[1],
			DiscoveredOn: parts[2],
			Source:       parts[3],
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return candidates, nil
}

func Find(candidates []Candidate, oldPath string) (Candidate, bool) {
	cleanOld := filepath.Clean(oldPath)
	for _, c := range candidates {
		if filepath.Clean(c.OldPath) == cleanOld {
			return c, true
		}
	}
	return Candidate{}, false
}

func Restore(candidate Candidate) error {
	if candidate.OldPath == "" || candidate.TargetPath == "" {
		return errors.New("candidate old path and target path are required")
	}

	info, err := os.Lstat(candidate.OldPath)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			current, readErr := os.Readlink(candidate.OldPath)
			if readErr == nil && current == candidate.TargetPath {
				return nil
			}
			return fmt.Errorf("%s is already a symlink to %s", candidate.OldPath, current)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s exists and is not an empty mountpoint directory", candidate.OldPath)
		}
		entries, readErr := os.ReadDir(candidate.OldPath)
		if readErr != nil {
			return readErr
		}
		if len(entries) > 0 {
			return fmt.Errorf("%s is not empty; unmount it before restoring the symlink", candidate.OldPath)
		}
		if removeErr := os.Remove(candidate.OldPath); removeErr != nil {
			return removeErr
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	return os.Symlink(candidate.TargetPath, candidate.OldPath)
}
