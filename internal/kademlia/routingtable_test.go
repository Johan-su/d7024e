package kademlia

import (
	"fmt"
	"testing"
)


func TestFindClosestContact(t *testing.T) { // should probalbly be updated to include more contacts
	rt := NewRoutingTable(NewContact(NewKademliaID("FFFFFFFF00000000000000000000000000000000"), "localhost:8000"))
	contact := NewContact(NewKademliaID("0000000100000000000000000000000000000000"), "localhost:8001")
	rt.AddContact(contact)
	closestContact := rt.FindClosestContacts(NewKademliaID("0000000100000000000000000000000000000000"), 1)

	if closestContact[0].ID.String() != "0000000100000000000000000000000000000000" {
		t.Fatalf("Expected closest contact to be 0000000100000000000000000000000000000000, got %s", closestContact[0].ID.String())
	}
}

func TestRoutingTableContactCount(t *testing.T) {
	amountOfContacts := 16
	rt := NewRoutingTable(NewContact(NewKademliaID("FFFFFFFF00000000000000000000000000000000"), "localhost:8000"))
	for i := 0; i < amountOfContacts; i++ {
		stringI := fmt.Sprintf("%02d", i)
		contact := NewContact(NewKademliaID("0000000"+stringI+"00000000000000000000000000000000"), "localhost:800"+stringI)
		rt.AddContact(contact)
	}
	contacts := rt.FindClosestContacts(NewKademliaID("2111111400000000000000000000000000000000"), amountOfContacts)
	if len(contacts) != amountOfContacts {
		t.Fatalf("Expected %d contacts, got %d", amountOfContacts, len(contacts))
	}
}

func TestRoutingTableCorrectContactsAdded(t *testing.T) {
	amountOfContacts := 16
	rt := NewRoutingTable(NewContact(NewKademliaID("FFFFFFFF00000000000000000000000000000000"), "localhost:8000"))
	for i := 0; i < amountOfContacts; i++ {
		stringI := fmt.Sprintf("%02d", i)
		contact := NewContact(NewKademliaID("0000000"+stringI+"00000000000000000000000000000000"), "localhost:800"+stringI)
		rt.AddContact(contact)
	}
	contacts := rt.FindClosestContacts(NewKademliaID("2111111400000000000000000000000000000000"), amountOfContacts)
	for i := 0; i < amountOfContacts; i++ {
		stringI := fmt.Sprintf("%02d", i)
		expectedContact := NewContact(NewKademliaID("0000000"+stringI+"00000000000000000000000000000000"), "localhost:800"+stringI)
		found := false
		for _, contact := range contacts {
			if contact.ID.String() == expectedContact.ID.String() {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Expected contact %s not found", expectedContact.String())
		}
	}
}

func TestRoutingTableNoDuplicateContacts(t *testing.T) {
	amountOfContacts := 16
	rt := NewRoutingTable(NewContact(NewKademliaID("FFFFFFFF00000000000000000000000000000000"), "localhost:8000"))
	for i := 0; i < amountOfContacts; i++ {
		stringI := fmt.Sprintf("%02d", i)
		contact := NewContact(NewKademliaID("0000000"+stringI+"00000000000000000000000000000000"), "localhost:800"+stringI)
		rt.AddContact(contact)
		rt.AddContact(contact)
	}
	contacts := rt.FindClosestContacts(NewKademliaID("2111111400000000000000000000000000000000"), amountOfContacts)
	uniqueContacts := make(map[string]struct{})
	for _, contact := range contacts {
		uniqueContacts[contact.ID.String()] = struct{}{}
	}
	if len(uniqueContacts) != len(contacts) {
		t.Fatal("Expected no duplicate contacts")
	}
}
