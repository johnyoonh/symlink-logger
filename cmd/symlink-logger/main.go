package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/johnyoonh/symlink-logger/internal/logger"
	"github.com/johnyoonh/symlink-logger/internal/registry"
	"github.com/johnyoonh/symlink-logger/internal/symlinkfs"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "scan":
		err = runScan(os.Args[2:])
	case "plan":
		err = runPlan(os.Args[2:])
	case "replace-all":
		err = runReplaceAll(os.Args[2:])
	case "replace":
		err = runReplace(os.Args[2:])
	case "mount":
		err = runMount(os.Args[2:])
	case "mount-all":
		err = runMountAll(os.Args[2:])
	case "unmount":
		err = runUnmount(os.Args[2:])
	case "unmount-all":
		err = runUnmountAll(os.Args[2:])
	case "restore":
		err = runRestore(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "symlink-logger:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  symlink-logger scan --root /Users/john/repos [--recursive]")
	fmt.Fprintln(os.Stderr, "  symlink-logger plan --root /Users/john/repos [--recursive]")
	fmt.Fprintln(os.Stderr, "  symlink-logger replace-all --root ROOT [--recursive] [--apply]")
	fmt.Fprintln(os.Stderr, "  symlink-logger replace --old OLD [--registry PATH]")
	fmt.Fprintln(os.Stderr, "  symlink-logger mount --old OLD [--target TARGET] [--registry PATH] [--replace]")
	fmt.Fprintln(os.Stderr, "  symlink-logger mount-all --root ROOT [--recursive] [--replace]")
	fmt.Fprintln(os.Stderr, "  symlink-logger unmount --old OLD [--restore] [--registry PATH]")
	fmt.Fprintln(os.Stderr, "  symlink-logger unmount-all [--registry PATH] [--restore]")
	fmt.Fprintln(os.Stderr, "  symlink-logger restore --old OLD [--registry PATH]")
}

func runScan(args []string) error {
	flags := flag.NewFlagSet("scan", flag.ExitOnError)
	root := flags.String("root", defaultRoot(), "root directory to scan for top-level symlinks")
	recursive := flags.Bool("recursive", false, "scan symlinks recursively")
	if err := flags.Parse(args); err != nil {
		return err
	}

	candidates, err := registry.ScanWithOptions(*root, time.Now(), *recursive)
	if err != nil {
		return err
	}
	return registry.WriteTSV(os.Stdout, candidates)
}

func runPlan(args []string) error {
	flags := flag.NewFlagSet("plan", flag.ExitOnError)
	root := flags.String("root", defaultRoot(), "root directory to scan")
	recursive := flags.Bool("recursive", false, "scan symlinks recursively")
	if err := flags.Parse(args); err != nil {
		return err
	}
	candidates, err := registry.ScanWithOptions(*root, time.Now(), *recursive)
	if err != nil {
		return err
	}
	fmt.Printf("Symlink replacement plan\n")
	fmt.Printf("Root: %s\n", *root)
	fmt.Printf("Recursive: %t\n", *recursive)
	fmt.Printf("Count: %d\n\n", len(candidates))
	for _, c := range candidates {
		fmt.Printf("replace %s -> %s\n", c.OldPath, c.TargetPath)
	}
	return nil
}

