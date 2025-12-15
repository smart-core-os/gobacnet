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

package gobacnet

import (
	"context"
	"errors"
	"fmt"

	"github.com/smart-core-os/gobacnet/encoding"
	"github.com/smart-core-os/gobacnet/enum/errorcode"
	bactype "github.com/smart-core-os/gobacnet/types"
)

// ReadMultiProperty uses the given device and read property request to read
// from a device. Along with being able to read multiple properties from a
// device, it can also read these properties from multiple objects. This is a
// good feature to read all present values of every object in the device. This
// is a batch operation compared to a ReadProperty and should be used in place
// when reading more than two objects/properties.
func (c *Client) ReadMultiProperty(ctx context.Context, dev bactype.Device, rp bactype.ReadMultipleProperty) (bactype.ReadMultipleProperty, error) {
	var out bactype.ReadMultipleProperty

	id, err := c.tsm.ID(ctx)
	if err != nil {
		return out, fmt.Errorf("unable to get transaction id: %w", err)
	}
	defer c.tsm.Put(id)

	udp, err := c.LocalUDPAddress()
	if err != nil {
		return out, err
	}
	src := bactype.UDPToAddress(udp)

	enc := encoding.NewEncoder()
	enc.NPDU(bactype.NPDU{
		Version:               bactype.ProtocolVersion,
		Destination:           &dev.Addr,
		Source:                &src,
		IsNetworkLayerMessage: false,
		ExpectingReply:        true,
		Priority:              bactype.Normal,
		HopCount:              bactype.DefaultHopCount,
	})
	err = enc.ReadMultipleProperty(id, rp)
	if err != nil {
		return out, fmt.Errorf("encoding read multiple property failed: %w", err)
	}

	pack := enc.Bytes()
	if dev.MaxApdu < uint32(len(pack)) {
		return out, fmt.Errorf("read multiple property is too large (max: %d given: %d)", dev.MaxApdu, len(pack))
	}
	return c.sendReadMultipleProperty(ctx, id, dev, pack)
}

func (c *Client) sendReadMultipleProperty(ctx context.Context, id uint8, dev bactype.Device, request []byte) (bactype.ReadMultipleProperty, error) {
	var out bactype.ReadMultipleProperty
	_, err := c.send(dev.Addr, request)
	if err != nil {
		return out, err
	}

	raw, err := c.tsm.Receive(ctx, id)
	if err != nil {
		return out, fmt.Errorf("unable to receive id %d: %w", id, err)
	}

	var b []byte
	switch v := raw.(type) {
	case error:
		return out, v
	case []byte:
		b = v
	default:
		return out, fmt.Errorf("received unknown datatype %T", raw)
	}

	dec := encoding.NewDecoder(b)

	var apdu bactype.APDU
	dec.APDU(&apdu)
	err = dec.ReadMultiplePropertyAck(&out)
	if err != nil {
		c.Log.Debugf("WEIRD PACKET: %v: %v", err, b)
		return out, err
	}
	return out, err
}

// ReadProperties uses ReadMultiProperty if available or falls back to ReadProperty if not.
func (c *Client) ReadProperties(ctx context.Context, dev bactype.Device, property bactype.ReadMultipleProperty) (bactype.ReadMultipleProperty, error) {
	res, err := c.ReadMultiProperty(ctx, dev, property)
	if err == nil {
		return res, nil
	}
	var out bactype.ReadMultipleProperty
	out.Objects = make([]bactype.Object, len(property.Objects))

	// the ReadProperty calls below will all fail if ctx is Done so try and surface the correct error
	select {
	case <-ctx.Done():
		return res, err
	default:
	}

	// errors that were sent from the server
	if bacErr := (bactype.Error{}); errors.As(err, &bacErr) {

		if !shouldRetryIndividually(bacErr.Code) {
			return out, err
		}
	}

	for i, object := range property.Objects {

		select {
		case <-ctx.Done():
			return bactype.ReadMultipleProperty{}, ctx.Err()
		default:
		}

		propRes, err := c.ReadProperty(ctx, dev, bactype.ReadPropertyData{Object: object})
		if err != nil {
			return bactype.ReadMultipleProperty{}, err
		}
		out.Objects[i] = propRes.Object
	}
	return out, nil
}

// shouldRetryIndividually determines if a BACnet error code indicates that
// individual property reads might succeed when a batch ReadMultipleProperty failed.
// Returns true for errors related to the batch operation itself (size limits, service support),
// false for errors related to the actual data (permissions, unknown objects, etc.)
func shouldRetryIndividually(code errorcode.ErrorCode) bool {
	switch code {
	// Service/operation not supported - device might support individual reads
	case errorcode.OptionalFunctionalityNotSupported, errorcode.ServiceRequestDenied:
		return true

	// Message size issues - individual reads are smaller
	case errorcode.AbortBufferOverflow, errorcode.AbortSegmentationNotSupported, errorcode.MessageTooLong, errorcode.AbortApduTooLong:
		return true

	// Temporary issues that might resolve with smaller requests
	case errorcode.DeviceBusy, errorcode.Timeout, errorcode.Busy, errorcode.AbortOutOfResources:
		return true

	default:
		return false
	}
}
