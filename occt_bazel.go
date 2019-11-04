package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"bitbucket.org/creachadair/stringset"
	"github.com/google/subcommands"
	"github.com/looplab/tarjan"
)

var cacheMu sync.RWMutex
var _cache = make(map[string]string)

func subdirFromInclude_mem(filename string) (string, error) {
	cacheMu.RLock()
	e := _cache[filename]
	cacheMu.RUnlock()

	if e != "" {
		return e, nil
	}

	var err error
	e, err = subdirFromInclude(filename)
	if err != nil {
		return "", err
	}

	cacheMu.Lock()
	_cache[filename] = e
	cacheMu.Unlock()

	return e, nil
}

func subdirFromInclude(filename string) (string, error) {
	glob := filepath.Join(*srcdir, "*", filename)
	m, err := filepath.Glob(glob)
	if err != nil {
		return "", err
	}

	if len(m) != 1 {
		if len(m) == 0 {
			log.Printf("goddammit opencascade is broken again; referencing a non-existent file. file=%q; skipping glob=%q; matches=%v\n", filename, glob, m)
			return "", nil
		} else {
			return "", fmt.Errorf("F")
		}
	}

	r, err := filepath.Rel(*srcdir, m[0])
	if err != nil {
		return "", err
	}
	d, _ := filepath.Split(r)

	return d, nil
}

// findDeps determines the diretories this dir depends on by scanning for
// #includes in the contained files. Example findDeps("src/dirA") ->
// ["src/dirB", "src/dirC"]
func findDeps(dir string) (stringset.Set, error) {
	var deps stringset.Set
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	inclPat := regexp.MustCompile("#include (\"|<)(?P<path>.+)(\"|>)")

	for _, fi := range files {
		fip := filepath.Join(dir, fi.Name())
		fmt.Printf("  LOOKING AT FILE: %s\n", fip)
		b, err := ioutil.ReadFile(fip)
		if err != nil {
			return nil, err
		}
		s := string(b)
		m := inclPat.FindAllStringSubmatch(s, -1)
		for _, g := range m {
			incl := g[2]
			dep, err := subdirFromInclude_mem(incl)
			if err != nil {
				return nil, err
			}
			if dep == "" {
				log.Println("skipping empty dep")
				continue
			}
			deps.Add(dep)
		}
	}

	return deps, nil
}

// genGraph saves the dependency graph to a json file in the form:
// {
//   "dirA": ["dirB", "dirC"],
//   "dirG": ["dirC"]
// }
func genGraph(jsonfile string) error {
	// get all dirs under 'src'
	var dirs []string
	err := filepath.Walk(*srcdir, func(path string, info os.FileInfo, err error) error {
		if path == *srcdir || path == "" {
			return nil
		}
		if info.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, d := range dirs {
		fmt.Println(d)
	}

	// for each pkg, record a list of which pkgs it #includes things from
	graph := make(map[string]stringset.Set)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, d := range dirs {
		if d == "" {
			continue
		}
		wg.Add(1)
		log.Printf("launching thread!\n")
		go func(d string) {
			defer wg.Done()
			deps, err := findDeps(d)
			if err != nil {
				log.Printf("yep it's broken again")
			} else {
				reld, err := filepath.Rel(*srcdir, d)
				if err != nil {
					log.Printf("can't relativize: %v\n", err)
					return
				}
				if reld == "" {
					log.Printf("ignoring empty reldir")
				}
				mu.Lock()
				graph[reld+"/"] = deps
				mu.Unlock()
			}
		}(d)
	}
	wg.Wait()

	// slices serialize to a more sensible format better than stringsets
	graph2 := make(map[string][]string)
	for key, deps := range graph {
		graph2[key] = deps.Elements()
	}

	b, err := json.MarshalIndent(graph2, "", "  ")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(jsonfile, b, 0644)
	if err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", jsonfile)
	return nil
}

func main() {
	subcommands.Register(&genDepgraphSubcommand{}, "")
	subcommands.Register(&genBazelSubcommand{}, "")

	flag.Parse()
	ctx := context.Background()
	os.Exit(int(subcommands.Execute(ctx)))
}

type genDepgraphSubcommand struct {
	srcdir string
}

func (*genDepgraphSubcommand) Name() string { return "depgraph" }
func (*genDepgraphSubcommand) Synopsis() string {
	return "Scan the OCCT --srcdir and determine dependencies based on #includes. Write the resulting dependency graph to stdout in json format."
}
func (*genDepgraphSubcommand) Usage() string {
	return "depgraph --srcdir /path/to/occt/src"
}
func (s *genDepgraphSubcommand) SetFlags(f *flag.FlagSet) {
	f.StringVar(&s.srcdir, "srcdir", "", "Path to 'src' subdirectory of OCCT project")
}

func (s *genDepgraphSubcommand) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if s.srcdir == "" {
		return subcommands.ExitFailure
	}
	fmt.Printf("using srcdir: %q\n", *srcdir)

	if err := genGraph("graph.json"); err != nil {
		log.Fatal(err)
		return subcommands.ExitSuccess // TODO
	}
	return subcommands.ExitSuccess // TODO
}

type genBazelSubcommand struct {
}

func (*genBazelSubcommand) Name() string { return "genbazel" }
func (*genBazelSubcommand) Synopsis() string {
	return "Reads the dependency graph from a json file and uses it to generate a Bazel BUILD file"
}
func (*genBazelSubcommand) Usage() string {
	// TODO
	return "genbazel --graph"
}
func (s *genBazelSubcommand) SetFlags(f *flag.FlagSet) {
	// f.StringVar(&s.srcdir, "srcdir", "", "Path to 'src' subdirectory of OCCT project")
}

func (s *genBazelSubcommand) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	// TODO
	return subcommands.ExitSuccess
}

