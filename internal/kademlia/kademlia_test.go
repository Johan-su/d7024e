package kademlia

import (
	"bytes"
	"testing"
	// "fmt"
)

func RemoveUnordered[T any](arr []T, index int) []T {
	len := len(arr)
	arr[index] = arr[len - 1]
	arr = arr[:len - 1]
	return arr
}

func ExpectSend(t *testing.T, net *MockNetwork, address string, typ RPCType) {
	found := false
	var lh RPCHeader
	{
		net.mu.Lock()
		log := net.send_log[address]
		for i := 0; i < len(log); i += 1 {
			l_msg := log[i]
			PartialRead(bytes.NewReader(l_msg.data), &lh)
			if lh.Typ == typ {
				found = true
				net.send_log[address] = RemoveUnordered(net.send_log[address], i)
				break
			}
		}
		net.mu.Unlock()
	}

	if !found {
		t.Errorf("[%v] %v was not found in send log\n", address, typ)
	}
}

func ExpectReceive(t *testing.T, net *MockNetwork, address string, from_address string, typ RPCType) {
	found := false
	var lh RPCHeader
	{
		net.mu.Lock()
		log := net.receive_log[address]
		for i := 0; i < len(log); i += 1 {
			l_msg := log[i]
			PartialRead(bytes.NewReader(l_msg.data), &lh)
			if lh.Typ == typ && l_msg.from_address == from_address {
				found = true
				net.receive_log[address] = RemoveUnordered(net.receive_log[address], i)
				break
			}
		}
		net.mu.Unlock()
	}

	if !found {
		t.Errorf("[%v] %v from %v was not found in receive log\n", address, typ, from_address)
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


	network.WaitForSettledNetwork()

	ExpectSend(t, network, "0", RPCTypeFindNode)
	ExpectReceive(t, network, "1", "0", RPCTypeFindNode)
	ExpectReceive(t, network, "0", "1", RPCTypeFindNodeReply)
	

	val := <- network.nodes[0].findNodeResponses
	if !val.contacts[0].id.Equals(c.ID) {
		t.Errorf("Expected %v got %v", c.ID, &val.contacts[0].id)
	}
}

func TestFindValue(t *testing.T) {
	
	
	// TODO: change to 1000 nodes after fix
	network := NewMockNetwork(20, 0)
	network.AllNodesListen()
	
	for i := 1; i < len(network.nodes); i += 1 {
		network.nodes[i].Join(network.nodes[0].routingTable.me)
	}
	network.WaitForSettledNetwork()

	data := []byte("krakel")
	hash := Sha1toKademlia((data))


	storedHash, err := network.nodes[1].Store(data)
	if err != nil {
		t.Fatal("Store failed")
	}
	if !storedHash.Equals(hash) {
		t.Fatal("hash is diffrent?")
	}

	network.WaitForSettledNetwork()



	dat, _, exists := network.nodes[5].LookupData(storedHash.String())

	if !exists {
		t.Errorf("Expected %v got %v\n", true, exists)
	}

	if !bytes.Equal(dat, data) {
		t.Errorf("Expected %v got %v\n", data, dat)
	}	


	// ExpectSend(t, network, "1", RPCTypeFindValue)
	// ExpectSend(t, network, "0", RPCTypeFindValue)

	// ExpectReceive(t, network, "2", "1", RPCTypeFindValue)
	// ExpectReceive(t, network, "3", "0", RPCTypeFindValue)

	// ExpectSend(t, network, "2", RPCTypeFindValueReply)
	// ExpectSend(t, network, "3", RPCTypeFindValueReply)

	// ExpectReceive(t, network, "1", "2", RPCTypeFindValueReply)
	// ExpectReceive(t, network, "0", "3", RPCTypeFindValueReply)
	
	
	// if len(network.nodes[1].find_responses) != 1 {
	// 	t.Fatalf("Expected response")
	// }

	// if len(network.nodes[0].find_responses) != 1 {
	// 	t.Fatalf("Expected response")
	// }


	// v1 := <- network.nodes[1].find_responses
	// v2 := <- network.nodes[0].find_responses


	// if v1.contact_count == 0 && v1.data_size != uint64(v1.contact_count) {
	// 	t.Fatalf("Expected response to return contacts")
	// }
	
	// if v2.data_size == 0 && v2.data_size != uint64(v2.contact_count) {
	// 	t.Fatalf("Expected response to return data")
	// }

	// if !bytes.Equal(v2.data, data) {
	// 	t.Errorf("Expected %v got %v", data, v2.data)
	// }
}

func TestJoin(t *testing.T) {
	//TODO maybe make mock network have a local seed
	network := NewMockNetwork(100, 0)
	network.AllNodesListen()

	for i := 1; i < len(network.nodes); i += 1 {
		network.nodes[i].Join(network.nodes[0].routingTable.me)
	}
	network.WaitForSettledNetwork()
	// TODO maybe check if it actually works
}


func TestLookupLogicMockNetwork(t *testing.T) {
	// network := NewMockNetwork(20, 0)

	// // create target and some initial contacts
	// target := NewContact(NewKademliaID("FFFFFFFF00000000000000000000000000000000"), "target")

	// // add some mock contacts to the routing table for testing
	// for i := 0; i < 5; i++ {
	// 	contact := NewContact(NewRandomKademliaID(), fmt.Sprintf("node-%d", i))
	// 	contact.CalcDistance(target.ID) // Calculate distance for display
	// 	network.nodes[0].routingTable.AddContact(contact)
	// }

	// // test the algorithm with mock network function
	// result := network.nodes[0].LookupContact(&target)

	// for _, contact := range result {
	// 	contact.CalcDistance(target.ID)
	// }

	// if len(result) == 0 {
	// 	t.Error("Lookup should return results even without real network")
	// }

	// // Test that results are sorted by distance to target
	// for i := 0; i < len(result)-1; i++ {
	// 	result[i].CalcDistance(target.ID)
	// 	result[i+1].CalcDistance(target.ID)

	// 	if !result[i].Less(&result[i+1]) {
	// 		t.Errorf("Results should be sorted by distance to target. %s should be closer than %s",
	// 			result[i].distance, result[i+1].distance)
	// 	} else {
	// 	}
	// }
}

func TestStoreWithNodeTracking(t *testing.T) {

	// network := NewMockNetwork(4, 0)

	// for i := 1; i < 4; i += 1 {
	// 	network.nodes[0].routingTable.AddContact(network.nodes[i].routingTable.me)
	// }
	// network.nodes[0].routingTable.AddContact(network.nodes[0].routingTable.me) // add self last to see if it gets selected

	// testData := []byte("test data for storage")
	// data_hash := Sha1toKademlia(testData)

	// t.Logf("Data hash: %v\n", data_hash)
	// t.Logf("Data content: %s\n", string(testData))

	// hash, err := network.nodes[0].Store(testData)

	// if !hash.Equals(data_hash) {
	// 	t.Errorf("Expected %v got %v", data_hash, hash)
	// }

	// if err != nil {
	// 	t.Logf("Store completed with errors: %v", err)
	// } else {
	// 	t.Log("Store completed successfully!")
	// }

	// // verify local storage
	// if _, exists := network.nodes[0].kv_store[*data_hash]; !exists {
	// 	t.Fatal("Data was not stored locally")
	// }
	// t.Log("Data successfully stored locally")

}
