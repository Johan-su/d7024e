package kademlia

import (
	"testing"
	//"fmt"
	"bytes"
	"strconv"
	"strings"
	"time"
)

func TestMockNetwork(t *testing.T) {
	node_count := 10000
	var packet_loss float32 = 0

	network := NewMockNetwork(node_count, packet_loss)

	network.AllNodesListen()


       messages := []struct {
	       address      string
	       from_address string
	       data         []byte
       }{
	       {"1", "0", []byte{1,2,3,4,5}},
	       {"2", "0", []byte{10,20,30}},
	       {"1", "2", []byte{99,98,97}},
	       {"3", "1", []byte{42,43,44,45}},
       }

       // Send all messages
       for _, msg := range messages {
	       idx, err := strconv.Atoi(msg.from_address)
	       if err != nil {
		       t.Fatalf("Invalid from_address: %v", msg.from_address)
	       }
	       network.nodes[idx].Net.SendData(msg.address, msg.data)
       }


       // Count expected messages per address
       expectedCounts := make(map[string]int)
       for _, msg := range messages {
	       expectedCounts[msg.address]++
       }

       // Wait for all expected messages to arrive at each address
       timeout := time.Now().Add(2 * time.Second)
       for address, count := range expectedCounts {
	       for {
		       network.mu.Lock()
		       received := len(network.receive_log[address])
		       network.mu.Unlock()
		       if received >= count {
			       break
		       }
		       if time.Now().After(timeout) {
			       t.Errorf("Timeout waiting for %d messages to %v (got %d)", count, address, received)
			       break
		       }
		       time.Sleep(10 * time.Millisecond)
	       }
       }

       // Check all messages received correctly
       for _, msg := range messages {
	       msgData, msgFrom := ExpectReceiveData(t, network, msg.address, msg.from_address, RPCTypePing)
	       if !bytes.Equal(msgData, msg.data) {
		       t.Errorf("Expected data %v, got %v", msg.data, msgData)
	       }
	       if msgFrom != msg.from_address {
		       t.Errorf("Expected from_address %v, got %v", msg.from_address, msgFrom)
	       }
       }

}


func ExpectReceiveData(t *testing.T, net *MockNetwork, address string,
	from_address string, typ RPCType) ([]byte, string) {
	net.mu.Lock()
	defer net.mu.Unlock()
	l_msg := net.receive_log[address][0]
	msg_data := bytes.Clone(l_msg.data)
	msg_address := strings.Clone(l_msg.from_address)
	net.receive_log[address] = net.receive_log[address][1:]
	return msg_data, msg_address
}