func runReplaceAll(args []string) error {
	flags := flag.NewFlagSet("replace-all", flag.ExitOnError)
	root := flags.String("root", defaultRoot(), "root directory to scan")
	recursive := flags.Bool("recursive", false, "scan symlinks recursively")
	apply := flags.Bool("apply", false, "actually replace symlinks with mountpoint directories")
	maxApply := flags.Int("max-apply", 100, "maximum replacements allowed in one apply run")
	if err := flags.Parse(args); err != nil {
		return err
	}
	candidates, err := registry.ScanWithOptions(*root, time.Now(), *recursive)
	if err != nil {
		return err
	}
	candidates, skipped := directoryCandidates(candidates)
	if !*apply {
		fmt.Printf("Dry run: would replace %d directory symlinks. Add --apply to execute.\n", len(candidates))
		fmt.Printf("Skipped non-directory symlinks: %d\n\n", skipped)
		return registry.WriteTSV(os.Stdout, candidates)
	}
	if len(candidates) > *maxApply {
		return fmt.Errorf("refusing to replace %d symlinks; raise --max-apply after reviewing the plan", len(candidates))
	}
	for _, candidate := range candidates {
		if err := registry.Replace(candidate); err != nil {
			return err
		}
		fmt.Printf("replaced %s -> %s\n", candidate.OldPath, candidate.TargetPath)
	}
	return nil
}

func runReplace(args []string) error {
	flags := flag.NewFlagSet("replace", flag.ExitOnError)
	oldPath := flags.String("old", "", "old symlink path to replace with an empty mountpoint directory")
	registryPath := flags.String("registry", defaultRegistry(), "candidate registry TSV")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *oldPath == "" {
		return fmt.Errorf("replace requires --old")
	}
	candidate, err := candidateFromRegistry(*registryPath, *oldPath)
	if err != nil {
		return err
	}
	return registry.Replace(candidate)
}

func runMount(args []string) error {
	flags := flag.NewFlagSet("mount", flag.ExitOnError)
	oldPath := flags.String("old", "", "old path mountpoint")
	targetPath := flags.String("target", "", "real target directory; defaults from registry")
	registryPath := flags.String("registry", defaultRegistry(), "candidate registry TSV")
	logPath := flags.String("log", defaultAccessLog(), "JSONL access log path")
	replace := flags.Bool("replace", false, "replace the symlink with a mountpoint before mounting")
	debug := flags.Bool("debug", false, "enable go-fuse debug logging")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *oldPath == "" {
		return fmt.Errorf("mount requires --old")
	}
	if *targetPath == "" {
		candidate, err := candidateFromRegistry(*registryPath, *oldPath)
		if err != nil {
			return err
		}
		*targetPath = registry.ResolvedTargetPath(candidate)
		if *replace {
			if err := registry.Replace(candidate); err != nil {
				return err
			}
		}
	} else if *replace {
		return fmt.Errorf("--replace requires using --registry target lookup, not explicit --target")
	}

	cleanOld, err := filepath.Abs(*oldPath)
	if err != nil {
		return err
	}
	cleanTarget, err := filepath.Abs(*targetPath)
	if err != nil {
		return err
	}
	if info, err := os.Stat(cleanOld); err != nil {
		return err
	} else if !info.IsDir() {
		return fmt.Errorf("%s is not a directory mountpoint", cleanOld)
	}
	if info, err := os.Stat(cleanTarget); err != nil {
		return err
	} else if !info.IsDir() {
		return fmt.Errorf("%s is not a target directory", cleanTarget)
	}

	accessLogger, err := logger.Open(*logPath, cleanOld, cleanTarget)
	if err != nil {
		return err
	}
	defer accessLogger.Close()

	root, err := symlinkfs.NewRoot(cleanTarget, accessLogger)
	if err != nil {
		return err
	}
	opts := &fs.Options{
		MountOptions: fuse.MountOptions{
			Debug: *debug,
			Name:  "symlink-logger",
			Options: []string{
				"default_permissions",
			},
		},
	}
	server, err := fs.Mount(cleanOld, root, opts)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Mounted %s -> %s\n", cleanOld, cleanTarget)
	fmt.Fprintf(os.Stderr, "Access log: %s\n", accessLogger.Path())

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		_ = server.Unmount()
	}()

	server.Wait()
	return nil
}

