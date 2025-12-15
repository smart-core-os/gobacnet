package gobacnet

import (
	"context"
	"fmt"

	"github.com/smart-core-os/gobacnet/encoding"
	bactype "github.com/smart-core-os/gobacnet/types"
)

func (c *Client) WriteProperty(ctx context.Context, dest bactype.Device, wp bactype.ReadPropertyData, priority uint) error {

	id, err := c.tsm.ID(ctx)
	if err != nil {
		return fmt.Errorf("unable to get an transaction id: %w", err)
	}
	defer c.tsm.Put(id)

	udp, err := c.LocalUDPAddress()
	if err != nil {
		return err
	}
	src := bactype.UDPToAddress(udp)

	enc := encoding.NewEncoder()
	enc.NPDU(bactype.NPDU{
		Version:               bactype.ProtocolVersion,
		Destination:           &dest.Addr,
		Source:                &src,
		IsNetworkLayerMessage: false,
		ExpectingReply:        true,
		Priority:              bactype.Normal,
		HopCount:              bactype.DefaultHopCount,
	})

	enc.WriteProperty(uint8(id), wp, priority)
	if enc.Error() != nil {
		return err
	}

	// the value filled doesn't matter. it just needs to be non nil
	err = fmt.Errorf("go")
	for count := 0; err != nil && count < 2; count++ {
		var b []byte
		var raw interface{}
		_, err = c.send(dest.Addr, enc.Bytes())
		if err != nil {
			continue
		}

		raw, err = c.tsm.Receive(ctx, id)
		if err != nil {
			continue
		}
		switch v := raw.(type) {
		case error:
			return v
		case []byte:
			b = v
		default:
			return fmt.Errorf("received unknown datatype %T", raw)
		}

		dec := encoding.NewDecoder(b)

		var apdu bactype.APDU
		dec.APDU(&apdu)
		if err = dec.Error(); err != nil {
			continue
		}
		return err
	}
	return err
}
