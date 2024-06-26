// Code generated by moq; DO NOT EDIT.
// github.com/matryer/moq

package controller

import (
	"sync"
)

var (
	lockRepositoryMockCommand      sync.RWMutex
	lockRepositoryMockConfigFolder sync.RWMutex
	lockRepositoryMockCreate       sync.RWMutex
	lockRepositoryMockDelete       sync.RWMutex
	lockRepositoryMockExists       sync.RWMutex
	lockRepositoryMockKill         sync.RWMutex
	lockRepositoryMockRunning      sync.RWMutex
	lockRepositoryMockStart        sync.RWMutex
	lockRepositoryMockStop         sync.RWMutex
)

// Ensure, that RepositoryMock does implement Repository.
// If this is not the case, regenerate this file with moq.
var _ Repository = &RepositoryMock{}

// RepositoryMock is a mock implementation of Repository.
//
//     func TestSomethingThatUsesRepository(t *testing.T) {
//
//         // make and configure a mocked Repository
//         mockedRepository := &RepositoryMock{
//             CommandFunc: func(name string, command SocketCommand) ([]byte, error) {
// 	               panic("mock out the Command method")
//             },
//             ConfigFolderFunc: func() string {
// 	               panic("mock out the ConfigFolder method")
//             },
//             CreateFunc: func(name string, config []byte) error {
// 	               panic("mock out the Create method")
//             },
//             DeleteFunc: func(name string) error {
// 	               panic("mock out the Delete method")
//             },
//             ExistsFunc: func(name string) bool {
// 	               panic("mock out the Exists method")
//             },
//             KillFunc: func(name string)  {
// 	               panic("mock out the Kill method")
//             },
//             RunningFunc: func(name string) bool {
// 	               panic("mock out the Running method")
//             },
//             StartFunc: func(name string, command RunningConfig) error {
// 	               panic("mock out the Start method")
//             },
//             StopFunc: func(name string)  {
// 	               panic("mock out the Stop method")
//             },
//         }
//
//         // use mockedRepository in code that requires Repository
//         // and then make assertions.
//
//     }
type RepositoryMock struct {
	// CommandFunc mocks the Command method.
	CommandFunc func(name string, command SocketCommand) ([]byte, error)

	// ConfigFolderFunc mocks the ConfigFolder method.
	ConfigFolderFunc func() string

	// CreateFunc mocks the Create method.
	CreateFunc func(name string, config []byte) error

	// DeleteFunc mocks the Delete method.
	DeleteFunc func(name string) error

	// ExistsFunc mocks the Exists method.
	ExistsFunc func(name string) bool

	// KillFunc mocks the Kill method.
	KillFunc func(name string)

	// RunningFunc mocks the Running method.
	RunningFunc func(name string) bool

	// StartFunc mocks the Start method.
	StartFunc func(name string, runningConfig RunningConfig) error

	// StopFunc mocks the Stop method.
	StopFunc func(name string)

	// calls tracks calls to the methods.
	calls struct {
		// Command holds details about calls to the Command method.
		Command []struct {
			// Name is the name argument value.
			Name string
			// Command is the command argument value.
			Command SocketCommand
		}
		// ConfigFolder holds details about calls to the ConfigFolder method.
		ConfigFolder []struct {
		}
		// Create holds details about calls to the Create method.
		Create []struct {
			// Name is the name argument value.
			Name string
			// Config is the config argument value.
			Config []byte
		}
		// Delete holds details about calls to the Delete method.
		Delete []struct {
			// Name is the name argument value.
			Name string
		}
		// Exists holds details about calls to the Exists method.
		Exists []struct {
			// Name is the name argument value.
			Name string
		}
		// Kill holds details about calls to the Kill method.
		Kill []struct {
			// Name is the name argument value.
			Name string
		}
		// Running holds details about calls to the Running method.
		Running []struct {
			// Name is the name argument value.
			Name string
		}
		// Start holds details about calls to the Start method.
		Start []struct {
			// Name is the name argument value.
			Name string
			// RunningConfig is the command argument value.
			RunningConfig RunningConfig
		}
		// Stop holds details about calls to the Stop method.
		Stop []struct {
			// Name is the name argument value.
			Name string
		}
	}
}

