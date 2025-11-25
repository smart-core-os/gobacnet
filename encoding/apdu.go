/*Copyright (C) 2017 Alex Beltran

This program is free software; you can redistribute it and/or
modify it under the terms of the GNU General Public License
as published by the Free Software Foundation; either version 2
of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program; if not, write to:
The Free Software Foundation, Inc.
59 Temple Place - Suite 330
Boston, MA  02111-1307, USA.

As a special exception, if other files instantiate templates or
use macros or inline functions from this file, or you compile
this file and link it with other works to produce a work based
on this file, this file does not by itself cause the resulting
work to be covered by the GNU General Public License. However
the source code for this file must still be made available in
accordance with section (3) of the GNU General Public License.

This exception does not invalidate any other reasons why a work
based on this file might be covered by the GNU General Public
License.
*/

package encoding

import (
	"fmt"

	"github.com/smart-core-os/gobacnet/enum/errorclass"
	"github.com/smart-core-os/gobacnet/enum/errorcode"
	"github.com/smart-core-os/gobacnet/enum/pdutype"
	bactype "github.com/smart-core-os/gobacnet/types"
)

func (e *Encoder) APDU(a bactype.APDU) error {
	meta := APDUMetadata(0)
	meta.setDataType(a.DataType)
	meta.setMoreFollows(a.MoreFollows)
	meta.setSegmentedMessage(a.SegmentedMessage)
	meta.setSegmentedAccepted(a.SegmentedResponseAccepted)
	e.write(meta)

	switch a.DataType {
	case pdutype.ComplexAck:
		e.apduComplexAck(a)
	case pdutype.UnconfirmedServiceRequest:
		e.apduUnconfirmed(a)
	case pdutype.ConfirmedServiceRequest:
		e.apduConfirmed(a)
	case pdutype.SegmentAck:
		return fmt.Errorf("decoded segmented")
	case pdutype.Error:
		return fmt.Errorf("decoded error")
	case pdutype.Reject:
		return fmt.Errorf("decoded rejected")
	case pdutype.Abort:
		return fmt.Errorf("decoded aborted")
	default:
		return fmt.Errorf("unknown PDU type:%d", meta.DataType())
	}
	return nil
}

func (e *Encoder) apduConfirmed(a bactype.APDU) {
	e.maxSegsMaxApdu(a.MaxSegs, a.MaxApdu)
	e.write(a.InvokeId)
	if a.SegmentedMessage {
		e.write(a.Sequence)
		e.write(a.WindowNumber)
	}
	e.write(a.Service)
}

func (e *Encoder) apduUnconfirmed(a bactype.APDU) {
	e.write(a.UnconfirmedService)
}

func (e *Encoder) apduComplexAck(a bactype.APDU) {
	e.write(a.InvokeId)
	e.write(a.Service)
}

func (d *Decoder) APDU(a *bactype.APDU) error {
	var meta APDUMetadata
	d.decode(&meta)
	a.SegmentedMessage = meta.isSegmentedMessage()
	a.SegmentedResponseAccepted = meta.segmentedResponseAccepted()
	a.MoreFollows = meta.moreFollows()
	a.DataType = meta.DataType()

	switch a.DataType {
	case pdutype.ComplexAck:
		return d.apduComplexAck(a)
	case pdutype.SimpleAck:
		return d.apduSimpleAck(a)
	case pdutype.UnconfirmedServiceRequest:
		return d.apduUnconfirmed(a)
	case pdutype.ConfirmedServiceRequest:
		return d.apduConfirmed(a)
	case pdutype.SegmentAck:
		return fmt.Errorf("segmented")
	case pdutype.Error:
		return d.apduError(a)
	case pdutype.Reject:
		return fmt.Errorf("rejected")
	case pdutype.Abort:
		return fmt.Errorf("aborted")
	default:
		return fmt.Errorf("unknown PDU type:%d", a.DataType)
	}
}

func (d *Decoder) apduError(a *bactype.APDU) error {
	d.decode(&a.InvokeId)
	d.decode(&a.Service)
	class, err := d.AppData()
	if err != nil {
		return err
	}

	c, ok := class.(uint32)
	if !ok {
		return fmt.Errorf("unable to decode error class")
	}
	a.Error.Class = errorclass.ErrorClass(c)

	code, err := d.AppData()
	if err != nil {
		return err
	}

	c, ok = code.(uint32)
	if !ok {
		return fmt.Errorf("unable to decode error code")
	}
	a.Error.Code = errorcode.ErrorCode(c)

	return nil
}

func (d *Decoder) apduComplexAck(a *bactype.APDU) error {
	d.decode(&a.InvokeId)
	d.decode(&a.Service)
	return d.Error()
}

func (d *Decoder) apduSimpleAck(a *bactype.APDU) error {
	d.decode(&a.InvokeId)
	d.decode(&a.Service)
	return d.Error()
}

func (d *Decoder) apduUnconfirmed(a *bactype.APDU) error {
	d.decode(&a.UnconfirmedService)
	a.RawData = make([]byte, d.len())
	d.decode(a.RawData)
	return d.Error()
}
func (d *Decoder) apduConfirmed(a *bactype.APDU) error {
	a.MaxSegs, a.MaxApdu = d.maxSegsMaxApdu()

	d.decode(&a.InvokeId)
	if a.SegmentedMessage {
		d.decode(&a.Sequence)
		d.decode(&a.WindowNumber)
	}

	d.decode(&a.Service)
	if d.len() > 0 {
		a.RawData = make([]byte, d.len())
		d.decode(&a.RawData)
	}

	return d.Error()
}

type APDUMetadata byte

const (
	apduMaskSegmented         = 1 << 3
	apduMaskMoreFollows       = 1 << 2
	apduMaskSegmentedAccepted = 1 << 1
	// Bit 0 is reserved
)

func (meta *APDUMetadata) setInfoMask(b bool, mask byte) {
	*meta = APDUMetadata(setInfoMask(byte(*meta), b, mask))
}

// CheckMask uses mask to check bit position
func (meta *APDUMetadata) checkMask(mask byte) bool {
	return (*meta & APDUMetadata(mask)) > 0
}

func (meta *APDUMetadata) isSegmentedMessage() bool {
	return meta.checkMask(apduMaskSegmented)
}

func (meta *APDUMetadata) moreFollows() bool {
	return meta.checkMask(apduMaskMoreFollows)
}

func (meta *APDUMetadata) segmentedResponseAccepted() bool {
	return meta.checkMask(apduMaskSegmentedAccepted)
}

func (meta *APDUMetadata) setSegmentedMessage(b bool) {
	meta.setInfoMask(b, apduMaskSegmented)
}

func (meta *APDUMetadata) setMoreFollows(b bool) {
	meta.setInfoMask(b, apduMaskMoreFollows)
}

func (meta *APDUMetadata) setSegmentedAccepted(b bool) {
	meta.setInfoMask(b, apduMaskSegmentedAccepted)
}

func (meta *APDUMetadata) setDataType(t pdutype.PDUType) {
	// clean the first 4 bits
	*meta = (*meta & APDUMetadata(0xF0)) | APDUMetadata(t)
}
func (meta *APDUMetadata) DataType() pdutype.PDUType {
	// clean the first 4 bits
	return pdutype.PDUType(0xF0) & pdutype.PDUType(*meta)
}
