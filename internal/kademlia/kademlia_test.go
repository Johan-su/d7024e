package kademlia

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func RemoveUnordered[T any](arr []T, index int) []T {
	len := len(arr)
	arr[index] = arr[len-1]
	arr = arr[:len-1]
	return arr
}

// TODO make expect not care about order
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

// TODO make expect not care about order
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

func TestPing(t *testing.T) {

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

func TestFindContact(t *testing.T) {

	network := NewMockNetwork(3, 0)
	network.AllNodesListen()

	network.nodes[2].routingTable.me.ID = NewKademliaID("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")
	c := network.nodes[2].routingTable.me

	network.nodes[1].routingTable.AddContact(c)
	network.nodes[0].SendFindContactMessage("1", c.ID)

	for i, n := range network.nodes {
		fmt.Printf("node %v %v\n", i, n.routingTable.me.ID)
	}

	network.WaitForSettledNetwork()

	ExpectSend(t, network, "0", RPCTypeFindNode)
	ExpectReceive(t, network, "1", "0", RPCTypeFindNode)
	ExpectReceive(t, network, "0", "1", RPCTypeFindNodeReply)

	val := <-network.nodes[0].find_responses
	if !val.contacts[0].id.Equals(c.ID) {
		t.Errorf("Expected %v got %v", c.ID, &val.contacts[0].id)
	}
}

func TestJoin(t *testing.T) {
	rand.Seed(0)
	network := NewMockNetwork(100, 0)
	network.AllNodesListen()

	for i := 1; i < len(network.nodes); i += 1 {
		network.nodes[i].Join(network.nodes[0].routingTable.me)
	}
	network.WaitForSettledNetwork()
	// TODO maybe check if it actually works
}

func TestLookupLogicMockNetwork(t *testing.T) {
	network := NewMockNetwork(1, 0)
	// add the node itself to its routing table
	network.nodes[0].routingTable.AddContact(network.nodes[0].routingTable.me)

	// create target and some initial contacts
	target := NewContact(NewKademliaID("FFFFFFFF00000000000000000000000000000000"), "target")

	// add some mock contacts to the routing table for testing
	for i := 0; i < 5; i++ {
		contact := NewContact(NewRandomKademliaID(), fmt.Sprintf("node-%d", i))
		contact.CalcDistance(target.ID) // Calculate distance for display
		network.nodes[0].routingTable.AddContact(contact)
	}

	// test the algorithm with mock network function
	result := network.nodes[0].LookupContact(&target)

	for _, contact := range result {
		contact.CalcDistance(target.ID)
	}

	if len(result) == 0 {
		t.Error("Lookup should return results even without real network")
	}

	// Test that results are sorted by distance to target
	for i := 0; i < len(result)-1; i++ {
		result[i].CalcDistance(target.ID)
		result[i+1].CalcDistance(target.ID)

		if !result[i].Less(&result[i+1]) {
			t.Errorf("Results should be sorted by distance to target. %s should be closer than %s",
				result[i].distance, result[i+1].distance)
		} else {
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
		t.Fatal("Data was not stored locally")
	}
	t.Log("Data successfully stored locally")
}

func TestLookupDataSingleHop(t *testing.T) {
	network := NewMockNetwork(2, 0)
	network.AllNodesListen()

	// store test data on node 1
	testData := []byte("hello kademlia")
	key := Sha1toKademlia(testData)
	network.nodes[1].kv_store[*key] = testData

	// make node 0 aware of node 1
	network.nodes[0].routingTable.AddContact(network.nodes[1].routingTable.me)

	data, found, _ := network.nodes[0].LookupData(key.String())

	if !found {
		t.Fatalf("Expected to find data, but LookupData returned not found")
	}
	if string(data) != string(testData) {
		t.Errorf("Expected '%s', got '%s'", string(testData), string(data))
	}
}

func TestLookupDataMultipleHops(t *testing.T) {
	network := NewMockNetwork(4, 0)
	network.AllNodesListen()

	// store test data on node 3
	testData := []byte("multi-hop data")
	key := Sha1toKademlia(testData)
	network.nodes[3].kv_store[*key] = testData

	// set up routing chain
	network.nodes[0].routingTable.AddContact(network.nodes[1].routingTable.me)
	network.nodes[1].routingTable.AddContact(network.nodes[2].routingTable.me)
	network.nodes[2].routingTable.AddContact(network.nodes[3].routingTable.me)

	// lookup data starting from node 0
	data, found, _ := network.nodes[0].LookupData(key.String())

	if !found {
		t.Fatalf("Expected to find data through multiple hops, but LookupData returned not found")
	}
	if string(data) != string(testData) {
		t.Errorf("Expected '%s', got '%s'", string(testData), string(data))
	}

	// check which node that didnt have the data is closest to the target
	xorDistance := func(a, b *KademliaID) KademliaID {
		var d KademliaID
		for i := 0; i < IDLength; i++ {
			d[i] = a[i] ^ b[i]
		}
		return d
	}

	d1 := xorDistance(network.nodes[1].routingTable.me.ID, key)
	d2 := xorDistance(network.nodes[2].routingTable.me.ID, key)

	closestNode := &network.nodes[1]
	if d2.Less(&d1) {
		closestNode = &network.nodes[2]
	}

	time.Sleep(10 * time.Millisecond) // Wait for the StoreRPC to be processed

	foundStored := false

	if _, exists := closestNode.kv_store[*key]; exists {
		foundStored = true
	}

	if !foundStored {
		t.Errorf("Expected closest node (%v) to store the data, but it did not", closestNode.routingTable.me.Address)
	}
}

func TestLookupDataReturnC(t *testing.T) {
	network := NewMockNetwork(5, 0)
	network.AllNodesListen()

	nonexistentKey := NewRandomKademliaID()

	network.nodes[0].routingTable.AddContact(network.nodes[1].routingTable.me)
	network.nodes[1].routingTable.AddContact(network.nodes[2].routingTable.me)
	network.nodes[2].routingTable.AddContact(network.nodes[3].routingTable.me)
	network.nodes[3].routingTable.AddContact(network.nodes[4].routingTable.me)

	for i := 0; i < 5; i++ {
		if _, exists := network.nodes[i].kv_store[*nonexistentKey]; exists {
			t.Fatalf("Node %d should not have the data initially", i)
		}
	}

	// should return nil, false, [closest contacts]
	data, found, closestContacts := network.nodes[0].LookupData(nonexistentKey.String())

	if found {
		t.Fatalf("Expected data not to be found, but it was found: %s", string(data))
	}

	if data != nil {
		t.Errorf("Expected nil data when not found, got %v", data)
	}

	if closestContacts == nil {
		t.Fatal("Expected closest contacts to be returned when data is not found")
	}

	if len(closestContacts) == 0 {
		t.Fatal("Expected non-empty closest contacts list")
	}

	for i := 0; i < len(closestContacts)-1; i++ {
		closestContacts[i].CalcDistance(nonexistentKey)
		closestContacts[i+1].CalcDistance(nonexistentKey)

		if !closestContacts[i].Less(&closestContacts[i+1]) {
			t.Errorf("Contacts should be sorted by distance. %s should be closer than %s",
				closestContacts[i].ID, closestContacts[i+1].ID)
		}
	}
}
