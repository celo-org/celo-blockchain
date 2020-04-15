package main

import (
	"fmt"
	"log"
	"context"
	"github.com/ethereum/go-ethereum/ethclient"
)
func main() {
	client, err := ethclient.Dial("http://localhost:8545")
	if err != nil {
		log.Fatal(err)
	}

	gf, err := client.GetGatewayFee(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(gf)
}