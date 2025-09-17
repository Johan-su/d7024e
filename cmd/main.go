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
)

func main() {
	fmt.Println("Running Kademlia app...")
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("%v\n", err)
	}
	addr, err := net.ResolveUDPAddr("udp", hostname+":8000")
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	node := kademlia.NewKademlia(addr.String(), kademlia.NewUDPNode())
	go node.HandleResponse()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		if !scanner.Scan() {
			err := scanner.Err()
			if err != nil {
				log.Fatalf("%v\n", err)
			}
		}
		s := scanner.Text()
		s = strings.TrimSuffix(s, "\n")
		s = strings.TrimSuffix(s, "\r")

		strs := strings.Split(s, " ")

		if strs[0] == "exit" {
			break
		} else if strs[0] == "put" {
			hash, err := node.Store([]byte(strs[1]))
			if err != nil {
				fmt.Printf("Failed to store because of %v\n", err)
			}
			fmt.Printf("data hash: `%s`\n", hash)
		} else if strs[0] == "get" {
			dat, id, exists := node.LookupData(strs[1])
			if exists {
				fmt.Printf("data: %s\n", dat)
				fmt.Printf("id: %s\n", id.String())
			} else {
				fmt.Printf("data not found\n")
			}
		} else {
			fmt.Printf("Invalid command `%s`\n", strs[0])
		}

	}
}
