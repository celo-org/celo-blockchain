package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/urfave/cli.v1"
)

var (

	//	Mon Jan 2 15:04:05 -0700 MST 2006 - the go format ref time
	// 03-25|14:07:31.066 - an example time from the logs
	timeFormat = "[01-02|15:04:05.000]"

	log = cli.StringFlag{
		Name:  "log",
		Usage: "Path to the log",
	}
	filter = cli.StringFlag{
		Name:  "filter",
		Usage: "filter log lines to those containing this string",
	}
	timeRange = cli.StringFlag{
		Name: "range",
		Usage: `
		filter log lines to those with timestamps within this range (inclusive).
		Timestamps have the followng format 03-25|14:07:31.066 and the range flag
		expects 2 timestamps separated by a comma and no spaces.`,
	}
	app = &cli.App{
		Name:                 filepath.Base(os.Args[0]),
		Usage:                "loganalyze",
		Writer:               os.Stdout,
		HideVersion:          true,
		EnableBashCompletion: true,
		Flags:                []cli.Flag{log, filter, timeRange},
		Action:               analyze,
	}
)

func main() {
	err := app.Run(os.Args)
	if err == nil {
		os.Exit(0)
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func analyze(c *cli.Context) error {
	f, err := os.Open(c.String(log.Name))
	if err != nil {
		return err
	}
	defer f.Close()
	var prev time.Time
	var durations []time.Duration
	var totalDuration time.Duration

	var start, end time.Time
	rangeString := c.String(timeRange.Name)
	if rangeString != "" {
		fields := strings.Split(c.String(timeRange.Name), ",")
		start, err = time.Parse(timeFormat, fields[0])
		if err != nil {
			return err
		}
		end, err = time.Parse(timeFormat, fields[1])
		if err != nil {
			return err
		}
	}

	scanner := bufio.NewScanner(f)
	filterString := c.String(filter.Name)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, filterString) {
			fields := strings.Fields(line)
			// second field is the time
			t, err := time.Parse(timeFormat, fields[1])
			if err != nil {
				return err
			}
			// Check bounds
			if rangeString != "" {
				if t.Sub(end) > 0 {
					break
				}
				if t.Sub(start) < 0 {
					continue
				}
			}
			if prev.IsZero() {
				prev = t
				continue
			}
			d := t.Sub(prev)
			totalDuration += d
			durations = append(durations, d)
			prev = t
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	// Process durations
	avgDur := totalDuration / time.Duration(len(durations))

	fmt.Printf("Periods analysed: %v\n", len(durations))
	fmt.Printf("Total duration: %v\n", totalDuration)
	fmt.Printf("Avg duration: %v\n", avgDur)
	return nil
}
