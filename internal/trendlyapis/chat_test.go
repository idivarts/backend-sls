package trendlyapis_test

import (
	"fmt"
	"testing"
)

func TestNilTest(t *testing.T) {
	userObject := map[string]interface{}{
		"name": nil, // Example of nil or missing value
	}

	// Check if the key exists and is a string
	name, _ := userObject["name"].(string)
	// ok {
	// 	fmt.Println("Name:", name) // Successfully cast to string
	// } else {
	// 	fmt.Println("Name is either nil or not a string")
	// }
	fmt.Println("Name:", name)
}
