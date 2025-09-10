package kademlia

import (
	"net"
	"log"
	"fmt"
	"sync"
)

type Response struct {
	from_address string
	data []byte
}

type Node interface {
	Listen() Response
	SendData(recipient *Contact, data []byte)
}

type UDPNode struct {
	listen_ip string
	listen_port int
}

type MockNetwork struct {
	mu sync.Mutex
	ip_to_queue map[string]chan Response
}

func NewMockNetwork() MockNetwork {
	var n MockNetwork
	n.ip_to_queue = make(map[string]chan Response)
	return n
}

type MockNode struct {
	listen_ip string
	network *MockNetwork	
}

func NewMockNode(listen_ip string, network *MockNetwork) Node {
	node := new(MockNode)
	node.listen_ip = listen_ip
	node.network = network
	return node
}

// TODO: make channel buffer count global constant
func (node *MockNode) Listen() Response {

	fmt.Printf("listening...\n")
	node.network.mu.Lock()
	channel := node.network.ip_to_queue[node.listen_ip]
	if channel == nil {
		node.network.ip_to_queue[node.listen_ip] = make(chan Response, 8)	
		channel = node.network.ip_to_queue[node.listen_ip]
	}
	node.network.mu.Unlock()
	rep := <- channel
	n := len(rep.data)
	fmt.Printf("Received %v bytes %v\n", n, string(rep.data[0:n - 1]))
	return rep
}
	
func (node *MockNode) SendData(recipient *Contact, data []byte) {
	node.network.mu.Lock()
	channel := node.network.ip_to_queue[recipient.Address]
	node.network.mu.Unlock()
	if channel != nil {
		channel <- Response{recipient.Address, data} 
	}
}

func NewUDPNetwork(listen_ip string, listen_port int) Node {

	net := new(UDPNode)
	net.listen_ip = listen_ip
	net.listen_port = listen_port
	return net
}

func (network *UDPNode) Listen() Response {
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
	return Response{rec_addr.String(), buf}
}
	
func (network *UDPNode) SendData(recipient *Contact, data []byte) {
	// TODO maybe make port a global parameter
	addr := net.UDPAddr{Port: 8000, IP: net.ParseIP(recipient.Address)}
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