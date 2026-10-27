package encoding

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/smart-core-os/gobacnet/enum/errorclass"
	"github.com/smart-core-os/gobacnet/enum/errorcode"
	"github.com/smart-core-os/gobacnet/enum/pdutype"
	"github.com/smart-core-os/gobacnet/property"
	bactype "github.com/smart-core-os/gobacnet/types"
	"github.com/smart-core-os/gobacnet/types/objecttype"
)

func TestEncoder_ReadPropertyAck_knownSample(t *testing.T) {
	// the same sample TestReadPropertyResponse decodes: analog-input 1 present-value -5.0
	want := []byte{48, 1, 12, 12, 0, 0, 0, 1, 25, 85, 62, 68, 192, 160, 0, 0, 63}
	e := NewEncoder()
	err := e.ReadPropertyAck(1, bactype.ReadPropertyData{Object: bactype.Object{
		ID:         bactype.ObjectID{Type: objecttype.AnalogInput, Instance: 1},
		Properties: []bactype.Property{{ID: property.PresentValue, ArrayIndex: ArrayAll, Data: float32(-5)}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("got  % x\nwant % x", got, want)
	}
}

func TestEncoder_ReadPropertyAck_array(t *testing.T) {
	list := []interface{}{
		bactype.ObjectID{Type: objecttype.Device, Instance: 10},
		bactype.ObjectID{Type: objecttype.AnalogValue, Instance: 1},
		bactype.ObjectID{Type: objecttype.AnalogValue, Instance: 2},
	}
	e := NewEncoder()
	err := e.ReadPropertyAck(7, bactype.ReadPropertyData{Object: bactype.Object{
		ID:         bactype.ObjectID{Type: objecttype.Device, Instance: 10},
		Properties: []bactype.Property{{ID: property.ObjectList, ArrayIndex: ArrayAll, Data: list}},
	}})
	if err != nil {
		t.Fatal(err)
	}

	d := NewDecoder(e.Bytes())
	var apdu bactype.APDU
	if err := d.APDU(&apdu); err != nil {
		t.Fatal(err)
	}
	var got bactype.ReadPropertyData
	if err := d.ReadProperty(&got); err != nil {
		t.Fatal(err)
	}
	if data := got.Object.Properties[0].Data; !reflect.DeepEqual(data, list) {
		t.Fatalf("got %v, want %v", data, list)
	}
}

func TestEncoder_ReadMultiplePropertyAck_knownSample(t *testing.T) {
	// the trace TestReadMultiple decodes: the object-name of four objects from a sample server
	want := []byte{12, 2, 0, 4, 210, 30, 41, 77, 78, 117, 13, 0, 83, 105, 109,
		112, 108, 101, 83, 101, 114, 118, 101, 114, 79, 31, 12, 2, 128, 0, 0, 30,
		41, 77, 78, 117, 7, 0, 70, 73, 76, 69, 32, 48, 79, 31, 12, 2, 128, 0, 1, 30,
		41, 77, 78, 117, 7, 0, 70, 73, 76, 69, 32, 49, 79, 31, 12, 2, 128, 0, 2, 30,
		41, 77, 78, 117, 7, 0, 70, 73, 76, 69, 32, 50, 79, 31}
	name := func(t bactype.ObjectType, i bactype.ObjectInstance, name string) bactype.Object {
		return bactype.Object{
			ID:         bactype.ObjectID{Type: t, Instance: i},
			Properties: []bactype.Property{{ID: property.ObjectName, ArrayIndex: ArrayAll, Data: name}},
		}
	}
	e := NewEncoder()
	err := e.ReadMultiplePropertyAck(3, bactype.ReadMultipleProperty{Objects: []bactype.Object{
		name(objecttype.Device, 1234, "SimpleServer"),
		name(objecttype.File, 0, "FILE 0"),
		name(objecttype.File, 1, "FILE 1"),
		name(objecttype.File, 2, "FILE 2"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := e.Bytes()
	if !bytes.Equal(got[:3], []byte{byte(pdutype.ComplexAck), 3, byte(bactype.ServiceConfirmedReadPropMultiple)}) {
		t.Fatalf("APDU header % x", got[:3])
	}
	if !bytes.Equal(got[3:], want) {
		t.Fatalf("got  % x\nwant % x", got[3:], want)
	}
}

func TestEncoder_ReadMultiplePropertyAck_propertyError(t *testing.T) {
	obj := bactype.ObjectID{Type: objecttype.AnalogValue, Instance: 5}
	e := NewEncoder()
	err := e.ReadMultiplePropertyAck(1, bactype.ReadMultipleProperty{Objects: []bactype.Object{{
		ID: obj,
		Properties: []bactype.Property{
			{ID: property.PresentValue, ArrayIndex: ArrayAll, Data: float32(21.5)},
			{ID: property.Polarity, ArrayIndex: ArrayAll, Data: bactype.Error{Class: errorclass.Property, Code: errorcode.UnknownProperty}},
			{ID: property.Units, ArrayIndex: ArrayAll, Data: bactype.Enumerated(62)},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}

	d := NewDecoder(e.Bytes())
	var apdu bactype.APDU
	if err := d.APDU(&apdu); err != nil {
		t.Fatal(err)
	}
	var got bactype.ReadMultipleProperty
	err = d.ReadMultiplePropertyAck(&got)
	var propErr bactype.PropertyAccessError
	if !errors.As(err, &propErr) || propErr.Property != property.Polarity || propErr.Err.Code != errorcode.UnknownProperty {
		t.Fatalf("want an unknown-property error for polarity, got %v", err)
	}
	props := got.Objects[0].Properties
	if len(props) != 2 || props[0].Data != float32(21.5) || props[1].Data != uint32(62) {
		t.Fatalf("unexpected properties %+v", props)
	}
}

func TestDecoder_ReadMultipleProperty(t *testing.T) {
	// analog-input 16 present-value and reliability, the ReadPropertyMultiple example in
	// ANSI/ASHRAE 135 Annex F
	raw := []byte{0x02, 0x02, 0x0F, 0x0E, 0x0C, 0x00, 0x00, 0x00, 0x10, 0x1E, 0x09, 0x55, 0x09, 0x67, 0x1F}
	d := NewDecoder(raw)
	var apdu bactype.APDU
	if err := d.APDU(&apdu); err != nil {
		t.Fatal(err)
	}
	if apdu.Service != bactype.ServiceConfirmedReadPropMultiple || apdu.InvokeId != 15 || apdu.MaxApdu != 206 {
		t.Fatalf("unexpected APDU %+v", apdu)
	}
	var got bactype.ReadMultipleProperty
	if err := NewDecoder(apdu.RawData).ReadMultipleProperty(&got); err != nil {
		t.Fatal(err)
	}
	want := bactype.ReadMultipleProperty{Objects: []bactype.Object{{
		ID: bactype.ObjectID{Type: objecttype.AnalogInput, Instance: 16},
		Properties: []bactype.Property{
			{ID: property.PresentValue, ArrayIndex: ArrayAll},
			{ID: property.Reliability, ArrayIndex: ArrayAll},
		},
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestDecoder_ReadMultipleProperty_roundTrip(t *testing.T) {
	want := bactype.ReadMultipleProperty{Objects: []bactype.Object{
		{
			ID: bactype.ObjectID{Type: objecttype.Device, Instance: 4001},
			Properties: []bactype.Property{
				{ID: property.ObjectList, ArrayIndex: 0},
				{ID: property.ObjectList, ArrayIndex: 300},
				{ID: property.PropertyList, ArrayIndex: ArrayAll},
			},
		},
		{
			ID:         bactype.ObjectID{Type: objecttype.AnalogValue, Instance: 70000},
			Properties: []bactype.Property{{ID: property.All, ArrayIndex: ArrayAll}},
		},
	}}
	e := NewEncoder()
	if err := e.ReadMultipleProperty(9, want); err != nil {
		t.Fatal(err)
	}
	d := NewDecoder(e.Bytes())
	var apdu bactype.APDU
	if err := d.APDU(&apdu); err != nil {
		t.Fatal(err)
	}
	var got bactype.ReadMultipleProperty
	if err := NewDecoder(apdu.RawData).ReadMultipleProperty(&got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestDecoder_ReadMultipleProperty_malformed(t *testing.T) {
	for name, raw := range map[string][]byte{
		"no closing tag":   {0x0C, 0x00, 0x00, 0x00, 0x10, 0x1E, 0x09, 0x55},
		"no properties":    {0x0C, 0x00, 0x00, 0x00, 0x10, 0x1E, 0x1F},
		"no opening tag":   {0x0C, 0x00, 0x00, 0x00, 0x10, 0x09, 0x55},
		"application tags": {0xC4, 0x00, 0x00, 0x00, 0x10, 0x1E, 0x09, 0x55, 0x1F},
	} {
		var got bactype.ReadMultipleProperty
		if err := NewDecoder(raw).ReadMultipleProperty(&got); err == nil {
			t.Errorf("%s: want an error, got %+v", name, got)
		}
	}
}

func TestEncoder_ErrorPDU(t *testing.T) {
	e := NewEncoder()
	bacErr := bactype.Error{Class: errorclass.Object, Code: errorcode.UnknownObject}
	if err := e.ErrorPDU(4, bactype.ServiceConfirmedReadProperty, bacErr); err != nil {
		t.Fatal(err)
	}
	want := []byte{0x50, 4, 12, 0x91, 1, 0x91, 31}
	if got := e.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("got  % x\nwant % x", got, want)
	}
	var apdu bactype.APDU
	if err := NewDecoder(e.Bytes()).APDU(&apdu); err != nil {
		t.Fatal(err)
	}
	if apdu.DataType != pdutype.Error || apdu.InvokeId != 4 || apdu.Error != bacErr {
		t.Fatalf("decoded %+v", apdu)
	}
}

func TestEncoder_RejectAndAbortPDU(t *testing.T) {
	e := NewEncoder()
	if err := e.RejectPDU(8, bactype.RejectUnrecognizedService); err != nil {
		t.Fatal(err)
	}
	if got, want := e.Bytes(), []byte{0x60, 8, 9}; !bytes.Equal(got, want) {
		t.Errorf("reject: got % x, want % x", got, want)
	}
	e = NewEncoder()
	if err := e.AbortPDU(8, true, bactype.AbortSegmentationNotSupported); err != nil {
		t.Fatal(err)
	}
	if got, want := e.Bytes(), []byte{0x71, 8, 4}; !bytes.Equal(got, want) {
		t.Errorf("abort: got % x, want % x", got, want)
	}
}
