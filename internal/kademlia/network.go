package kademlia

import (
	"net"
	"log"
	"fmt"
	"encoding/binary"
)

type RPCType uint16
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

type Network struct {
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
	data_hash KademliaID
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

type Triple struct {
	address [4]uint8
	port uint16
	node_id KademliaID 
}

type RPCFindNodeReply struct {
	header RPCHeader
	triple_count uint8	
	triples []Triple
}

// returns either data or triples
type RPCFindValueReply struct {
	header RPCHeader
	data_size uint64
	triple_count uint8
	data []byte
	triples []Triple
}

func Listen(ip string, port int) {

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

		s_buf := string(buf[0:n - 1])

		fmt.Printf("Received %v bytes %v\n", n, s_buf)
		
		if (s_buf == "ping") {
			fmt.Printf("writing ping...\n")
			_, err := conn.WriteTo([]byte("pong"), rec_addr)
			if err != nil {
				log.Fatalf("write error %v\n", err)
			}
			continue
		}
		
		var header RPCHeader
		binary.Decode(buf, binary.BigEndian, header)
		// receiving
		switch header.typ {
			case RPCTypeInvalid: {
				log.Printf("Got Invalid RPC\n")
			}
			case RPCTypePing: {
				var rpc RPCPingReply
				rpc.header.typ = RPCTypePingReply
				rpc.header.id = *NewRandomKademliaID()
				write_buf, err := binary.Append(nil, binary.BigEndian, rpc)
				if err != nil {
					log.Fatalf("Failed %v\n", err)
				}
				_, err = conn.WriteTo(write_buf, rec_addr)
				if err != nil {
					log.Fatalf("RPC ping write error %v\n", err)
				}
			}
			case RPCTypeStore: {
				var store RPCStore
				binary.Decode(buf, binary.BigEndian, store)

				panic("TODO receive Store RPC")
			}
			case RPCTypeFindNode: {
				var find_node RPCFindNode
				binary.Decode(buf, binary.BigEndian, find_node)

				panic("TODO Find node RPC")
			}
			case RPCTypeFindValue: {
				var find_value RPCFindValue
				binary.Decode(buf, binary.BigEndian, find_value)

				panic("TODO Find value RPC")
			}
			case RPCTypePingReply: {
				var ping_reply RPCPingReply
				binary.Decode(buf, binary.BigEndian, ping_reply)
				// update bucket
				panic("TODO update bucket when receiving ping reply")
			}
			case RPCTypeStoreReply: {
				var store_reply RPCStoreReply
				binary.Decode(buf, binary.BigEndian, store_reply)
				// update bucket
				panic("TODO Store RPC reply")
			}
			case RPCTypeFindNodeReply: {
				var find_node_reply RPCFindNodeReply
				binary.Decode(buf, binary.BigEndian, find_node_reply)
				// update bucket
				panic("TODO find node RPC reply")
			}
			case RPCTypeFindValueReply: {
				var find_value_reply RPCFindValueReply
				binary.Decode(buf, binary.BigEndian, find_value_reply)
				// update bucket
				panic("TODO find value RPC reply")
			}
		}
		conn.Close()
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

func (network *Network) SendPingMessage(recipient *Contact) {
	var rpc RPCPing
	rpc.header.typ = RPCTypePing
	rpc.header.id = *NewRandomKademliaID()

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)

	SendData(recipient, write_buf)
}

func (network *Network) SendFindContactMessage(recipient *Contact, key *KademliaID) {
	var rpc RPCFindNode
	rpc.header.typ = RPCTypeFindNode
	rpc.header.id = *NewRandomKademliaID()

	rpc.target_node_id = *key

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)


	SendData(recipient, write_buf)
}

func (network *Network) SendFindDataMessage(recipient *Contact, hash string) {
	var rpc RPCFindValue
	rpc.header.typ = RPCTypeFindValue
	rpc.header.id = *NewRandomKademliaID()

	rpc.target_key_id = *NewKademliaID(hash)
	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)


	SendData(recipient, write_buf)
}

func (network *Network) SendStoreMessage(recipient *Contact, data []byte) {
	var rpc RPCStore
	rpc.header.typ = RPCTypeStore
	rpc.header.id = *NewRandomKademliaID()
	
	rpc.data_hash = *NewKademliaID(string(data))
	rpc.data_size = uint64(len(data))
	rpc.data = data

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)

	SendData(recipient, write_buf)
}
