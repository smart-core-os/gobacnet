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
	"fmt"
	"net"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/smart-core-os/gobacnet/tsm"
	bactype "github.com/smart-core-os/gobacnet/types"
	"github.com/smart-core-os/gobacnet/utsm"
)

const defaultStateSize = 20

type Client struct {
	netInterface     *net.Interface
	myAddress        string
	broadcastAddress net.IP
	port             int
	tsm              *tsm.TSM // used to transfer incoming messages to the correct handler
	utsm             *utsm.Manager
	listener         *net.UDPConn
	Log              *logrus.Logger
}

// getBroadcast uses the given address with subnet to return the broadcast address
func getBroadcast(addr string) (net.IP, error) {
	_, ipnet, err := net.ParseCIDR(addr)
	if err != nil {
		return net.IP{}, err
	}
	broadcast := net.IP(make([]byte, 4))
	for i := range broadcast {
		broadcast[i] = ipnet.IP[i] | ^ipnet.Mask[i]
	}
	return broadcast, nil
}

// NewClient creates a new client with the given interface and
// port.
func NewClient(inter string, port int, opts ...Option) (*Client, error) {
	cfg := DefaultOptions.
		With(WithInterface(inter), WithPort(uint16(port))).
		With(opts...)
	c, err := cfg.NewClient()
	if err == nil {
		go c.listen()
	}
	return c, err
}

func (cfg ClientOptions) NewClient() (*Client, error) {
	c := &Client{}

	inter := cfg.Interface
	var networkInterface *net.Interface
	if inter == "" {
		i, err := net.Interfaces()
		if err != nil {
			return nil, err
		}
		if len(i) == 0 {
			return nil, fmt.Errorf("no network interfaces fouund")
		}
		networkInterface = &i[0]
	} else {
		i, err := net.InterfaceByName(inter)
		if err != nil {
			return nil, err
		}
		networkInterface = i
	}

	c.netInterface = networkInterface
	c.port = int(cfg.Port)

	uni, err := c.netInterface.Addrs()
	if err != nil {
		return c, err
	}

	if len(uni) == 0 {
		return c, fmt.Errorf("interface %s has no addresses", inter)
	}

	// Clear out the value
	c.myAddress = ""
	// Find the first IP4 ip
	for _, adr := range uni {
		IP, _, _ := net.ParseCIDR(adr.String())

		// To4 is non nil when the type is ip4
		if IP.To4() != nil {
			c.myAddress = adr.String()
			break
		}
	}
	if len(c.myAddress) == 0 {
		// We couldn't find a interface or all of them are ip6
		return nil, fmt.Errorf("No valid broadcasting address was found on interface %s", inter)
	}

	broadcast, err := getBroadcast(c.myAddress)
	if err != nil {
		return c, err
	}
	c.broadcastAddress = broadcast

	c.tsm = tsm.New(cfg.MaxConcurrentTransactions)
	options := []utsm.ManagerOption{
		utsm.DefaultSubscriberTimeout(time.Second * time.Duration(10)),
		utsm.DefaultSubscriberLastReceivedTimeout(time.Second * time.Duration(2)),
	}
	c.utsm = utsm.NewManager(options...)
	udp, _ := net.ResolveUDPAddr("udp4", fmt.Sprintf(":%d", c.port))
	conn, err := net.ListenUDP("udp", udp)
	if err != nil {
		return nil, err
	}

	c.listener = conn
	c.Log = logrus.New()
	c.Log.Formatter = &logrus.TextFormatter{}
	c.Log.SetLevel(cfg.LogLevel)

	// Print out relevant information
	c.Log.Debug(fmt.Sprintf("Broadcast Address: %v", c.broadcastAddress))
	c.Log.Debug(fmt.Sprintf("Local Address: %s", c.myAddress))
	c.Log.Debug(fmt.Sprintf("Port: %x", c.port))
	return c, nil
}

func (c *Client) localAddress() (la bactype.Address, err error) {
	ip, _, _ := net.ParseCIDR(c.myAddress)
	ad := ip.To4()
	udp := net.UDPAddr{
		IP:   ad,
		Port: c.port,
	}
	la = bactype.UDPToAddress(&udp)
	return la, nil
}

func (c *Client) LocalUDPAddress() (*net.UDPAddr, error) {
	ip, _, _ := net.ParseCIDR(c.myAddress)
	netstr := fmt.Sprintf("%s:%d", ip.String(), c.port)
	return net.ResolveUDPAddr("udp4", netstr)
}

var DefaultOptions = ClientOptions{
	Port:                      DefaultPort,
	MaxConcurrentTransactions: defaultStateSize,
	LogLevel:                  logrus.DebugLevel,
}

type ClientOptions struct {
	Interface string
	Port      uint16

	MaxConcurrentTransactions uint8

	LogLevel logrus.Level
}

func (cfg ClientOptions) Clone() ClientOptions {
	return cfg
}

// With returns a new ClientOptions with the given Options applied.
// c is not modified by this call.
func (cfg ClientOptions) With(opts ...Option) ClientOptions {
	dst := cfg.Clone()
	for _, opt := range opts {
		opt(&dst)
	}
	return dst
}

// Option allows configuration of a Client during construction.
type Option func(*ClientOptions)

func WithInterface(inter string) Option {
	return func(client *ClientOptions) {
		client.Interface = inter
	}
}

// WithPort sets the port on which the client will listen.
func WithPort(port uint16) Option {
	if port == 0 {
		port = DefaultPort
	}
	return func(client *ClientOptions) {
		client.Port = port
	}
}

// WithMaxConcurrentTransactions sets the maximum number of concurrent transactions.
func WithMaxConcurrentTransactions(size uint8) Option {
	if size == 0 {
		size = defaultStateSize
	}
	return func(client *ClientOptions) {
		client.MaxConcurrentTransactions = size
	}
}

// WithLogLevel sets the log level.
func WithLogLevel(level logrus.Level) Option {
	return func(client *ClientOptions) {
		client.LogLevel = level
	}
}
