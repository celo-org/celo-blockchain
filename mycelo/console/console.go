package console

import (
	"fmt"
	"os"

	. "github.com/logrusorgru/aurora"
)

// Error log
func Error(a interface{}) {
	fmt.Println(Bold(Red(a)))
}

func Errorf(format string, a ...interface{}) {
	Error(fmt.Sprintf(format, a...))
}

// Fatal message & then exist
func Fatalf(format string, a ...interface{}) {
	Errorf(format, a...)
	os.Exit(1)
}

// Fatal message & then exist
func Fatal(a interface{}) {
	Error(a)
	os.Exit(1)
}

func ExitOnError(err error) {
	if err != nil {
		Fatal(err)
	}
}

func Requiref(condition bool, format string, args ...interface{}) {
	if !condition {
		Fatalf(format, args...)
	}
}

func Require(condition bool, msg string) {
	if !condition {
		Fatal(msg)
	}
}

func Info(a interface{}) {
	fmt.Println(Blue(a))
}

func Infof(format string, a ...interface{}) {
	Info(fmt.Sprintf(format, a...))
}