var (
	graphFile = flag.String("graph", "graph.json", "path to graph json file")
)

type libSpec struct {
	name    string
	pkgs    []string
	pkgDeps []string
}

func main() {
	flag.Parse()

	b, err := ioutil.ReadFile(*graphFile)
	if err != nil {
		log.Fatal(err)
	}
	graph := make(map[string][]string)
	if err := json.Unmarshal(b, &graph); err != nil {
		log.Fatal(err)
	}

	graph2 := make(map[interface{}][]interface{})
	for k, v := range graph {
		for _, s := range v {
			graph2[k] = append(graph2[k], s)
		}
	}

	// get a list of strongly connected components using tarjan's algorithm
	ccs := tarjan.Connections(graph2)

	// pkg name to pkg
	pkgmap := make(map[string]*libSpec)
	for _, cc := range ccs {
		nm := strings.TrimSuffix(cc[0].(string), "/")
		l := &libSpec{
			name: nm,
		}
		for _, c := range cc {
			l.pkgs = append(l.pkgs, c.(string))
		}
		pkgmap[nm] = l
	}

	dirToPkg := make(map[string]*libSpec)
	for _, pkg := range pkgmap {
		for _, d := range pkg.pkgs {
			dirToPkg[d] = pkg
		}
	}

	for _, pkg := range pkgmap {
		pkgdeps := stringset.Set{}
		for _, d := range pkg.pkgs {
			deps := graph2[d]
			for _, dep := range deps {
				deppkg := dirToPkg[dep.(string)]
				if deppkg.name == pkg.name {
					// don't self-edge
					continue
				}
				pkgdeps.Add(deppkg.name)
			}
		}
		pkg.pkgDeps = pkgdeps.Elements()
	}

	var libs []*libSpec
	for _, l := range pkgmap {
		libs = append(libs, l)
	}

	if err := genAllBzl(libs); err != nil {
		log.Fatal(err)
	}
}

func genAllBzl(libs []*libSpec) error {
	for _, l := range libs {
		if err := genBzl(l); err != nil {
			return err
		}
	}
	return nil
}

// TODO: remove trailing "/"
var extraDeps = map[string][]string{
	"Font/": []string{"@freetype2"},
	"Draw/": []string{"@tcl"},
}

func genBzl(l *libSpec) error {
	bzl := "cc_library(\n"
	bzl += fmt.Sprintf("  name=\"%s\",\n", l.name)

	// srcs
	bzl += fmt.Sprintf("  srcs=glob([\n")
	for _, pkg := range l.pkgs {
		bzl += fmt.Sprintf("    \"%s\",\n", filepath.Join(pkg, "*.cxx"))
	}
	bzl += "  ]),\n"

	// hdrs
	bzl += fmt.Sprintf("  hdrs=glob([\n")
	for _, pkg := range l.pkgs {
		bzl += fmt.Sprintf("    \"%s\",\n", filepath.Join(pkg, "*.gxx"))
		bzl += fmt.Sprintf("    \"%s\",\n", filepath.Join(pkg, "*.pxx"))
		bzl += fmt.Sprintf("    \"%s\",\n", filepath.Join(pkg, "*.hxx"))
		bzl += fmt.Sprintf("    \"%s\",\n", filepath.Join(pkg, "*.lxx"))
		bzl += fmt.Sprintf("    \"%s\",\n", filepath.Join(pkg, "*.h"))
	}
	bzl += "  ]),\n"

	// include
	bzl += fmt.Sprintf("  includes=[\n")
	for _, pkg := range l.pkgs {
		bzl += fmt.Sprintf("    \"%s/\",\n", filepath.Join(pkg))
	}
	bzl += "  ],\n"

	// deps
	bzl += fmt.Sprintf("  deps=[\n")
	for _, dep := range l.pkgDeps {
		bzl += fmt.Sprintf("    \":%s\",\n", dep)
	}
	for _, pkg := range l.pkgs {
		// fmt.Printf("LOOKUP %s", pkg)
		edeps := extraDeps[pkg]
		for _, e := range edeps {
			bzl += fmt.Sprintf("    \"%s\",\n", e)
		}
	}
	bzl += "  ],\n"

	bzl += ")"
	bzl += "\n"

	fmt.Println(bzl)

	return nil
}
