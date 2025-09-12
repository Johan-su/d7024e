package kademlia

import (
	"crypto/sha1"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"sync"
	"encoding/binary"
	"bytes"
	"reflect"
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

type RPCHeader struct {
	Typ RPCType
	Node_id KademliaID
	Rpc_id KademliaID
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

type RPCFindNodeReply struct {
	header        RPCHeader
	contact_count uint16
	contacts      []Contact
}

// returns either data or triples
type RPCFindValueReply struct {
	header        RPCHeader
	data_size     uint64
	contact_count uint16
	data          []byte
	contacts      []Contact
}

const alpha = 3

type Reply struct {
	rpc_id KademliaID


	// only important for handling full bucket updates 
	remove_from_bucket_if_timeout bool 
	node_id_to_remove KademliaID
	contact_to_add_if_remove Contact 
}

type Kademlia struct {
	routingTable *RoutingTable
	kv_store map[KademliaID][]byte

	mu_reply_list sync.Mutex
	reply_list []Reply

	net Node
}

func NewKademlia(address string, net Node) Kademlia {
	var k Kademlia
	id := NewRandomKademliaID()
	k.routingTable = NewRoutingTable(NewContact(id, address))
	k.kv_store = make(map[KademliaID][]byte) 
	k.net = net
	return k
}

func (kademlia *Kademlia) RemoveIfInReplyList(id KademliaID) bool {
	kademlia.mu_reply_list.Lock()
	defer kademlia.mu_reply_list.Unlock()
	reply_len := len(kademlia.reply_list)
	
	for i := 0; i < reply_len; i += 1 {
		if (id.Equals(&kademlia.reply_list[i].rpc_id)) {
			kademlia.reply_list[i] = kademlia.reply_list[reply_len - 1]
			kademlia.reply_list = kademlia.reply_list[:reply_len - 1]
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

	if (bucket.Len() != bucketSize) {
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
			case RPCTypeInvalid: {
				log.Printf("Got Invalid RPC\n")
			}
			case RPCTypePing: {
				log.Printf("[%v] ping\n", meaddr)
				kademlia.SendPingReplyMessage(response.from_address, &header.Rpc_id)
				// TODO maybe update bucket
			}
			case RPCTypeStore: {
				log.Printf("[%v] store\n", meaddr)
				var store RPCStore
				{
					store.header = header
					err = PartialRead(reader, &store.data_size)
					if err != nil {
						log.Printf("%v\n", err)
					}
					store.data = make([]byte, store.data_size)
					reader.Read(store.data)
				}
				
				kademlia.Store(store.data)
				// TODO maybe send back a error if it failed to store
				kademlia.SendStoreReplyMessage(response.from_address, &header.Rpc_id, RPCErrorNoError)
			}
			case RPCTypeFindNode: {
				log.Printf("[%v] find_node\n", meaddr)
				var find_node RPCFindNode
				contacts := kademlia.routingTable.FindClosestContacts(&find_node.target_node_id, bucketSize)
				kademlia.SendFindContactReplyMessage(response.from_address, &header.Rpc_id, contacts)
			}
			case RPCTypeFindValue: {
				log.Printf("[%v] find_value\n", meaddr)
				var find_value RPCFindValue
				
				var bytes []byte
				var contacts []Contact

				bytes, ok := kademlia.LookupData(find_value.target_key_id.String())
				if !ok {
					contacts = kademlia.routingTable.FindClosestContacts(&find_value.target_key_id, bucketSize)
				}
				kademlia.SendFindDataReplyMessage(response.from_address, &header.Rpc_id, bytes, contacts)
			}
			case RPCTypePingReply: {
				log.Printf("[%v] ping_reply\n", meaddr)
				
				if kademlia.RemoveIfInReplyList(header.Rpc_id) {
					// var ping_reply RPCPingReply
					kademlia.BucketUpdate(response.from_address, header.Node_id)
				} else {
					log.Printf("Got unexpected ping reply, might have timed out\n")
				}
			}
			case RPCTypeStoreReply: {
				log.Printf("[%v] store_reply\n", meaddr)
				if kademlia.RemoveIfInReplyList(header.Rpc_id) {
					// var store_reply RPCStoreReply
					kademlia.BucketUpdate(response.from_address, header.Node_id)
					panic("TODO Store RPC reply")
				} else {
					log.Printf("Got unexpected store reply, might have timed out\n")
				}
			}
			case RPCTypeFindNodeReply: {
				log.Printf("[%v] find_node_reply\n", meaddr)
				if kademlia.RemoveIfInReplyList(header.Rpc_id) {
					// var find_node_reply RPCFindNodeReply
					kademlia.BucketUpdate(response.from_address, header.Node_id)
					panic("TODO find node RPC reply")
				} else {
					log.Printf("[%v] Got unexpected find node reply, might have timed out\n", meaddr)
				}
			}
			case RPCTypeFindValueReply: {
				log.Printf("[%v] find_value_reply\n", meaddr)
				if kademlia.RemoveIfInReplyList(header.Rpc_id) {
					var find_value_reply RPCFindValueReply
					binary.Decode(response.data, binary.NativeEndian, find_value_reply)
					kademlia.BucketUpdate(response.from_address, header.Node_id)
					panic("TODO find value RPC reply")
				} else {
					log.Printf("Got unexpected find value reply, might have timed out\n")
				}
			}
		}

	}
}

func (kademlia *Kademlia) LookupData(hash string) ([]byte, bool) {
	// TODO
	return nil, false
}

func (kademlia *Kademlia) LookupContact(target *Contact) []Contact {
	return kademlia.LookupContactInternal(target, kademlia.mockQueryNodes) // TODO: change to real func that uses SendFindContactMessage to find contact nodes
}

func (kademlia *Kademlia) LookupContactInternal(
	target *Contact,
	queryFn func([]Contact, *KademliaID) map[Contact][]Contact, //function that will return the queried Node and its Contacts
) []Contact {

	// "The first alpha contacts selected are used to create a shortlist for the search."
	initalCandidates := kademlia.routingTable.FindClosestContacts(target.ID, alpha)
	shortlist := make([]Contact, len(initalCandidates))
	copy(shortlist, initalCandidates)

	//write sorting helper
	sortByDistance(shortlist, target.ID)
	queried := make(map[string]bool)

	unchangedRounds := 0

	for unchangedRounds < 3 {
		toQuery := selectUnqueriedNodes(shortlist, queried, bucketSize) // helper to select nodes that hasnt been queried already

		for _, contact := range toQuery {
			queried[contact.ID.String()] = true
		}

		oldClosest := shortlist[0]

		if len(toQuery) == 0 {
			break
		}

		responseMap := queryFn(toQuery, target.ID) // mock until real with network is implemented

		for _, newContacts := range responseMap {
			shortlist = mergeAndSort(shortlist, newContacts, target.ID)
		}

		if shortlist[0].ID.Equals(oldClosest.ID) {
			unchangedRounds++
		} else {
			unchangedRounds = 0
		}

	}
	return getTopContacts(shortlist, bucketSize)

}

func (kademlia *Kademlia) Store(data []byte) error {
	return kademlia.StoreInternal(data, kademlia.mockStoreNodes)
}

func (kademlia *Kademlia) StoreInternal(
	data []byte,
	storeFn func([]Contact, []byte) map[Contact]error,
) error {
	key := Sha1toKademlia(data)

	kademlia.kv_store[*key] = data

	target := NewContact(key, "")
	closestContacts := kademlia.LookupContact(&target) // find k closest contacts to send store rpc to

	if len(closestContacts) == 0 {
		return fmt.Errorf("no nodes found for storage replication")
	}

	// function that sends the store rpc to each contact
	results := storeFn(closestContacts, data)

	var errors []error
	for contact, err := range results {
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to store on node %s: %v", contact.Address, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("store completed with %d errors: %v", len(errors), errors)
	}

	return nil

}

func (kademlia *Kademlia) mockStoreNodes(contacts []Contact, data []byte) map[Contact]error {
	results := make(map[Contact]error)
	var wg sync.WaitGroup
	var mutex sync.Mutex

	// Calculate data hash to compute distances
	hasher := sha1.New()
	hasher.Write(data)
	hashBytes := hasher.Sum(nil)
	dataHash := KademliaID{}
	copy(dataHash[:], hashBytes)

	// Log which nodes we're attempting to store on with distances
	log.Printf("Attempting to store on %d nodes (distance to data hash %s):", len(contacts), dataHash.String()[:8])
	for i, contact := range contacts {
		contact.CalcDistance(&dataHash)
		log.Printf("  %d: %s (distance: %s)", i+1, contact.ID.String()[:8], contact.distance.String()[:8])
	}

	for _, contact := range contacts {
		wg.Add(1)
		go func(c Contact) {
			defer wg.Done()

			// Calculate distance for this specific contact
			c.CalcDistance(&dataHash)

			// Mock storage with 90% success rate
			if rand.Intn(10) < 9 { // 90% success
				log.Printf("Mock store successful on node %s (distance: %s)", c.ID.String()[:8], c.distance.String()[:8])
				mutex.Lock()
				results[c] = nil
				mutex.Unlock()
			} else {
				// Simulate various failure scenarios
				failures := []error{
					fmt.Errorf("node unavailable"),
					fmt.Errorf("storage quota exceeded"),
					fmt.Errorf("network timeout"),
				}
				err := failures[rand.Intn(len(failures))]
				log.Printf("Mock store failed on node %s (distance: %s): %v", c.ID.String()[:8], c.distance.String()[:8], err)
				mutex.Lock()
				results[c] = err
				mutex.Unlock()
			}
		}(contact)
	}

	wg.Wait()
	return results
}

// func (kademlia *Kademlia) realStoreNodes(contacts []Contact, data []byte) map[Contact]error {
//     results := make(map[Contact]error)
//     var wg sync.WaitGroup
//     var mutex sync.Mutex

//     for _, contact := range contacts {
//         wg.Add(1)
//         go func(c Contact) {
//             defer wg.Done()

//             err := kademlia.SendStoreMessage(&c, data)
//             if err != nil {
//                 mutex.Lock()
//                 results[c] = err
//                 mutex.Unlock()
//             } else {
//                 mutex.Lock()
//                 results[c] = nil
//                 mutex.Unlock()
//             }
//         }(contact)
//     }

//     wg.Wait()
//     return results
// }

// func (kademlia *Kademlia) queryNodes(contactsToQuery []Contact, targetID *KademliaID) map[Contact][]Contact {
// 	responseMap := make(map[Contact][]Contact)
// 	var wg sync.WaitGroup
// 	var mutex sync.Mutex

// 	for _, contact := range contactsToQuery {
// 		wg.Add(1)
// 		go func(c Contact) {
// 			defer wg.Done()

// 			foundContacts, err := net.SendFindContactMessage(&c, targetID)
// 			if err != nil {
// 				log.Printf("Failed to query node %s: %v\n", c.Address, err)
// 				return
// 			}

// 			mutex.Lock()
// 			responseMap[c] = foundContacts
// 			mutex.Unlock()
// 		}(contact)
// 	}

// 	wg.Wait()
// 	return responseMap
// }

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

	for i := range unique {
		unique[i].CalcDistance(target)
	}
	sort.Slice(unique, func(i, j int) bool {
		return unique[i].Less(&unique[j])
	})
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

// ai generated mock network
func (k *Kademlia) mockQueryNodes(contactsToQuery []Contact, targetID *KademliaID) map[Contact][]Contact {
	responseMap := make(map[Contact][]Contact)
	var wg sync.WaitGroup
	var mutex sync.Mutex

	for _, contact := range contactsToQuery {
		wg.Add(1)
		go func(c Contact) {
			defer wg.Done()

			mockResponse := []Contact{}

			// Generate between 2–4 new contacts
			numNew := rand.Intn(3) + 2
			for i := 0; i < numNew; i++ {
				mockID := NewRandomKademliaID()

				// 50% chance: make ID share a prefix with target (close node)
				if rand.Intn(2) == 0 {
					copy(mockID[:2], targetID[:2]) // share 2 bytes of prefix
				}

				mockContact := NewContact(mockID, fmt.Sprintf("mock-%s-%d", c.ID.String()[:6], i))
				mockContact.CalcDistance(targetID)
				mockResponse = append(mockResponse, mockContact)
			}

			mutex.Lock()
			responseMap[c] = mockResponse
			mutex.Unlock()
		}(contact)
	}

	wg.Wait()
	return responseMap
}

func (kademlia *Kademlia) SendPingMessage(address string, remove_from_bucket_if_timeout bool) {
	var rpc RPCPing
	rpc.header.Typ = RPCTypePing
	rpc.header.Rpc_id = *NewRandomKademliaID()
	rpc.header.Node_id = *kademlia.routingTable.me.ID
	var reply Reply
	reply.rpc_id = rpc.header.Rpc_id
	reply.remove_from_bucket_if_timeout = remove_from_bucket_if_timeout
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
	var rpc RPCFindNodeReply
	rpc.header.Typ = RPCTypeFindNodeReply
	rpc.header.Rpc_id = *id
	rpc.header.Node_id = *kademlia.routingTable.me.ID

	rpc.contact_count = uint16(len(contacts))
	rpc.contacts = contacts

	write_buf, _ := binary.Append(nil, binary.NativeEndian, rpc)

	kademlia.net.SendData(address, write_buf)
}

func (kademlia *Kademlia) SendFindDataReplyMessage(address string, id *KademliaID, data []byte, contacts []Contact) {
	var rpc RPCFindValueReply
	rpc.header.Typ = RPCTypeFindValue
	rpc.header.Rpc_id = *id
	rpc.header.Node_id = *kademlia.routingTable.me.ID

	rpc.data_size = uint64(len(data))
	rpc.contact_count = uint16(len(contacts))

	rpc.data = data
	rpc.contacts = contacts

	write_buf, _ := binary.Append(nil, binary.NativeEndian, rpc)

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
