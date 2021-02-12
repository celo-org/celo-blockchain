package console

import (
	"fmt"
	"os"

	. "github.com/logrusorgru/aurora"
)

// Error logs en error
func Error(a interface{}) {
	fmt.Println(Bold(Red(a)))
}

// Errorf logs an error (printf format)
func Errorf(format string, a ...interface{}) {
	Error(fmt.Sprintf(format, a...))
}

// Fatalf logs an error, then exits (printf format)
func Fatalf(format string, a ...interface{}) {
	Errorf(format, a...)
	os.Exit(1)
}

// Fatal logs an error, then exits
func Fatal(a interface{}) {
	Error(a)
	os.Exit(1)
}

// Info render informational message
func Info(a interface{}) {
	fmt.Println(Blue(a))
}

// Infof render informational message (printf version)
func Infof(format string, a ...interface{}) {
	Info(fmt.Sprintf(format, a...))
}
