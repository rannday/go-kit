// Run from the repository root: go run scripts/verify-migration.go -sources ..
// This verifier reads the backup repositories without modifying them.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type inventoryEntry struct {
	Module      string `json:"module"`
	Source      string `json:"source"`
	File        string `json:"file"`
	Destination string `json:"destination"`
	Repository  bool   `json:"repository"`
	SHA256      string `json:"sha256"`
}

type apiCounts struct {
	Functions, Methods, Types, Fields, Constants, Variables, Errors int
}

var sourceModules = map[string]string{
	"go-log": "logx", "go-env": "envx", "go-conf": "confx", "go-build-bin": "buildx",
}

func main() {
	sources := flag.String("sources", "..", "parent directory containing the four original repositories")
	flag.Parse()
	if err := verify(*sources); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func verify(sources string) error {
	data, err := os.ReadFile("docs/source-inventory.json")
	if err != nil {
		return err
	}
	var entries []inventoryEntry
	if err := json.Unmarshal(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}), &entries); err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("source inventory is empty")
	}
	goFiles := make(map[string]map[string]bool)
	counts := make(map[string]apiCounts)
	modulesChecked := make(map[string]bool)
	for _, entry := range entries {
		if sourceModules[entry.Source] != entry.Module || entry.Module == "" {
			return fmt.Errorf("unknown source/module pair: %s/%s", entry.Source, entry.Module)
		}
		if err := safePath(entry.File); err != nil {
			return err
		}
		if err := safePath(entry.Destination); err != nil {
			return err
		}
		srcPath := filepath.Join(sources, entry.Source, filepath.FromSlash(entry.File))
		dstPath := filepath.Join("modules", entry.Module, filepath.FromSlash(entry.Destination))
		if entry.Repository {
			if entry.File != ".editorconfig" && entry.File != ".gitattributes" && entry.File != ".gitignore" && entry.File != "LICENSE" && !strings.HasPrefix(entry.File, ".agents/skills/") && !(entry.File == "AGENTS.md" && entry.Destination == "docs/"+entry.Module+".md") {
				return fmt.Errorf("%s: only shared configuration, licenses, skills, and module guidance may move to the repository root", entry.File)
			}
			dstPath = filepath.FromSlash(entry.Destination)
		}
		original, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(original)); !strings.EqualFold(got, entry.SHA256) {
			return fmt.Errorf("%s: original SHA256 changed: got %s, inventory %s", srcPath, got, entry.SHA256)
		}
		copied, err := os.ReadFile(dstPath)
		if err != nil {
			return err
		}
		isGo := strings.HasSuffix(entry.File, ".go")
		if !isGo && entry.File != "go.mod" {
			continue
		}
		expected := normalize(original)
		for source, module := range sourceModules {
			expected = strings.ReplaceAll(expected, "github.com/rannday/"+source, "github.com/rannday/go-kit/modules/"+module)
		}
		// Approved minimum-version adjustments based on each module's language
		// and standard-library requirements. No dependency text is changed.
		if entry.File == "go.mod" {
			minimum := map[string]string{"logx": "1.21", "envx": "1.18", "confx": "1.23", "buildx": "1.18"}[entry.Module]
			expected = strings.Replace(expected, "go 1.26", "go "+minimum, 1)
		}
		if entry.Module == "buildx" && (entry.File == "build/args.go" || entry.File == "build/help.go" || entry.File == "build/help_test.go") {
			expected = strings.ReplaceAll(expected, "go-build-bin", "buildx")
		}
		// Approved pre-release test-only fix: match the existing %q error prefix
		// including its delimiter on every platform. Production errors are unchanged.
		if entry.Module == "confx" && entry.File == "config_test.go" {
			expected = strings.Replace(expected, "\"path/filepath\"\n", "\"path/filepath\"\n\t\"strconv\"\n", 1)
			expected = strings.Replace(expected, "strings.Contains(err.Error(), `read \"`+path+`\"`)", "strings.HasPrefix(err.Error(), \"read \"+strconv.Quote(path)+\": \")", 1)
			expected = strings.Replace(expected, "strings.Contains(err.Error(), `decode \"`+path+`\"`)", "strings.HasPrefix(err.Error(), \"decode \"+strconv.Quote(path)+\": \")", 1)
		}
		// Keep logx's no-output test logger available at its declared Go 1.21
		// minimum; slog.DiscardHandler requires Go 1.24.
		if entry.Module == "logx" && entry.File == "httpx/logging_test.go" {
			expected = strings.ReplaceAll(expected, "slog.New(slog.DiscardHandler)", "slog.New(slog.NewTextHandler(io.Discard, nil))")
		}
		actual := normalize(copied)
		if expected != actual {
			return fmt.Errorf("%s: source differs beyond approved imports/CLI branding/module Go minimum/line endings/confx and logx test compatibility expectations", dstPath)
		}
		if !isGo {
			modulesChecked[entry.Module] = true
			continue
		}
		if goFiles[entry.Module] == nil {
			goFiles[entry.Module] = make(map[string]bool)
		}
		if entry.File != entry.Destination || goFiles[entry.Module][entry.File] {
			return fmt.Errorf("%s: duplicate or renamed Go source", dstPath)
		}
		goFiles[entry.Module][entry.File] = true
		if strings.HasSuffix(entry.File, "_test.go") {
			continue
		}
		oldAPI, n, err := exportedAPI(expected)
		if err != nil {
			return fmt.Errorf("%s: %w", srcPath, err)
		}
		newAPI, _, err := exportedAPI(actual)
		if err != nil {
			return fmt.Errorf("%s: %w", dstPath, err)
		}
		if oldAPI != newAPI {
			return fmt.Errorf("%s: exported API differs", dstPath)
		}
		total := counts[entry.Module]
		total.Functions += n.Functions
		total.Methods += n.Methods
		total.Types += n.Types
		total.Fields += n.Fields
		total.Constants += n.Constants
		total.Variables += n.Variables
		total.Errors += n.Errors
		counts[entry.Module] = total
	}
	modules := []string{"logx", "envx", "confx", "buildx"}
	for _, module := range modules {
		if !modulesChecked[module] || len(goFiles[module]) == 0 {
			return fmt.Errorf("%s: inventory lacks go.mod or Go source", module)
		}
		for source, name := range sourceModules {
			if name == module {
				if err := checkGoFiles(filepath.Join(sources, source), goFiles[module]); err != nil {
					return err
				}
			}
		}
		if err := checkGoFiles(filepath.Join("modules", module), goFiles[module]); err != nil {
			return err
		}
		n := counts[module]
		fmt.Printf("%s: %d Go files; API unchanged: %d functions, %d methods, %d types, %d fields, %d constants, %d variables, %d named error types\n",
			module, len(goFiles[module]), n.Functions, n.Methods, n.Types, n.Fields, n.Constants, n.Variables, n.Errors)
	}
	fmt.Printf("Verified %d source hashes and destination files (including shared repository files); module paths/dependencies, approved Go minimums, and normalized Go implementations preserved.\n", len(entries))
	return nil
}

