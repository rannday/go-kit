// Local integration uses replacements only in a disposable external module.
// Tagged mode verifies one published module without replacements.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	root := flag.String("root", ".", "repository root")
	module := flag.String("module", "", "published module to verify")
	version := flag.String("version", "", "published version, without directory prefix")
	flag.Parse()
	if err := run(*root, *module, *version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root, module, version string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if (module == "") != (version == "") {
		return fmt.Errorf("module and version required together")
	}
	if module != "" && module != "logx" && module != "envx" && module != "confx" && module != "buildx" {
		return fmt.Errorf("invalid module")
	}
	tmp, err := os.MkdirTemp("", "go-kit-consumer-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	bin := filepath.Join(tmp, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		return err
	}
	environment := []string{}
	for _, value := range os.Environ() {
		key := strings.ToUpper(strings.SplitN(value, "=", 2)[0])
		if key != "GOWORK" && key != "GOBIN" && key != "GOMODCACHE" {
			environment = append(environment, value)
		}
	}
	environment = append(environment, "GOWORK=off", "GOBIN="+bin, "GOMODCACHE="+filepath.Join(tmp, ".modcache"))
	command := func(dir string, args ...string) error {
		fmt.Println("CHECK external:", strings.Join(args, " "))
		c := exec.Command("go", args...)
		c.Dir = dir
		c.Env = environment
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	}
	write := func(name, content string) error { return os.WriteFile(filepath.Join(tmp, name), []byte(content), 0644) }
	modules := []string{"logx", "envx", "confx"}
	if module != "" {
		modules = []string{module}
	}
	mod := "module example.com/go-kit-consumer\n\ngo 1.26\n"
	source := "package main\nimport (\n"
	for _, name := range modules {
		path := "github.com/rannday/go-kit/modules/" + name
		v := version
		if v == "" {
			v = "v0.0.0"
		}
		if name != "buildx" {
			mod += "require " + path + " " + v + "\n"
			source += "_ " + strconv.Quote(path) + "\n"
			if name == "logx" {
				source += "_ " + strconv.Quote(path+"/httpx") + "\n"
			}
			if module == "" {
				mod += "replace " + path + " => " + strconv.Quote(filepath.ToSlash(filepath.Join(root, "modules", name))) + "\n"
			}
		}
	}
	source += ")\n"
	if err := write("go.mod", mod); err != nil {
		return err
	}
	if module != "buildx" {
		if err := write("consumer_test.go", source); err != nil {
			return err
		}
		if err := command(tmp, "mod", "tidy"); err != nil {
			return err
		}
		if err := command(tmp, "test", "-mod=readonly", "./..."); err != nil {
			_ = command(tmp, "mod", "tidy", "-diff")
			return err
		}
		for _, name := range modules {
			if module != "" {
				if err := command(tmp, "mod", "download", "github.com/rannday/go-kit/modules/"+name+"@"+version); err != nil {
					return err
				}
			}
		}
	}
	if module == "" || module == "buildx" {
		if module == "" {
			err = command(filepath.Join(root, "modules", "buildx"), "install", ".")
		} else {
			err = command(tmp, "install", "github.com/rannday/go-kit/modules/buildx@"+version)
		}
		if err != nil {
			return err
		}
		binary := filepath.Join(bin, "buildx")
		if os.PathSeparator == '\\' {
			binary += ".exe"
		}
		c := exec.Command(binary, "--help")
		c.Dir = tmp
		c.Env = environment
		help, err := c.CombinedOutput()
		if err != nil {
			return fmt.Errorf("help: %w: %s", err, help)
		}
		if !strings.Contains(string(help), "buildx [options]") || strings.Contains(string(help), "go-build-bin") {
			return fmt.Errorf("wrong executable branding")
		}
		if module == "" {
			if err := write("main.go", "package main\nfunc main() {}\n"); err != nil {
				return err
			}
			c = exec.Command(binary, "-v", "0.1.0", "--name", "buildx", "--main", ".", "--out", filepath.Join(tmp, "dist"))
			c.Dir = tmp
			c.Env = environment
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return err
			}
			if err := command(root, "run", filepath.Join(root, "scripts", "verify-artifacts.go"), filepath.Join(tmp, "dist"), "0.1.0"); err != nil {
				return err
			}
		}
	}
	fmt.Println("External consumer verified; temporary module, replacements, cache, and GOBIN removed.")
	return nil
}
