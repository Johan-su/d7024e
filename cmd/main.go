// TODO: Add package documentation for `main`, like this:
// Package main something something...
package main

import (
	"bufio"
	"d7024e/internal/kademlia"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type objectHandler struct {
	node *kademlia.Kademlia
}

func (oh *objectHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	scanner := bufio.NewScanner(request.Body)
	switch request.Method {
		case "POST": {
			if !scanner.Scan() {
				err := scanner.Err()
				if err != nil {
					log.Fatalf("%v\n", err)
				}
			}
			dat := scanner.Bytes()
			hash, err := oh.node.Store(dat)
			if err != nil {
				log.Printf("Failed to store because of %v\n", err)
			} else {
				writer.Header().Add("Location", fmt.Sprintf("/objects/%v", hash.String()))
				writer.WriteHeader(201)
			}
		}
		case "GET": {
			strs := strings.Split(request.URL.Path, "/")
			if len(strs) == 3 {
				hash := strs[2]
				dat, exists, _  := oh.node.LookupData(hash)
				if exists {
					writer.WriteHeader(200)
					writer.Write(dat)
				} else {
					writer.WriteHeader(404)
				}
			} else {
				writer.WriteHeader(404)
			}
		}
	}
}

func HttpApi(node *kademlia.Kademlia) {

	oh := new(objectHandler)
	oh.node = node
	http.Handle("/objects", oh)
	http.Handle("/objects/", oh)

	err := http.ListenAndServe("0.0.0.0:80", nil)
	if err != nil {
		log.Fatalf("%v\n", err)
	}
}

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

	node := kademlia.NewKademlia(addr.String(), id, kademlia.NewUDPNode(), 15 * time.Second, 10 * time.Second)
	node := kademlia.NewKademlia(addr.String(), id, kademlia.NewUDPNode(), 15*time.Second, 10*time.Second, 10)
	node.Listen()
	go node.HandleResponse()


	if !isBootNode {
		fmt.Printf("Sleeping...\n")
		time.Sleep(30 * time.Second)
		bootAddress := *bootIP+":8000"
		fmt.Printf("Joining Network at %v %v\n", bootAddress, *bootId)
		node.Join(kademlia.NewContact(kademlia.NewKademliaID(*bootId), bootAddress))
		// fmt.Printf("after join\n")
	}



	go HttpApi(&node)
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
			node.Net.Close()
			break
		} else if strs[0] == "put" {
			dat := []byte(strs[1])
			hash, err := node.Store(dat)
			if err != nil {
				fmt.Printf("Failed to store because of %v\n", err)
			} else {
				fmt.Printf("data hash: `%s`\n", hash.String())
			}
		} else if strs[0] == "get" {
			hash := strs[1]
			if len(hash) == 40 {
				dat, exists, contacts := node.LookupData(hash)
				if exists {
					fmt.Printf("data: %s\n", dat)
					fmt.Printf("id: %s\n", contacts[0].ID.String())
				} else {
					fmt.Printf("data not found\n")
				}
			} else {
				fmt.Printf("Hash has to be 20 bytes (40 hex characters) long\n")
			}
		} else if strs[0] == "forget" {
			hash := strs[1]
			if len(hash) == 40 {
				err := node.Forget(hash)
				if err != nil {
					fmt.Printf("%v\n", err)
				} else {
					fmt.Printf("Forgot %s\n", hash)
				}
			} else {
				fmt.Printf("Hash has to be 20 bytes (40 hex characters) long\n")
			}
		} else {
			fmt.Printf("Invalid command `%s`\n", strs[0])
		}
	}
}
