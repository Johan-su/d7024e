package kademlia

import (
	"testing"
	"strconv" 
)

func TestNetwork(t *testing.T) {
	// TODO: Add actual network tests
	network := NewMockNetwork()
	var nodes []MockNode
	for i := range(0, 100) {
		address := strconv.FormatInt(i, 10)
		nodes = append(nodes, MockNode{address, &network})
	}

	for i, node := range nodes {
		rep := node.Listen()
	}
	// TODO finish
		
	t.Log("Node test placeholder")
}
