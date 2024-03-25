package main

import (
	"bufio"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	mapset "github.com/deckarep/golang-set/v2"
)

var outfileFlag string

func init() {
	const (
		outputDefault = ""
		outputUsage   = "output file"
	)
	flag.StringVar(&outfileFlag, "outfile", outputDefault, outputUsage)
	flag.StringVar(&outfileFlag, "o", outputDefault, outputUsage+" (shorthand)")
}

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

func readUrl(url string, hosts chan<- hostResult) {
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
}

func writeHosts(outfile string, hostTemplate string, hosts <-chan hostResult) error {
	tmpl, err := template.New("block line").Parse(hostTemplate + "\n")
	if err != nil {
		return fmt.Errorf("error while parsing template: %s", err)
	}

	uniqueHosts := mapset.NewSet[string]()

	var writer *bufio.Writer
	if outfile != "" {
		file, err := os.OpenFile(outfile, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return fmt.Errorf("error while opening file '%s': %s", outfile, err)
		}
		defer file.Close()
		writer = bufio.NewWriter(file)
	} else {
		writer = bufio.NewWriter(os.Stdout)
	}

	for item := range hosts {
		if item.err != nil {
			return fmt.Errorf("error while gathering: %s", item.err)
		}

		if uniqueHosts.Add(item.host) {
			err = tmpl.Execute(writer, TemplateData{Host: item.host})
			if err != nil {
				return fmt.Errorf("error while executing template: %s", err)
			}
		}
	}
	writer.Flush()
	return nil
}

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		log.Fatalln("no config file given on command line")
	}

	hosts := make(chan hostResult)

	configFile := flag.Arg(0)
	var conf Config
	_, err := toml.DecodeFile(configFile, &conf)
	if err != nil {
		log.Fatalf("error while parsing configuration: %s", err)
	}

	var wg sync.WaitGroup
	for _, list := range conf.Lists {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			readUrl(url, hosts)
		}(list)
	}

	go func() {
		wg.Wait()
		close(hosts)
	}()

	err = writeHosts(outfileFlag, conf.Template, hosts)
	if err != nil {
		log.Fatalf("error while writing: %s", err)
	}
}
