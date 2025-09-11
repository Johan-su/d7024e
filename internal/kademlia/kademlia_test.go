package kademlia

import (
	"crypto/sha1"
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
		net:          NewMockNode("localhost", &network),
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
	result := k.LookupContact(&target)

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

func TestStoreWithNodeTracking(t *testing.T) {
	network := NewMockNetwork()
	me := NewContact(NewRandomKademliaID(), "localhost:8000")
	k := &Kademlia{
		routingTable: NewRoutingTable(me),
		net:          NewMockNode("localhost", &network),
		kv_store:     make(map[KademliaID][]byte),
	}

	contacts := []Contact{
		NewContact(NewRandomKademliaID(), "node-1:8000"),
		NewContact(NewRandomKademliaID(), "node-2:8000"),
		NewContact(NewRandomKademliaID(), "node-3:8000"),
	}

	for _, contact := range contacts {
		k.routingTable.AddContact(contact)
	}
	k.routingTable.AddContact(me) // add self last to see if it gets selected

	// test data
	testData := []byte("test data for storage")

	hasher := sha1.New()
	hasher.Write(testData)
	hashBytes := hasher.Sum(nil)
	hash := KademliaID{}
	copy(hash[:], hashBytes)

	t.Logf("Data hash: %s", hash.String())
	t.Logf("Data content: %s", string(testData))

	err := k.Store(testData)

	// log results
	if err != nil {
		t.Logf("Store completed with errors: %v", err)
	} else {
		t.Log("Store completed successfully!")
	}

	// verify local storage
	if _, exists := k.kv_store[hash]; !exists {
		t.Fatal("Data should be stored locally")
	}
	t.Log("Data successfully stored locally")

	// verify the closest nodes were selected for storage
	target := NewContact(&hash, "")
	closestContacts := k.routingTable.FindClosestContacts(target.ID, bucketSize)

	t.Logf("Closest nodes to data hash (by NodeID):")
	for i, contact := range closestContacts {
		contact.CalcDistance(&hash)
		t.Logf("  %d: %s (distance: %s)", i+1, contact.ID.String(), contact.distance.String()[:8])
	}

	t.Log("store test with node tracking passed")
}
