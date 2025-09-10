package kademlia

import (
	"net"
	"log"
	"fmt"
	"sync"
)

type Message struct {
	from_address string
	data []byte
}

type Node interface {
	Listen() Message
	SendData(address string, data []byte)
}

type MockNetwork struct {
	mu_ip_to_queue sync.Mutex
	ip_to_queue map[string]chan Message


	nodes []Kademlia
}

func (net *MockNetwork) ExpectReceive(address string, msg Message) {
}

func (net *MockNetwork) ExpectSend(address string, msg Message) {
}

func NewMockNetwork(node_count int) MockNetwork {
	var n MockNetwork
	n.ip_to_queue = make(map[string]chan Message)
	for i := 0; i < node_count; i += 1 {
		address := fmt.Sprintf("%d", i)
		n.nodes = append(n.nodes, NewKademlia(address, NewMockNode(address, &n)))
	}
	return n
}

type MockNode struct {
	listen_ip string
	network *MockNetwork	
	send_log []Message
	receive_log []Message
}

func NewMockNode(listen_ip string, network *MockNetwork) Node {
	node := new(MockNode)
	node.listen_ip = listen_ip
	node.network = network
	return node
}

// TODO: make channel buffer count global constant
func (node *MockNode) Listen() Message {

	fmt.Printf("listening...\n")
	node.network.mu_ip_to_queue.Lock()
	channel := node.network.ip_to_queue[node.listen_ip]
	if channel == nil {
		node.network.ip_to_queue[node.listen_ip] = make(chan Message, 8)	
		channel = node.network.ip_to_queue[node.listen_ip]
	}
	node.network.mu_ip_to_queue.Unlock()
	rep := <- channel
	n := len(rep.data)
	fmt.Printf("Received %v bytes %v\n", n, string(rep.data[0:n - 1]))
	return rep
}
	
func (node *MockNode) SendData(address string, data []byte) {
	node.network.mu_ip_to_queue.Lock()
	channel := node.network.ip_to_queue[address]
	node.network.mu_ip_to_queue.Unlock()
	if channel != nil {
		channel <- Message{address, data} 
	}
}

type UDPNode struct {
	listen_ip string
	listen_port int
}

func NewUDPNode(listen_ip string, listen_port int) Node {

	net := new(UDPNode)
	net.listen_ip = listen_ip
	net.listen_port = listen_port
	return net
}

func (network *UDPNode) Listen() Message {
	addr := net.UDPAddr{Port: network.listen_port, IP: net.ParseIP(network.listen_ip)}
	
	fmt.Printf("listening...\n")
	conn, err := net.ListenUDP("udp", &addr)
	if (err != nil) {
		log.Fatalf("Failed to listen %v\n", err)
	}
	buf := make([]byte, 1000)
	
	n, rec_addr, err := conn.ReadFromUDP(buf)
	if err != nil {
		log.Fatalf("Failed to read packet %v\n", err)
	}

	fmt.Printf("Received %v bytes %v\n", n, string(buf[0:n - 1]))

	conn.Close()
	return Message{rec_addr.String(), buf}
}
	
func (network *UDPNode) SendData(address string, data []byte) {
	// TODO maybe make port a global parameter
	addr := net.UDPAddr{Port: 8000, IP: net.ParseIP(address)}
	conn, err := net.DialUDP("udp", nil, &addr)
	if err != nil {
		log.Fatalf("Failed to send data, %v\n", err)
	}
	defer conn.Close()
	_, err = conn.Write(data)
	if err != nil {
		log.Fatalf("write error %v\n", err)
	}
}