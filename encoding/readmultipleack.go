package encoding

import (
	"fmt"

	"go.uber.org/multierr"

	"github.com/smart-core-os/gobacnet/enum/errorclass"
	"github.com/smart-core-os/gobacnet/enum/errorcode"
	"github.com/smart-core-os/gobacnet/enum/pdutype"
	bactype "github.com/smart-core-os/gobacnet/types"
)

func (e *Encoder) ReadMultiplePropertyAck(invokeID uint8, data bactype.ReadMultipleProperty) error {
	a := bactype.APDU{
		DataType: pdutype.ComplexAck,
		Service:  bactype.ServiceConfirmedReadPropMultiple,
		InvokeId: invokeID,
	}
	e.APDU(a)
	err := e.objectsWithData(data.Objects)
	if err != nil {
		return err
	}

	return e.Error()
}

func (e *Encoder) objectsWithData(objects []bactype.Object) error {
	var tag uint8
	for _, obj := range objects {
		tag = 0
		e.contextObjectID(tag, obj.ID.Type, obj.ID.Instance)
		// Tag 1 - Opening Tag
		tag = 1
		e.openingTag(tag)

		e.propertiesWithData(obj.Properties)

		// Tag 1 - Closing Tag
		e.closingTag(tag)
	}
	return nil
}

func (e *Encoder) propertiesWithData(properties []bactype.Property) error {
	var tag uint8
	for _, prop := range properties {
		// Tag 2 - Property ID
		tag = 2
		e.contextPropertyID(tag, prop.ID)

		// Tag 3 (OPTIONAL) - Array Length
		tag++
		if prop.ArrayIndex != ArrayAll {
			e.contextUnsigned(tag, prop.ArrayIndex)
		}

		// Tag 4 Opening Tag
		tag++
		openedTag := tag
		e.openingTag(openedTag)
		e.write(prop.Data)
		e.closingTag(openedTag)

		e.write(prop.Data)
	}
	return e.Error()
}

func (d *Decoder) ReadMultiplePropertyAck(data *bactype.ReadMultipleProperty) error {
	err := d.objectsWithData(&data.Objects)
	if err != nil {
		d.err = err
	}
	return d.Error()
}

func (d *Decoder) bacError(bacErr *bactype.Error) error {
	data, err := d.AppData()
	if err != nil {
		return err
	}
	switch val := data.(type) {
	case uint32:
		bacErr.Class = errorclass.ErrorClass(val)
	default:
		return fmt.Errorf("receive bacnet error of unknown type")
	}

	data, err = d.AppData()
	if err != nil {
		return err
	}
	switch val := data.(type) {
	case uint32:
		bacErr.Code = errorcode.ErrorCode(val)
	default:
		return fmt.Errorf("receive bacnet error of unknown type")
	}
	return nil
}

func (d *Decoder) objectsWithData(objects *[]bactype.Object) error {
	var errs []error
	obj := bactype.Object{}
	for d.Error() == nil && d.len() > 0 {
		obj.Properties = []bactype.Property{}

		// Tag 0 - Object ID
		var expectedTag uint8
		tag, meta, length := d.tagNumberAndValueLen()
		objType, instance := d.objectId()

		if tag != expectedTag {
			return &ErrorIncorrectTag{Expected: expectedTag, Given: tag}
		}
		if !meta.isContextSpecific() {
			return &ErrorWrongTagType{ContextTag}
		}

		obj.ID.Type = objType
		obj.ID.Instance = instance

		// Tag 1 - Opening Tag
		expectedTag++
		tag, meta = d.tagNumber()
		if tag != expectedTag {
			return &ErrorIncorrectTag{Expected: expectedTag, Given: tag}
		}
		if !meta.isOpening() {
			return &ErrorWrongTagType{OpeningTag}
		}
		// Tag 2 - Property Tag
		tag, meta, length = d.tagNumberAndValueLen()

		for d.len() > 0 && tag == 2 && !meta.isClosing() {
			expectedTag = 2
			if !meta.isContextSpecific() {
				return &ErrorWrongTagType{ContextTag}
			}
			if tag != expectedTag {
				return &ErrorIncorrectTag{Expected: expectedTag, Given: tag}
			}
			prop := bactype.Property{}
			prop.ID = d.propertyId(int(length))

			// Tag 3 - (Optional) Array Length
			tag, meta = d.tagNumber()
			if tag == 3 {
				if !meta.isContextSpecific() {
					return &ErrorWrongTagType{ContextTag}
				}
				length = d.valueLen(meta)
				prop.ArrayIndex = d.unsigned(int(length))
				// Move to the next tag
				tag, meta = d.tagNumber()
			} else {
				prop.ArrayIndex = ArrayAll
			}

			switch tag {
			case 4: // Property Value
				if !meta.isOpening() {
					return &ErrorWrongTagType{OpeningTag}
				}
				data, err := d.AppData()
				if err != nil {
					return err
				}
				prop.Data = data
				obj.Properties = append(obj.Properties, prop)

				tag, meta = d.tagNumber()
				expectedTag = 4
				if tag != expectedTag {
					return &ErrorIncorrectTag{Expected: expectedTag, Given: tag}
				}
				if !meta.isClosing() {
					return &ErrorWrongTagType{ClosingTag}
				}
				tag, meta, length = d.tagNumberAndValueLen()

			case 5: // Property Access Error
				if !meta.isOpening() {
					return &ErrorWrongTagType{OpeningTag}
				}
				var bacErr bactype.Error
				err := d.bacError(&bacErr)
				if err != nil {
					return err
				}
				errs = append(errs, bactype.PropertyAccessError{
					Err:      bacErr,
					ObjectID: obj.ID,
					Property: prop.ID,
				})

				tag, meta = d.tagNumber()
				if tag != 5 {
					return &ErrorIncorrectTag{Expected: 5, Given: tag}
				}
				if !meta.isClosing() {
					return &ErrorWrongTagType{ClosingTag}
				}
				tag, meta, length = d.tagNumberAndValueLen()

			default:
				return &ErrorIncorrectTag{Expected: expectedTag, Given: tag}
			}
		}
		*objects = append(*objects, obj)
	}
	// prefer parse errors
	if err := d.Error(); err != nil {
		return err
	}
	return multierr.Combine(errs...)
}
