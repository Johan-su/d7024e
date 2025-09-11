package kademlia

import (
	"testing"
	"time"
	"bytes"
	"fmt"
)

func ExpectSend(t *testing.T, net *MockNetwork, address string, typ RPCType) {
	net.mu.Lock()
	l_msg := net.send_log[address][0]
	net.send_log[address] = net.send_log[address][1:]	
	net.mu.Unlock()

	var lh RPCHeader

	PartialRead(bytes.NewReader(l_msg.data), &lh)

	if lh.Typ != typ {
		t.Errorf("[%v] Expected send %v got %v\n", address, lh.Typ, typ)
	}
}

func ExpectReceive(t *testing.T, net *MockNetwork, address string, from_address string, typ RPCType) {
	net.mu.Lock()
	l_msg := net.receive_log[address][0]
	net.receive_log[address] = net.receive_log[address][1:]	
	net.mu.Unlock()

	var lh RPCHeader

	PartialRead(bytes.NewReader(l_msg.data), &lh)

	if !(l_msg.from_address == from_address && lh.Typ == typ) {
		t.Errorf("[%v] Expected Receive %v got %v\n", address, lh.Typ, typ)
	}
}

func TestKademlia(t *testing.T) {

	network := NewMockNetwork(20, 0)

	for i := range network.nodes {
		go network.nodes[i].HandleResponse()
	}
	time.Sleep(500 * time.Millisecond)

	network.nodes[4].SendPingMessage("15")
	network.nodes[5].SendPingMessage("15")
	network.nodes[6].SendPingMessage("15")
	network.nodes[7].SendPingMessage("15")

	network.nodes[15].SendPingMessage("4")


	time.Sleep(500 * time.Millisecond)


	ExpectSend(t, &network, "4", RPCTypePing)
	ExpectSend(t, &network, "5", RPCTypePing)
	ExpectSend(t, &network, "6", RPCTypePing)
	ExpectSend(t, &network, "7", RPCTypePing)
	ExpectSend(t, &network, "15", RPCTypePing)
	ExpectReceive(t, &network, "15", "4", RPCTypePing)
	ExpectReceive(t, &network, "15", "5", RPCTypePing)
	ExpectReceive(t, &network, "15", "6", RPCTypePing)
	ExpectReceive(t, &network, "15", "7", RPCTypePing)
	ExpectReceive(t, &network, "4", "15", RPCTypePing)
	ExpectSend(t, &network, "15", RPCTypePingReply)
	ExpectSend(t, &network, "15", RPCTypePingReply)
	ExpectSend(t, &network, "15", RPCTypePingReply)
	ExpectSend(t, &network, "15", RPCTypePingReply)
	ExpectSend(t, &network, "4", RPCTypePingReply)
}

func TestLookupLogicMockNetwork(t *testing.T) {
	// xreate a mock Kademlia node
	network := NewMockNetwork(0, 0)
	me := NewContact(NewRandomKademliaID(), "localhost:8000")
	k := &Kademlia{
		routingTable: NewRoutingTable(me),
		net: NewMockNode("localhost", &network),
	}

	// add the node itself to its routing table
	k.routingTable.AddContact(me)

	// areate target and some initial contacts
	target := NewContact(NewKademliaID("FFFFFFFF00000000000000000000000000000000"), "target")
	fmt.Printf("Target contact: %s\n", target.String())

	// add some mock contacts to the routing table for testing
	fmt.Println("=== Adding contacts to routing table ===")
	for i := 0; i < 5; i++ {
		contact := NewContact(NewRandomKademliaID(), fmt.Sprintf("node-%d", i))
		contact.CalcDistance(target.ID) // Calculate distance for display
		fmt.Printf("Added contact %d: %s (distance: %s)\n", i, contact.String(), contact.distance)
		k.routingTable.AddContact(contact)
	}
	fmt.Println()

	// test the algorithm with mock network function
	fmt.Println("=== Starting LookupContactInternal ===")
	result := k.LookupContactInternal(&target, func(contactsToQuery []Contact, targetID *KademliaID) map[Contact][]Contact {
		fmt.Printf("Querying %d nodes: ", len(contactsToQuery))
		for i, contact := range contactsToQuery {
			contact.CalcDistance(targetID)
			fmt.Printf("%s (dist: %s)", contact.ID.String()[:8], contact.distance)
			if i < len(contactsToQuery)-1 {
				fmt.Print(", ")
			}
		}
		fmt.Println()

		// mock function with more realistic responses
		responseMap := make(map[Contact][]Contact)
		for _, contact := range contactsToQuery {
			// simulate network response with 2-3 random contacts
			mockResponse := []Contact{
				NewContact(NewRandomKademliaID(), "mock-node-1"),
				NewContact(NewRandomKademliaID(), "mock-node-2"),
				NewContact(NewRandomKademliaID(), "mock-node-3"),
			}

			// calculate distances for the mock responses
			for j := range mockResponse {
				mockResponse[j].CalcDistance(targetID)
			}

			responseMap[contact] = mockResponse
			fmt.Printf("  Node %s returned %d contacts\n", contact.ID.String()[:8], len(mockResponse))
		}
		fmt.Println()
		return responseMap
	})

	fmt.Println("=== Final Results ===")
	fmt.Printf("Found %d results:\n", len(result))
	for i, contact := range result {
		contact.CalcDistance(target.ID)
		fmt.Printf("  %d: %s (distance: %s)\n", i+1, contact.String(), contact.distance)
	}
	fmt.Println()

	if len(result) == 0 {
		t.Error("Lookup should return results even without real network")
	}

	// Test that results are sorted by distance to target
	fmt.Println("=== Verifying sort order ===")
	for i := 0; i < len(result)-1; i++ {
		result[i].CalcDistance(target.ID)
		result[i+1].CalcDistance(target.ID)
		fmt.Printf("Comparing %s (dist: %s) vs %s (dist: %s)\n",
			result[i].ID.String()[:8], result[i].distance,
			result[i+1].ID.String()[:8], result[i+1].distance)

		if !result[i].Less(&result[i+1]) {
			t.Errorf("Results should be sorted by distance to target. %s should be closer than %s",
				result[i].distance, result[i+1].distance)
		} else {
			fmt.Printf("Correct order: %s < %s\n", result[i].distance.String()[:8], result[i+1].distance.String()[:8])
		}
	}
}
