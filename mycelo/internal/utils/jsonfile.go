package utils

import (
	"encoding/json"
	"os"
)

func ReadJson(out interface{}, filepath string) error {
	byteValue, err := os.ReadFile(filepath)
	if err != nil {
		return err
	}

	return json.Unmarshal(byteValue, out)
}

func WriteJson(in interface{}, filepath string) error {
	byteValue, err := json.MarshalIndent(in, " ", " ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath, byteValue, 0644)
}
