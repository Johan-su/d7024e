package kademlia

import (
	"testing"
	// "strconv" 
)

func TestMockNetwork(t *testing.T) {
	// // TODO: Add actual network tests
	// network := NewMockNetwork(0)
	// var nodes []MockNode
	// node_count := 10000
	// for i := 0; i < node_count; i += 1 {
	// 	address := strconv.FormatInt(int64(i), 10)
	// 	nodes = append(nodes, MockNode{address, &network})
	// }

	// for i := 1; i < node_count; i += 1 {
	// 	go func() {
	// 		rep := nodes[i].Listen()
	// 		if rep.data[0] != byte(i) {
	// 			t.Errorf("got %v expected %v", rep.data[0], i)
	// 		}
	// 	}()
	// }

	// for i := 1; i < node_count; i += 1 {
	// 	address := strconv.FormatInt(int64(i), 10)
	// 	nodes[0].SendData(address, []byte{uint8(i)})
	// }

}
