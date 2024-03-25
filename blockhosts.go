package main

import (
	"bufio"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	mapset "github.com/deckarep/golang-set/v2"
)

type Config struct {
	Template string
	Lists    []string
}

type TemplateData struct {
	Host string
}

type hostResult struct {
	err  error
	host string
}

type writeResult struct {
	err error
}

func readUrl(url string, hosts chan<- hostResult, done chan<- struct{}) {
	resp, err := http.Get(url)
	if err != nil {
		hosts <- hostResult{err: err}
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "0.0.0.0") {
			parts := strings.Fields(line)
			hosts <- hostResult{host: parts[1]}
		}
	}
	if err := scanner.Err(); err != nil {
		hosts <- hostResult{err: err}
	}
	done <- struct{}{}
}

func writeHosts(hostTemplate string, hosts <-chan hostResult, done chan<- writeResult) {
	tmpl, err := template.New("block line").Parse(hostTemplate + "\n")
	if err != nil {
		done <- writeResult{err: fmt.Errorf("error while parsing template: %s", err)}
	}

	uniqueHosts := mapset.NewSet[string]()

	for item := range hosts {
		if item.err != nil {
			done <- writeResult{err: fmt.Errorf("error while gathering: %s", item.err)}
		}

		if uniqueHosts.Add(item.host) {
			err = tmpl.Execute(os.Stdout, TemplateData{Host: item.host})
			if err != nil {
				done <- writeResult{err: fmt.Errorf("error while executing template: %s", err)}
			}
		}
	}
	done <- writeResult{err: nil}
}

func main() {
	if len(os.Args) <= 1 {
		log.Fatalln("no config file given on command line")
	}

	hosts := make(chan hostResult)
	doneGathering := make(chan struct{})
	doneWriting := make(chan writeResult)

	configFile := os.Args[1]
	var conf Config
	_, err := toml.DecodeFile(configFile, &conf)
	if err != nil {
		log.Fatalf("error while parsing configuration: %s", err)
	}

	go writeHosts(conf.Template, hosts, doneWriting)
	go readUrl(conf.Lists[0], hosts, doneGathering)

	<-doneGathering
	close(hosts)
	result := <-doneWriting
	if result.err != nil {
		log.Fatalf("error while writing: %s", result.err)
	}
}
