package kademlia

import (
	"testing"
	"bytes"
	"strings"
	"fmt"
)

func RemoveUnordered[T any](arr []T, index int) []T {
	len := len(arr)
	arr[index] = arr[len - 1]
	arr = arr[:len - 1]
	return arr
}

//TODO make expect not care about order 
func ExpectSend(t *testing.T, net *MockNetwork, address string, typ RPCType) {
	var msg_data []byte
	{
		net.mu.Lock()
		l_msg := net.send_log[address][0]
		msg_data = bytes.Clone(l_msg.data)
		net.send_log[address] = net.send_log[address][1:]
		net.mu.Unlock()
	}

	var lh RPCHeader

	PartialRead(bytes.NewReader(msg_data), &lh)

	if lh.Typ != typ {
		t.Errorf("[%v] Expected send %v got %v\n", address, lh.Typ, typ)
	}
}

//TODO make expect not care about order 
func ExpectReceive(t *testing.T, net *MockNetwork, address string, from_address string, typ RPCType) {
	var msg_data []byte
	var msg_address string
	{
		net.mu.Lock()
		l_msg := net.receive_log[address][0]
		msg_data = bytes.Clone(l_msg.data)
		msg_address = strings.Clone(l_msg.from_address)
		net.receive_log[address] = net.receive_log[address][1:]
		net.mu.Unlock()
	}

	var lh RPCHeader

	PartialRead(bytes.NewReader(msg_data), &lh)

	if !(msg_address == from_address && lh.Typ == typ) {
		t.Errorf("[%v] Expected Receive [%v] %v got [%v] %v\n", address, from_address, typ, msg_address, lh.Typ)
	}
}


func TestKademlia(t *testing.T) {

	network := NewMockNetwork(20, 0)
	network.AllNodesListen()
	
	network.nodes[0].SendPingMessage("19", false)
	network.nodes[4].SendPingMessage("15", false)
	network.nodes[5].SendPingMessage("15", false)
	network.nodes[6].SendPingMessage("15", false)
	network.nodes[7].SendPingMessage("15", false)

	network.nodes[15].SendPingMessage("4", false)

	network.WaitForSettledNetwork()

	//TODO change so the expect function allows for any message order
	ExpectSend(t, network, "4", RPCTypePing)
	ExpectSend(t, network, "5", RPCTypePing)
	ExpectSend(t, network, "6", RPCTypePing)
	ExpectSend(t, network, "7", RPCTypePing)
	ExpectSend(t, network, "15", RPCTypePing)
	ExpectReceive(t, network, "15", "4", RPCTypePing)
	ExpectReceive(t, network, "15", "5", RPCTypePing)
	ExpectReceive(t, network, "15", "6", RPCTypePing)
	ExpectReceive(t, network, "15", "7", RPCTypePing)
	ExpectReceive(t, network, "4", "15", RPCTypePing)
	ExpectSend(t, network, "15", RPCTypePingReply)
	ExpectSend(t, network, "15", RPCTypePingReply)
	ExpectSend(t, network, "15", RPCTypePingReply)
	ExpectSend(t, network, "15", RPCTypePingReply)
	ExpectSend(t, network, "4", RPCTypePingReply)
}

func TestLookupLogicMockNetwork(t *testing.T) {
	network := NewMockNetwork(1, 0)
	// add the node itself to its routing table
	network.nodes[0].routingTable.AddContact(network.nodes[0].routingTable.me)

	// create target and some initial contacts
	target := NewContact(NewKademliaID("FFFFFFFF00000000000000000000000000000000"), "target")
	fmt.Printf("Target contact: %s\n", target.String())

	// add some mock contacts to the routing table for testing
	fmt.Println("=== Adding contacts to routing table ===")
	for i := 0; i < 5; i++ {
		contact := NewContact(NewRandomKademliaID(), fmt.Sprintf("node-%d", i))
		contact.CalcDistance(target.ID) // Calculate distance for display
		fmt.Printf("Added contact %d: %s (distance: %s)\n", i, contact.String(), contact.distance)
		network.nodes[0].routingTable.AddContact(contact)
	}
	fmt.Println()

	// test the algorithm with mock network function
	fmt.Println("=== Starting LookupContactInternal ===")
	result := network.nodes[0].LookupContact(&target)

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

	network := NewMockNetwork(4, 0)

	for i := 1; i < 4; i += 1 {
		network.nodes[0].routingTable.AddContact(network.nodes[i].routingTable.me)
	}
	network.nodes[0].routingTable.AddContact(network.nodes[0].routingTable.me) // add self last to see if it gets selected

	testData := []byte("test data for storage")
	data_hash := Sha1toKademlia(testData)

	t.Logf("Data hash: %v\n", data_hash)
	t.Logf("Data content: %s\n", string(testData))

	err := network.nodes[0].Store(testData)

	if err != nil {
		t.Logf("Store completed with errors: %v", err)
	} else {
		t.Log("Store completed successfully!")
	}

	// verify local storage
	if _, exists := network.nodes[0].kv_store[*data_hash]; !exists {
		t.Fatal("Data should be stored locally")
	}
	t.Log("Data successfully stored locally")

}
