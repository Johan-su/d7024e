package kademlia

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"reflect"
	"sort"
	"sync"
)

func assertPanic(c bool, format string, a ...any) {
	if (!c) {
		log.Panicf(format, a...)
	}
}

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
	RPCErrorNoNodesFound
)

type RPCHeader struct {
	Typ RPCType
	NodeId KademliaID
	RpcId KademliaID
	RpcError RPCError
}

type RPCPing struct {
	header RPCHeader
}

type RPCStore struct {
	header    RPCHeader
	dataSize uint64
	data      []byte
}

type RPCFindNode struct {
	header         RPCHeader
	targetNodeId KademliaID
}

type RPCFindValue struct {
	header        RPCHeader
	targetKeyId KademliaID
}

type RPCPingReply struct {
	header RPCHeader
}

type RPCStoreReply struct {
	header RPCHeader
}

type KademliaTriple struct {
	id KademliaID
	addrLen uint64
	address string
}

// returns either data or triples
type RPCFindReply struct {
	header        RPCHeader
	dataSize     uint64
	contactCount uint16
	data          []byte
	contacts      []KademliaTriple
}

// max parallel RPCS
const alpha = 3

type Reply struct {
	RpcId KademliaID

	// only important for handling full bucket updates 
	removeFromBucketIfTimeout bool 
	nodeIdToRemove KademliaID
	contactToAddIfRemove Contact 
}

type Value struct {
	dat []byte
	exists bool
}

type Kademlia struct {
	routingTable *RoutingTable
	kvStore map[KademliaID]Value

	muReplyList sync.Mutex
	replyList []Reply

	findNodeResponses chan RPCFindReply
	findValueResponses chan RPCFindReply

	net Node
}

func NewKademlia(address string, net Node) Kademlia {
	var k Kademlia
	id := NewRandomKademliaID()
	k.routingTable = NewRoutingTable(NewContact(id, address))
	k.kvStore = make(map[KademliaID]Value) 
	k.findNodeResponses = make(chan RPCFindReply, 4*alpha)
	k.findValueResponses = make(chan RPCFindReply, 4*alpha)
	k.net = net
	return k
}

func (kademlia *Kademlia) RemoveIfInReplyList(id KademliaID) bool {
	kademlia.muReplyList.Lock()
	defer kademlia.muReplyList.Unlock()
	reply_len := len(kademlia.replyList)
	
	for i := 0; i < reply_len; i += 1 {
		if (id.Equals(&kademlia.replyList[i].RpcId)) {
			kademlia.replyList[i] = kademlia.replyList[reply_len - 1]
			kademlia.replyList = kademlia.replyList[:reply_len - 1]
			return true
		}
	}
	return false
}

func (kademlia *Kademlia) AddToReplyList(reply Reply) {
	kademlia.muReplyList.Lock()
	kademlia.replyList = append(kademlia.replyList, reply)
	kademlia.muReplyList.Unlock()
}

func PartialRead[T any](reader *bytes.Reader, val *T) error {
	len := reflect.TypeOf(*val).Size()
	buf := make([]byte, len)
	reader.Read(buf)
	_, err := binary.Decode(buf, binary.NativeEndian, val)
	if err != nil {
		return err
	}
	return nil
}

//TODO: both Find RPCS can reply with lists that has the sender id in it.
// right now we ignore attempts for a node to add itself to its own routing table 
func (kademlia *Kademlia) BucketUpdate(address string, node_id KademliaID) {
	if node_id.Equals(kademlia.routingTable.me.ID) {
		// ignore attempts to add itself to contact
		return
		// log.Panicf("%v %s, tried to add itself to routingTable", address, node_id.String())
	}
	bucket_index := kademlia.routingTable.getBucketIndex(&node_id)
	bucket := kademlia.routingTable.buckets[bucket_index]

	if (bucket.Len() != bucketSize) {
		bucket.AddContact(Contact{&node_id, address, nil})
	} else {
		exists := bucket.AddContact(Contact{&node_id, address, nil})
		if !exists {
			front_contact := bucket.list.Front()
			//TODO ignore for now
			if false {
				kademlia.SendPingMessage(front_contact.Value.(Contact).Address, true)
			}
		}
	}
}