func runMountAll(args []string) error {
	flags := flag.NewFlagSet("mount-all", flag.ExitOnError)
	rootPath := flags.String("root", defaultRoot(), "root directory to scan")
	recursive := flags.Bool("recursive", false, "scan symlinks recursively")
	registryPath := flags.String("registry", "", "candidate registry TSV; when set, use it instead of scanning")
	logPath := flags.String("log", defaultAccessLog(), "JSONL access log path")
	replace := flags.Bool("replace", false, "replace symlinks with mountpoints before mounting")
	debug := flags.Bool("debug", false, "enable go-fuse debug logging")
	maxMounts := flags.Int("max-mounts", 100, "maximum mounts allowed in one run")
	if err := flags.Parse(args); err != nil {
		return err
	}

	candidates, err := candidatesForAll(*registryPath, *rootPath, *recursive)
	if err != nil {
		return err
	}
	candidates, skipped := directoryCandidates(candidates)
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "Skipping %d non-directory symlinks\n", skipped)
	}
	if len(candidates) > *maxMounts {
		return fmt.Errorf("refusing to mount %d candidates; raise --max-mounts after reviewing the plan", len(candidates))
	}

	var servers mountedServers
	var loggers []*logger.Logger
	defer func() {
		for _, accessLogger := range loggers {
			_ = accessLogger.Close()
		}
	}()

	for _, candidate := range candidates {
		if *replace {
			if err := replaceIfSymlink(candidate); err != nil {
				servers.unmountAll()
				return err
			}
		}
		server, accessLogger, err := mountCandidate(candidate, *logPath, *debug)
		if err != nil {
			servers.unmountAll()
			return err
		}
		servers = append(servers, server)
		loggers = append(loggers, accessLogger)
		fmt.Fprintf(os.Stderr, "Mounted %s -> %s\n", candidate.OldPath, candidate.TargetPath)
	}
	fmt.Fprintf(os.Stderr, "Mounted %d candidates\n", len(servers))
	fmt.Fprintf(os.Stderr, "Access log: %s\n", *logPath)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		servers.unmountAll()
	}()

	servers.waitAll()
	return nil
}

func runUnmount(args []string) error {
	flags := flag.NewFlagSet("unmount", flag.ExitOnError)
	oldPath := flags.String("old", "", "old mountpoint path")
	restore := flags.Bool("restore", false, "restore symlink after unmount")
	registryPath := flags.String("registry", defaultRegistry(), "candidate registry TSV")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *oldPath == "" {
		return fmt.Errorf("unmount requires --old")
	}
	cleanOld, err := filepath.Abs(*oldPath)
	if err != nil {
		return err
	}
	if err := syscall.Unmount(cleanOld, 0); err != nil && err != syscall.EINVAL {
		return err
	}
	if *restore {
		candidate, err := candidateFromRegistry(*registryPath, cleanOld)
		if err != nil {
			return err
		}
		return registry.Restore(candidate)
	}
	return nil
}

func runUnmountAll(args []string) error {
	flags := flag.NewFlagSet("unmount-all", flag.ExitOnError)
	rootPath := flags.String("root", defaultRoot(), "root directory to scan when --registry is omitted")
	recursive := flags.Bool("recursive", false, "scan symlinks recursively when --registry is omitted")
	registryPath := flags.String("registry", defaultRegistry(), "candidate registry TSV")
	restore := flags.Bool("restore", false, "restore symlinks after unmount")
	if err := flags.Parse(args); err != nil {
		return err
	}
	candidates, err := candidatesForAll(*registryPath, *rootPath, *recursive)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		_ = syscall.Unmount(candidate.OldPath, 0)
		if *restore {
			if err := registry.Restore(candidate); err != nil {
				return err
			}
		}
	}
	return nil
}

func runRestore(args []string) error {
	flags := flag.NewFlagSet("restore", flag.ExitOnError)
	oldPath := flags.String("old", "", "old path to restore")
	registryPath := flags.String("registry", defaultRegistry(), "candidate registry TSV")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *oldPath == "" {
		return fmt.Errorf("restore requires --old")
	}

	candidate, err := candidateFromRegistry(*registryPath, *oldPath)
	if err != nil {
		return err
	}
	return registry.Restore(candidate)
}

