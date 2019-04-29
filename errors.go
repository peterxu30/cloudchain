package cloudchain

import "fmt"

type collisionError struct {
	hash string
}

func (e *collisionError) Error() string {
	return fmt.Sprintf("A block with this hash already exists. Hash: %s", e.hash)
}