func (kademlia *Kademlia) HandleResponse() {
	var err error
	meaddr := kademlia.routingTable.me.Address
	for {
		response := kademlia.net.Listen(meaddr)
		reader := bytes.NewReader(response.data)
		var header RPCHeader
		err = PartialRead(reader, &header)
		if err != nil {
			log.Printf("%v\n", err)
		}
		// receiving
		switch header.Typ {
			case RPCTypeInvalid: {
				log.Printf("[%v] invalid\n", meaddr)
			}
			case RPCTypePing: {
				log.Printf("[%v] ping\n", meaddr)
				kademlia.BucketUpdate(response.from_address, header.NodeId)
				kademlia.SendPingReplyMessage(response.from_address, &header.RpcId)
			}
			case RPCTypeStore: {
				log.Printf("[%v] store\n", meaddr)
				kademlia.BucketUpdate(response.from_address, header.NodeId)
				var store RPCStore
				{
					store.header = header
					err = PartialRead(reader, &store.dataSize)
					if err != nil {
						log.Fatalf("%v\n", err)
					}
					store.data = make([]byte, store.dataSize)
					reader.Read(store.data)
				}
				
				key := Sha1toKademlia(store.data)
				kademlia.kvStore[*key] = Value{store.data, true}
				// TODO maybe send back a error if it failed to store
				kademlia.SendStoreReplyMessage(response.from_address, &header.RpcId, RPCErrorNoError)
			}
			case RPCTypeFindNode: {
				log.Printf("[%v] find_node\n", meaddr)
				kademlia.BucketUpdate(response.from_address, header.NodeId)
				var find_node RPCFindNode
				{
					find_node.header = header
					PartialRead(reader, &find_node.targetNodeId)
					if err != nil {
						log.Fatalf("%v\n", err)
					}
				}
				contacts := kademlia.routingTable.FindClosestContacts(&find_node.targetNodeId, bucketSize)
				kademlia.SendFindContactReplyMessage(response.from_address, &header.RpcId, contacts)
			}
			case RPCTypeFindValue: {
				log.Printf("[%v] find_value\n", meaddr)
				kademlia.BucketUpdate(response.from_address, header.NodeId)
				var find_value RPCFindValue
				
				var bytes []byte
				var contacts []Contact

				bytes, _, ok := kademlia.LookupData(find_value.targetKeyId.String())
				if !ok {
					contacts = kademlia.routingTable.FindClosestContacts(&find_value.targetKeyId, bucketSize)
				}
				kademlia.SendFindDataReplyMessage(response.from_address, &header.RpcId, bytes, contacts)
			}
			case RPCTypePingReply: {
				log.Printf("[%v] ping_reply\n", meaddr)
				
				if kademlia.RemoveIfInReplyList(header.RpcId) {
					// var ping_reply RPCPingReply
					kademlia.BucketUpdate(response.from_address, header.NodeId)
				} else {
					log.Printf("[%v] Got unexpected ping reply, might have timed out\n", meaddr)
				}
			}
			case RPCTypeStoreReply: {
				log.Printf("[%v] store_reply\n", meaddr)
				if kademlia.RemoveIfInReplyList(header.RpcId) {
					// var store_reply RPCStoreReply
					kademlia.BucketUpdate(response.from_address, header.NodeId)
					//TODO: maybe handle errors or smth
				} else {
					log.Printf("Got unexpected store reply, might have timed out\n")
				}
			}
			case RPCTypeFindNodeReply: {
				log.Printf("[%v] find_node_reply\n", meaddr)
				if kademlia.RemoveIfInReplyList(header.RpcId) {
					var find_node_reply RPCFindReply
					{
						find_node_reply.header = header
						err = PartialRead(reader, &find_node_reply.dataSize)
						if err != nil {
							log.Fatalf("%v\n", err)
						}
						err = PartialRead(reader, &find_node_reply.contactCount)
						if err != nil {
							log.Fatalf("%v\n", err)
						}
						find_node_reply.contacts = make([]KademliaTriple, find_node_reply.contactCount)
						for i := 0; i < int(find_node_reply.contactCount); i += 1 {
							var tri KademliaTriple
							PartialRead(reader, &tri.id)
							if err != nil {
								log.Fatalf("%v\n", err)
							}
							PartialRead(reader, &tri.addrLen)
							if err != nil {
								log.Fatalf("%v\n", err)
							}
							b := make([]byte, tri.addrLen)
							err = binary.Read(reader, binary.NativeEndian, &b)
							if err != nil {
								log.Fatalf("%v\n", err)
							}
							tri.address = string(b)

							find_node_reply.contacts[i] = tri
						}
					}

					kademlia.BucketUpdate(response.from_address, header.NodeId)
					kademlia.findNodeResponses <- find_node_reply
				} else {
					log.Printf("[%v] Got unexpected find node reply, might have timed out\n", meaddr)
				}
			}
			case RPCTypeFindValueReply: {
				log.Printf("[%v] find_value_reply\n", meaddr)
				if kademlia.RemoveIfInReplyList(header.RpcId) {
					var find_value_reply RPCFindReply
					{
						find_value_reply.header = header
						err = PartialRead(reader, &find_value_reply.dataSize)
						if err != nil {
							log.Fatalf("%v\n", err)
						}
						if find_value_reply.dataSize == 0 {
							err = PartialRead(reader, &find_value_reply.contactCount)
							if err != nil {
								log.Fatalf("%v\n", err)
							}
							find_value_reply.contacts = make([]KademliaTriple, find_value_reply.contactCount)
							for i := 0; i < int(find_value_reply.contactCount); i += 1 {
								var tri KademliaTriple
								PartialRead(reader, &tri)
								if err != nil {
									log.Fatalf("%v\n", err)
								}
								find_value_reply.contacts[i] = tri
							}
						} else {
							find_value_reply.data = make([]byte, find_value_reply.dataSize)
							reader.Read(find_value_reply.data)
						}
					}

					kademlia.BucketUpdate(response.from_address, header.NodeId)
					kademlia.findValueResponses <- find_value_reply
				} else {
					log.Printf("Got unexpected find value reply, might have timed out\n")
				}
			}
		}

	}
}

