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
	Typ       RPCType
	Node_id   KademliaID
	Rpc_id    KademliaID
	Rpc_error RPCError
}

type RPCPing struct {
	header RPCHeader
}

type RPCStore struct {
	header    RPCHeader
	data_size uint64
	data      []byte
}

type RPCFindNode struct {
	header         RPCHeader
	target_node_id KademliaID
}

type RPCFindValue struct {
	header        RPCHeader
	target_key_id KademliaID
}

type RPCPingReply struct {
	header RPCHeader
}

type RPCStoreReply struct {
	header RPCHeader
}

type KademliaTriple struct {
	id       KademliaID
	addr_len uint64
	address  string
}

// returns either data or triples
type RPCFindReply struct {
	header        RPCHeader
	data_size     uint64
	contact_count uint16
	data          []byte
	contacts      []KademliaTriple
}

// max parallel RPCS
const alpha = 3

type Reply struct {
	rpc_id KademliaID

	// only important for handling full bucket updates
	remove_from_bucket_if_timeout bool
	node_id_to_remove             KademliaID
	contact_to_add_if_remove      Contact
}

type Kademlia struct {
	routingTable *RoutingTable
	kv_store     map[KademliaID][]byte

	mu_reply_list sync.Mutex
	reply_list    []Reply

	find_responses chan RPCFindReply

	net Node
}

func NewKademlia(address string, net Node) Kademlia {
	var k Kademlia
	id := NewRandomKademliaID()
	k.routingTable = NewRoutingTable(NewContact(id, address))
	k.kv_store = make(map[KademliaID][]byte)
	k.find_responses = make(chan RPCFindReply, 4*alpha)
	k.net = net
	return k
}

func (kademlia *Kademlia) RemoveIfInReplyList(id KademliaID) bool {
	kademlia.mu_reply_list.Lock()
	defer kademlia.mu_reply_list.Unlock()
	reply_len := len(kademlia.reply_list)

	for i := 0; i < reply_len; i += 1 {
		if id.Equals(&kademlia.reply_list[i].rpc_id) {
			kademlia.reply_list[i] = kademlia.reply_list[reply_len-1]
			kademlia.reply_list = kademlia.reply_list[:reply_len-1]
			return true
		}
	}
	return false
}

