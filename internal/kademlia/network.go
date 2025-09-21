package kademlia

import (
	"net"
	"log"
	"fmt"
	"sync"
	"time"
	"math/rand"
)

type Message struct {
	from_address string
	data []byte
}

type Node interface {
	Listen(address string)
	Close()
	Receive() Message
	SendData(address string, data []byte)
}

type MockNetwork struct {
	nodes []Kademlia
	packet_loss float32 // 0-1 probability of packet loss

	mu sync.Mutex
	ip_to_queue map[string]chan Message

	send_log map[string][]Message
	receive_log map[string][]Message
}

// the created nodes' addresses are equal to their positions in the nodes array
func NewMockNetwork(node_count int, packet_loss float32) *MockNetwork {
	n := new(MockNetwork)
	n.ip_to_queue = make(map[string]chan Message)
	n.send_log = make(map[string][]Message)
	n.receive_log = make(map[string][]Message)
	for i := 0; i < node_count; i += 1 {
		address := fmt.Sprintf("%d", i)
		mock_node := new(MockNode)
		mock_node.network = n
		n.nodes = append(n.nodes, NewKademlia(address, NewRandomKademliaID(), mock_node))
		n.ip_to_queue[address] = make(chan Message, 10*alpha)
	}
	return n
}

func (network *MockNetwork) AllNodesListen() {
	for i := range network.nodes {
		network.nodes[i].Listen()
		go network.nodes[i].HandleResponse()
	}
}

func (net *MockNetwork) WaitForSettledNetwork() {
	prev_send_count := 0
	prev_receive_count := 0
	for {
		send_count := 0
		receive_count := 0
		net.mu.Lock()
		for _, v := range net.send_log {
			send_count += len(v)
		}
		for _, v := range net.receive_log {
			receive_count += len(v)
		}
		net.mu.Unlock()

		if send_count == prev_send_count && receive_count == prev_receive_count {
			break
		} else {
			prev_send_count = send_count
			prev_receive_count = receive_count
		}
		time.Sleep(30 * time.Millisecond)
	}
}

type MockNode struct {
	address string
	network *MockNetwork	
}


func (node *MockNode) Listen(address string) {
	node.address = address
}

func (node *MockNode) Close() {
}

func (node *MockNode) Receive() Message {
	assertPanic(node.address != "", "address cannot be empty")

	node.network.mu.Lock()
	channel := node.network.ip_to_queue[node.address]
	node.network.mu.Unlock()
	rep := <- channel
	node.network.mu.Lock()
	node.network.receive_log[node.address] = append(node.network.receive_log[node.address], rep)
	node.network.mu.Unlock()
	return rep
}
	
func (node *MockNode) SendData(address string, data []byte) {
	assertPanic(address != "", "address cannot be empty")
	node.network.mu.Lock()
	channel := node.network.ip_to_queue[address]
	node.network.mu.Unlock()

	dat := Message{node.address, data}
	node.network.mu.Lock()
	node.network.send_log[node.address] = append(node.network.send_log[node.address], dat)
	node.network.mu.Unlock()
	if node.network.packet_loss < rand.Float32() {
		select {
			case channel <- dat:
			case <-time.After(3 * time.Second): {
				log.Fatalf("Mock SendData Timedout\n")
			} 
		}
	}
}

type UDPNode struct {
	conn *net.UDPConn
}

func NewUDPNode() Node {

	net := new(UDPNode)
	return net
}

func (network *UDPNode) Listen(address string) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		log.Fatalf("Invalid UDP/IP address, %v\n", err)
	}
	
	// log.Printf("listening... on %v\n", address)
	conn, err := net.ListenUDP("udp", addr)
	if (err != nil) {
		log.Fatalf("Failed to listen %v\n", err)
	}
	network.conn = conn
}

func (node *UDPNode) Close() {
	node.conn.Close()
}

func (node *UDPNode) Receive() Message {
	buf := make([]byte, 1000)
	_, rec_addr, err := node.conn.ReadFromUDP(buf)
	if err != nil {
		log.Fatalf("Failed to read packet %v\n", err)
	}
	return Message{rec_addr.String(), buf}
}

func (network *UDPNode) SendData(address string, data []byte) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		log.Fatalf("Invalid UDP/IP address, %v\n", err)
	}

	_, err = network.conn.WriteToUDP(data, addr)
	if err != nil {
		log.Fatalf("write error %v\n", err)
	}
}