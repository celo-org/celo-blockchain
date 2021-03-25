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
	log = cli.StringFlag{
		Name:  "log",
		Usage: "Path to the log",
	}
	filter = cli.StringFlag{
		Name:  "filter",
		Usage: "filter log lines to those containing this string",
	}
	app = &cli.App{
		Name:                 filepath.Base(os.Args[0]),
		Usage:                "loganalyze",
		Writer:               os.Stdout,
		HideVersion:          true,
		EnableBashCompletion: true,
		Flags:                []cli.Flag{log, filter},
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
	scanner := bufio.NewScanner(f)
	filterString := c.String(filter.Name)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, filterString) {
			fields := strings.Fields(line)
			// second field is the time
			// strings.Split(fields[1], "|")
			//	Mon Jan 2 15:04:05 -0700 MST 2006
			// 03-25|14:07:31.066
			t, err := time.Parse("[01-02|15:04:05.000]", fields[1])
			if err != nil {
				return err
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
