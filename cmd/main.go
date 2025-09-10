// TODO: Add package documentation for `main`, like this:
// Package main something something...
package main

import (
	"d7024e/internal/kademlia"
	"fmt"
)


func server(ip string, port int) {

}

func main() {
	fmt.Println("Pretending to run the kademlia app...")
	// Using stuff from the kademlia package here. Something like...
	// id := kademlia.NewKademliaID("FFFFFFFF00000000000000000000000000000000")
	// contact := kademlia.NewContact(id, "localhost:8000")
	// fmt.Println(contact.String())
	// fmt.Printf("%v\n", contact)


	var net kademlia.Network
	net = kademlia.NewIP()

	channel := make(chan kademlia.Response)

	go net.Listen("0.0.0.0", 8000, channel)

	node := kademlia.NewKademlia()
	node.HandleResponse(channel)


	for {}
}
