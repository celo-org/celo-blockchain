package testutil

import (
	"reflect"
	"testing"

	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/params"
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
		t.Parallel()
		g := NewGomegaWithT(t)
		fnValue := reflect.ValueOf(fn)

		argsValues := vmRunnerArguments(FailingVmRunner{}, args...)
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
		t.Parallel()
		g := NewGomegaWithT(t)
		fnValue := reflect.ValueOf(fn)

		argsValues := vmRunnerArguments(FailingVmRunner{}, args...)
		outs := fnValue.Call(argsValues)

		retValue := outs[0].Interface()
		g.Expect(retValue).To(Equal(defaultValue))
	})
}

// TestFailsWhenContractNotDeployed check calls fails when contract is not registered
func TestFailsWhenContractNotDeployed(t *testing.T, expectedError error, fn interface{}, args ...interface{}) {
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

	t.Run("should fail when contract not in registry", func(t *testing.T) {
		t.Parallel()
		g := NewGomegaWithT(t)
		fnValue := reflect.ValueOf(fn)

		vmRunner := NewMockEVMRunner()
		registrMock := NewRegistryMock()
		vmRunner.RegisterContract(params.RegistrySmartContractAddress, registrMock)

		argsValues := vmRunnerArguments(vmRunner, args...)
		outs := fnValue.Call(argsValues)

		err := outs[1].Interface()
		g.Expect(err).To(MatchError(expectedError))
	})
}

func vmRunnerArguments(vmRunner vm.EVMRunner, args ...interface{}) []reflect.Value {
	finalArgs := make([]interface{}, len(args)+1)
	finalArgs[0] = vmRunner
	finalArgs = append(finalArgs, args...)
	return mapToValues(finalArgs...)
}

func mapToValues(args ...interface{}) []reflect.Value {
	argsValues := make([]reflect.Value, len(args))
	for i, v := range args {
		argsValues[i] = reflect.ValueOf(v)
	}
	return argsValues
}
