package kademlia

import (
	"container/list"
)

// bucket definition
// contains a List
type bucket struct {
	list *list.List
}

// newBucket returns a new instance of a bucket
func newBucket() *bucket {
	bucket := &bucket{}
	bucket.list = list.New()
	return bucket
}

func (bucket *bucket) RemoveContact(contact Contact) {
	panic("TODO")
}

// AddContact adds the Contact to the front of the bucket
// or moves it to the front of the bucket if it already existed
// returns true if it already exists
func (bucket *bucket) AddContact(contact Contact) bool {
	var exists bool
	var element *list.Element

	// Check if contact already exists
	for e := bucket.list.Front(); e != nil; e = e.Next() {
		nodeID := e.Value.(Contact).ID
		if contact.ID.Equals(nodeID) {
			element = e
			break
		}
	}

	if element == nil {
		// Contact doesn't exist
		if bucket.list.Len() < bucketSize {
			bucket.list.PushFront(contact)
		} else {
			// Bucket is full - implement LRU eviction
			// Remove the least recently used (tail) and add new to front
			bucket.list.Remove(bucket.list.Back())
			bucket.list.PushFront(contact)
			return false // Still return false to indicate it wasn't a simple add
		}
	} else {
		// Contact exists - move to front
		exists = true
		bucket.list.MoveToFront(element)
	}
	return exists
}

// GetContactAndCalcDistance returns an array of Contacts where
// the distance has already been calculated
func (bucket *bucket) GetContactAndCalcDistance(target *KademliaID) []Contact {
	var contacts []Contact

	for elt := bucket.list.Front(); elt != nil; elt = elt.Next() {
		contact := elt.Value.(Contact)
		contact.CalcDistance(target)
		contacts = append(contacts, contact)
	}

	return contacts
}

// Len return the size of the bucket
func (bucket *bucket) Len() int {
	return bucket.list.Len()
}
