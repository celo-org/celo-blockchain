// +build android

package snark

/*
#cgo LDFLAGS: -L../../target/x86_64-linux-android/release -L../../target/i686-linux-android/release -L../../target/armv7-linux-androideabi/release -L../../target/aarch64-linux-android/release -lepoch_snark -ldl -lm
*/
import "C"

