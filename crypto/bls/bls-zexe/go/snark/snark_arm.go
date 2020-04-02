// +build linux,arm,!arm7

package snark

/*
#cgo LDFLAGS: -L../../target/arm-unknown-linux-gnueabi/release -lepoch_snark -ldl -lm
*/
import "C"