// Command calls CommandFunc.
func (mock *RepositoryMock) Command(name string, command SocketCommand) ([]byte, error) {
	if mock.CommandFunc == nil {
		panic("RepositoryMock.CommandFunc: method is nil but Repository.Command was just called")
	}
	callInfo := struct {
		Name    string
		Command SocketCommand
	}{
		Name:    name,
		Command: command,
	}
	lockRepositoryMockCommand.Lock()
	mock.calls.Command = append(mock.calls.Command, callInfo)
	lockRepositoryMockCommand.Unlock()
	return mock.CommandFunc(name, command)
}

// CommandCalls gets all the calls that were made to Command.
// Check the length with:
//     len(mockedRepository.CommandCalls())
func (mock *RepositoryMock) CommandCalls() []struct {
	Name    string
	Command SocketCommand
} {
	var calls []struct {
		Name    string
		Command SocketCommand
	}
	lockRepositoryMockCommand.RLock()
	calls = mock.calls.Command
	lockRepositoryMockCommand.RUnlock()
	return calls
}

// ConfigFolder calls ConfigFolderFunc.
func (mock *RepositoryMock) ConfigFolder() string {
	if mock.ConfigFolderFunc == nil {
		panic("RepositoryMock.ConfigFolderFunc: method is nil but Repository.ConfigFolder was just called")
	}
	callInfo := struct {
	}{}
	lockRepositoryMockConfigFolder.Lock()
	mock.calls.ConfigFolder = append(mock.calls.ConfigFolder, callInfo)
	lockRepositoryMockConfigFolder.Unlock()
	return mock.ConfigFolderFunc()
}

// ConfigFolderCalls gets all the calls that were made to ConfigFolder.
// Check the length with:
//     len(mockedRepository.ConfigFolderCalls())
func (mock *RepositoryMock) ConfigFolderCalls() []struct {
} {
	var calls []struct {
	}
	lockRepositoryMockConfigFolder.RLock()
	calls = mock.calls.ConfigFolder
	lockRepositoryMockConfigFolder.RUnlock()
	return calls
}

// Create calls CreateFunc.
func (mock *RepositoryMock) Create(name string, config []byte) error {
	if mock.CreateFunc == nil {
		panic("RepositoryMock.CreateFunc: method is nil but Repository.Create was just called")
	}
	callInfo := struct {
		Name   string
		Config []byte
	}{
		Name:   name,
		Config: config,
	}
	lockRepositoryMockCreate.Lock()
	mock.calls.Create = append(mock.calls.Create, callInfo)
	lockRepositoryMockCreate.Unlock()
	return mock.CreateFunc(name, config)
}

// CreateCalls gets all the calls that were made to Create.
// Check the length with:
//     len(mockedRepository.CreateCalls())
func (mock *RepositoryMock) CreateCalls() []struct {
	Name   string
	Config []byte
} {
	var calls []struct {
		Name   string
		Config []byte
	}
	lockRepositoryMockCreate.RLock()
	calls = mock.calls.Create
	lockRepositoryMockCreate.RUnlock()
	return calls
}

// Delete calls DeleteFunc.
func (mock *RepositoryMock) Delete(name string) error {
	if mock.DeleteFunc == nil {
		panic("RepositoryMock.DeleteFunc: method is nil but Repository.Delete was just called")
	}
	callInfo := struct {
		Name string
	}{
		Name: name,
	}
	lockRepositoryMockDelete.Lock()
	mock.calls.Delete = append(mock.calls.Delete, callInfo)
	lockRepositoryMockDelete.Unlock()
	return mock.DeleteFunc(name)
}

// DeleteCalls gets all the calls that were made to Delete.
// Check the length with:
//     len(mockedRepository.DeleteCalls())
func (mock *RepositoryMock) DeleteCalls() []struct {
	Name string
} {
	var calls []struct {
		Name string
	}
	lockRepositoryMockDelete.RLock()
	calls = mock.calls.Delete
	lockRepositoryMockDelete.RUnlock()
	return calls
}

// Exists calls InstancesFunc.
func (mock *RepositoryMock) Instances() []string {
	return []string{"test1", "test2"}
}

// Exists calls ExistsFunc.
func (mock *RepositoryMock) Exists(name string) bool {
	if mock.ExistsFunc == nil {
		panic("RepositoryMock.ExistsFunc: method is nil but Repository.Exists was just called")
	}
	callInfo := struct {
		Name string
	}{
		Name: name,
	}
	lockRepositoryMockExists.Lock()
	mock.calls.Exists = append(mock.calls.Exists, callInfo)
	lockRepositoryMockExists.Unlock()
	return mock.ExistsFunc(name)
}

