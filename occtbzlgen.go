package main

import (
	"bitbucket.org/creachadair/stringset"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

var (
	srcdir = flag.String("srcdir", "", "Path to 'src' subdir of occt")
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

	fmt.Printf("LOOKING AT DIR: %s\n", dir)
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

	// slices serialize better than stringsets
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
	flag.Parse()
	if *srcdir == "" {
		log.Fatalf("no --srcdir set")
	}
	fmt.Printf("using srcdir: %q\n", *srcdir)

	if err := genGraph("graph.json"); err != nil {
		log.Fatal(err)
	}
}
