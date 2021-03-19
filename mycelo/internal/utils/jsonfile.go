package utils

import (
	"encoding/json"
	"io/ioutil"
)

func ReadJson(out interface{}, filepath string) error {
	byteValue, err := ioutil.ReadFile(filepath)
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

	return ioutil.WriteFile(filepath, byteValue, 0644)
}
