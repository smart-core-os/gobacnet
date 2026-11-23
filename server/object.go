// Package server implements the device side of BACnet/IP: a local BACnet device whose
// objects other devices can discover and read.
//
// The server executes Who-Is, ReadProperty and ReadPropertyMultiple. Other confirmed services
// are rejected as unrecognised, and segmented requests and responses are not supported.
package server

import (
	"github.com/smart-core-os/gobacnet/enum/errorclass"
	"github.com/smart-core-os/gobacnet/enum/errorcode"
	"github.com/smart-core-os/gobacnet/property"
	"github.com/smart-core-os/gobacnet/types"
)

// Object is a BACnet object served by a Server.
// Implementations must be safe for concurrent use.
type Object interface {
	// ObjectID returns the object's type and instance. It must not change.
	ObjectID() types.ObjectID
	// PropertyIDs lists the properties the object supports, in the order they are returned when
	// all properties are read. Property_List is handled by the server and should not be included.
	PropertyIDs() []property.ID
	// ReadProperty returns the value of a property: a value supported by encoding.Encoder.AppData,
	// an Array for BACnetARRAY properties, or a List for BACnetLIST properties.
	// Return a types.Error to send a specific BACnet error, such as ErrUnknownProperty.
	ReadProperty(id property.ID) (interface{}, error)
}

// Array is the value of a BACnetARRAY property. Elements can be read individually by index.
type Array []interface{}

// List is the value of a BACnetLIST property, read only as a whole.
type List []interface{}

var (
	ErrUnknownObject        = types.Error{Class: errorclass.Object, Code: errorcode.UnknownObject}
	ErrUnknownProperty      = types.Error{Class: errorclass.Property, Code: errorcode.UnknownProperty}
	ErrInvalidArrayIndex    = types.Error{Class: errorclass.Property, Code: errorcode.InvalidArrayIndex}
	ErrPropertyIsNotAnArray = types.Error{Class: errorclass.Property, Code: errorcode.PropertyIsNotAnArray}
)

// Special property identifiers that ReadPropertyMultiple expands to a set of properties.
const (
	propertyOptional property.ID = 80
	propertyRequired property.ID = 105
)

// propertyList is the Property_List of an object: every property except the four that
// all objects have (clause 12.1.1.4.1).
func propertyList(o Object) Array {
	var list Array
	for _, id := range o.PropertyIDs() {
		switch id {
		case property.ObjectIdentifier, property.ObjectName, property.ObjectType, property.PropertyList:
			continue
		}
		list = append(list, types.Enumerated(id))
	}
	return list
}

// statusFlags encodes a BACnetStatusFlags bit string: in-alarm, fault, overridden, out-of-service.
func statusFlags(inAlarm, fault, overridden, outOfService bool) types.BitString {
	var b byte
	for i, set := range []bool{inAlarm, fault, overridden, outOfService} {
		if set {
			b |= 0x80 >> i
		}
	}
	return types.BitString{Bytes: []byte{b}, IgnoreTrailingBits: 4}
}

// bitString returns a bit string of n bits with the given bits set.
func bitString(n int, set ...int) types.BitString {
	bs := types.BitString{Bytes: make([]byte, (n+7)/8), IgnoreTrailingBits: uint8((8 - n%8) % 8)}
	for _, i := range set {
		bs.Bytes[i/8] |= 0x80 >> (i % 8)
	}
	return bs
}
