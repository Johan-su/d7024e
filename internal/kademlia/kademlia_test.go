package kademlia

import (
	"fmt"
	"testing"
	//"strconv"
)

func TestKademlia(t *testing.T) {

	/* network := NewMockNetwork()

	var nodes []Kademlia
	for i := 0; i < 1000; i += 1 {
		address :=fmt.Sprintf("%d", i)
		nodes = append(nodes, NewKademlia(NewMockNode(address, &network)))
	} 

	for _, node := range nodes {
		go node.HandleResponse()
	}

	network.nodes[fmt.Sprintf("%d", 5)]
	//TODO finish */
}

func TestLookupLogicMockNetwork(t *testing.T) {
	// xreate a mock Kademlia node
	network := NewMockNetwork()
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
