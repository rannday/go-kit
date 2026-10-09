package build

import (
	"fmt"
	"io"
)

func PrintUsage(w io.Writer) {
	targets := DefaultTargetStrings()

	if _, err := fmt.Fprintln(w, "Usage:"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  buildx [options] -v <version>"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "Options:"); err != nil {
		return
	}
	printHelpOption(w, "-v, --version <version>", "release version (required)")
	printHelpOption(w, "-n, --name <name>", "binary/release name (default: repo directory)")
	printHelpOption(w, "-m, --main <package>", "main package (default: ./cmd/<name>, then repo root)")
	printHelpOption(w, "--version-var <symbol>", "Go symbol set with -ldflags -X")
	printHelpOption(w, "-o, --out <dir>", "output directory (default: dist/<version>)")
	printHelpOption(w, "-c, --clean", "remove output directory before building")
	printHelpOption(w, "-f, --force", "overwrite existing artifacts")
	printHelpOption(w, "-t, --target <target>", "build target, repeatable: GOOS/GOARCH[:zip|tar.gz]")
	printHelpOption(w, "--ldflags <value>", "additional linker flags")
	printHelpOption(w, "--no-strip", "do not add -s -w linker flags")
	printHelpOption(w, "--go <path>", "Go command to run (default: go)")
	printHelpOption(w, "--checksum-name <name>", "checksum file name (default: "+DefaultChecksumName+")")
	printHelpOption(w, "--verbose", "print build commands and Go output")
	if _, err := fmt.Fprintln(w); err != nil {
		return
	}
	printHelpOption(w, "-h, --help", "display this help and exit")
	if _, err := fmt.Fprintln(w); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "Default targets:"); err != nil {
		return
	}
	if _, err := fmt.Fprintf(w, "  %s, %s, %s,\n", targets[0], targets[1], targets[2]); err != nil {
		return
	}
	if _, err := fmt.Fprintf(w, "  %s, %s\n", targets[3], targets[4]); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "Output:"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  archives: <name>-<version>-<goos>-<goarch>.<format>"); err != nil {
		return
	}
	if _, err := fmt.Fprintf(w, "  checksum: %s\n", DefaultChecksumName); err != nil {
		return
	}
}

func printHelpOption(w io.Writer, name, desc string) {
	if _, err := fmt.Fprintf(w, "  %-29s %s\n", name, desc); err != nil {
		return
	}
}
