package types

import (
	"fmt"

	"github.com/smart-core-os/gobacnet/enum/errorclass"
	"github.com/smart-core-os/gobacnet/enum/errorcode"
	"github.com/smart-core-os/gobacnet/property"
)

// Error represents an error response from a BACnet device.
type Error struct {
	Class errorclass.ErrorClass
	Code  errorcode.ErrorCode
}

func (e Error) Error() string {
	return fmt.Sprintf("response class:%v code:%v", e.Class, e.Code)
}

// PropertyAccessError represents an error response as part of a ListOfResults response, typically from ReadPropertyMultiple.
type PropertyAccessError struct {
	Err      Error
	ObjectID ObjectID
	Property property.ID
}

func (e PropertyAccessError) Error() string {
	return fmt.Sprintf("%v object:%v:%v property:%v", e.Err, e.ObjectID.Type, e.ObjectID.Instance, e.Property)
}

func (e PropertyAccessError) Unwrap() error {
	return e.Err
}
