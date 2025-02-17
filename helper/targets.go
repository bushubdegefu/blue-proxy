package helper

import (
	"encoding/json"
	"fmt"
	"os"
)

type Target struct {
	Targets []string `json:"targets"`
}

var Targets Target

func LoadData() {
	// Open the JSON file
	file, err := os.Open("targets.json")
	if err != nil {
		fmt.Println("Error opening targets JSON file:", err)
		panic("No targets to proxy too")
	}
	defer file.Close() // Defer closing the file until the function returns

	// Decode the JSON content into the data structure

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&Targets)
	if err != nil {
		fmt.Println("Error decoding JSON:", err)
		panic("Error decoding targets to proxy too from json file")
	}

}