func (kademlia *Kademlia) Refresh(bucketIndex int) {
	bit_pos := bucketIndex % 8
	byte_pos := bucketIndex / 8


	var id KademliaID

	var partially_random byte
	partially_random = 1 << bit_pos
	
	for i := bit_pos + 1; i < 8; i += 1 {
		if rand.Float32() > 0.5 {
			partially_random |= 1 << i
		}
	}

	bytes := make([]byte, IDLength - (byte_pos + 1))
	rand.Read(bytes)

	j := 0
	for i := byte_pos + 1; i < IDLength; i += 1 {
		id[i] = bytes[j]
		j += 1
	}


	kademlia.LookupContact(&Contact{&id, "", nil})
}

func (kademlia *Kademlia) Join(bootstrapContact Contact) {

	kademlia.routingTable.AddContact(bootstrapContact)
	closestContacts := kademlia.LookupContact(&kademlia.routingTable.me)
	
	bucketIndicies := make([]int, len(closestContacts))

	for i, c := range closestContacts {
		bucketIndicies[i] = kademlia.routingTable.getBucketIndex(c.ID)
	}

	sort.Slice(bucketIndicies, func(i, j int) bool {
		return bucketIndicies[i] < bucketIndicies[j]
	})

	for i := 1; i < len(bucketIndicies); i += 1 {
		kademlia.Refresh(bucketIndicies[i])
	}
}

