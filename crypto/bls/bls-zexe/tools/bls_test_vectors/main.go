package main

import (
  "fmt"
  "golang.org/x/crypto/blake2s"
  "math/rand"
  "encoding/hex"
)

func main() {
  rand.Seed(1000)

  hashLength := 96
  xof, _ := blake2s.NewXOF(uint16(hashLength), nil)
  msg := make([]byte, 140)
  rand.Read(msg)
  fmt.Printf("msg: %s\n", hex.EncodeToString(msg))

  xof.Write(msg)

  hash := make([]byte, hashLength)
  xof.Read(hash)
  fmt.Printf("hash: %s\n", hex.EncodeToString(hash))
}
