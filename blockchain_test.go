package cloudchain

// import (
// 	"log"
// 	"strconv"
// 	"sync"
// 	"testing"

// 	"github.com/google/go-cmp/cmp"
// )

// const (
// 	testDbDir  = ".testing_db"
// 	difficulty = 10 // Easy difficulty for testing purposes.
// )

// func TestBlockchainHappyPath(t *testing.T) {
// 	log.Println("Test start.")

// 	bc, err := NewBlockChain(testDbDir, difficulty, nil)
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	// Normally call bc.Close() instead of DeleteBlockchain in order to persist the backing boltDb store. We delete the store here for testing.
// 	defer DeleteBlockchain(bc)

// 	log.Println("Blockchain created.")

// 	msg1 := "John has 2 more PrestigeCoin than Jane"
// 	msg2 := "Jane has 10 more PrestigeCoin than David"

// 	_, err = bc.AddBlock([]byte(msg1))
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	_, err = bc.AddBlock([]byte(msg2))
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	log.Println("Blocks added.")

// 	bci := bc.Iterator()

// 	currBlock, _ := bci.Next()
// 	currMsg := string(currBlock.Data)
// 	if currMsg != msg2 {
// 		t.Errorf("Block held incorrect data. Expected: %s but got %s", msg2, currMsg)
// 	}

// 	currBlock, _ = bci.Next()
// 	currMsg = string(currBlock.Data)
// 	if string(currBlock.Data) != msg1 {
// 		t.Errorf("Block held incorrect data. Expected: %s but got %s", msg1, currMsg)
// 	}

// 	currBlock, _ = bci.Next()
// 	currMsg = string(currBlock.Data)
// 	if string(currBlock.Data) != "Genesis Block" {
// 		t.Errorf("Block held incorrect data. Expected: %s but got %s", "Genesis Block", currMsg)
// 	}

// 	currBlock, _ = bci.Next()
// 	if currBlock != nil {
// 		t.Errorf("Blockchain should be of length 3 but was not.")
// 	}
// }

// func TestBlockchainConcurrentInsert(t *testing.T) {
// 	log.Println("Test start.")

// 	genesisMessage := "Genesis Block"
// 	bc, err := NewBlockChain(testDbDir, difficulty, []byte(genesisMessage))
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	// Normally call bc.Close() instead of DeleteBlockchain in order to persist the backing boltDb store. We delete the store here for testing.
// 	defer DeleteBlockchain(bc)

// 	log.Println("Blockchain created.")

// 	// Create 1000 go routines that each add 1 block
// 	routines := 1000
// 	expectedMessages := map[string]int{genesisMessage: 1}
// 	addNumberBlocksConccurently(t, bc, routines, expectedMessages)

// 	log.Println("Blocks added.")

// 	// Verify 20 blocks added and messages are 0-19
// 	counter := readBlockchain(t, bc, expectedMessages)

// 	// + 1 for the Genesis Block
// 	if counter != routines+1 {
// 		t.Errorf("Retrieved %v blocks but expected %v blocks", counter, routines)
// 	}
// }

// func TestBlockchainConcurrentRead(t *testing.T) {
// 	log.Println("Test start.")

// 	genesisMessage := "Genesis Block"
// 	bc, err := NewBlockChain(testDbDir, difficulty, []byte(genesisMessage))
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	// Normally call bc.Close() instead of DeleteBlockchain in order to persist the backing boltDb store. We delete the store here for testing.
// 	defer DeleteBlockchain(bc)

// 	log.Println("Blockchain created.")

// 	// Create 100 go routines that each add 1 block
// 	addRoutines := 100
// 	expectedValues := map[string]int{genesisMessage: 1}
// 	addNumberBlocksConccurently(t, bc, addRoutines, expectedValues)

// 	// Create 30 go routines that each read the blockchain
// 	readRoutines := 30
// 	readBlockchainConcurrently(t, bc, readRoutines, expectedValues, addRoutines+1)
// }

// func TestBlockchainIteratorAtBlock(t *testing.T) {
// 	log.Println("Test start.")