func candidateFromRegistry(registryPath, oldPath string) (registry.Candidate, error) {
	candidates, err := registry.ReadTSV(registryPath)
	if err != nil {
		return registry.Candidate{}, err
	}
	candidate, ok := registry.Find(candidates, oldPath)
	if !ok {
		return registry.Candidate{}, fmt.Errorf("%s is not in %s", oldPath, registryPath)
	}
	return candidate, nil
}

func candidatesForAll(registryPath, rootPath string, recursive bool) ([]registry.Candidate, error) {
	if registryPath != "" {
		return registry.ReadTSV(registryPath)
	}
	return registry.ScanWithOptions(rootPath, time.Now(), recursive)
}

func directoryCandidates(candidates []registry.Candidate) ([]registry.Candidate, int) {
	var dirs []registry.Candidate
	skipped := 0
	for _, candidate := range candidates {
		info, err := os.Stat(registry.ResolvedTargetPath(candidate))
		if err != nil || !info.IsDir() {
			skipped++
			continue
		}
		dirs = append(dirs, candidate)
	}
	return dirs, skipped
}

func replaceIfSymlink(candidate registry.Candidate) error {
	info, err := os.Lstat(candidate.OldPath)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	return registry.Replace(candidate)
}

func mountCandidate(candidate registry.Candidate, logPath string, debug bool) (*fuse.Server, *logger.Logger, error) {
	cleanOld, err := filepath.Abs(candidate.OldPath)
	if err != nil {
		return nil, nil, err
	}
	cleanTarget, err := filepath.Abs(registry.ResolvedTargetPath(candidate))
	if err != nil {
		return nil, nil, err
	}
	if info, err := os.Stat(cleanOld); err != nil {
		return nil, nil, err
	} else if !info.IsDir() {
		return nil, nil, fmt.Errorf("%s is not a directory mountpoint", cleanOld)
	}
	if info, err := os.Stat(cleanTarget); err != nil {
		return nil, nil, err
	} else if !info.IsDir() {
		return nil, nil, fmt.Errorf("%s is not a target directory", cleanTarget)
	}

	accessLogger, err := logger.Open(logPath, cleanOld, cleanTarget)
	if err != nil {
		return nil, nil, err
	}
	root, err := symlinkfs.NewRoot(cleanTarget, accessLogger)
	if err != nil {
		_ = accessLogger.Close()
		return nil, nil, err
	}
	opts := &fs.Options{
		MountOptions: fuse.MountOptions{
			Debug: debug,
			Name:  "symlink-logger",
			Options: []string{
				"default_permissions",
			},
		},
	}
	server, err := fs.Mount(cleanOld, root, opts)
	if err != nil {
		_ = accessLogger.Close()
		return nil, nil, err
	}
	return server, accessLogger, nil
}

type mountedServers []*fuse.Server

func (servers mountedServers) unmountAll() {
	for i := len(servers) - 1; i >= 0; i-- {
		_ = servers[i].Unmount()
	}
}

func (servers mountedServers) waitAll() {
	done := make(chan struct{}, len(servers))
	for _, server := range servers {
		go func(s *fuse.Server) {
			s.Wait()
			done <- struct{}{}
		}(server)
	}
	for range servers {
		<-done
	}
}

func defaultRoot() string {
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, "repos")
	}
	return "."
}

func defaultRegistry() string {
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, "repos", "repo-symlink-candidates.tsv")
	}
	return "repo-symlink-candidates.tsv"
}

func defaultAccessLog() string {
	if value := os.Getenv("SYMLINK_LOGGER_LOG"); value != "" {
		return value
	}
	if value := os.Getenv("REPO_REDIRECT_FUSE_LOG"); value != "" {
		return value
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "state", "symlink-logger", "access.jsonl")
	}
	return "access.jsonl"
}
