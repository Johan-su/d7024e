package kademlia

import (
	"net"
	"log"
	"fmt"
	"encoding/binary"
)

type RPCType uint8
const (
	RPCTypeInvalid = iota
	RPCTypePing
	RPCTypeStore
	RPCTypeFindNode
	RPCTypeFindValue
	RPCTypePingReply
	RPCTypeStoreReply
	RPCTypeFindNodeReply
	RPCTypeFindValueReply
)

type RPCError uint8
const (
	RPCErrorNoError = iota
	RPCErrorLackOfSpace
)

type Response struct {
	from_address string
	data []byte
}

type Network interface {
	Listen(ip string, port int, channel chan Response)
	SendPingMessage(recipient *Contact)
	SendFindContactMessage(recipient *Contact, key *KademliaID)
	SendFindDataMessage(recipient *Contact, hash string) 
	SendStoreMessage(recipient *Contact, data []byte)
	SendPingReplyMessage(recipient *Contact, id *KademliaID) 
	SendFindContactReplyMessage(recipient *Contact, id *KademliaID, contacts []Contact) 
	SendFindDataReplyMessage(recipient *Contact, id *KademliaID, data []byte, contacts []Contact) 
	SendStoreReplyMessage(recipient *Contact, id *KademliaID, err RPCError) 
	
	
	
}


type IPNetwork struct {
}

type MockNetwork struct {
	net map[KademliaID]Kademlia
}

func NewIP() Network {

	ip := new(IPNetwork)
	return ip
}


type RPCHeader struct {
	typ RPCType
	id KademliaID
	rpc_error RPCError
}

type RPCPing struct {
	header RPCHeader
}

type RPCStore struct {
	header RPCHeader
	data_size uint64
	data []byte
}

type RPCFindNode struct {
	header RPCHeader
	target_node_id KademliaID
}

type RPCFindValue struct {
	header RPCHeader
	target_key_id KademliaID
}

type RPCPingReply struct {
	header RPCHeader
}

type RPCStoreReply struct {
	header RPCHeader
}

type RPCFindNodeReply struct {
	header RPCHeader
	contact_count uint16	
	contacts []Contact
}

// returns either data or triples
type RPCFindValueReply struct {
	header RPCHeader
	data_size uint64
	contact_count uint16
	data []byte
	contacts []Contact
}



func (network *IPNetwork) Listen(ip string, port int, channel chan Response) {
	addr := net.UDPAddr{Port: port, IP: net.ParseIP(ip)}
	
	for {
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
		channel <- Response{rec_addr.String(), buf}
	}
}

func SendData(recipient *Contact, data []byte) {
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

func (network *IPNetwork) SendPingMessage(recipient *Contact) {
	var rpc RPCPing
	rpc.header.typ = RPCTypePing
	rpc.header.id = *NewRandomKademliaID()

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)

	SendData(recipient, write_buf)
}

func (network *IPNetwork) SendFindContactMessage(recipient *Contact, key *KademliaID) {
	var rpc RPCFindNode
	rpc.header.typ = RPCTypeFindNode
	rpc.header.id = *NewRandomKademliaID()

	rpc.target_node_id = *key

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)


	SendData(recipient, write_buf)
}

func (network *IPNetwork) SendFindDataMessage(recipient *Contact, hash string) {
	var rpc RPCFindValue
	rpc.header.typ = RPCTypeFindValue
	rpc.header.id = *NewRandomKademliaID()

	rpc.target_key_id = *NewKademliaID(hash)
	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)


	SendData(recipient, write_buf)
}

func (network *IPNetwork) SendStoreMessage(recipient *Contact, data []byte) {
	var rpc RPCStore
	rpc.header.typ = RPCTypeStore
	rpc.header.id = *NewRandomKademliaID()
	
	rpc.data_size = uint64(len(data))
	rpc.data = data

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)

	SendData(recipient, write_buf)
}

func (network *IPNetwork) SendPingReplyMessage(recipient *Contact, id *KademliaID) {
	var rpc RPCPingReply
	rpc.header.typ = RPCTypePingReply
	rpc.header.id = *id

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)

	SendData(recipient, write_buf)
}

func (network *IPNetwork) SendFindContactReplyMessage(recipient *Contact, id *KademliaID, contacts []Contact) {
	var rpc RPCFindNodeReply
	rpc.header.typ = RPCTypeFindNodeReply
	rpc.header.id = *id

	rpc.contact_count = uint16(len(contacts))
	rpc.contacts = contacts

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)

	SendData(recipient, write_buf)
}

func (network *IPNetwork) SendFindDataReplyMessage(recipient *Contact, id *KademliaID, data []byte, contacts []Contact) {
	var rpc RPCFindValueReply
	rpc.header.typ = RPCTypeFindValue
	rpc.header.id = *id

	rpc.data_size = uint64(len(data))
	rpc.contact_count = uint16(len(contacts))

	rpc.data = data
	rpc.contacts = contacts

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)

	SendData(recipient, write_buf)
}

func (network *IPNetwork) SendStoreReplyMessage(recipient *Contact, id *KademliaID, err RPCError) {
	var rpc RPCStoreReply
	rpc.header.typ = RPCTypeStoreReply
	rpc.header.id = *id
	rpc.header.rpc_error = err

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)

	SendData(recipient, write_buf)
}

