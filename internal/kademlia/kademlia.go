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
	"time"
	"context"
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
	dataHash KademliaID
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
	exists bool
	// only important for handling full bucket updates 
	removeFromBucketIfTimeout bool 
	nodeIdToRemove KademliaID
	contactToAddIfRemove Contact 
}

type Value struct {
	dat []byte
	exists bool
	expiry time.Time // time when the value expires
}

type Kademlia struct {

	// config
	expiryTime time.Duration
	republishTime time.Duration
	//



	muRoutingTable sync.Mutex
	routingTable *RoutingTable

	muUploadedData sync.Mutex
	uploadedData map[KademliaID]context.CancelFunc

	muKvStore sync.Mutex
	kvStore map[KademliaID]Value


	// general responses also handles find responses
	muReplyResponses sync.Mutex
	replyResponses map[KademliaID]Reply

	// find responses
	muFindNodeResponses sync.Mutex
	findNodeResponses map[KademliaID]chan RPCFindReply

	muFindValueResponses sync.Mutex
	findValueResponses map[KademliaID]chan RPCFindReply

	muStoreResponses sync.Mutex
	storeResponses map[KademliaID]chan RPCStoreReply

	Net Node
}

func NewKademlia(address string, id *KademliaID, net Node, expiryTime time.Duration, republishTime time.Duration, b int) Kademlia {
	var k Kademlia
	k.routingTable = NewRoutingTable(NewContact(id, address), b) // TODO: Change "1" (b) to parameter
	k.uploadedData = make(map[KademliaID]context.CancelFunc)
	k.kvStore = make(map[KademliaID]Value) 
	k.replyResponses = make(map[KademliaID]Reply)
	k.findNodeResponses = make(map[KademliaID]chan RPCFindReply)
	k.findValueResponses = make(map[KademliaID]chan RPCFindReply)
	k.storeResponses = make(map[KademliaID]chan RPCStoreReply)
	k.Net = net
	k.expiryTime = expiryTime
	k.republishTime = republishTime
	// if !(k.expiryTime > k.republishTime) {
	// 	log.Fatalf("expiry time %v has to be longer than republish time %v\n", k.expiryTime, k.republishTime)
	// }
	return k
}

func (kademlia *Kademlia) RemoveIfInReplyList(id KademliaID) bool {
	kademlia.muReplyResponses.Lock()
	reply := kademlia.replyResponses[id]
	delete(kademlia.replyResponses, id)
	kademlia.muReplyResponses.Unlock()

	return reply.exists
}

func (kademlia *Kademlia) AddToReplyList(id KademliaID, reply Reply) {
	kademlia.muReplyResponses.Lock()
	kademlia.replyResponses[id] = reply
	kademlia.muReplyResponses.Unlock()
}


func GetAndRemoveResponse[T any](id KademliaID, mapLock *sync.Mutex, responseMap map[KademliaID]chan T) (T, bool) {
	var channel chan T
	mapLock.Lock()
	channel = responseMap[id]
	if channel == nil {
		channel = make(chan T, 1)
		responseMap[id] = channel
	}
	mapLock.Unlock()
	
	var reply T
	select {
		case reply = <- channel: {
			mapLock.Lock()
			delete(responseMap, id)
			mapLock.Unlock()
			return reply, true
		}
		case <-time.After(3 * time.Second): {
			mapLock.Lock()
			delete(responseMap, id)
			mapLock.Unlock()
			return reply, false
		}
	}
}

