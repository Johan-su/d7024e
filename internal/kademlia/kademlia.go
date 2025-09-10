package kademlia

import (
	"fmt"
	"sort"
	"sync"
)

type Kademlia struct {
	routingTable *RoutingTable
	network      *Network
}

const alpha = 3

func (kademlia *Kademlia) LookupData(hash string) {
	// TODO
}

func (kademlia *Kademlia) Store(data []byte) {
	// TODO
}

func (kademlia *Kademlia) LookupContact(target *Contact) []Contact {
	//
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
func (kademlia *Kademlia) mockQueryNodes(contactsToQuery []Contact, targetID *KademliaID) map[Contact][]Contact {
	responseMap := make(map[Contact][]Contact)
	var wg sync.WaitGroup
	var mutex sync.Mutex

	for _, contact := range contactsToQuery {
		wg.Add(1)
		go func(c Contact) {
			defer wg.Done()

			// More realistic: return contacts that are somewhat close to the target
			mockResponse := []Contact{}
			for i := 0; i < 3; i++ {
				// Create ID that's partially similar to target (more realistic)
				mockID := NewRandomKademliaID()
				// Make it somewhat similar to target for realism
				for j := 0; j < IDLength/2; j++ {
					mockID[j] = targetID[j] // Copy first half from target
				}
				mockContact := NewContact(mockID, fmt.Sprintf("mock-node-%d", i))
				mockContact.CalcDistance(targetID) // Calculate distance
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
