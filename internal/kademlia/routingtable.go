package kademlia

import (
	"fmt"
	"sort"
	"strings"
)

const bucketSize = 20

// RoutingTable definition
// keeps a refrence contact of me and an array of buckets
//
//	type RoutingTable struct {
//		me      Contact
//		buckets [IDLength * 8]*bucket
//	}
type RoutingTable struct {
	me   Contact
	b    int // Branching parameter
	root *TreeNode
}

type TreeNode struct {
	low    *KademliaID
	high   *KademliaID
	depth  int
	bucket *bucket
	left   *TreeNode
	right  *TreeNode
}

// NewRoutingTable returns a new instance of a RoutingTable
// func NewRoutingTable(me Contact) *RoutingTable {
// 	routingTable := &RoutingTable{}
// 	for i := 0; i < IDLength*8; i++ {
// 		routingTable.buckets[i] = newBucket()
// 	}
// 	routingTable.me = me
// 	return routingTable
// }

func NewRoutingTable(me Contact, b int) *RoutingTable {
	minID := NewKademliaID("0000000000000000000000000000000000000000")
	maxID := NewKademliaID("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")
	routingTable := &RoutingTable{}
	routingTable.me = me
	routingTable.b = b

	routingTable.root = &TreeNode{}
	routingTable.root.depth = 0
	routingTable.root.low = minID
	routingTable.root.high = maxID
	routingTable.root.bucket = newBucket()
	return routingTable
}

// AddContact adds a new contact to the routing table
func (routingTable *RoutingTable) AddContact(contact Contact) {
	node := routingTable.findLeafNode(contact.ID)
	wasFull := node.bucket.Len() == bucketSize

	added := node.bucket.AddContact(contact)

	if !added && wasFull {
		if routingTable.splitRule(node) {
			routingTable.splitNode(node)
			routingTable.AddContact(contact)
		}
	}
}

// findLeafNode traverses the tree to find the leaf node (bucket) for the target ID
func (routingTable *RoutingTable) findLeafNode(id *KademliaID) *TreeNode {
	current := routingTable.root

	for {
		if current.left == nil && current.right == nil {
			return current
		}

		if getBit(id, current.depth) == 0 {
			current = current.left
		} else {
			current = current.right
		}
	}
}

// getBit returns a specific bit from a Kademlia ID
func getBit(id *KademliaID, pos int) byte {
	byteIndex := pos / 8
	bitIndex := 7 - (pos % 8)
	return (id[byteIndex] >> bitIndex) & 1
}

// splitNode splits a leaf node into two child nodes and redistributes its contacts
func (routingTable *RoutingTable) splitNode(node *TreeNode) {
	if node.depth >= IDLength*8 {
		return
	}

	midpoint := calcMidpoint(node.low, node.high)

	node.left = &TreeNode{
		low:    node.low,
		high:   midpoint,
		depth:  node.depth + 1,
		bucket: newBucket(),
	}

	node.right = &TreeNode{
		low:    incID(midpoint),
		high:   node.high,
		depth:  node.depth + 1,
		bucket: newBucket(),
	}

	for e := node.bucket.list.Front(); e != nil; e = e.Next() {
		contact := e.Value.(Contact)
		if routingTable.idInRange(contact.ID, node.left.low, node.left.high) {
			node.left.bucket.AddContact(contact)
		} else {
			node.right.bucket.AddContact(contact)
		}
	}

	node.bucket = newBucket()
}

// calcMidpoint calculates the midpoint between two IDs
func calcMidpoint(low, high *KademliaID) *KademliaID {
	mid := &KademliaID{}
	carry := 0
	for i := 0; i < IDLength; i++ {
		sum := int(low[i]) + int(high[i]) + carry
		mid[i] = byte(sum / 2)
		carry = (sum % 2) * 256
	}
	return mid
}

// incID increments a Kademlia ID by 1
func incID(id *KademliaID) *KademliaID {
	newID := &KademliaID{}
	copy(newID[:], id[:])

	for i := IDLength - 1; i >= 0; i-- {
		if newID[i] < 0xFF {
			newID[i]++
			break
		} else {
			newID[i] = 0
		}
	}
	return newID
}

// splitRule determines if a bucket should split or not, according to the new rules
func (routingTable *RoutingTable) splitRule(node *TreeNode) bool {
	if node.bucket.Len() < bucketSize {
		return false
	}

	if routingTable.idInRange(routingTable.me.ID, node.low, node.high) {
		return true
	}

	if node.depth%routingTable.b != 0 {
		return true
	}
	return false
}

// idInRange checks if a Kademlia ID falls within a specific range (low, high)
func (routingTable *RoutingTable) idInRange(id, low, high *KademliaID) bool {
	for i := 0; i < IDLength; i++ {
		if id[i] < low[i] {
			return false
		}
		if id[i] > high[i] {
			return false
		}
	}
	return true
}

// FindClosestContacts find the closest contacts to a target ID
func (routingTable *RoutingTable) FindClosestContacts(target *KademliaID, count int) []Contact {
	var candidates ContactCandidates

	if candidates.Len() < count {
		routingTable.collectNearbyBuckets(target, &candidates, count)
	}

	candidates.Sort()
	if count > candidates.Len() {
		count = candidates.Len()
	}
	return candidates.GetContacts(count)

}

// collectNearbyBuckets collect contacts from leaf nodes, sorted by how close they are to the target
func (routingTable *RoutingTable) collectNearbyBuckets(target *KademliaID, candidates *ContactCandidates, count int) {
	leafNodes := routingTable.getAllLeafNodes()

	routingTable.sortLeafNodes(leafNodes, target)

	for _, node := range leafNodes {
		if candidates.Len() >= count {
			break
		}
		candidates.Append(node.bucket.GetContactAndCalcDistance(target))
	}
}