func (kademlia *Kademlia) LookupData(hash string) ([]byte, KademliaID, bool) {

	target := NewKademliaID(hash)
	val := kademlia.kvStore[*target]
	if val.exists {
		return val.dat, *kademlia.routingTable.me.ID, true
	} 
	// "The first alpha contacts selected are used to create a shortlist for the search."
	initalCandidates := kademlia.routingTable.FindClosestContacts(target, alpha)
	if len(initalCandidates) == 0 {
		return nil, KademliaID{}, false
	}
	shortlist := make([]Contact, len(initalCandidates))
	copy(shortlist, initalCandidates)

	sortByDistance(shortlist, target)
	queried := make(map[string]bool)

	unchangedRounds := 0

	for unchangedRounds < 3 {
		toQuery := selectUnqueriedNodes(shortlist, queried, alpha) // helper to select nodes that hasnt been queried already

		for _, contact := range toQuery {
			queried[contact.ID.String()] = true
		}

		oldClosest := shortlist[0]

		if len(toQuery) == 0 {
			break
		}

		contacts, data, foundId, foundData := kademlia.queryFindNodes(toQuery, *target)
		if foundData {
			return data, foundId, foundData
		}
		shortlist = mergeAndSort(shortlist, contacts, target)

		if shortlist[0].ID.Equals(oldClosest.ID) {
			unchangedRounds++
		} else {
			unchangedRounds = 0
		}

	}
	return nil, KademliaID{}, false
}

func (kademlia *Kademlia) LookupContact(target *Contact) []Contact {

	// "The first alpha contacts selected are used to create a shortlist for the search."
	initalCandidates := kademlia.routingTable.FindClosestContacts(target.ID, alpha)
	if len(initalCandidates) == 0 {
		return nil
	}
	shortlist := make([]Contact, len(initalCandidates))
	copy(shortlist, initalCandidates)

	sortByDistance(shortlist, target.ID)
	queried := make(map[string]bool)

	unchangedRounds := 0

	for unchangedRounds < 3 {
		toQuery := selectUnqueriedNodes(shortlist, queried, alpha) // helper to select nodes that hasnt been queried already

		for _, contact := range toQuery {
			queried[contact.ID.String()] = true
		}

		oldClosest := shortlist[0]

		if len(toQuery) == 0 {
			break
		}

		responselist := kademlia.queryNodes(toQuery, target.ID)
		shortlist = mergeAndSort(shortlist, responselist, target.ID)

		if shortlist[0].ID.Equals(oldClosest.ID) {
			unchangedRounds++
		} else {
			unchangedRounds = 0
		}

	}
	return getTopContacts(shortlist, bucketSize)
}

func (kademlia *Kademlia) Store(data []byte) (KademliaID, error) {

	key := Sha1toKademlia(data)

	kademlia.kvStore[*key] = Value{data, true}

	target := NewContact(key, "")
	closestContacts := kademlia.LookupContact(&target) // find k closest contacts to send store rpc to

	if len(closestContacts) == 0 {
		return *key, fmt.Errorf("no nodes found for storage replication")
	}

	for _, c := range closestContacts {
		go kademlia.SendStoreMessage(c.Address, data)
	}

	return *key, nil
}

func (kademlia *Kademlia) queryFindNodes(contactsToQuery []Contact, targetHash KademliaID) ([]Contact, []byte, KademliaID, bool) {
	length := len(contactsToQuery)
	assertPanic(length <= alpha, "Illegal")

	var responses []Contact

	// strict parallelism
	for i := 0; i < length; i += 1 {
		go kademlia.SendFindDataMessage(contactsToQuery[i].Address, targetHash)
	}

	foundData := false
	var data []byte
	var foundId KademliaID

	//TODO handle timeouts/packet loss
	for i := 0; i < length; i += 1 {
		response := <- kademlia.findValueResponses
		if response.dataSize == 0 {
			for _, triple := range response.contacts {
				c := Contact{&triple.id, triple.address, nil}
				responses = append(responses, c)
			}
		} else if !foundData {
			foundData = true
			foundId = response.header.NodeId
			data = response.data
		}
	}
	return responses, data, foundId, foundData
}


func (kademlia *Kademlia) queryNodes(contactsToQuery []Contact, targetID *KademliaID) []Contact {
	length := len(contactsToQuery)
	assertPanic(length <= alpha, "Illegal")

	var responses []Contact

	// strict parallelism
	for i := 0; i < length; i += 1 {
		go kademlia.SendFindContactMessage(contactsToQuery[i].Address, targetID)
	}

	//TODO handle timeouts/packet loss
	for i := 0; i < length; i += 1 {
		response := <- kademlia.findNodeResponses
		for _, triple := range response.contacts {
			c := Contact{&triple.id, triple.address, nil}
			responses = append(responses, c)
		}
	}
	return responses
}

