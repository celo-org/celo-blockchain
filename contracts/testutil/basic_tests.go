package testutil

import (
	"reflect"
	"testing"

	. "github.com/onsi/gomega"
)

// TestFailOnFailingRunner checks that function doesn't silent VMRunner error
func TestFailOnFailingRunner(t *testing.T, fn interface{}, args ...interface{}) {
	fnType := reflect.TypeOf(fn)
	if fnType.Kind() != reflect.Func {
		t.Fatalf("Bad test: Calling testFailOnFailingRunner without a function")
	}

	if fnType.NumIn() != len(args)+1 {
		t.Fatalf("Bad test: function bad number of arguments")
	}

	if fnType.NumOut() != 2 {
		t.Fatalf("Bad test: function must return (value, error)")
	}

	t.Run("should fail if vmRunner fails", func(t *testing.T) {
		g := NewGomegaWithT(t)
		fnValue := reflect.ValueOf(fn)

		argsValues := make([]reflect.Value, len(args)+1)
		argsValues[0] = reflect.ValueOf(FailingVmRunner{})
		for i, v := range args {
			argsValues[i+1] = reflect.ValueOf(v)
		}

		outs := fnValue.Call(argsValues)

		err := outs[1].Interface()
		g.Expect(err).To(MatchError(ErrFailingRunner))
	})
}

// TestReturnsDefaultOnFailingRunner checks that function will return a default value if there's an error on VMRunner
func TestReturnsDefaultOnFailingRunner(t *testing.T, defaultValue interface{}, fn interface{}, args ...interface{}) {
	fnType := reflect.TypeOf(fn)
	if fnType.Kind() != reflect.Func {
		t.Fatalf("Bad test: Calling testFailOnFailingRunner without a function")
	}

	if fnType.NumIn() != len(args)+1 {
		t.Fatalf("Bad test: function bad number of arguments")
	}

	if fnType.NumOut() != 1 {
		t.Fatalf("Bad test: function must return single value")
	}

	t.Run("should returns default if vmRunner fails", func(t *testing.T) {
		g := NewGomegaWithT(t)
		fnValue := reflect.ValueOf(fn)

		argsValues := make([]reflect.Value, len(args)+1)
		argsValues[0] = reflect.ValueOf(FailingVmRunner{})
		for i, v := range args {
			argsValues[i+1] = reflect.ValueOf(v)
		}

		outs := fnValue.Call(argsValues)

		retValue := outs[0].Interface()
		g.Expect(retValue).To(Equal(defaultValue))
	})
}
