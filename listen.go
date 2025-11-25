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
	"net"
	"os"

	"github.com/smart-core-os/gobacnet/encoding"
	"github.com/smart-core-os/gobacnet/enum/pdutype"
	bactype "github.com/smart-core-os/gobacnet/types"
)

// Close free resources for the client. Always call this function when using NewClient
func (c *Client) Close() {
	if c.listener == nil {
		return
	}
	c.listener.Close()
	if f, ok := c.Log.Out.(*os.File); ok {
		f.Close()
	}
}

func (c *Client) handleMsg(src *net.UDPAddr, b []byte) {
	var header bactype.BVLC
	var npdu bactype.NPDU
	var apdu bactype.APDU

	dec := encoding.NewDecoder(b)
	err := dec.BVLC(&header)
	if err != nil {
		c.Log.Error(err)
		return
	}

	if header.Function == bactype.BacFuncBroadcast || header.Function == bactype.BacFuncUnicast || header.Function == bactype.BacFuncForwardedNPDU {
		// Remove the header information
		b = b[mtuHeaderLength:]
		err = dec.NPDU(&npdu)
		if err != nil {
			return
		}

		if npdu.IsNetworkLayerMessage {
			c.Log.Debug("Ignored Network Layer Message")
			return
		}

		// We want to keep the APDU intact so we will get a snapshot before decoding
		// further
		send := dec.Bytes()
		err = dec.APDU(&apdu)
		if err != nil {
			c.Log.Errorf("Issue decoding APDU: %v", err)
			return
		}

		switch apdu.DataType {
		case pdutype.UnconfirmedServiceRequest:
			if apdu.UnconfirmedService == bactype.ServiceUnconfirmedIAm {
				dec = encoding.NewDecoder(apdu.RawData)
				var iam bactype.IAm

				err = dec.IAm(&iam)

				// For whatever reason, the IP section won't be populated until
				// we set the type.
				src.IP = src.IP.To4()
				iam.Addr = bactype.UDPToAddress(src)
				if src := npdu.Source; src != nil {
					iam.Addr.Net = src.Net
					iam.Addr.Adr = src.Adr
				}
				c.Log.Debugf("Received IAm Message %+v", iam)
				if err != nil {
					c.Log.Error(err)
					return
				}
				c.utsm.Publish(int(iam.ID.Instance), iam)
			} else if apdu.UnconfirmedService == bactype.ServiceUnconfirmedWhoIs {
				dec := encoding.NewDecoder(apdu.RawData)
				var low, high int32
				dec.WhoIs(&low, &high)
				// For now we are going to ignore who is request.
				// Log.WithFields(Log.Fields{"low": low, "high": high}).Debug("WHO IS Request")
			} else {
				c.Log.Errorf("Unconfirmed: %d %v", apdu.UnconfirmedService, apdu.RawData)
			}
		case pdutype.SimpleAck:
			c.Log.Debug("Received Simple Ack")
			err := c.tsm.Send(apdu.InvokeId, send)
			if err != nil {
				return
			}
		case pdutype.ComplexAck:
			c.Log.Debug("Received Complex Ack")
			err := c.tsm.Send(apdu.InvokeId, send)
			if err != nil {
				return
			}
		case pdutype.ConfirmedServiceRequest:
			c.Log.Debug("Received  Confirmed Service Request")
			err := c.tsm.Send(apdu.InvokeId, send)
			if err != nil {
				return
			}
		case pdutype.Error:
			c.Log.Debug("Received Error")
			err := c.tsm.Send(apdu.InvokeId, apdu.Error)
			if err != nil {
				c.Log.Debugf("unable to send error to %d: %v", apdu.InvokeId, err)
			}
		default:
			// Ignore it
			// Log.WithFields(Log.Fields{"raw": b}).Debug("An ignored packet went through")
		}
	}

	if header.Function == bactype.BacFuncForwardedNPDU {
		// Right now we are ignoring the NPDU data that is stored in the packet. Eventually
		// we will need to check it for any additional information we can gleam.
		// NDPU has source
		b = b[forwardHeaderLength:]
		c.Log.Debug("Ignored NDPU Forwarded")
	}

}

// listen for incoming bacnet packets.
func (c *Client) listen() error {
	var err error = nil

	// While connection is opened
	for err == nil {
		var (
			adr *net.UDPAddr
			i   int
		)

		b := make([]byte, 2048)
		i, adr, err = c.listener.ReadFromUDP(b)
		if err != nil {
			c.Log.Error(err)
			continue
		}
		go c.handleMsg(adr, b[:i])
	}
	return nil
}
