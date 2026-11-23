package encoding

import (
	"github.com/smart-core-os/gobacnet/enum/pdutype"
	bactype "github.com/smart-core-os/gobacnet/types"
)

// ErrorPDU encodes a BACnet-Error-PDU, the failure response to a confirmed service request.
func (e *Encoder) ErrorPDU(invokeID uint8, service bactype.ServiceConfirmed, bacErr bactype.Error) error {
	e.write(uint8(pdutype.Error))
	e.write(invokeID)
	e.write(service)
	e.AppData(bactype.Enumerated(bacErr.Class))
	e.AppData(bactype.Enumerated(bacErr.Code))
	return e.Error()
}

// RejectPDU encodes a BACnet-Reject-PDU, sent when a confirmed request is malformed or names a
// service the device does not support.
func (e *Encoder) RejectPDU(invokeID uint8, reason bactype.RejectReason) error {
	e.write(uint8(pdutype.Reject))
	e.write(invokeID)
	e.write(reason)
	return e.Error()
}

// AbortPDU encodes a BACnet-Abort-PDU. fromServer is true when the device responding to a
// request is the one aborting the transaction.
func (e *Encoder) AbortPDU(invokeID uint8, fromServer bool, reason bactype.AbortReason) error {
	t := uint8(pdutype.Abort)
	if fromServer {
		t |= 0x01
	}
	e.write(t)
	e.write(invokeID)
	e.write(reason)
	return e.Error()
}
