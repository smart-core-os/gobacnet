package server

import (
	"sync"

	"github.com/smart-core-os/gobacnet/enum/engineeringunit"
	"github.com/smart-core-os/gobacnet/enum/eventstate"
	"github.com/smart-core-os/gobacnet/enum/reliability"
	"github.com/smart-core-os/gobacnet/property"
	"github.com/smart-core-os/gobacnet/types"
	"github.com/smart-core-os/gobacnet/types/objecttype"
)

// AnalogValue is a read-only Analog Value object whose value is set locally.
type AnalogValue struct {
	id    types.ObjectID
	name  string
	units engineeringunit.EngineeringUnit

	mu           sync.RWMutex
	presentValue float32
	description  string
	reliability  reliability.Reliability
	outOfService bool
}

// NewAnalogValue returns an Analog Value with a Present_Value of 0 and no fault.
func NewAnalogValue(instance types.ObjectInstance, name string, units engineeringunit.EngineeringUnit) *AnalogValue {
	return &AnalogValue{
		id:    types.ObjectID{Type: objecttype.AnalogValue, Instance: instance},
		name:  name,
		units: units,
	}
}

func (o *AnalogValue) SetPresentValue(v float32) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.presentValue = v
}

func (o *AnalogValue) SetDescription(s string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.description = s
}

// SetReliability sets Reliability. Anything other than NoFaultDetected also sets the fault
// bit of Status_Flags.
func (o *AnalogValue) SetReliability(r reliability.Reliability) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.reliability = r
}

func (o *AnalogValue) SetOutOfService(b bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.outOfService = b
}

func (o *AnalogValue) ObjectID() types.ObjectID {
	return o.id
}

func (o *AnalogValue) PropertyIDs() []property.ID {
	return []property.ID{
		property.ObjectIdentifier, property.ObjectName, property.ObjectType, property.PresentValue,
		property.Description, property.StatusFlags, property.EventState, property.Reliability,
		property.OutOfService, property.Units,
	}
}

func (o *AnalogValue) ReadProperty(id property.ID) (interface{}, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	switch id {
	case property.ObjectIdentifier:
		return o.id, nil
	case property.ObjectName:
		return o.name, nil
	case property.ObjectType:
		return types.Enumerated(objecttype.AnalogValue), nil
	case property.PresentValue:
		return o.presentValue, nil
	case property.Description:
		return o.description, nil
	case property.StatusFlags:
		return statusFlags(false, o.reliability != reliability.NoFaultDetected, false, o.outOfService), nil
	case property.EventState:
		return types.Enumerated(eventstate.Normal), nil
	case property.Reliability:
		return types.Enumerated(o.reliability), nil
	case property.OutOfService:
		return o.outOfService, nil
	case property.Units:
		return types.Enumerated(o.units), nil
	}
	return nil, ErrUnknownProperty
}
