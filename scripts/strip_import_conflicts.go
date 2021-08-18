package main

import (
        "bufio"
        "fmt"
        "os"
        "regexp"
        "strings"
)

func main() {
        re := regexp.MustCompile(`github.com/celo-org/celo-blockchain`)

        if len(os.Args) != 2 {
                fmt.Fprintln(os.Stderr, "requires 1 argument: the filename to operate upon")
                os.Exit(1)
        }
        f, err := os.Open(os.Args[1])
        if err != nil {
                fmt.Fprintln(os.Stderr, "failed to open file:", err)
        }
        scanner := bufio.NewScanner(f)
        var lines []string
        for scanner.Scan() {
                lines = append(lines, scanner.Text())
        }
        f.Close()
        if err := scanner.Err(); err != nil {
                fmt.Fprintln(os.Stderr, "reading file:", err)
        }

        f, err = os.Create(os.Args[1])
        defer f.Close()
        if err != nil {
                fmt.Fprintln(os.Stderr, "failed to open file for writing:", err)
        }

        first, last := import_delims(lines)
        for i, line := range lines {
                if i > first && i < last {
                        if !(strings.HasPrefix(line, "<<<<<<<") || strings.HasPrefix(line, "=======") || strings.HasPrefix(line, ">>>>>>>")) {
                                fmt.Fprintln(f, re.ReplaceAllString(line, "github.com/celo-org/celo-blockchain"))

                        }
                } else {
                        fmt.Fprintln(f, line)

                }
        }

}

func import_delims(lines []string) (first, last int) {
        for i, line := range lines {
                if line == "import (" {
                        first = i
                } else if strings.HasPrefix(line, "import") {
                        first = i
                        last = i
                        return
                }
                if line == ")" && first != 0 {
                        last = i
                        return
                }
        }
        return
}
