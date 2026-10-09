package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rannday/go-kit/modules/buildx/archive"
	"github.com/rannday/go-kit/modules/buildx/checksum"
)

type Result struct {
	RepoRoot     string
	Name         string
	Version      string
	MainPackage  string
	OutputDir    string
	OutputDirRel string
	Archives     []string
	ChecksumPath string
}

func Run(opts Options) (Result, error) {
	repoRoot, err := FindRepoRoot()
	if err != nil {
		return Result{}, err
	}

	if err := ValidateVersion(opts.Version); err != nil {
		return Result{}, err
	}

	if opts.GoBinary == "" {
		opts.GoBinary = "go"
	}
	if opts.ChecksumName == "" {
		opts.ChecksumName = DefaultChecksumName
	}

	name := opts.Name
	if name == "" {
		name = filepath.Base(repoRoot)
	}
	if err := validateArtifactName("name", name); err != nil {
		return Result{}, err
	}
	if err := validateArtifactName("checksum name", opts.ChecksumName); err != nil {
		return Result{}, err
	}

	mainPkg := opts.MainPackage
	if mainPkg == "" {
		mainPkg, err = ResolveDefaultMain(repoRoot, name)
		if err != nil {
			return Result{}, err
		}
	}

	outAbs, outRel, err := ResolveOutputDir(repoRoot, opts.Version, opts.OutDir)
	if err != nil {
		return Result{}, err
	}

	targets := opts.Targets
	if len(targets) == 0 {
		targets = DefaultTargets()
	}

	if err := validateTargets(targets, opts.GoBinary); err != nil {
		return Result{}, err
	}

	if err := ValidateUniqueArchiveNames(name, opts.Version, targets); err != nil {
		return Result{}, err
	}
	for _, target := range targets {
		if strings.EqualFold(opts.ChecksumName, ArchiveName(name, opts.Version, target)) {
			return Result{}, fmt.Errorf("checksum filename collides with archive: %s", opts.ChecksumName)
		}
	}

	if opts.Clean {
		if err := validateCleanOutput(repoRoot, outAbs); err != nil {
			return Result{}, err
		}
		if err := os.RemoveAll(outAbs); err != nil {
			return Result{}, err
		}
	}

	if err := os.MkdirAll(outAbs, 0o755); err != nil {
		return Result{}, err
	}

	if !opts.Force {
		empty, err := dirIsEmpty(outAbs)
		if err != nil {
			return Result{}, err
		}
		if !empty {
			return Result{}, fmt.Errorf("output directory not empty: %s", outAbs)
		}
	}

	stage, err := os.MkdirTemp(outAbs, ".build-release-*")
	if err != nil {
		return Result{}, err
	}
	keepStage := false
	defer func() {
		if !keepStage {
			_ = os.RemoveAll(stage)
		}
	}()
	archives, checksumEntries, err := buildTargets(repoRoot, stage, name, mainPkg, opts, targets)
	if err != nil {
		return Result{}, err
	}

	checksumPath, err := checksum.WriteAtomic(stage, opts.ChecksumName, checksumEntries)
	if err != nil {
		return Result{}, err
	}
	keepStage, err = publishRelease(stage, outAbs, append(append([]string{}, archives...), checksumPath))
	if err != nil {
		return Result{}, err
	}
	for i := range archives {
		archives[i] = filepath.Join(outAbs, filepath.Base(archives[i]))
	}
	checksumPath = filepath.Join(outAbs, opts.ChecksumName)

	return Result{
		RepoRoot:     repoRoot,
		Name:         name,
		Version:      opts.Version,
		MainPackage:  mainPkg,
		OutputDir:    outAbs,
		OutputDirRel: outRel,
		Archives:     archives,
		ChecksumPath: checksumPath,
	}, nil
}

func validateTargets(targets []TargetSpec, goBinary string) error {
	for _, target := range targets {
		if err := ValidateTargetPlatform(target, goBinary); err != nil {
			return err
		}
	}
	return nil
}

type targetBuildResult struct {
	archivePath string
	entry       checksum.Entry
}

func buildTargets(repoRoot, outAbs, name, mainPkg string, opts Options, targets []TargetSpec) ([]string, []checksum.Entry, error) {
	results := make([]targetBuildResult, len(targets))
	errs := make([]error, len(targets))

	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		go func(i int, target TargetSpec) {
			defer wg.Done()
			results[i], errs[i] = buildOneTarget(repoRoot, outAbs, name, mainPkg, opts, target)
		}(i, target)
	}
	wg.Wait()

	var firstErr error
	created := make([]string, 0, len(targets))
	for i := range targets {
		if errs[i] != nil {
			if firstErr == nil {
				firstErr = errs[i]
			}
			continue
		}
		created = append(created, results[i].archivePath)
	}

	if firstErr != nil {
		removePaths(created)
		return nil, nil, firstErr
	}

	archives := make([]string, 0, len(targets))
	checksumEntries := make([]checksum.Entry, 0, len(targets))
	for i := range targets {
		archives = append(archives, results[i].archivePath)
		checksumEntries = append(checksumEntries, results[i].entry)
	}

	return archives, checksumEntries, nil
}

func buildOneTarget(repoRoot, outAbs, name, mainPkg string, opts Options, target TargetSpec) (targetBuildResult, error) {
	archivePath := filepath.Join(outAbs, ArchiveName(name, opts.Version, target))
	buildDir, err := os.MkdirTemp(outAbs, ".build-*")
	if err != nil {
		return targetBuildResult{}, err
	}
	defer os.RemoveAll(buildDir)

	binaryPath := filepath.Join(buildDir, BinaryName(name, target.GOOS))
	if err := buildTarget(repoRoot, mainPkg, opts, target, binaryPath); err != nil {
		return targetBuildResult{}, err
	}

	if err := archive.WriteAtomic(archivePath, target.Format, []archive.Item{
		{Name: BinaryName(name, target.GOOS), Path: binaryPath, Mode: 0o755},
	}); err != nil {
		return targetBuildResult{}, err
	}

	sum, err := checksum.SumFile(archivePath)
	if err != nil {
		removePaths([]string{archivePath})
		return targetBuildResult{}, err
	}

	return targetBuildResult{
		archivePath: archivePath,
		entry: checksum.Entry{
			Name: filepath.Base(archivePath),
			Sum:  sum,
		},
	}, nil
}

func buildTarget(repoRoot, mainPkg string, opts Options, target TargetSpec, binaryPath string) error {
	ldflags := BuildLdflags(opts.Version, opts.VersionVar, opts.Ldflags, !opts.NoStrip)
	args := []string{"build", "-trimpath", "-buildvcs=false", "-o", binaryPath}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, mainPkg)

	cmd := exec.Command(opts.GoBinary, args...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"GOOS="+target.GOOS,
		"GOARCH="+target.GOARCH,
		"CGO_ENABLED=0",
	)

	if opts.Verbose {
		fmt.Fprintf(os.Stdout, "%s\n", strings.Join(cmd.Args, " "))
	}

	output, err := cmd.CombinedOutput()
	if opts.Verbose && len(output) > 0 {
		_, _ = os.Stdout.Write(output)
	}
	if err != nil {
		if len(output) > 0 {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
		}
		return err
	}

	return nil
}

func dirIsEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if isIgnorableBuildArtifact(entry.Name()) {
			continue
		}
		return false, nil
	}
	return true, nil
}

func isIgnorableBuildArtifact(name string) bool {
	return strings.HasPrefix(name, ".build-") || strings.Contains(name, ".tmp-")
}

func removePaths(paths []string) {
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		_ = os.Remove(path)
	}
}
