package main

import (
	"encoding/json"
	"strings"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"

	"bitbucket.org/creachadair/stringset"
	"github.com/looplab/tarjan"
)

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

	// fmt.Println("graph stuff")

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
		// nm := fmt.Sprintf("lib%d", i)
		nm := strings.TrimSuffix(cc[0].(string), "/")
		l := &libSpec{
			name: nm,
			// pkgs: cc,
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
				// pkg.pkgDeps = append(pkg.pkgDeps, deppkg.name)
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
