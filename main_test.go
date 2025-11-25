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
	"encoding/json"
	"fmt"
	"log"
	"net"
	"testing"
	"time"

	"github.com/smart-core-os/gobacnet/types/objecttype"

	"github.com/smart-core-os/gobacnet/property"

	"github.com/smart-core-os/gobacnet/types"
)

const interfaceName = "eth0"
const testServer = 1234

func localInterfaceName() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("net.Interfaces() %w", err)
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback > 0 {
			return iface.Name, nil
		}
	}

	return interfaceName, nil
}

// TestNewClient are general test
func TestNewClient(t *testing.T) {
	interfaceName, err := localInterfaceName()
	if err != nil {
		t.Fatalf("getting local interface: %v", err)
	}
	c, err := NewClient(interfaceName, DefaultPort)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.Close()

	d, err := NewClient("pizzainterfacenotreal", DefaultPort)
	if err == nil {
		d.Close()
		t.Fatal("Successfully passed a false interface.")
	}
}
func TestGetBroadcast(t *testing.T) {
	failTest := func(addr string) {
		_, err := getBroadcast(addr)
		if err == nil {
			t.Fatalf("%s is not a valid parameter, but it did not gracefully crash", addr)
		}
	}

	failTest("frog")
	failTest("frog/dog")
	failTest("frog/24")
	failTest("16.18.dog/32")

	s, err := getBroadcast("192.168.23.1/24")
	if err != nil {
		t.Fatal(err)
	}
	correct := "192.168.23.255"
	if s.String() != correct {
		t.Fatalf("%s is incorrect. It should be %s", s.String(), correct)
	}
}

func TestMac(t *testing.T) {
	var mac []byte
	json.Unmarshal([]byte("\"ChQAzLrA\""), &mac)
	l := len(mac)
	p := uint16(mac[l-1])<<8 | uint16(mac[l-1])
	log.Printf("%d", p)
}

func TestServices(t *testing.T) {
	t.Skip("Only supported when running a test server")

	interfaceName, err := localInterfaceName()
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewClient(interfaceName, DefaultPort)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	t.Run("Read Property", func(t *testing.T) {
		testReadPropertyService(c, t)
	})

	t.Run("Who Is", func(t *testing.T) {
		testWhoIs(c, t)
	})

	t.Run("WriteProperty", func(t *testing.T) {
		testWritePropertyService(c, t)
	})

}

func testReadPropertyService(c *Client, t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	dev, err := c.WhoIs(ctx, testServer, testServer)
	read := types.ReadPropertyData{
		Object: types.Object{
			ID: types.ObjectID{
				Type:     objecttype.AnalogValue,
				Instance: 1,
			},
			Properties: []types.Property{
				types.Property{
					ID:         property.ObjectName, // Present value
					ArrayIndex: ArrayAll,
				},
			},
		},
	}
	if len(dev) == 0 {
		t.Fatalf("Unable to find device id %d", testServer)
	}
	resp, err := c.ReadProperty(ctx, dev[0], read)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Response: %v", resp.Object.Properties[0].Data)
}

func testWhoIs(c *Client, t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	dev, err := c.WhoIs(ctx, testServer-1, testServer+1)
	if err != nil {
		t.Fatal(err)
	}
	if len(dev) == 0 {
		t.Fatalf("Unable to find device id %d", testServer)
	}
}

// This test will first cconver the name of an analogue sensor to a different
// value, read the property to make sure the name was changed, revert back, and
// ensure that the revert was successful
func testWritePropertyService(c *Client, t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	const targetName = "Hotdog"
	dev, err := c.WhoIs(ctx, testServer, testServer)
	wp := types.ReadPropertyData{
		Object: types.Object{
			ID: types.ObjectID{
				Type:     objecttype.AnalogValue,
				Instance: 1,
			},
			Properties: []types.Property{
				types.Property{
					ID:         property.ObjectName, // Present value
					ArrayIndex: ArrayAll,
				},
			},
		},
	}

	if len(dev) == 0 {
		t.Fatalf("Unable to find device id %d", testServer)
	}
	resp, err := c.ReadProperty(ctx, dev[0], wp)
	if err != nil {
		t.Fatal(err)
	}
	// Store the original response since we plan to put it back in after
	org := resp.Object.Properties[0].Data
	t.Logf("original name is: %d", org)

	wp.Object.Properties[0].Data = targetName
	err = c.WriteProperty(ctx, dev[0], wp, 0)
	if err != nil {
		t.Fatal(err)
	}

	resp, err = c.ReadProperty(ctx, dev[0], wp)
	if err != nil {
		t.Fatal(err)
	}

	d := resp.Object.Properties[0].Data
	s, ok := d.(string)
	if !ok {
		log.Fatalf("unexpected return type %T", d)
	}

	if s != targetName {
		log.Fatalf("write to name %s did not successed, name was %s", targetName, s)
	}

	// Revert Changes
	wp.Object.Properties[0].Data = org
	err = c.WriteProperty(ctx, dev[0], wp, 0)
	if err != nil {
		t.Fatal(err)
	}

	resp, err = c.ReadProperty(ctx, dev[0], wp)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Object.Properties[0].Data != org {
		t.Fatalf("unable to revert name back to original value %v: name is %v", org, resp.Object.Properties[0].Data)
	}
}
