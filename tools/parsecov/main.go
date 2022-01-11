package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/urfave/cli.v1"
)

var (
	packagePrefixFlagName = "packagePrefix"
)

type count struct {
	totalStatements   int
	coveredStatements int
}

func (c count) percentage() float32 {
	return 100 * float32(c.coveredStatements) / float32(c.totalStatements)
}

func parse(c *cli.Context) error {
	packageCoverage := make(map[string]count)
	profile, err := os.Open(c.Args()[0])
	if err != nil {
		return err
	}
	defer profile.Close()

	// First line is "mode: foo", where foo is "set", "count", or "atomic".
	// Rest of file is in the format
	//	encoding/base64/base64.go:34.44,37.40 3 1
	// where the fields are: name.go:line.column,line.column numberOfStatements count

	// This program only handles set mode
	s := bufio.NewScanner(profile)
	s.Scan()
	line := s.Text()
	if line != "mode: set" {
		return fmt.Errorf("invalid coverage mode, expecting 'mode: set' as the first line, but got %q", line)
	}
	linenum := 1
	for s.Scan() {
		line := s.Text()
		linenum++
		lastColon := strings.LastIndex(line, ":")
		if lastColon == -1 {
			return fmt.Errorf("line %d invalid: %q", linenum, line)
		}

		packageName := path.Dir(line[:lastColon])
		numStatements, covered, err := getStats(line[lastColon+1:])
		if err != nil {
			return fmt.Errorf("line %d invalid: %q", linenum, line)
		}
		c := packageCoverage[packageName]
		c.totalStatements += numStatements
		if covered {
			c.coveredStatements += numStatements
		}
		packageCoverage[packageName] = c
	}

	packageNames := make([]string, 0, len(packageCoverage))
	for name := range packageCoverage {
		packageNames = append(packageNames, name)
	}
	sort.Strings(packageNames)

	var totalCount count
	for _, v := range packageCoverage {
		totalCount.totalStatements += v.totalStatements
		totalCount.coveredStatements += v.coveredStatements
	}
	fmt.Printf("coverage: %5.1f%% of statements across all listed packages\n", totalCount.percentage())

	for _, name := range packageNames {
		cov := packageCoverage[name].percentage()
		fmt.Printf("coverage: %5.1f%% of statements in %s\n", cov, strings.TrimPrefix(name, c.String(packagePrefixFlagName)))
	}

	return nil
}

// Gets the stats from the end of the line, how many statements and were they
// covered?
//
// E.G line -> encoding/base64/base64.go:34.44,37.40 3 1
// End of line -> 34.44,37.40 3 1
// Would return 3 true
func getStats(lineEnd string) (numStatements int, covered bool, err error) {
	parts := strings.Split(lineEnd, " ")
	if len(parts) != 3 {
		return 0, false, fmt.Errorf("invalid line end")
	}
	numStatements, err = strconv.Atoi(parts[1])
	return numStatements, parts[2] == "1", err
}

func main() {
	app := cli.NewApp()
	app.Name = "parsecov"
	app.Usage = `parses coverage files outputting a per package breakdown
   of coverage as well as a total for all the packages.`
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name: packagePrefixFlagName,
			Usage: `a common prefix that is stripped from the front 
     of all packages in order to make output more concise.`,
		},
	}
	app.Action = parse

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("Failed to parse coverage: %v", err)
	}
}
