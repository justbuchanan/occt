package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
)

var (
	graphFile = flag.String("graph", "graph.json", "path to graph json file")
)

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

	fmt.Println("digraph G {")
	for k, v := range graph {
		for _, s := range v {
			fmt.Printf("  %s -> %s;\n", strings.TrimSuffix(k, "/"), strings.TrimSuffix(s, "/"))
		}
	}
	fmt.Println("}")
}