func selectUnqueriedNodes(shortlist []Contact, queried map[string]bool, n int) []Contact {
var result []Contact
	for _, contact := range shortlist {
		if !queried[contact.ID.String()] && len(result) < n {
			result = append(result, contact)
		}
	}
	return result
}

func mergeAndSort(shortlist, newContacts []Contact, target *KademliaID) []Contact {
	combined := append(shortlist, newContacts...)

	seen := make(map[string]bool)
	unique := make([]Contact, 0, len(combined))
	for _, c := range combined {
		id := c.ID.String()
		if !seen[id] {
			seen[id] = true
			unique = append(unique, c)
		}
	}

	sortByDistance(unique, target)
	if len(unique) > bucketSize {
		return unique[:bucketSize]
	}
	return unique
}

func getTopContacts(contacts []Contact, n int) []Contact {
	if n > len(contacts) {
		return contacts
	}
	return contacts[:n]
}

func sortByDistance(contacts []Contact, target *KademliaID) {
	for i := range contacts {
		contacts[i].CalcDistance(target)
	}

	sort.Slice(contacts, func(i, j int) bool {
		return contacts[i].Less(&contacts[j])
	})
}

// RPCS as defined in the kademlia spec
func (kademlia *Kademlia) SendPingMessage(address string, removeFromBucketIfTimeout bool) {
	var rpc RPCPing
	rpc.header.Typ = RPCTypePing
	rpc.header.RpcId = *NewRandomKademliaID()
	rpc.header.NodeId = *kademlia.routingTable.me.ID
	var reply Reply
	reply.RpcId = rpc.header.RpcId
	reply.removeFromBucketIfTimeout = removeFromBucketIfTimeout
	// TODO: handle timeouts correctly
	kademlia.AddToReplyList(reply) 
	
	writeBuf, err := binary.Append(nil, binary.NativeEndian, rpc)
	assertPanic(err == nil, "%v\n", err)

	kademlia.net.SendData(address, writeBuf)
}

func (kademlia *Kademlia) SendFindContactMessage(address string, key *KademliaID) {
	var rpc RPCFindNode
	rpc.header.Typ = RPCTypeFindNode
	rpc.header.RpcId = *NewRandomKademliaID()
	rpc.header.NodeId = *kademlia.routingTable.me.ID
	var reply Reply
	reply.RpcId = rpc.header.RpcId
	kademlia.AddToReplyList(reply) 

	rpc.targetNodeId = *key

	writeBuf, err := binary.Append(nil, binary.NativeEndian, rpc)
	assertPanic(err == nil, "%v\n", err)


	kademlia.net.SendData(address, writeBuf)
}

func (kademlia *Kademlia) SendFindDataMessage(address string, targetKey KademliaID) {
	var rpc RPCFindValue
	rpc.header.Typ = RPCTypeFindValue
	rpc.header.RpcId = *NewRandomKademliaID()
	rpc.header.NodeId = *kademlia.routingTable.me.ID
	var reply Reply
	reply.RpcId = rpc.header.RpcId
	kademlia.AddToReplyList(reply) 

	rpc.targetKeyId = targetKey
	writeBuf, err := binary.Append(nil, binary.NativeEndian, rpc)
	assertPanic(err == nil, "%v\n", err)


	kademlia.net.SendData(address, writeBuf)
}

func (kademlia *Kademlia) SendStoreMessage(address string, data []byte) {
	var rpc RPCStore
	rpc.header.Typ = RPCTypeStore
	rpc.header.RpcId = *NewRandomKademliaID()
	rpc.header.NodeId = *kademlia.routingTable.me.ID
	var reply Reply
	reply.RpcId = rpc.header.RpcId
	kademlia.AddToReplyList(reply) 
	
	rpc.dataSize = uint64(len(data))
	rpc.data = data

	var writeBuf []byte
	var err error


	writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, rpc.header)
	assertPanic(err == nil, "%v\n", err)

	writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, rpc.dataSize)
	assertPanic(err == nil, "%v\n", err)

	writeBuf = append(writeBuf, rpc.data...)

	kademlia.net.SendData(address, writeBuf)
}