// ExistsCalls gets all the calls that were made to Exists.
// Check the length with:
//     len(mockedRepository.ExistsCalls())
func (mock *RepositoryMock) ExistsCalls() []struct {
	Name string
} {
	var calls []struct {
		Name string
	}
	lockRepositoryMockExists.RLock()
	calls = mock.calls.Exists
	lockRepositoryMockExists.RUnlock()
	return calls
}

// Kill calls KillFunc.
func (mock *RepositoryMock) Kill(name string) {
	if mock.KillFunc == nil {
		panic("RepositoryMock.KillFunc: method is nil but Repository.Kill was just called")
	}
	callInfo := struct {
		Name string
	}{
		Name: name,
	}
	lockRepositoryMockKill.Lock()
	mock.calls.Kill = append(mock.calls.Kill, callInfo)
	lockRepositoryMockKill.Unlock()
	mock.KillFunc(name)
}

// KillCalls gets all the calls that were made to Kill.
// Check the length with:
//     len(mockedRepository.KillCalls())
func (mock *RepositoryMock) KillCalls() []struct {
	Name string
} {
	var calls []struct {
		Name string
	}
	lockRepositoryMockKill.RLock()
	calls = mock.calls.Kill
	lockRepositoryMockKill.RUnlock()
	return calls
}

// Running calls RunningFunc.
func (mock *RepositoryMock) Running(name string) bool {
	if mock.RunningFunc == nil {
		panic("RepositoryMock.RunningFunc: method is nil but Repository.Running was just called")
	}
	callInfo := struct {
		Name string
	}{
		Name: name,
	}
	lockRepositoryMockRunning.Lock()
	mock.calls.Running = append(mock.calls.Running, callInfo)
	lockRepositoryMockRunning.Unlock()
	return mock.RunningFunc(name)
}

// RunningCalls gets all the calls that were made to Running.
// Check the length with:
//     len(mockedRepository.RunningCalls())
func (mock *RepositoryMock) RunningCalls() []struct {
	Name string
} {
	var calls []struct {
		Name string
	}
	lockRepositoryMockRunning.RLock()
	calls = mock.calls.Running
	lockRepositoryMockRunning.RUnlock()
	return calls
}

// Start calls StartFunc.
func (mock *RepositoryMock) Start(name string, runningConfig RunningConfig) error {
	if mock.StartFunc == nil {
		panic("RepositoryMock.StartFunc: method is nil but Repository.Start was just called")
	}
	callInfo := struct {
		Name          string
		RunningConfig RunningConfig
	}{
		Name:          name,
		RunningConfig: runningConfig,
	}
	lockRepositoryMockStart.Lock()
	mock.calls.Start = append(mock.calls.Start, callInfo)
	lockRepositoryMockStart.Unlock()
	return mock.StartFunc(name, runningConfig)
}

// StartCalls gets all the calls that were made to Start.
// Check the length with:
//     len(mockedRepository.StartCalls())
func (mock *RepositoryMock) StartCalls() []struct {
	Name          string
	RunningConfig RunningConfig
} {
	var calls []struct {
		Name          string
		RunningConfig RunningConfig
	}
	lockRepositoryMockStart.RLock()
	calls = mock.calls.Start
	lockRepositoryMockStart.RUnlock()
	return calls
}

// Stop calls StopFunc.
func (mock *RepositoryMock) Stop(name string) {
	if mock.StopFunc == nil {
		panic("RepositoryMock.StopFunc: method is nil but Repository.Stop was just called")
	}
	callInfo := struct {
		Name string
	}{
		Name: name,
	}
	lockRepositoryMockStop.Lock()
	mock.calls.Stop = append(mock.calls.Stop, callInfo)
	lockRepositoryMockStop.Unlock()
	mock.StopFunc(name)
}

// StopCalls gets all the calls that were made to Stop.
// Check the length with:
//     len(mockedRepository.StopCalls())
func (mock *RepositoryMock) StopCalls() []struct {
	Name string
} {
	var calls []struct {
		Name string
	}
	lockRepositoryMockStop.RLock()
	calls = mock.calls.Stop
	lockRepositoryMockStop.RUnlock()
	return calls
}
