// +build darwin,386

package snark

/*
#cgo LDFLAGS: -L../../target/i686-apple-darwin/release -L../../target/release -lepoch_snark -ldl -lm
*/
import "C"
