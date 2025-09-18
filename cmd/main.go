// TODO: Add package documentation for `main`, like this:
// Package main something something...
package main

import (
	"bufio"
	"d7024e/internal/kademlia"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
	"flag"
)

func main() {
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("%v\n", err)
	}
	addr, err := net.ResolveUDPAddr("udp", hostname+":8000")
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	bootIP := flag.String("bip", "127.0.0.1", "Boot Node IP")
	bootId := flag.String("bid", "0000000000000000000000000000000000000000", "Boot Node ID")

	flag.Parse()

	var isBootNode bool

	if addr.IP.String() == *bootIP {
		isBootNode = true
		fmt.Printf("Is boot node\n")
	}

	var id *kademlia.KademliaID

	if isBootNode {
		id = kademlia.NewKademliaID(*bootId)
	} else {
		id =  kademlia.NewRandomKademliaID()
	}

	fmt.Printf("Kademlia Node Address %v ID %s\n", addr.String(), id.String())

	node := kademlia.NewKademlia(addr.String(), id, kademlia.NewUDPNode())
	go node.HandleResponse()


	if !isBootNode {
		fmt.Printf("Sleeping...\n")
		time.Sleep(15 * time.Second)
		bootAddress := *bootIP+":8000"
		fmt.Printf("Joining Network at %v %v\n", bootAddress, *bootId)
		node.Join(kademlia.NewContact(kademlia.NewKademliaID(*bootId), bootAddress))
		// fmt.Printf("after join\n")
	}



	// fmt.Printf("begin scan\n")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		if !scanner.Scan() {
			err := scanner.Err()
			if err != nil {
				log.Fatalf("%v\n", err)
			}
		}
		// fmt.Printf("after scan\n")
		s := scanner.Text()
		s = strings.TrimSuffix(s, "\n")
		s = strings.TrimSuffix(s, "\r")

		strs := strings.Split(s, " ")

		if strs[0] == "exit" {
			break
		} else if strs[0] == "put" {
			go func(dat []byte) {
				hash, err := node.Store(dat)
				if err != nil {
					fmt.Printf("Failed to store because of %v\n", err)
				} else {
					fmt.Printf("data hash: `%s`\n", hash.String())
				}
			}([]byte(strs[1]))
		} else if strs[0] == "get" {
			go func(hash string) {
				dat, exists, contacts := node.LookupData(hash)
				if exists {
					fmt.Printf("data: %s\n", dat)
					fmt.Printf("id: %s\n", contacts[0].ID.String())
				} else {
					fmt.Printf("data not found\n")
				}
			}(strs[1])
		} else {
			fmt.Printf("Invalid command `%s`\n", strs[0])
		}
	}
}
