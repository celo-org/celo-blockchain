// +build linux,mipsle

package snark

/*
#cgo LDFLAGS: -L../../target/mipsel-unknown-linux-gnu/release -lepoch_snark -ldl -lm
*/
import "C"