// getAllLeafNodes collects all leaf nodes (buckets) in the tree
func (routingTable *RoutingTable) getAllLeafNodes() []*TreeNode {
	var leaves []*TreeNode
	routingTable.collectLeaves(routingTable.root, &leaves)
	return leaves
}

// collectLeaves rec. collects all leaf nodes from a subtree
func (routingTable *RoutingTable) collectLeaves(node *TreeNode, leaves *[]*TreeNode) {
	if node == nil {
		return
	}

	if node.left == nil && node.right == nil {
		*leaves = append(*leaves, node)
		return
	}

	routingTable.collectLeaves(node.left, leaves)
	routingTable.collectLeaves(node.right, leaves)
}

// sortLeafNodes sort leaf nodes by their midpoints distance to the target ID
func (routingTable *RoutingTable) sortLeafNodes(nodes []*TreeNode, target *KademliaID) {
	sort.Slice(nodes, func(i, j int) bool {
		midpointI := calcMidpoint(nodes[i].low, nodes[i].high)
		midpointJ := calcMidpoint(nodes[j].low, nodes[j].high)

		distanceI := midpointI.CalcDistance(target)
		distanceJ := midpointJ.CalcDistance(target)

		return distanceI.Less(distanceJ)
	})
}

// AddContact add a new contact to the correct Bucket
// func (routingTable *RoutingTable) AddContact(contact Contact) {
// 	bucketIndex := routingTable.getBucketIndex(contact.ID)
// 	bucket := routingTable.buckets[bucketIndex]
// 	bucket.AddContact(contact)
// }

// FindClosestContacts finds the count closest Contacts to the target in the RoutingTable
// func (routingTable *RoutingTable) FindClosestContacts(target *KademliaID, count int) []Contact {
// 	var candidates ContactCandidates
// 	bucketIndex := routingTable.getBucketIndex(target)
// 	bucket := routingTable.buckets[bucketIndex]

// 	candidates.Append(bucket.GetContactAndCalcDistance(target))

// 	for i := 1; (bucketIndex-i >= 0 || bucketIndex+i < IDLength*8) && candidates.Len() < count; i++ {
// 		if bucketIndex-i >= 0 {
// 			bucket = routingTable.buckets[bucketIndex-i]
// 			candidates.Append(bucket.GetContactAndCalcDistance(target))
// 		}
// 		if bucketIndex+i < IDLength*8 {
// 			bucket = routingTable.buckets[bucketIndex+i]
// 			candidates.Append(bucket.GetContactAndCalcDistance(target))
// 		}
// 	}

// 	candidates.Sort()

// 	if count > candidates.Len() {
// 		count = candidates.Len()
// 	}

// 	return candidates.GetContacts(count)
// }

// getBucketIndex get the correct Bucket index for the KademliaID
func (routingTable *RoutingTable) getBucketIndex(id *KademliaID) int {
	distance := id.CalcDistance(routingTable.me.ID)
	for i := 0; i < IDLength; i++ {
		for j := 0; j < 8; j++ {
			if (distance[i]>>uint8(7-j))&0x1 != 0 {
				return i*8 + j
			}
		}
	}

	return IDLength*8 - 1
}

// prints the routing tree in a readable format
func (routingTable *RoutingTable) DebugPrintTree() {
	fmt.Println("=== Routing Table Tree Structure ===")
	routingTable.printNode(routingTable.root, 0)

	total := routingTable.countContacts(routingTable.root)
	fmt.Printf("=== TOTAL CONTACTS IN TABLE: %d ===\n", total)
	fmt.Println("====================================")
}

func (routingTable *RoutingTable) printNode(node *TreeNode, indent int) {
	if node == nil {
		return
	}
	prefix := strings.Repeat("  ", indent)

	treeSymbol := "├─"
	if indent == 0 {
		treeSymbol = "└─" // root
	}

	// check if this is a leaf node
	nodeType := "INTERNAL"
	if node.left == nil && node.right == nil {
		nodeType = "LEAF"
	}

	// mark if this bucket contains *my own ID*
	marker := ""
	if routingTable.idInRange(routingTable.me.ID, node.low, node.high) {
		marker = " (*)"
	}

	fmt.Printf("%s%sDepth=%d Range=[%s, %s] Contacts=%d/%d (%s)%s\n",
		prefix, treeSymbol, node.depth,
		node.low.String()[:8]+"...", // first 8 chars for brevity
		node.high.String()[:8]+"...",
		node.bucket.Len(), bucketSize, nodeType, marker)

	// print contacts in leaf nodes
	if node.left == nil && node.right == nil && node.bucket.Len() > 0 {
		contactPrefix := strings.Repeat("  ", indent+1)
		fmt.Printf("%sContacts: ", contactPrefix)
		for e := node.bucket.list.Front(); e != nil; e = e.Next() {
			contact := e.Value.(Contact)
			fmt.Printf("%s ", contact.ID.String()[:8]+"...")
		}
		fmt.Println()
	}

	// recurse into children
	if node.left != nil || node.right != nil {
		routingTable.printNode(node.left, indent+1)
		routingTable.printNode(node.right, indent+1)
	}
}

// countContacts recursively sums up all contacts in the leaves.
func (routingTable *RoutingTable) countContacts(node *TreeNode) int {
	if node == nil {
		return 0
	}
	if node.left == nil && node.right == nil {
		return node.bucket.Len()
	}
	return routingTable.countContacts(node.left) + routingTable.countContacts(node.right)
}
