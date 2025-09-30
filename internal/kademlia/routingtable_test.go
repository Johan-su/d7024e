package kademlia

import (
	"fmt"
	"log"
	"os"
	"testing"
	"unsafe"
)


func graphviz_out(route *RoutingTable, filePath string) {
	f, err := os.Create(filePath)
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	fmt.Fprintf(f, "digraph G {\n")

	var nodeStack []*TreeNode

	nodeStack = append(nodeStack, route.root)
	for len(nodeStack) != 0 {
		var x *TreeNode
		x, nodeStack = nodeStack[len(nodeStack)-1], nodeStack[:len(nodeStack)-1]


		fmt.Fprintf(f, "n%v [label=\"[%v]%s-%s\"]\n", uintptr(unsafe.Pointer(x)), x.depth, x.low.String(), x.high.String())

		if x.left != nil {
			fmt.Fprintf(f, "n%v -> n%v\n", uintptr(unsafe.Pointer(x)), uintptr(unsafe.Pointer(x.left)))
			nodeStack = append(nodeStack, x.left)
		}
		if x.right != nil {
			fmt.Fprintf(f, "n%v -> n%v\n", uintptr(unsafe.Pointer(x)), uintptr(unsafe.Pointer(x.right)))
			nodeStack = append(nodeStack, x.right)
		}

	}

	fmt.Fprintf(f, "}\n")
}


func TestFindClosestContact(t *testing.T) {
	rt := NewRoutingTable(NewContact(NewKademliaID("FFFFFFFF00000000000000000000000000000000"), "localhost:8000"), 1)
	contacts := []Contact{
		NewContact(NewKademliaID("0000000100000000000000000000000000000000"), "localhost:8001"),
		NewContact(NewKademliaID("0000000200000000000000000000000000000000"), "localhost:8002"),
		NewContact(NewKademliaID("0000000300000000000000000000000000000000"), "localhost:8003"),
		NewContact(NewKademliaID("0000000400000000000000000000000000000000"), "localhost:8004"),
	}
	for _, c := range contacts {
		rt.AddContact(c)
	}
	closestContact := rt.FindClosestContacts(NewKademliaID("0000000200000000000000000000000000000000"), 1)

	if closestContact[0].ID.String() != "0000000200000000000000000000000000000000" {
		t.Fatalf("Expected closest contact to be 0000000200000000000000000000000000000000, got %s", closestContact[0].ID.String())
	}
}

func TestRoutingTableContactCount(t *testing.T) {
	amountOfContacts := 16
	rt := NewRoutingTable(NewContact(NewKademliaID("FFFFFFFF00000000000000000000000000000000"), "localhost:8000"), 1)
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
	rt := NewRoutingTable(NewContact(NewKademliaID("FFFFFFFF00000000000000000000000000000000"), "localhost:8000"), 1)
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
	rt := NewRoutingTable(NewContact(NewKademliaID("FFFFFFFF00000000000000000000000000000000"), "localhost:8000"), 1)
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

func TestGeneralKademlia(t *testing.T) {
	me := NewContact(NewKademliaID("0000000000000000000000000000000000000000"), "me")
	rt := NewRoutingTable(me, 3)

	// add _ random contacts
	for i := 0; i < 10000; i++ {
		id := NewRandomKademliaID()
		contact := NewContact(id, fmt.Sprintf("node%d", i))
		rt.AddContact(contact)
	}
	rt.DebugPrintTree()
	graphviz_out(rt, "output.dot")
}