func (kademlia *Kademlia) AddToReplyList(reply Reply) {
	kademlia.mu_reply_list.Lock()
	kademlia.reply_list = append(kademlia.reply_list, reply)
	kademlia.mu_reply_list.Unlock()
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

func (kademlia *Kademlia) BucketUpdate(address string, node_id KademliaID) {
	bucket_index := kademlia.routingTable.getBucketIndex(&node_id)
	bucket := kademlia.routingTable.buckets[bucket_index]

	if bucket.Len() != bucketSize {
		bucket.AddContact(Contact{&node_id, address, nil})
	} else {
		exists := bucket.AddContact(Contact{&node_id, address, nil})
		if !exists {
			front_contact := bucket.list.Front()
			kademlia.SendPingMessage(front_contact.Value.(Contact).Address, true)
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
		case RPCTypeInvalid:
			{
				log.Printf("[%v] invalid\n", meaddr)
			}
		case RPCTypePing:
			{
				log.Printf("[%v] ping\n", meaddr)
				kademlia.SendPingReplyMessage(response.from_address, &header.Rpc_id)
				// TODO maybe update bucket
			}
		case RPCTypeStore:
			{
				log.Printf("[%v] store\n", meaddr)
				var store RPCStore
				{
					store.header = header
					err = PartialRead(reader, &store.data_size)
					if err != nil {
						log.Fatalf("%v\n", err)
					}
					store.data = make([]byte, store.data_size)
					reader.Read(store.data)
				}

				key := Sha1toKademlia(store.data)
				kademlia.kv_store[*key] = store.data
				// TODO maybe send back a error if it failed to store
				kademlia.SendStoreReplyMessage(response.from_address, &header.Rpc_id, RPCErrorNoError)
			}
		case RPCTypeFindNode:
			{
				log.Printf("[%v] find_node\n", meaddr)
				var find_node RPCFindNode
				{
					find_node.header = header
					PartialRead(reader, &find_node.target_node_id)
					if err != nil {
						log.Fatalf("%v\n", err)
					}
				}
				contacts := kademlia.routingTable.FindClosestContacts(&find_node.target_node_id, bucketSize)
				kademlia.SendFindContactReplyMessage(response.from_address, &header.Rpc_id, contacts)
			}
		case RPCTypeFindValue:
			{
				log.Printf("[%v] find_value\n", meaddr)
				var find_value RPCFindValue

				{
					find_value.header = header
					PartialRead(reader, &find_value.target_key_id)
					if err != nil {
						log.Fatalf("%v\n", err)
					}
				}

				if data, exists := kademlia.kv_store[find_value.target_key_id]; exists {
					kademlia.SendFindDataReplyMessage(response.from_address, &header.Rpc_id, data, nil)
				} else {
					contacts := kademlia.routingTable.FindClosestContacts(&find_value.target_key_id, bucketSize)
					kademlia.SendFindDataReplyMessage(response.from_address, &header.Rpc_id, nil, contacts)
				}
			}
		case RPCTypePingReply:
			{
				log.Printf("[%v] ping_reply\n", meaddr)

				if kademlia.RemoveIfInReplyList(header.Rpc_id) {
					// var ping_reply RPCPingReply
					kademlia.BucketUpdate(response.from_address, header.Node_id)
				} else {
					log.Printf("Got unexpected ping reply, might have timed out\n")
				}
			}
		case RPCTypeStoreReply:
			{
				log.Printf("[%v] store_reply\n", meaddr)
				if kademlia.RemoveIfInReplyList(header.Rpc_id) {
					// var store_reply RPCStoreReply
					kademlia.BucketUpdate(response.from_address, header.Node_id)
					//TODO: maybe handle errors or smth
				} else {
					log.Printf("Got unexpected store reply, might have timed out\n")
				}
			}
		case RPCTypeFindNodeReply:
			{
				log.Printf("[%v] find_node_reply\n", meaddr)
				if kademlia.RemoveIfInReplyList(header.Rpc_id) {
					var find_node_reply RPCFindReply
					{
						find_node_reply.header = header
						err = PartialRead(reader, &find_node_reply.data_size)
						if err != nil {
							log.Fatalf("%v\n", err)
						}
						err = PartialRead(reader, &find_node_reply.contact_count)
						if err != nil {
							log.Fatalf("%v\n", err)
						}
						find_node_reply.contacts = make([]KademliaTriple, find_node_reply.contact_count)
						for i := 0; i < int(find_node_reply.contact_count); i += 1 {
							var tri KademliaTriple
							PartialRead(reader, &tri.id)
							if err != nil {
								log.Fatalf("%v\n", err)
							}
							PartialRead(reader, &tri.addr_len)
							if err != nil {
								log.Fatalf("%v\n", err)
							}
							b := make([]byte, tri.addr_len)
							err = binary.Read(reader, binary.NativeEndian, &b)
							if err != nil {
								log.Fatalf("%v\n", err)
							}

							find_node_reply.contacts[i] = tri
						}
					}

					kademlia.BucketUpdate(response.from_address, header.Node_id)
					kademlia.find_responses <- find_node_reply
				} else {
					log.Printf("[%v] Got unexpected find node reply, might have timed out\n", meaddr)
				}
			}
		case RPCTypeFindValueReply:
			{
				log.Printf("[%v] find_value_reply\n", meaddr)
				if kademlia.RemoveIfInReplyList(header.Rpc_id) {
					var find_value_reply RPCFindReply
					{
						find_value_reply.header = header
						err = PartialRead(reader, &find_value_reply.data_size)
						if err != nil {
							log.Fatalf("%v\n", err)
						}
						if find_value_reply.data_size == 0 {
							err = PartialRead(reader, &find_value_reply.contact_count)
							if err != nil {
								log.Fatalf("%v\n", err)
							}
							find_value_reply.contacts = make([]KademliaTriple, find_value_reply.contact_count)
							for i := 0; i < int(find_value_reply.contact_count); i += 1 {
								var tri KademliaTriple
								PartialRead(reader, &tri)
								if err != nil {
									log.Fatalf("%v\n", err)
								}
								find_value_reply.contacts[i] = tri
							}
						} else {
							find_value_reply.data = make([]byte, find_value_reply.data_size)
							reader.Read(find_value_reply.data)
						}
					}

					kademlia.BucketUpdate(response.from_address, header.Node_id)
					kademlia.find_responses <- find_value_reply
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

	bytes := make([]byte, IDLength-(byte_pos+1))
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

func (kademlia *Kademlia) LookupData(hash string) ([]byte, bool, []Contact) {
	key := NewKademliaID(hash)
	shortlist := kademlia.routingTable.FindClosestContacts(key, alpha)
	sortByDistance(shortlist, key)

	queried := make(map[string]Contact)

	var foundData []byte
	var nodesWithoutData []Contact // track nodes that didnt have the data

	unchangedRounds := 0
	dataFound := false

	for unchangedRounds < 3 && foundData == nil {
		toQuery := selectUnqueriedNodes(shortlist, queried, alpha)

		for _, c := range toQuery {
			queried[c.ID.String()] = c
		}

		oldClosest := shortlist[0]

		if len(toQuery) == 0 {
			break
		}

		responses := kademlia.queryDataNodes(toQuery, hash)

		// check if value is found
		for i, response := range responses {
			if len(response.data) > 0 && !dataFound {
				foundData = response.data
				dataFound = true
			} else {
				nodesWithoutData = append(nodesWithoutData, toQuery[i])
			}
			// if the data wasnt found, merge the found contacts (just like lookupContact)
			if !dataFound && response.contacts != nil {
				var newContacts []Contact
				for _, triple := range response.contacts {
					newContacts = append(newContacts, Contact{&triple.id, triple.address, nil})
				}
				shortlist = mergeAndSort(shortlist, newContacts, key)
			}
		}

		if dataFound {
			break
		}

		if shortlist[0].ID.Equals(oldClosest.ID) {
			unchangedRounds++
		} else {
			unchangedRounds = 0
		}
	}

	// store to closest not without the data
	if dataFound && len(nodesWithoutData) > 0 {
		for i := range nodesWithoutData {
			nodesWithoutData[i].CalcDistance(key)
		}
		sortByDistance(nodesWithoutData, key)
		closestNode := nodesWithoutData[0]
		kademlia.SendStoreMessage(closestNode.Address, foundData)
	}

	if dataFound {
		return foundData, true, nil
	} else {
		return nil, false, getTopContacts(shortlist, bucketSize)
	}
}

func (kademlia *Kademlia) LookupContact(target *Contact) []Contact {

	// "The first alpha contacts selected are used to create a shortlist for the search."
	initalCandidates := kademlia.routingTable.FindClosestContacts(target.ID, alpha)
	shortlist := make([]Contact, len(initalCandidates))
	copy(shortlist, initalCandidates)

	sortByDistance(shortlist, target.ID)
	queried := make(map[string]Contact)

	unchangedRounds := 0

	for unchangedRounds < 3 {
		toQuery := selectUnqueriedNodes(shortlist, queried, alpha) // helper to select nodes that hasnt been queried already

		for _, contact := range toQuery {
			queried[contact.ID.String()] = contact
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

func (kademlia *Kademlia) Store(data []byte) error {

	key := Sha1toKademlia(data)

	kademlia.kv_store[*key] = data

	target := NewContact(key, "")
	closestContacts := kademlia.LookupContact(&target) // find k closest contacts to send store rpc to

	if len(closestContacts) == 0 {
		return fmt.Errorf("no nodes found for storage replication")
	}

	for _, c := range closestContacts {
		kademlia.SendStoreMessage(c.Address, data)
	}

	return nil
}

func (kademlia *Kademlia) queryNodes(contactsToQuery []Contact, targetID *KademliaID) []Contact {
	length := len(contactsToQuery)
	if length > alpha {
		panic("illegal")
	}

	var responses []Contact

	// strict parallelism
	for i := 0; i < length; i += 1 {
		go kademlia.SendFindContactMessage(contactsToQuery[i].Address, targetID)
	}

	chan_len := len(kademlia.find_responses)
	for chan_len > 0 {
		response := <-kademlia.find_responses
		for _, triple := range response.contacts {
			c := Contact{&triple.id, triple.address, nil}
			responses = append(responses, c)
		}
	}
	return responses
}

func (kademlia *Kademlia) queryDataNodes(contactsToQuery []Contact, hash string) []RPCFindReply {
	length := len(contactsToQuery)
	if length > alpha {
		panic("illegal")
	}

	var responses []RPCFindReply

	for i := 0; i < length; i++ {
		go kademlia.SendFindDataMessage(contactsToQuery[i].Address, hash)
	}

	for i := 0; i < length; i++ {
		response := <-kademlia.find_responses
		responses = append(responses, response)
	}

	return responses
}

func selectUnqueriedNodes(shortlist []Contact, queried map[string]Contact, n int) []Contact {
	var result []Contact
	for _, contact := range shortlist {
		if _, alreadyQueried := queried[contact.ID.String()]; !alreadyQueried && len(result) < n {
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
func (kademlia *Kademlia) SendPingMessage(address string, remove_from_bucket_if_timeout bool) {
	var rpc RPCPing
	rpc.header.Typ = RPCTypePing
	rpc.header.Rpc_id = *NewRandomKademliaID()
	rpc.header.Node_id = *kademlia.routingTable.me.ID
	var reply Reply
	reply.rpc_id = rpc.header.Rpc_id
	reply.remove_from_bucket_if_timeout = remove_from_bucket_if_timeout
	// TODO: handle timeouts correctly
	kademlia.AddToReplyList(reply)

	write_buf, _ := binary.Append(nil, binary.NativeEndian, rpc)

	kademlia.net.SendData(address, write_buf)
}

func (kademlia *Kademlia) SendFindContactMessage(address string, key *KademliaID) {
	var rpc RPCFindNode
	rpc.header.Typ = RPCTypeFindNode
	rpc.header.Rpc_id = *NewRandomKademliaID()
	rpc.header.Node_id = *kademlia.routingTable.me.ID
	var reply Reply
	reply.rpc_id = rpc.header.Rpc_id
	kademlia.AddToReplyList(reply)

	rpc.target_node_id = *key

	write_buf, _ := binary.Append(nil, binary.NativeEndian, rpc)

	kademlia.net.SendData(address, write_buf)
}

func (kademlia *Kademlia) SendFindDataMessage(address string, hash string) {
	var rpc RPCFindValue
	rpc.header.Typ = RPCTypeFindValue
	rpc.header.Rpc_id = *NewRandomKademliaID()
	rpc.header.Node_id = *kademlia.routingTable.me.ID
	var reply Reply
	reply.rpc_id = rpc.header.Rpc_id
	kademlia.AddToReplyList(reply)

	rpc.target_key_id = *NewKademliaID(hash)
	write_buf, _ := binary.Append(nil, binary.NativeEndian, rpc)

	kademlia.net.SendData(address, write_buf)
}

func (kademlia *Kademlia) SendStoreMessage(address string, data []byte) {
	var rpc RPCStore
	rpc.header.Typ = RPCTypeStore
	rpc.header.Rpc_id = *NewRandomKademliaID()
	rpc.header.Node_id = *kademlia.routingTable.me.ID
	var reply Reply
	reply.rpc_id = rpc.header.Rpc_id
	kademlia.AddToReplyList(reply)

	rpc.data_size = uint64(len(data))
	rpc.data = data

	write_buf, _ := binary.Append(nil, binary.NativeEndian, rpc)

	kademlia.net.SendData(address, write_buf)
}

func (kademlia *Kademlia) SendPingReplyMessage(address string, id *KademliaID) {
	var rpc RPCPingReply
	rpc.header.Typ = RPCTypePingReply
	rpc.header.Rpc_id = *id
	rpc.header.Node_id = *kademlia.routingTable.me.ID

	write_buf, _ := binary.Append(nil, binary.NativeEndian, rpc)

	kademlia.net.SendData(address, write_buf)
}

func (kademlia *Kademlia) SendFindContactReplyMessage(address string, id *KademliaID, contacts []Contact) {
	var rpc RPCFindReply
	rpc.header.Typ = RPCTypeFindNodeReply
	rpc.header.Rpc_id = *id
	rpc.header.Node_id = *kademlia.routingTable.me.ID

	rpc.contact_count = uint16(len(contacts))
	for _, c := range contacts {
		rpc.contacts = append(rpc.contacts, KademliaTriple{*c.ID, uint64(len(c.Address)), c.Address})
	}

	var write_buf []byte
	var err error

	write_buf, err = binary.Append(write_buf, binary.NativeEndian, rpc.header)
	if err != nil {
		log.Fatalf("%v\n", err)
	}
	write_buf, err = binary.Append(write_buf, binary.NativeEndian, rpc.data_size)
	if err != nil {
		log.Fatalf("%v\n", err)
	}
	write_buf, err = binary.Append(write_buf, binary.NativeEndian, rpc.contact_count)
	if err != nil {
		log.Fatalf("%v\n", err)
	}
	for _, c := range rpc.contacts {
		write_buf, err = binary.Append(write_buf, binary.NativeEndian, c.id)
		if err != nil {
			log.Fatalf("%v\n", err)
		}
		write_buf, err = binary.Append(write_buf, binary.NativeEndian, c.addr_len)
		if err != nil {
			log.Fatalf("%v\n", err)
		}
		write_buf = append(write_buf, []byte(c.address)...)
	}
	kademlia.net.SendData(address, write_buf)
}

func (kademlia *Kademlia) SendFindDataReplyMessage(address string, id *KademliaID, data []byte, contacts []Contact) {
	var rpc RPCFindReply
	rpc.header.Typ = RPCTypeFindValueReply
	rpc.header.Rpc_id = *id
	rpc.header.Node_id = *kademlia.routingTable.me.ID

	rpc.data_size = uint64(len(data))
	rpc.contact_count = uint16(len(contacts))

	if rpc.data_size > 0 && rpc.contact_count > 0 {
		log.Fatalf("FindData Rpc can either have data or contacts not both")
	}

	//var write_buf []byte
	//var err error

	write_buf, err := binary.Append(nil, binary.NativeEndian, rpc.header)
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	write_buf, err = binary.Append(write_buf, binary.NativeEndian, rpc.data_size)
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	if rpc.data_size == 0 {
		write_buf, err = binary.Append(write_buf, binary.NativeEndian, rpc.contact_count)
		if err != nil {
			log.Fatalf("%v\n", err)
		}
		for _, c := range contacts {
			write_buf, err = binary.Append(write_buf, binary.NativeEndian, c.ID)
			if err != nil {
				log.Fatalf("%v\n", err)
			}
			addrLen := uint64(len(c.Address))
			write_buf, err = binary.Append(write_buf, binary.NativeEndian, addrLen)
			if err != nil {
				log.Fatalf("%v\n", err)
			}
			write_buf = append(write_buf, c.Address...)
		}
	} else {
		write_buf = append(write_buf, data...)
	}

	kademlia.net.SendData(address, write_buf)
}

func (kademlia *Kademlia) SendStoreReplyMessage(address string, id *KademliaID, err RPCError) {
	var rpc RPCStoreReply
	rpc.header.Typ = RPCTypeStoreReply
	rpc.header.Rpc_id = *id
	rpc.header.Node_id = *kademlia.routingTable.me.ID
	rpc.header.Rpc_error = err

	write_buf, _ := binary.Append(nil, binary.NativeEndian, rpc)

	kademlia.net.SendData(address, write_buf)
}
