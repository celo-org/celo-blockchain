package rlp

import (
	"reflect"
	"testing"
)

func TestCombine(t *testing.T) {
	one_str, _ := EncodeToBytes("Hello")
	two_str, _ := EncodeToBytes("World")
	list_of_strs, _ := EncodeToBytes([]string{"Hello", "World"})

	one_list, _ := EncodeToBytes([]string{"1"})
	two_list, _ := EncodeToBytes([]string{"2"})
	list_of_lists, _ := EncodeToBytes([][]string{{"1"}, {"2"}})

	str_and_list, _ := EncodeToBytes([]interface{}{"Hello", []string{"2"}})
	list_and_str, _ := EncodeToBytes([]interface{}{[]string{"1"}, "World"})

	empty_list, _ := EncodeToBytes([]byte{})
	str_and_empty_list, _ := EncodeToBytes([]interface{}{"Hello", []byte{}})
	empty_list_and_str, _ := EncodeToBytes([]interface{}{[]byte{}, "World"})
	two_empty_lists, _ := EncodeToBytes([]interface{}{[]byte{}, []byte{}})

	empty_str, _ := EncodeToBytes("")
	two_empty_str, _ := EncodeToBytes([]string{"", ""})

	testCases := []struct {
		name           string
		one            []byte
		two            []byte
		expectedResult []byte
	}{
		{
			name:           "Two strings",
			one:            one_str,
			two:            two_str,
			expectedResult: list_of_strs,
		},
		{
			name:           "Two empty strings",
			one:            empty_str,
			two:            empty_str,
			expectedResult: two_empty_str,
		},
		{
			name:           "Two lists",
			one:            one_list,
			two:            two_list,
			expectedResult: list_of_lists,
		},
		{
			name:           "Two empty lists",
			one:            empty_list,
			two:            empty_list,
			expectedResult: two_empty_lists,
		},
		{
			name:           "String and list",
			one:            one_str,
			two:            two_list,
			expectedResult: str_and_list,
		},
		{
			name:           "String and empty list",
			one:            one_str,
			two:            empty_list,
			expectedResult: str_and_empty_list,
		},
		{
			name:           "List and string",
			one:            one_list,
			two:            two_str,
			expectedResult: list_and_str,
		},
		{
			name:           "Empty list and string",
			one:            empty_list,
			two:            two_str,
			expectedResult: empty_list_and_str,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := Combine(c.one, c.two)
			if !reflect.DeepEqual(result, c.expectedResult) {
				t.Error("Expected", c.expectedResult, "got", result)
			}
		})
	}
}
