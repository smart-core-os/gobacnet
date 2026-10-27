package encoding

import (
	"fmt"

	"github.com/smart-core-os/gobacnet/enum/pdutype"
	bactype "github.com/smart-core-os/gobacnet/types"
)

func (e *Encoder) ReadMultipleProperty(invokeID uint8, data bactype.ReadMultipleProperty) error {
	a := bactype.APDU{
		DataType:         pdutype.ConfirmedServiceRequest,
		Service:          bactype.ServiceConfirmedReadPropMultiple,
		MaxSegs:          0,
		MaxApdu:          MaxAPDU,
		InvokeId:         invokeID,
		SegmentedMessage: false,
		// SegmentedResponseAccepted stays false: the client can't reassemble a segmented response.
		// A device whose reply doesn't fit aborts with segmentation-not-supported instead, and
		// Client.ReadProperties falls back to reading each property on its own.
	}
	e.APDU(a)
	err := e.objects(data.Objects)
	if err != nil {
		return err
	}

	return e.Error()
}

func (e *Encoder) objects(objects []bactype.Object) error {
	var tag uint8
	for _, obj := range objects {
		tag = 0
		e.contextObjectID(tag, obj.ID.Type, obj.ID.Instance)
		// Tag 1 - Opening Tag
		tag = 1
		e.openingTag(tag)

		e.properties(obj.Properties)

		// Tag 1 - Closing Tag
		e.closingTag(tag)
	}
	return nil
}

func (e *Encoder) properties(properties []bactype.Property) error {
	// for each property
	var tag uint8
	for _, prop := range properties {
		// Tag 0 - Property ID
		tag = 0
		e.contextPropertyID(tag, prop.ID)

		// Tag 1 (OPTIONAL) - Array Length
		if prop.ArrayIndex != ArrayAll {
			tag = 1
			e.contextUnsigned(tag, prop.ArrayIndex)
		}
	}
	return nil
}

// ReadMultipleProperty decodes the service data of a ReadPropertyMultiple-Request, the
// counterpart to Encoder.ReadMultipleProperty. Properties without an array index have
// ArrayIndex set to ArrayAll.
func (d *Decoder) ReadMultipleProperty(data *bactype.ReadMultipleProperty) error {
	for d.Error() == nil && d.len() > 0 {
		var obj bactype.Object

		// Tag 0 - Object ID
		tag, meta, length := d.tagNumberAndValueLen()
		if tag != 0 {
			return &ErrorIncorrectTag{Expected: 0, Given: tag}
		}
		if !meta.isContextSpecific() || length != objectIDLen {
			return &ErrorWrongTagType{ContextTag}
		}
		obj.ID.Type, obj.ID.Instance = d.objectId()

		// Tag 1 - Opening Tag
		tag, meta = d.tagNumber()
		if tag != 1 {
			return &ErrorIncorrectTag{Expected: 1, Given: tag}
		}
		if meta.isClosing() || !meta.isOpening() {
			return &ErrorWrongTagType{OpeningTag}
		}

		tag, meta, length = d.tagNumberAndValueLen()
		for d.Error() == nil && !(tag == 1 && meta.isClosing()) {
			// Tag 0 - Property ID
			if tag != 0 {
				return &ErrorIncorrectTag{Expected: 0, Given: tag}
			}
			if !meta.isContextSpecific() {
				return &ErrorWrongTagType{ContextTag}
			}
			prop := bactype.Property{ID: d.propertyId(int(length)), ArrayIndex: ArrayAll}
			if d.len() == 0 {
				return fmt.Errorf("missing closing tag for object %v", obj.ID)
			}

			// Tag 1 (OPTIONAL) - Array Index
			tag, meta, length = d.tagNumberAndValueLen()
			if tag == 1 && !meta.isClosing() {
				prop.ArrayIndex = d.unsigned(int(length))
				if d.len() == 0 {
					return fmt.Errorf("missing closing tag for object %v", obj.ID)
				}
				tag, meta, length = d.tagNumberAndValueLen()
			}
			obj.Properties = append(obj.Properties, prop)
		}
		if len(obj.Properties) == 0 {
			return fmt.Errorf("no properties requested for object %v", obj.ID)
		}
		data.Objects = append(data.Objects, obj)
	}
	return d.Error()
}