func normalize(data []byte) string {
	return strings.TrimRight(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
}

func safePath(path string) error {
	if path == "" || filepath.IsAbs(path) || filepath.VolumeName(path) != "" || strings.Contains(path, "\\") {
		return fmt.Errorf("unsafe inventory path: %q", path)
	}
	for _, part := range strings.Split(path, "/") {
		lower := strings.ToLower(part)
		if part == "" || part == "." || part == ".." || lower == ".env" || strings.HasPrefix(lower, ".env.") || strings.HasSuffix(lower, ".pem") || lower == ".git" {
			return fmt.Errorf("unsafe or protected inventory path: %q", path)
		}
	}
	return nil
}

func checkGoFiles(root string, expected map[string]bool) error {
	seen := make(map[string]bool)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == ".serena" || entry.Name() == ".codex" || entry.Name() == ".grok" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !expected[rel] {
			return fmt.Errorf("%s: Go source absent from inventory", path)
		}
		seen[rel] = true
		return nil
	})
	if err != nil {
		return err
	}
	for name := range expected {
		if !seen[name] {
			return fmt.Errorf("%s: missing Go source %s", root, name)
		}
	}
	return nil
}

func exportedAPI(source string) (string, apiCounts, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "api.go", source, 0)
	if err != nil {
		return "", apiCounts{}, err
	}
	ast.FileExports(file)
	var counts apiCounts
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			d.Body = nil
			if d.Recv == nil {
				counts.Functions++
			} else {
				counts.Methods++
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					counts.Types++
					if strings.HasSuffix(s.Name.Name, "Error") {
						counts.Errors++
					}
					ast.Inspect(s.Type, func(node ast.Node) bool {
						if st, ok := node.(*ast.StructType); ok {
							for _, field := range st.Fields.List {
								counts.Fields += len(field.Names)
								if len(field.Names) == 0 {
									counts.Fields++
								}
							}
						}
						return true
					})
				case *ast.ValueSpec:
					if d.Tok == token.CONST {
						counts.Constants += len(s.Names)
					} else if d.Tok == token.VAR {
						counts.Variables += len(s.Names)
					}
				}
			}
		}
	}
	var out bytes.Buffer
	if err := format.Node(&out, fset, file); err != nil {
		return "", counts, err
	}
	return out.String(), counts, nil
}