func AddToResponses[T any](id KademliaID, reply T, mapLock *sync.Mutex, responseMap map[KademliaID]chan T) {
	mapLock.Lock()
	channel := responseMap[id]
	if channel == nil {
		channel = make(chan T, 1)
		responseMap[id] = channel
	} else {
		channel = responseMap[id]
	}
	mapLock.Unlock()
	channel <- reply
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
func (kademlia *Kademlia) BucketUpdate(address string, node_id KademliaID) {
	assertPanic(!node_id.Equals(kademlia.routingTable.me.ID), "Cannot add itself to routing table") 
	kademlia.muRoutingTable.Lock()
	defer kademlia.muRoutingTable.Unlock()

	node := kademlia.routingTable.findLeafNode(&node_id)
	bucket := node.bucket

	if (bucket.Len() != bucketSize) {
		bucket.AddContact(Contact{&node_id, address, nil})
	} else {
		exists := bucket.AddContact(Contact{&node_id, address, nil})
		if !exists {
			front_contact := bucket.list.Front()
			//TODO ignore for now
			if false {
				kademlia.SendPingMessage(*NewRandomKademliaID(), front_contact.Value.(Contact).Address, true)
			}
		}
	}
}

func (kademlia *Kademlia) Listen() {
	meaddr := kademlia.routingTable.me.Address
	kademlia.Net.Listen(meaddr)
}

func (kademlia *Kademlia) HandleResponse() {
	meaddr := kademlia.routingTable.me.Address

	requests := make(chan Message, 100)

	for i := 0; i < 3; i += 1 {
		go kademlia.worker(requests)
	}

	for {
		response := kademlia.Net.Receive()
		select {
			case requests <- response:
			default: {
				log.Printf("[%v] <- [%v] Dropped\n", meaddr, response.from_address)
			}
		}
	}
}

func (kademlia *Kademlia) worker(incResponses chan Message) {
	var err error
	meaddr := kademlia.routingTable.me.Address
	for {
		var response Message
		select {
			case response = <- incResponses:
		}
		reader := bytes.NewReader(response.data)
		var header RPCHeader
		err = PartialRead(reader, &header)
		if err != nil {
			log.Printf("%v\n", err)
		}
		// receiving
		switch header.Typ {
			case RPCTypeInvalid: {
				log.Printf("[%v] <- [%v] Invalid\n", meaddr, response.from_address)
			}
			case RPCTypePing: {
				log.Printf("[%v] <- [%v] Ping\n", meaddr, response.from_address)
				kademlia.BucketUpdate(response.from_address, header.NodeId)
				kademlia.SendPingReplyMessage(response.from_address, &header.RpcId)
			}
			case RPCTypeStore: {
				log.Printf("[%v] <- [%v] Store\n", meaddr, response.from_address)
				kademlia.BucketUpdate(response.from_address, header.NodeId)
				var store RPCStore
				{
					store.header = header

					err := PartialRead(reader, &store.dataHash)
					assertPanic(err == nil, "")

					err = PartialRead(reader, &store.dataSize)
					assertPanic(err == nil, "")

					if store.dataSize != 0 {
						store.data = make([]byte, store.dataSize)
						reader.Read(store.data)
					}
				}
				if store.dataSize == 0 {
					kademlia.muKvStore.Lock()
					v := kademlia.kvStore[store.dataHash]
					if v.exists && v.expiry.Before(time.Now()) {
						v.expiry = time.Now().Add(kademlia.expiryTime)
						kademlia.kvStore[store.dataHash] = v
					}
					kademlia.muKvStore.Unlock()
					
				} else {
					kademlia.muKvStore.Lock()
					kademlia.kvStore[store.dataHash] = Value{store.data, true, time.Now().Add(kademlia.expiryTime)}
					kademlia.muKvStore.Unlock()
				}
				// TODO maybe send back a error if it failed to store
				kademlia.SendStoreReplyMessage(response.from_address, &header.RpcId, RPCErrorNoError)
			}
			case RPCTypeFindNode: {
				log.Printf("[%v] <- [%v] FindNode\n", meaddr, response.from_address)
				kademlia.BucketUpdate(response.from_address, header.NodeId)
				var find_node RPCFindNode
				{
					find_node.header = header
					PartialRead(reader, &find_node.targetNodeId)
					if err != nil {
						log.Fatalf("%v\n", err)
					}
				}
				kademlia.muRoutingTable.Lock()
				contacts := kademlia.routingTable.FindClosestContacts(&find_node.targetNodeId, bucketSize)
				kademlia.muRoutingTable.Unlock()
				kademlia.SendFindContactReplyMessage(response.from_address, &header.RpcId, contacts)
			}
			case RPCTypeFindValue: {
				log.Printf("[%v] <- [%v] FindValue\n", meaddr, response.from_address)
				kademlia.BucketUpdate(response.from_address, header.NodeId)
				var find_value RPCFindValue
				
				{
					find_value.header = header
					err := PartialRead(reader, &find_value.targetKeyId)
					assertPanic(err == nil, "%v\n", err)
				}

				var bytes []byte
				var contacts []Contact
				
				kademlia.muKvStore.Lock()
				val := kademlia.kvStore[find_value.targetKeyId]
				kademlia.muKvStore.Unlock()
				if val.exists && time.Now().Before(val.expiry) {
					bytes = val.dat
				} else {
					kademlia.muRoutingTable.Lock()
					contacts = kademlia.routingTable.FindClosestContacts(&find_value.targetKeyId, bucketSize)
					kademlia.muRoutingTable.Unlock()
				}
				kademlia.SendFindDataReplyMessage(response.from_address, &header.RpcId, bytes, contacts)
			}
			case RPCTypePingReply: {
				log.Printf("[%v] <- [%v] PingReply\n", meaddr, response.from_address)
				
				if kademlia.RemoveIfInReplyList(header.RpcId) {
					// var ping_reply RPCPingReply
					kademlia.BucketUpdate(response.from_address, header.NodeId)
				} else {
					log.Printf("[%v] Got unexpected ping reply, might have timed out\n", meaddr)
				}
			}
			case RPCTypeStoreReply: {
				log.Printf("[%v] <- [%v] StoreReply\n", meaddr, response.from_address)
				if kademlia.RemoveIfInReplyList(header.RpcId) {
					// var store_reply RPCStoreReply
					kademlia.BucketUpdate(response.from_address, header.NodeId)
					//TODO: maybe handle errors or smth

					var storeReply RPCStoreReply
					{
						storeReply.header = header
						/* err = PartialRead(reader, &storeReply.dataSize)
						if err != nil {
							log.Fatalf("%v\n", err)
						}*/
					}

					AddToResponses(storeReply.header.RpcId, storeReply, &kademlia.muStoreResponses, kademlia.storeResponses)

					//kademlia.AddToFindNodeReponses(findNodeReply.header.RpcId, findNodeReply)
				} else {
					log.Printf("Got unexpected store reply, might have timed out\n")
				}
			}
			case RPCTypeFindNodeReply: {
				log.Printf("[%v] <- [%v] FindNodeReply\n", meaddr, response.from_address)
				if kademlia.RemoveIfInReplyList(header.RpcId) {
					var findNodeReply RPCFindReply
					{
						findNodeReply.header = header
						err = PartialRead(reader, &findNodeReply.dataSize)
						if err != nil {
							log.Fatalf("%v\n", err)
						}
						err = PartialRead(reader, &findNodeReply.contactCount)
						if err != nil {
							log.Fatalf("%v\n", err)
						}
						findNodeReply.contacts = make([]KademliaTriple, findNodeReply.contactCount)
						for i := 0; i < int(findNodeReply.contactCount); i += 1 {
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

							findNodeReply.contacts[i] = tri
						}
					}

					kademlia.BucketUpdate(response.from_address, header.NodeId)

					AddToResponses(findNodeReply.header.RpcId, findNodeReply, &kademlia.muFindNodeResponses, kademlia.findNodeResponses)
				} else {
					log.Printf("[%v] Got unexpected find node reply, might have timed out\n", meaddr)
				}
			}
			case RPCTypeFindValueReply: {
				log.Printf("[%v] <- [%v] FindValueReply\n", meaddr, response.from_address)
				if kademlia.RemoveIfInReplyList(header.RpcId) {
					var findValueReply RPCFindReply
					{
						findValueReply.header = header
						err = PartialRead(reader, &findValueReply.dataSize)
						assertPanic(err == nil, "%v\n", err)

						err = PartialRead(reader, &findValueReply.contactCount)
						assertPanic(err == nil, "%v\n", err)

						if findValueReply.dataSize == 0 {

							findValueReply.contacts = make([]KademliaTriple, findValueReply.contactCount)
							for i := 0; i < int(findValueReply.contactCount); i += 1 {
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

								findValueReply.contacts[i] = tri
							}
						} else {
							findValueReply.data = make([]byte, findValueReply.dataSize)
							reader.Read(findValueReply.data)
						}
					}

					kademlia.BucketUpdate(response.from_address, header.NodeId)
					AddToResponses(findValueReply.header.RpcId, findValueReply, &kademlia.muFindValueResponses, kademlia.findValueResponses)
				} else {
					log.Printf("Got unexpected find value reply, might have timed out\n")
				}
			}
		}

	}
}

func (kademlia *Kademlia) Forget(hash string) error {
	id := NewKademliaID(hash)
	
	
	kademlia.muUploadedData.Lock()
	cancel := kademlia.uploadedData[*id]
	delete(kademlia.uploadedData, *id)
	kademlia.muUploadedData.Unlock()
	if cancel != nil {
		cancel()
	}  else {
		return fmt.Errorf("tried to forget hash `%s` that is not in the node\n", hash)
	}
	return nil
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

	kademlia.muRoutingTable.Lock()
	kademlia.routingTable.AddContact(bootstrapContact)
	kademlia.muRoutingTable.Unlock()
	
	closestContacts := kademlia.LookupContact(&kademlia.routingTable.me)
	
	bucketIndicies := make([]int, len(closestContacts))

	kademlia.muRoutingTable.Lock()
	for i, c := range closestContacts {
		bucketIndicies[i] = kademlia.routingTable.getBucketIndex(c.ID)
	}
	kademlia.muRoutingTable.Unlock()

	sort.Slice(bucketIndicies, func(i, j int) bool {
		return bucketIndicies[i] < bucketIndicies[j]
	})

	for i := 1; i < len(bucketIndicies); i += 1 {
		kademlia.Refresh(bucketIndicies[i])
	}
}


func (kademlia *Kademlia) LookupData(hash string) ([]byte, bool, []Contact) {
	key := NewKademliaID(hash)
	kademlia.muRoutingTable.Lock()
	shortlist := kademlia.routingTable.FindClosestContacts(key, alpha)
	kademlia.muRoutingTable.Unlock()
	sortByDistance(shortlist, key)

	queried := make(map[string]Contact)

	var foundData []byte
	var nodesWithoutData []Contact // track nodes that didnt have the data

	fromNode := make([]Contact, 1)

	unchangedRounds := 0
	dataFound := false

	for unchangedRounds < 3 && foundData == nil {
		toQuery := kademlia.selectUnqueriedNodes(shortlist, queried, alpha)

		for _, c := range toQuery {
			queried[c.ID.String()] = c
		}

		oldClosest := shortlist[0]

		if len(toQuery) == 0 {
			break
		}

		responses := kademlia.queryDataNodes(toQuery, *key)

		// check if value is found
		for i, response := range responses {
			if len(response.data) > 0 && !dataFound {
				foundData = response.data
				dataFound = true
				fromNode[0] = toQuery[i]
			} else {
				nodesWithoutData = append(nodesWithoutData, toQuery[i])
			}
			// if the data wasnt found, merge the found contacts (just like lookupContact)
			if !dataFound && response.contacts != nil {
				var newContacts []Contact
				for _, triple := range response.contacts {
					newContacts = append(newContacts, Contact{&triple.id, triple.address, nil})
				}
				shortlist = kademlia.mergeAndSort(shortlist, newContacts, key)
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
		kademlia.SendStoreMessage(*NewRandomKademliaID(), closestNode.Address, nil, foundData)
	}

	if dataFound {
		return foundData, true, fromNode
	} else {
		return nil, false, getTopContacts(shortlist, bucketSize)
	}
}

func (kademlia *Kademlia) LookupContact(target *Contact) []Contact {

	// "The first alpha contacts selected are used to create a shortlist for the search."
	kademlia.muRoutingTable.Lock()
	shortlist := kademlia.routingTable.FindClosestContacts(target.ID, alpha)
	kademlia.muRoutingTable.Unlock()
	if len(shortlist) == 0 {
		return nil
	}

	sortByDistance(shortlist, target.ID)
	queried := make(map[string]Contact)

	unchangedRounds := 0

	for unchangedRounds < 3 {
		toQuery := kademlia.selectUnqueriedNodes(shortlist, queried, alpha) // helper to select nodes that hasnt been queried already

		for _, contact := range toQuery {
			queried[contact.ID.String()] = contact
		}

		oldClosest := shortlist[0]

		if len(toQuery) == 0 {
			break
		}

		responselist := kademlia.queryNodes(toQuery, target.ID)
		
		shortlist = kademlia.mergeAndSort(shortlist, responselist, target.ID)

		if shortlist[0].ID.Equals(oldClosest.ID) {
			unchangedRounds++
		} else {
			unchangedRounds = 0
		}

	}
	return getTopContacts(shortlist, bucketSize)
}

func (kademlia *Kademlia) republishWorker(ctx context.Context, dataHash KademliaID) {
	for {
		select {
			case <-ctx.Done(): {
				return
			}
			case <-time.After(kademlia.republishTime): {
				//TODO maybe check if already expired
				kademlia.muKvStore.Lock()
				v, exists := kademlia.kvStore[dataHash]
				if exists && v.expiry.After(time.Now()) {
    				v.expiry = time.Now().Add(kademlia.expiryTime)
    				kademlia.kvStore[dataHash] = v
				} else {
    				// just return if the value has expired 
    				kademlia.muKvStore.Unlock()
    				return
				}
				kademlia.muKvStore.Unlock()
				
				_, _, contacts := kademlia.LookupData(dataHash.String())
			
			
				rpcIds := make([]KademliaID, len(contacts))
				for i, c := range contacts {
					rpcIds[i] = *NewRandomKademliaID()
					kademlia.SendStoreMessage(rpcIds[i], c.Address, &dataHash, nil)
				}
		
			}
		}
	}
}

func (kademlia *Kademlia) Store(data []byte) (KademliaID, error) {

	key := Sha1toKademlia(data)

	kademlia.muKvStore.Lock()
	kademlia.kvStore[*key] = Value{data, true, time.Now().Add(kademlia.expiryTime)}
	kademlia.muKvStore.Unlock()

	
	ctx, cancel := context.WithCancel(context.Background())
	kademlia.muUploadedData.Lock()
	go kademlia.republishWorker(ctx, *key)
	kademlia.uploadedData[*key] = cancel
	kademlia.muUploadedData.Unlock()


	target := NewContact(key, "")
	closestContacts := kademlia.LookupContact(&target) // find k closest contacts to send store rpc to

	if len(closestContacts) == 0 {
		return *key, fmt.Errorf("no nodes found for storage replication")
	}

	length := len(closestContacts)
	rpcIds := make([]KademliaID, length)
	for i, c := range closestContacts {
		rpcIds[i] = *NewRandomKademliaID()
		kademlia.SendStoreMessage(rpcIds[i], c.Address, nil, data)
	}


	var responses []RPCStoreReply

	for _, id := range rpcIds {

		response, receivedData := GetAndRemoveResponse(id, &kademlia.muStoreResponses, kademlia.storeResponses)
		if receivedData {
			responses = append(responses, response)
		}

		//print responses
		log.Printf("Store response: %v\n", response)
	}

	return *key, nil
}

func (kademlia *Kademlia) queryDataNodes(contactsToQuery []Contact, targetHash KademliaID) []RPCFindReply {
	length := len(contactsToQuery)
	assertPanic(length <= alpha, "Illegal")

	var responses []RPCFindReply
	rpcIds := make([]KademliaID, length)
	for i := 0; i < length; i++ {
		rpcIds[i] = *NewRandomKademliaID()
		kademlia.SendFindDataMessage(rpcIds[i], contactsToQuery[i].Address, targetHash)
	}

	for _, id := range rpcIds {

		response, receivedData := GetAndRemoveResponse(id, &kademlia.muFindValueResponses, kademlia.findValueResponses)
		if receivedData {
			responses = append(responses, response)
		}
	}

	return responses
}

func (kademlia *Kademlia) queryNodes(contactsToQuery []Contact, targetID *KademliaID) []Contact {
	length := len(contactsToQuery)
	assertPanic(length <= alpha, "Illegal")


	var responses []Contact

	rpcIds := make([]KademliaID, length)
	// strict parallelism
	for i := 0; i < length; i += 1 {
		rpcIds[i] = *NewRandomKademliaID()
		kademlia.SendFindContactMessage(rpcIds[i], contactsToQuery[i].Address, targetID)
	}

	for  _, id := range rpcIds {

		response, receivedData := GetAndRemoveResponse(id, &kademlia.muFindNodeResponses, kademlia.findNodeResponses)
		if receivedData {
			for _, triple := range response.contacts {
				c := Contact{&triple.id, triple.address, nil}
				responses = append(responses, c)
			}
		}
	}
	return responses
}

func (kademlia *Kademlia) selectUnqueriedNodes(shortlist []Contact, queried map[string]Contact, n int) []Contact {
	var result []Contact
	for _, contact := range shortlist {
		if _, alreadyQueried := queried[contact.ID.String()]; !alreadyQueried && len(result) < n {
			result = append(result, contact)
		}
	}
	return result
}

func (kademlia *Kademlia) mergeAndSort(shortlist, newContacts []Contact, target *KademliaID) []Contact {
	combined := append(shortlist, newContacts...)

	seen := make(map[string]bool)
	// set own id to remove itself from the unique list 
	seen[kademlia.routingTable.me.ID.String()] = true
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
func (kademlia *Kademlia) SendPingMessage(rpcId KademliaID, address string, removeFromBucketIfTimeout bool) {
	log.Printf("[%v] -> [%v] Ping\n", kademlia.routingTable.me.Address, address)
	assertPanic(kademlia.routingTable.me.Address != address, "Illegal")
	var rpc RPCPing
	rpc.header.Typ = RPCTypePing
	rpc.header.RpcId = rpcId
	rpc.header.NodeId = *kademlia.routingTable.me.ID
	var reply Reply
	reply.exists = true
	reply.removeFromBucketIfTimeout = removeFromBucketIfTimeout
	// TODO: handle timeouts correctly
	kademlia.AddToReplyList(rpc.header.RpcId, reply) 
	
	writeBuf, err := binary.Append(nil, binary.NativeEndian, rpc)
	assertPanic(err == nil, "%v\n", err)

	kademlia.Net.SendData(address, writeBuf)
}

func (kademlia *Kademlia) SendFindContactMessage(rpcId KademliaID, address string, key *KademliaID) {
	log.Printf("[%v] -> [%v] FindContact\n", kademlia.routingTable.me.Address, address)
	assertPanic(kademlia.routingTable.me.Address != address, "Illegal")
	var rpc RPCFindNode
	rpc.header.Typ = RPCTypeFindNode
	rpc.header.RpcId = rpcId
	rpc.header.NodeId = *kademlia.routingTable.me.ID
	var reply Reply
	reply.exists = true
	kademlia.AddToReplyList(rpc.header.RpcId, reply) 

	rpc.targetNodeId = *key

	writeBuf, err := binary.Append(nil, binary.NativeEndian, rpc)
	assertPanic(err == nil, "%v\n", err)


	kademlia.Net.SendData(address, writeBuf)
}

func (kademlia *Kademlia) SendFindDataMessage(rpcId KademliaID, address string, targetKey KademliaID) {
	log.Printf("[%v] -> [%v] FindValue\n", kademlia.routingTable.me.Address, address)
	assertPanic(kademlia.routingTable.me.Address != address, "Illegal")
	var rpc RPCFindValue
	rpc.header.Typ = RPCTypeFindValue
	rpc.header.RpcId = rpcId
	rpc.header.NodeId = *kademlia.routingTable.me.ID
	var reply Reply
	reply.exists = true
	kademlia.AddToReplyList(rpc.header.RpcId, reply) 

	rpc.targetKeyId = targetKey
	writeBuf, err := binary.Append(nil, binary.NativeEndian, rpc)
	assertPanic(err == nil, "%v\n", err)


	kademlia.Net.SendData(address, writeBuf)
}

// if dataHash is nil get hash from sha1 on data
func (kademlia *Kademlia) SendStoreMessage(rpcId KademliaID, address string, dataHash *KademliaID, data []byte) {
	log.Printf("[%v] -> [%v] Store\n", kademlia.routingTable.me.Address, address)
	assertPanic(kademlia.routingTable.me.Address != address, "Illegal")
	var rpc RPCStore
	rpc.header.Typ = RPCTypeStore
	rpc.header.RpcId = rpcId
	rpc.header.NodeId = *kademlia.routingTable.me.ID
	var reply Reply
	reply.exists = true
	kademlia.AddToReplyList(rpc.header.RpcId, reply) 
	
	
	rpc.dataSize = uint64(len(data))
	if dataHash == nil {
		rpc.dataHash = *Sha1toKademlia(data)
	} else {
		rpc.dataHash = *dataHash
	}
	rpc.data = data

	var writeBuf []byte
	var err error


	writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, rpc.header)
	assertPanic(err == nil, "%v\n", err)

	writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, rpc.dataHash)
	assertPanic(err == nil, "%v\n", err)

	writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, rpc.dataSize)
	assertPanic(err == nil, "%v\n", err)

	writeBuf = append(writeBuf, rpc.data...)

	kademlia.Net.SendData(address, writeBuf)
}

func (kademlia *Kademlia) SendPingReplyMessage(address string, id *KademliaID) {
	log.Printf("[%v] -> [%v] PingReply\n", kademlia.routingTable.me.Address, address)
	assertPanic(kademlia.routingTable.me.Address != address, "Illegal")
	var rpc RPCPingReply
	rpc.header.Typ = RPCTypePingReply
	rpc.header.RpcId = *id
	rpc.header.NodeId = *kademlia.routingTable.me.ID

	writeBuf, err := binary.Append(nil, binary.NativeEndian, rpc)
	assertPanic(err == nil, "%v\n", err)

	kademlia.Net.SendData(address, writeBuf)
}

func (kademlia *Kademlia) SendFindContactReplyMessage(address string, id *KademliaID, contacts []Contact) {
	log.Printf("[%v] -> [%v] FindNodeReply\n", kademlia.routingTable.me.Address, address)
	assertPanic(kademlia.routingTable.me.Address != address, "Illegal")
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
	kademlia.Net.SendData(address, writeBuf)
}

func (kademlia *Kademlia) SendFindDataReplyMessage(address string, id *KademliaID, data []byte, contacts []Contact) {
	log.Printf("[%v] -> [%v] FindDataReply\n", kademlia.routingTable.me.Address, address)
	assertPanic(kademlia.routingTable.me.Address != address, "Illegal")
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


	kademlia.Net.SendData(address, writeBuf)
}

func (kademlia *Kademlia) SendStoreReplyMessage(address string, id *KademliaID, rpcErr RPCError) {
	log.Printf("[%v] -> [%v] StoreReply\n", kademlia.routingTable.me.Address, address)
	assertPanic(kademlia.routingTable.me.Address != address, "Illegal")
	var rpc RPCStoreReply
	rpc.header.Typ = RPCTypeStoreReply
	rpc.header.RpcId = *id
	rpc.header.NodeId = *kademlia.routingTable.me.ID
	rpc.header.RpcError = rpcErr

	var writeBuf []byte
	var err error

	writeBuf, err = binary.Append(writeBuf, binary.NativeEndian, rpc)
	assertPanic(err == nil, "%v\n", err)

	kademlia.Net.SendData(address, writeBuf)
}
