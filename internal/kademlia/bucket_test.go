package kademlia

import (
	"fmt"
	"testing"
)

func TestBucketInitialization(t *testing.T) {
	bucket := newBucket()
	if bucket == nil {
		t.Error("Expected bucket to be initialized, got nil")
	}
}

func TestAddContactToBucket(t *testing.T) {
	bucket := newBucket()
	contact := NewContact(NewKademliaID("0000000000000000000000000000000000000001"), "localhost:8001")
	bucket.AddContact(contact)
	firstElement := bucket.list.Front()
	if firstElement == nil {
		t.Errorf("Expected contact to be added, but bucket is empty")
	} else {
		firstContact := firstElement.Value.(Contact)
		if firstContact.ID.String() != contact.ID.String() {
			t.Errorf("Expected contact to be at the front of the bucket, got %s", firstContact.ID.String())
		}
	}
}

func TestBucketLength(t *testing.T) {
	bucket := newBucket()
	amountOfContacts := 5
	for i := 0; i < amountOfContacts; i++ {
		stringI := fmt.Sprintf("%02d", i)
		contact := NewContact(NewKademliaID("00000000000000000000000000000000000000"+stringI), "localhost:800"+stringI)
		bucket.AddContact(contact)
	}
	if bucket.Len() != amountOfContacts {
		t.Errorf("Expected bucket length to be %d, got %d", amountOfContacts, bucket.Len())
	}
}

func TestBucketContactOrder(t *testing.T) {
	bucket := newBucket()
	amountOfContacts := 5
	expectedOrder := []string{}
	for i := 0; i < amountOfContacts; i++ {
		stringI := fmt.Sprintf("%02d", i)
		contactString := "00000000000000000000000000000000000000" + stringI
		contact := NewContact(NewKademliaID(contactString), "localhost:800"+stringI)
		bucket.AddContact(contact)
		expectedOrder = append([]string{contactString}, expectedOrder...)
	}
	i := 0
	for e := bucket.list.Front(); e != nil; e = e.Next() {
		contact := e.Value.(Contact)
		if contact.ID.String() != expectedOrder[i] {
			t.Errorf("Expected contact at position %d to be %s, got %s", i, expectedOrder[i], contact.ID.String())
		}
		i++
	}
}

