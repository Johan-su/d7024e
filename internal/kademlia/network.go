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
	Listen(address string) Message
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
		mock_node.address = address
		n.nodes = append(n.nodes, NewKademlia(address, mock_node))
		// TODO: make channel buffer count global constant
		n.ip_to_queue[address] = make(chan Message, 8)
	}
	return n
}

func (network *MockNetwork) AllNodesListen() {
	for i := range network.nodes {
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
		time.Sleep(50 * time.Millisecond)
	}
}

type MockNode struct {
	address string
	network *MockNetwork	
}


func (node *MockNode) Listen(address string) Message {

	node.network.mu.Lock()
	channel := node.network.ip_to_queue[address]
	node.network.mu.Unlock()
	rep := <- channel
	node.network.mu.Lock()
	node.network.receive_log[address] = append(node.network.receive_log[address], rep)
	node.network.mu.Unlock()
	return rep
}
	
func (node *MockNode) SendData(address string, data []byte) {
	node.network.mu.Lock()
	channel := node.network.ip_to_queue[address]
	node.network.mu.Unlock()

	dat := Message{node.address, data}
	node.network.mu.Lock()
	node.network.send_log[node.address] = append(node.network.send_log[node.address], dat)
	node.network.mu.Unlock()
	if node.network.packet_loss < rand.Float32() {
		channel <- dat 
	}
}

type UDPNode struct {
}

func NewUDPNode() Node {

	net := new(UDPNode)
	return net
}

func (network *UDPNode) Listen(address string) Message {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		log.Fatalf("Invalid UDP/IP address, %v\n", err)
	}
	
	fmt.Printf("listening...\n")
	conn, err := net.ListenUDP("udp", addr)
	if (err != nil) {
		log.Fatalf("Failed to listen %v\n", err)
	}
	buf := make([]byte, 1000)
	
	_, rec_addr, err := conn.ReadFromUDP(buf)
	if err != nil {
		log.Fatalf("Failed to read packet %v\n", err)
	}

	conn.Close()
	return Message{rec_addr.String(), buf}
}
	
func (network *UDPNode) SendData(address string, data []byte) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		log.Fatalf("Invalid UDP/IP address, %v\n", err)
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Fatalf("Failed to send data, %v\n", err)
	}
	defer conn.Close()
	_, err = conn.Write(data)
	if err != nil {
		log.Fatalf("write error %v\n", err)
	}
}