package controller

// BlasterControllerError represents an blaster error.
type BlasterControllerError struct {
	ErrorString string
}

// Error implements error interface.
func (pe *BlasterControllerError) Error() string { return pe.ErrorString }

var (
	// ErrBlasterNotExists there is not blaster instance.
	ErrBlasterNotExists = &BlasterControllerError{"blaster instance does not exist"}
	// ErrBlasterRunning there is one BlasterInstance running.
	ErrBlasterRunning = &BlasterControllerError{"blaster instance is running"}
	// ErrBlasterNotRunning there is no BlasterInstance running.
	ErrBlasterNotRunning = &BlasterControllerError{"blaster instance is not running"}
)