// 	genesisMessage := "Genesis Block"
// 	bc, err := NewBlockChain(testDbDir, difficulty, []byte(genesisMessage))
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	// Normally call bc.Close() instead of DeleteBlockchain in order to persist the backing boltDb store. We delete the store here for testing.
// 	defer DeleteBlockchain(bc)

// 	log.Println("Blockchain created.")

// 	msg1 := "Message 1"
// 	msg2 := "Message 2"
// 	msg3 := "Message 3"

// 	b1, err := bc.AddBlock([]byte(msg1))
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	b2, err := bc.AddBlock([]byte(msg2))
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	_, err = bc.AddBlock([]byte(msg3))
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	log.Println("Blocks added.")

// 	bci := bc.IteratorAtBlock(b2.Header.Hash)

// 	b2FromStore, err := bci.Next()
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	if !cmp.Equal(b2FromStore, b2) {
// 		t.Errorf("Expected block with hash %v and data %s but got block with hash %v and data %s.", b2.Header.Hash, b2.Data, b2FromStore.Header.Hash, b2FromStore.Data)
// 	}

// 	b1FromStore, err := bci.Next()
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	if !cmp.Equal(b1FromStore, b1) {
// 		t.Errorf("Expected block with hash %v and data %s but got block with hash %v and data %s.", b1.Header.Hash, b1.Data, b1FromStore.Header.Hash, b1FromStore.Data)
// 	}

// 	genesisFromStore, err := bci.Next()
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	genesisFromStoreMessage := string(genesisFromStore.Data)
// 	if genesisFromStoreMessage != genesisMessage {
// 		t.Errorf("Expected genesis block but message was %s", genesisFromStoreMessage)
// 	}
// }

// func addNumberBlocksConccurently(t *testing.T, bc *Blockchain, routines int, expectedValues map[string]int) {
// 	var wg sync.WaitGroup
// 	wg.Add(routines)

// 	for i := 0; i < routines; i++ {
// 		expectedValues[strconv.Itoa(i)]++
// 		go func(msg int) {
// 			defer wg.Done()
// 			stringMsg := strconv.Itoa(msg)

// 			_, err := bc.AddBlock([]byte(stringMsg))
// 			if err != nil {
// 				t.Error(err)
// 			}
// 		}(i)
// 	}

// 	wg.Wait()
// }

// func readBlockchainConcurrently(t *testing.T, bc *Blockchain, routines int, expectedValues map[string]int, blockchainLength int) {
// 	var wg sync.WaitGroup
// 	wg.Add(routines)

// 	for i := 0; i < routines; i++ {
// 		go func() {
// 			defer wg.Done()

// 			bcLength := readBlockchain(t, bc, copyMap(expectedValues))
// 			if bcLength != blockchainLength {
// 				t.Errorf("Expected blockchain length %v but was %v", blockchainLength, bcLength)
// 			}
// 		}()
// 	}

// 	wg.Wait()
// }

// func readBlockchain(t *testing.T, bc *Blockchain, expectedValues map[string]int) int {
// 	bci := bc.Iterator()
// 	block, err := bci.Next()
// 	if err != nil {
// 		t.Errorf("Could not retrieve next block: %v", err)
// 	}

// 	totalBlockCount := 0
// 	for block != nil {
// 		msg := string(block.Data)

// 		if val, ok := expectedValues[msg]; ok && val > 0 {
// 			totalBlockCount++
// 			expectedValues[msg]--
// 		} else {
// 			t.Errorf("Unexpected val encountered: %v", msg)
// 		}

// 		block, err = bci.Next()
// 		if err != nil {
// 			t.Errorf("Could not retrieve next block: %v", err)
// 		}
// 	}

// 	for k, v := range expectedValues {
// 		if v != 0 {
// 			t.Errorf("Incorrect count for %v: %v.", k, v)
// 		}
// 	}

// 	return totalBlockCount
// }

// func copyMap(originalMap map[string]int) map[string]int {
// 	newMap := map[string]int{}
// 	for k, v := range originalMap {
// 		newMap[k] = v
// 	}
// 	return newMap
// }
