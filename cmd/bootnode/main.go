// Copyright 2015 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

// bootnode runs a bootstrap node for the Ethereum Discovery Protocol.
package main

import (
	"crypto/ecdsa"
	"flag"
	"fmt"
	"net"
	"os"

	"github.com/celo-org/celo-blockchain/cmd/utils"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/discover"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/p2p/nat"
	"github.com/celo-org/celo-blockchain/p2p/netutil"
)

func main() {
	var (
		listenAddr       = flag.String("addr", ":30301", "listen address")
		genKey           = flag.String("genkey", "", "generate a node key")
		writeAddr        = flag.Bool("writeaddress", false, "write out the node's public key and quit")
		nodeKeyFile      = flag.String("nodekey", "", "private key filename")
		nodeKeyHex       = flag.String("nodekeyhex", "", "private key as hex (for testing)")
		natdesc          = flag.String("nat", "none", "port mapping mechanism (any|none|upnp|pmp|extip:<IP>)")
		netrestrict      = flag.String("netrestrict", "", "restrict network communication to the given IP networks (CIDR masks)")
		runv4            = flag.Bool("v4", true, "run a v4 topic discovery bootnode")
		runv5            = flag.Bool("v5", false, "run a v5 topic discovery bootnode")
		verbosity        = flag.Int("verbosity", int(log.LvlInfo), "log verbosity (0-5)")
		vmodule          = flag.String("vmodule", "", "log verbosity pattern")
		networkId        = flag.Uint64("networkid", 0, "network ID")
		pingIPFromPacket = flag.Bool("ping-ip-from-packet", false, "Has the discovery protocol use the IP address given by a ping packet")

		nodeKey *ecdsa.PrivateKey
		err     error
	)
	flag.Parse()

	glogger := log.NewGlogHandler(log.StreamHandler(os.Stdout, log.JSONFormat()))
	glogger.Verbosity(log.Lvl(*verbosity))
	glogger.Vmodule(*vmodule)
	log.Root().SetHandler(glogger)

	natm, err := nat.Parse(*natdesc)
	if err != nil {
		utils.Fatalf("-nat: %v", err)
	}
	switch {
	case !*runv4 && !*runv5:
		utils.Fatalf("Must specify at least one discovery protocol (v4 or v5)")
	case *genKey != "":
		nodeKey, err = crypto.GenerateKey()
		if err != nil {
			utils.Fatalf("could not generate key: %v", err)
		}
		if err = crypto.SaveECDSA(*genKey, nodeKey); err != nil {
			utils.Fatalf("%v", err)
		}
		if !*writeAddr {
			return
		}
	case *nodeKeyFile == "" && *nodeKeyHex == "":
		utils.Fatalf("Use -nodekey or -nodekeyhex to specify a private key")
	case *nodeKeyFile != "" && *nodeKeyHex != "":
		utils.Fatalf("Options -nodekey and -nodekeyhex are mutually exclusive")
	case *nodeKeyFile != "":
		if nodeKey, err = crypto.LoadECDSA(*nodeKeyFile); err != nil {
			utils.Fatalf("-nodekey: %v", err)
		}
	case *nodeKeyHex != "":
		if nodeKey, err = crypto.HexToECDSA(*nodeKeyHex); err != nil {
			utils.Fatalf("-nodekeyhex: %v", err)
		}
	case *networkId == 0:
		utils.Fatalf("--networkid must be set to non zero number")
	case *runv5:
		utils.Fatalf("Celo networks currently do not support discovery v5")
	}

	if *writeAddr {
		fmt.Printf("%x\n", crypto.FromECDSAPub(&nodeKey.PublicKey)[1:])
		os.Exit(0)
	}

	var restrictList *netutil.Netlist
	if *netrestrict != "" {
		restrictList, err = netutil.ParseNetlist(*netrestrict)
		if err != nil {
			utils.Fatalf("-netrestrict: %v", err)
		}
	}

	addr, err := net.ResolveUDPAddr("udp", *listenAddr)
	if err != nil {
		utils.Fatalf("-ResolveUDPAddr: %v", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		utils.Fatalf("-ListenUDP: %v", err)
	}

	realaddr := conn.LocalAddr().(*net.UDPAddr)
	if natm != nil {
		if !realaddr.IP.IsLoopback() {
			go nat.Map(natm, nil, "udp", realaddr.Port, realaddr.Port, "ethereum discovery")
		}
		if ext, err := natm.ExternalIP(); err == nil {
			realaddr = &net.UDPAddr{IP: ext, Port: realaddr.Port}
		}
	}

	printNotice(&nodeKey.PublicKey, *realaddr)

	// If v4 and v5 are both enabled, a packet is first tried as v4
	// and then v5 if v4 decoding fails, following the same pattern as full
	// nodes that use v4 and v5:
	// https://github.com/celo-org/celo-blockchain/blob/7fbd6f3574f1c1c1e657c152fc63fb771adab3af/p2p/server.go#L588
	var unhandled chan discover.ReadPacket
	var sconn *p2p.SharedUDPConn
	db, _ := enode.OpenDB("")
	ln := enode.NewLocalNode(db, nodeKey, *networkId)
	cfg := discover.Config{
		PrivateKey:       nodeKey,
		NetRestrict:      restrictList,
		PingIPFromPacket: *pingIPFromPacket,
	}
	if *runv4 {
		if *runv5 {
			unhandled = make(chan discover.ReadPacket, 100)
			cfg.Unhandled = unhandled
			sconn = &p2p.SharedUDPConn{UDPConn: conn, Unhandled: unhandled}
		}
		if _, err := discover.ListenUDP(conn, ln, cfg); err != nil {
			utils.Fatalf("%v", err)
		}
	}

	if *runv5 {
		var err error
		if sconn != nil {
			_, err = discover.ListenV5(sconn, ln, cfg)
		} else {
			_, err = discover.ListenV5(conn, ln, cfg)
		}
		if err != nil {
			utils.Fatalf("%v", err)
		}
	}

	select {}
}

func printNotice(nodeKey *ecdsa.PublicKey, addr net.UDPAddr) {
	if addr.IP.IsUnspecified() {
		addr.IP = net.IP{127, 0, 0, 1}
	}
	n := enode.NewV4(nodeKey, addr.IP, 0, addr.Port)
	fmt.Println(n.URLv4())
	fmt.Println("Note: you're using cmd/bootnode, a developer tool.")
	fmt.Println("We recommend using a regular node as bootstrap node for production deployments.")
}
