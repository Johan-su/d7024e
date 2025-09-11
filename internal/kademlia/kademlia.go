package kademlia

import (
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
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
)

type RPCHeader struct {
	typ       RPCType
	id        KademliaID
	rpc_error RPCError
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

type Kademlia struct {
	routingTable *RoutingTable
	kv_store     map[KademliaID][]byte
	net          Node
}

func NewKademlia(net Node) Kademlia {
	var k Kademlia

	k.routingTable = new(RoutingTable)
	k.kv_store = make(map[KademliaID][]byte)
	k.net = net
	return k
}

const alpha = 3

func (kademlia *Kademlia) HandleResponse() {
	for {
		response := kademlia.net.Listen()
		var header RPCHeader
		binary.Decode(response.data, binary.BigEndian, header)
		// receiving
		switch header.typ {
		case RPCTypeInvalid:
			{
				log.Printf("Got Invalid RPC\n")
			}
		case RPCTypePing:
			{
				kademlia.SendPingReplyMessage(&Contact{nil, response.address, nil}, &header.id)
				// TODO maybe update bucket
			}
		case RPCTypeStore:
			{
				var store RPCStore
				binary.Decode(response.data, binary.BigEndian, store)

				kademlia.Store(store.data)
				// TODO maybe send back a error if it failed to store
				kademlia.SendStoreReplyMessage(&Contact{nil, response.address, nil}, &header.id, RPCErrorNoError)
			}
		case RPCTypeFindNode:
			{
				var find_node RPCFindNode
				binary.Decode(response.data, binary.BigEndian, find_node)
				contacts := kademlia.routingTable.FindClosestContacts(&find_node.target_node_id, bucketSize)
				kademlia.SendFindContactReplyMessage(&Contact{nil, response.address, nil}, &header.id, contacts)
			}
		case RPCTypeFindValue:
			{
				var find_value RPCFindValue
				binary.Decode(response.data, binary.BigEndian, find_value)

				var bytes []byte
				var contacts []Contact

				bytes, ok := kademlia.LookupData(find_value.target_key_id.String())
				if !ok {
					contacts = kademlia.routingTable.FindClosestContacts(&find_value.target_key_id, bucketSize)
				}
				kademlia.SendFindDataReplyMessage(&Contact{nil, response.address, nil}, &header.id, bytes, contacts)
			}
		case RPCTypePingReply:
			{
				var ping_reply RPCPingReply
				binary.Decode(response.data, binary.BigEndian, ping_reply)
				// update bucket
				panic("TODO update bucket when receiving ping reply")
			}
		case RPCTypeStoreReply:
			{
				var store_reply RPCStoreReply
				binary.Decode(response.data, binary.BigEndian, store_reply)
				// update bucket

				panic("TODO Store RPC reply")
			}
		case RPCTypeFindNodeReply:
			{
				var find_node_reply RPCFindNodeReply
				binary.Decode(response.data, binary.BigEndian, find_node_reply)
				// update bucket
				panic("TODO find node RPC reply")
			}
		case RPCTypeFindValueReply:
			{
				var find_value_reply RPCFindValueReply
				binary.Decode(response.data, binary.BigEndian, find_value_reply)
				// update bucket
				panic("TODO find value RPC reply")
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
	hasher := sha1.New()
	hasher.Write(data)
	hashBytes := hasher.Sum(nil)

	hash := KademliaID{}
	copy(hash[:], hashBytes)
	key := hash

	kademlia.kv_store[key] = data

	target := NewContact(&key, "")
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

	// log which nodes we're attempting to store on
	log.Printf("Attempting to store on %d nodes:", len(contacts))
	for i, contact := range contacts {
		log.Printf("  %d: %s (address: %s)", i+1, contact.ID, contact.Address)
	}

	for _, contact := range contacts {
		wg.Add(1)
		go func(c Contact) {
			defer wg.Done()

			// Mock storage with 90% success rate
			if rand.Intn(10) < 9 { // 90% success
				log.Printf("Mock store successful on node %s (%s)", c.ID, c.Address)
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
				log.Printf("Mock store failed on node %s (%s): %v", c.ID, c.Address, err)
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

func (kademlia *Kademlia) SendPingMessage(recipient *Contact) {
	var rpc RPCPing
	rpc.header.typ = RPCTypePing
	rpc.header.id = *NewRandomKademliaID()

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)

	kademlia.net.SendData(recipient, write_buf)
}

func (kademlia *Kademlia) SendFindContactMessage(recipient *Contact, key *KademliaID) {
	var rpc RPCFindNode
	rpc.header.typ = RPCTypeFindNode
	rpc.header.id = *NewRandomKademliaID()

	rpc.target_node_id = *key

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)

	kademlia.net.SendData(recipient, write_buf)
}

func (kademlia *Kademlia) SendFindDataMessage(recipient *Contact, hash string) {
	var rpc RPCFindValue
	rpc.header.typ = RPCTypeFindValue
	rpc.header.id = *NewRandomKademliaID()

	rpc.target_key_id = *NewKademliaID(hash)
	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)

	kademlia.net.SendData(recipient, write_buf)
}

func (kademlia *Kademlia) SendStoreMessage(recipient *Contact, data []byte) {
	var rpc RPCStore
	rpc.header.typ = RPCTypeStore
	rpc.header.id = *NewRandomKademliaID()

	rpc.data_size = uint64(len(data))
	rpc.data = data

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)

	kademlia.net.SendData(recipient, write_buf)
}

func (kademlia *Kademlia) SendPingReplyMessage(recipient *Contact, id *KademliaID) {
	var rpc RPCPingReply
	rpc.header.typ = RPCTypePingReply
	rpc.header.id = *id

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)

	kademlia.net.SendData(recipient, write_buf)
}

func (kademlia *Kademlia) SendFindContactReplyMessage(recipient *Contact, id *KademliaID, contacts []Contact) {
	var rpc RPCFindNodeReply
	rpc.header.typ = RPCTypeFindNodeReply
	rpc.header.id = *id

	rpc.contact_count = uint16(len(contacts))
	rpc.contacts = contacts

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)

	kademlia.net.SendData(recipient, write_buf)
}

func (kademlia *Kademlia) SendFindDataReplyMessage(recipient *Contact, id *KademliaID, data []byte, contacts []Contact) {
	var rpc RPCFindValueReply
	rpc.header.typ = RPCTypeFindValue
	rpc.header.id = *id

	rpc.data_size = uint64(len(data))
	rpc.contact_count = uint16(len(contacts))

	rpc.data = data
	rpc.contacts = contacts

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)

	kademlia.net.SendData(recipient, write_buf)
}

func (kademlia *Kademlia) SendStoreReplyMessage(recipient *Contact, id *KademliaID, err RPCError) {
	var rpc RPCStoreReply
	rpc.header.typ = RPCTypeStoreReply
	rpc.header.id = *id
	rpc.header.rpc_error = err

	write_buf, _ := binary.Append(nil, binary.BigEndian, rpc)

	kademlia.net.SendData(recipient, write_buf)
}
