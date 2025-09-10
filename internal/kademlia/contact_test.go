package kademlia

import (
	"testing"
)

func mockKademliaID(val byte) *KademliaID {

	id := [20]byte{}
	for i := range id {  //problably not good
		id[i] = val
	}
	newKademliaID := KademliaID{}
	for i := 0; i < IDLength; i++ {
		newKademliaID[i] = id[i]
	}

	return &newKademliaID
}


func TestNewContact(t *testing.T) {
	id := mockKademliaID(1)
	address := "127.0.0.1"
	contact := NewContact(id, address)
	if contact.ID != id || contact.Address != address || contact.distance != nil {
		t.Errorf("NewContact failed: %+v", contact)
	}
}

func TestCalcDistanceAndLess(t *testing.T) {
	id1 := mockKademliaID(1)
	id2 := mockKademliaID(2)
	contact1 := NewContact(id1, "a")
	contact2 := NewContact(id2, "b")
	contact1.CalcDistance(id2)
	contact2.CalcDistance(id1)
	if contact1.distance == nil || contact2.distance == nil {
		t.Error("CalcDistance did not set distance")
	}
	if !contact1.distance.Equals(contact2.distance) {
		t.Error("CalcDistance did not compute same distance for symmetric IDs")
	}
	if !contact1.Less(&contact2) && contact2.Less(&contact1) {
		t.Error("Less comparison failed")
	}
}



func TestContactCandidatesGetContacts(t *testing.T) {
	candidates := &ContactCandidates{}
	contacts := []Contact{
		NewContact(mockKademliaID(1), "a"),
		NewContact(mockKademliaID(2), "b"),
	}
	candidates.Append(contacts)
	got := candidates.GetContacts(1)
	if len(got) != 1 || got[0].Address != "a" {
		t.Errorf("GetContacts failed, got %+v", got)
	}
}

func TestContactCandidatesSwap(t *testing.T) {
	candidates := &ContactCandidates{}
	contacts := []Contact{
		NewContact(mockKademliaID(1), "a"),
		NewContact(mockKademliaID(2), "b"),
	}
	candidates.Append(contacts)
	candidates.Swap(0, 1)
	if candidates.contacts[0].Address != "b" {
		t.Errorf("Swap failed, got %+v", candidates.contacts)
	}
}

func TestContactCandidatesLess(t *testing.T) {
	candidates := &ContactCandidates{}
	c1 := NewContact(mockKademliaID(1), "a")
	c2 := NewContact(mockKademliaID(2), "b")
	c1.CalcDistance(mockKademliaID(2))
	c2.CalcDistance(mockKademliaID(1))
	candidates.Append([]Contact{c1, c2})
	if !candidates.Less(0, 1) && candidates.Less(1, 0) {
		t.Error("ContactCandidates Less failed")
	}
}

func TestContactCandidatesSort(t *testing.T) {
	candidates := &ContactCandidates{}
	c1 := NewContact(mockKademliaID(2), "b")
	c2 := NewContact(mockKademliaID(1), "a")
	c1.CalcDistance(mockKademliaID(0))
	c2.CalcDistance(mockKademliaID(0))
	candidates.Append([]Contact{c1, c2})
	candidates.Sort()
	if candidates.contacts[0].Address != "a" {
		t.Errorf("Sort failed, got %+v", candidates.contacts)
	}
}