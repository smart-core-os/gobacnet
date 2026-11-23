package server

import (
	"github.com/smart-core-os/gobacnet/encoding"
	"github.com/smart-core-os/gobacnet/property"
	"github.com/smart-core-os/gobacnet/types"
	"github.com/smart-core-os/gobacnet/types/objecttype"
)

// DeviceConfig describes the server's Device object.
type DeviceConfig struct {
	// Instance is the device instance number, unique on the BACnet internetwork.
	Instance types.ObjectInstance
	// Name is the Object_Name of the device, also unique on the internetwork.
	Name        string
	Description string

	VendorID                   uint32
	VendorName                 string
	ModelName                  string
	FirmwareRevision           string
	ApplicationSoftwareVersion string
}

const (
	// protocolRevision is the BACnet protocol revision the server claims, which fixes the sizes
	// of the services and object types supported bit strings.
	protocolRevision         = 14
	servicesSupportedBits    = 41 // BACnetServicesSupported, revision 14
	objectTypesSupportedBits = 55 // BACnetObjectTypesSupported, revision 14

	segmentationNone = 3 // BACnetSegmentation no-segmentation

	apduTimeoutMillis   = 3000
	numberOfAPDURetries = 3
)

// BACnetServicesSupported bit positions of the services the server executes.
const (
	serviceReadProperty         = 12
	serviceReadPropertyMultiple = 14
	serviceIAm                  = 26
	serviceWhoIs                = 34
)

// device is the server's Device object.
type device struct {
	cfg DeviceConfig
	srv *Server
}

func (d *device) ObjectID() types.ObjectID {
	return types.ObjectID{Type: objecttype.Device, Instance: d.cfg.Instance}
}

func (d *device) PropertyIDs() []property.ID {
	return []property.ID{
		property.ObjectIdentifier, property.ObjectName, property.ObjectType, property.SystemStatus,
		property.VendorName, property.VendorIdentifier, property.ModelName, property.FirmwareRevision,
		property.ApplicationSoftwareVersion, property.Description, property.ProtocolVersion,
		property.ProtocolRevision, property.ProtocolServicesSupported, property.ProtocolObjectTypesSupported,
		property.ObjectList, property.MaxApduLengthAccepted, property.SegmentationSupported,
		property.ApduTimeout, property.NumberOfApduRetries, property.DeviceAddressBinding,
		property.DatabaseRevision,
	}
}

func (d *device) ReadProperty(id property.ID) (interface{}, error) {
	switch id {
	case property.ObjectIdentifier:
		return d.ObjectID(), nil
	case property.ObjectName:
		return d.cfg.Name, nil
	case property.ObjectType:
		return types.Enumerated(objecttype.Device), nil
	case property.SystemStatus:
		return types.Enumerated(0), nil // operational
	case property.VendorName:
		return d.cfg.VendorName, nil
	case property.VendorIdentifier:
		return d.cfg.VendorID, nil
	case property.ModelName:
		return d.cfg.ModelName, nil
	case property.FirmwareRevision:
		return d.cfg.FirmwareRevision, nil
	case property.ApplicationSoftwareVersion:
		return d.cfg.ApplicationSoftwareVersion, nil
	case property.Description:
		return d.cfg.Description, nil
	case property.ProtocolVersion:
		return uint32(1), nil
	case property.ProtocolRevision:
		return uint32(protocolRevision), nil
	case property.ProtocolServicesSupported:
		return bitString(servicesSupportedBits, serviceReadProperty, serviceReadPropertyMultiple, serviceIAm, serviceWhoIs), nil
	case property.ProtocolObjectTypesSupported:
		return bitString(objectTypesSupportedBits, d.srv.objectTypes()...), nil
	case property.ObjectList:
		return d.srv.objectList(), nil
	case property.MaxApduLengthAccepted:
		return uint32(encoding.MaxAPDU), nil
	case property.SegmentationSupported:
		return types.Enumerated(segmentationNone), nil
	case property.ApduTimeout:
		return uint32(apduTimeoutMillis), nil
	case property.NumberOfApduRetries:
		return uint32(numberOfAPDURetries), nil
	case property.DeviceAddressBinding:
		return List{}, nil
	case property.DatabaseRevision:
		return d.srv.databaseRevision(), nil
	}
	return nil, ErrUnknownProperty
}