func (kademlia *Kademlia) SendPingReplyMessage(address string, id *KademliaID) {
	var rpc RPCPingReply
	rpc.header.Typ = RPCTypePingReply
	rpc.header.RpcId = *id
	rpc.header.NodeId = *kademlia.routingTable.me.ID

	writeBuf, err := binary.Append(nil, binary.NativeEndian, rpc)
	assertPanic(err == nil, "%v\n", err)

	kademlia.net.SendData(address, writeBuf)
}

func (kademlia *Kademlia) SendFindContactReplyMessage(address string, id *KademliaID, contacts []Contact) {
	var rpc RPCFindReply
	rpc.header.Typ = RPCTypeFindNodeReply
	rpc.header.RpcId = *id
	rpc.header.NodeId = *kademlia.routingTable.me.ID

	rpc.contactCount = uint16(len(contacts))
	for _, c := range contacts {
		rpc.contacts = append(rpc.contacts, KademliaTriple{*c.ID, uint64(len(c.Address)), c.Address})
	}

	var writeBuf []byte
	var err error

	writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, rpc.header)
	assertPanic(err == nil, "%v\n", err)

	writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, rpc.dataSize)
	assertPanic(err == nil, "%v\n", err)

	writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, rpc.contactCount)
	assertPanic(err == nil, "%v\n", err)

	for _, c := range rpc.contacts {
		writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, c.id)
		assertPanic(err == nil, "%v\n", err)

		writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, c.addrLen)
		assertPanic(err == nil, "%v\n", err)

		writeBuf = append(writeBuf, []byte(c.address)...)
	}
	kademlia.net.SendData(address, writeBuf)
}

func (kademlia *Kademlia) SendFindDataReplyMessage(address string, id *KademliaID, data []byte, contacts []Contact) {
	var rpc RPCFindReply
	rpc.header.Typ = RPCTypeFindValueReply
	rpc.header.RpcId = *id
	rpc.header.NodeId = *kademlia.routingTable.me.ID

	rpc.dataSize = uint64(len(data))
	rpc.contactCount = uint16(len(contacts))


	if (rpc.dataSize > 0 && rpc.contactCount > 0) {
		log.Fatalf("FindData Rpc can either have data or contacts not both")
	}

	rpc.data = data
	for _, c := range contacts {
		rpc.contacts = append(rpc.contacts, KademliaTriple{*c.ID, uint64(len(c.Address)), c.Address})
	}

	var writeBuf []byte
	var err error

	writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, rpc.header)
	assertPanic(err == nil, "%v\n", err)

	writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, rpc.dataSize)
	assertPanic(err == nil, "%v\n", err)

	writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, rpc.contactCount)
	assertPanic(err == nil, "%v\n", err)

	if rpc.dataSize == 0 {
		for _, c := range rpc.contacts {
			writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, c.id)
			assertPanic(err == nil, "%v\n", err)

			writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, c.addrLen)
			assertPanic(err == nil, "%v\n", err)

			writeBuf = append(writeBuf, []byte(c.address)...)
		}
	} else {
		writeBuf = append(writeBuf, rpc.data...)
	}


	kademlia.net.SendData(address, writeBuf)
}

func (kademlia *Kademlia) SendStoreReplyMessage(address string, id *KademliaID, rpcErr RPCError) {
	var rpc RPCStoreReply
	rpc.header.Typ = RPCTypeStoreReply
	rpc.header.RpcId = *id
	rpc.header.NodeId = *kademlia.routingTable.me.ID
	rpc.header.RpcError = rpcErr

	var writeBuf []byte
	var err error

	writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, rpc)
	assertPanic(err == nil, "%v\n", err)

	kademlia.net.SendData(address, writeBuf)
}
